// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package awss3

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/errors"

	"github.com/elastic/beats/v7/libbeat/common"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common/acker"
	"github.com/elastic/beats/v7/libbeat/logp"
)

const (
	contentTypeJSON   = "application/json"
	contentTypeNDJSON = "application/x-ndjson"
)

type s3API interface {
	GetObject(ctx context.Context, bucket, key string) (*s3.GetObjectResponse, error)
}

type eventACKTracker struct {
	sync.Mutex
	pendingACKs int64
	ctx         context.Context
	cancel      context.CancelFunc
}

func newEventACKTracker(ctx context.Context) *eventACKTracker {
	ctx, cancel := context.WithCancel(ctx)
	return &eventACKTracker{ctx: ctx, cancel: cancel}
}

func (a *eventACKTracker) Add(messageCount int64) {
	a.Lock()
	a.pendingACKs++
	a.Unlock()
}

func (a *eventACKTracker) ACK() {
	a.Lock()
	defer a.Unlock()

	if a.pendingACKs <= 0 {
		panic("misuse detected: negative ACK counter")
	}

	a.pendingACKs--
	if a.pendingACKs == 0 {
		a.cancel()
	}
}

func (a *eventACKTracker) Wait() {
	<-a.ctx.Done()
}

func newEventACKHandler() beat.ACKer {
	return acker.ConnectionOnly(
		acker.EventPrivateReporter(func(_ int, privates []interface{}) {
			for _, private := range privates {
				if ack, ok := private.(*eventACKTracker); ok {
					ack.ACK()
				}
			}
		}),
	)
}

type s3ObjectProcessor struct {
	publisher     beat.Client
	s3            s3API
	log           *logp.Logger
	fileSelectors []fileSelectorConfig
}

func newS3ObjectProcessor(log *logp.Logger, s3 s3API, publisher beat.Client, sel []fileSelectorConfig) *s3ObjectProcessor {
	if len(sel) == 0 {
		sel = []fileSelectorConfig{
			{ReaderConfig: defaultConfig().ReaderConfig},
		}
	}
	return &s3ObjectProcessor{log: log, s3: s3, publisher: publisher, fileSelectors: sel}
}

func (p *s3ObjectProcessor) ProcessS3Object(ctx context.Context, ack *eventACKTracker, obj s3EventV2) error {
	// TODO: add SQS message_id to logger for log correlation.
	log := p.log.With("bucket", obj.S3.Bucket.Name, "s3_object", obj.S3.Object.Key)
	log.Debug("Begin S3 object processing.")

	readerConfig := p.findReaderConfig(obj.S3.Object.Key)
	if readerConfig == nil {
		log.Debug("End S3 object processing. No file_selectors are a match.")
		return nil
	}
	defer log.Debug("End S3 object processing.")

	// Request object (download).
	contentType, body, err := p.download(ctx, obj)
	if err != nil {
		return errors.Wrap(err, "failed to get s3 object")
	}
	defer body.Close()

	reader, err := p.addGzipDecoderIfNeeded(body)
	if err != nil {
		return errors.Wrap(err, "failed checking for gzip content")
	}

	// Overwrite with user configured Content-Type.
	if readerConfig.ContentType != "" {
		contentType = readerConfig.ContentType
	}

	// Process object content stream.
	switch {
	case contentType == contentTypeJSON || contentType == contentTypeNDJSON:
		err = p.readJSON(ctx, ack, reader, obj, readerConfig)
	default:
		err = p.readFile(ctx, ack, reader, obj)
	}
	if err != nil {
		return err
	}

	return nil
}

func (p *s3ObjectProcessor) findReaderConfig(key string) *readerConfig {
	for _, sel := range p.fileSelectors {
		if sel.Regex == nil || sel.Regex.MatchString(key) {
			return &sel.ReaderConfig
		}
	}
	return nil
}

// download requests the S3 object from AWS and returns the object's
// Content-Type and reader to get the object's contents. The caller must
// close the returned reader.
func (p *s3ObjectProcessor) download(ctx context.Context, obj s3EventV2) (contentType string, body io.ReadCloser, err error) {
	resp, err := p.s3.GetObject(ctx, obj.S3.Bucket.Name, obj.S3.Object.Key)
	if err != nil {
		return "", nil, err
	}
	return *resp.ContentType, resp.Body, nil
}

func (p *s3ObjectProcessor) addGzipDecoderIfNeeded(body io.Reader) (io.Reader, error) {
	bufReader := bufio.NewReader(body)

	gzipped, err := isStreamGzipped(bufReader)
	if err != nil {
		return nil, err
	}
	if !gzipped {
		return bufReader, nil
	}

	return gzip.NewReader(bufReader)
}

func (p *s3ObjectProcessor) readJSON(ctx context.Context, ack *eventACKTracker, r io.Reader, obj s3EventV2, readerConfig *readerConfig) error {
	objHash := s3ObjectHash(obj)

	dec := json.NewDecoder(r)
	dec.UseNumber()

	for dec.More() && ctx.Err() == nil {
		offset := dec.InputOffset()

		var item json.RawMessage
		if err := dec.Decode(&item); err != nil {
			return err
		}

		if readerConfig.ExpandEventListFromField != "" {
			if err := p.splitEventList(item, offset, ack, obj, objHash, readerConfig.ExpandEventListFromField); err != nil {
				return err
			}
			continue
		}

		data, _ := item.MarshalJSON()
		evt := createEvent(string(data), offset, obj, objHash)
		p.publish(ack, &evt)
	}

	return nil
}

func (p *s3ObjectProcessor) splitEventList(raw json.RawMessage, offset int64, ack *eventACKTracker, obj s3EventV2, objHash string, key string) error {
	var jsonObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &jsonObject); err != nil {
		return err
	}

	p.log.Info("in split", key, string(raw))

	raw, found := jsonObject[key]
	if !found {
		return fmt.Errorf("%v is not in event", key)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return fmt.Errorf("%v is not an array", key)
	}
	p.log.Infof("token %T %#v", tok, tok)

	for dec.More() {
		p.log.Info("more")
		arrayOffset := dec.InputOffset()

		var item json.RawMessage
		if err := dec.Decode(&item); err != nil {
			return err
		}

		p.log.Info("more", string(item))
		data, _ := item.MarshalJSON()
		evt := createEvent(string(data), offset+arrayOffset, obj, objHash)
		p.publish(ack, &evt)
	}

	return nil
}

func (p *s3ObjectProcessor) readFile(ctx context.Context, ack *eventACKTracker, r io.Reader, obj s3EventV2) error {
	objHash := s3ObjectHash(obj)

	s := bufio.NewScanner(r)

	for s.Scan() && ctx.Err() == nil {
		evt := createEvent(s.Text(), 0, obj, objHash)
		p.publish(ack, &evt)
	}

	return s.Err()
}

func (p *s3ObjectProcessor) publish(ack *eventACKTracker, event *beat.Event) {
	ack.Add(1)
	event.Private = ack
	p.publisher.Publish(*event)
}

//
//func (c *s3Collector) createEventsFromS3Info(svc s3iface.ClientAPI, info s3Info, s3Ctx *s3Context) error {
//	// handle s3 objects that are not json content-type
//	encodingFactory, ok := encoding.FindEncoding(info.Encoding)
//	if !ok || encodingFactory == nil {
//		return fmt.Errorf("unable to find '%v' encoding", info.Encoding)
//	}
//	enc, err := encodingFactory(bodyReader)
//	if err != nil {
//		return fmt.Errorf("failed to initialize encoding: %v", err)
//	}
//	var r reader.Reader
//	r, err = readfile.NewEncodeReader(ioutil.NopCloser(bodyReader), readfile.Config{
//		Codec:      enc,
//		BufferSize: int(info.BufferSize),
//		Terminator: info.LineTerminator,
//		MaxBytes:   int(info.MaxBytes) * 4,
//	})
//	if err != nil {
//		return fmt.Errorf("failed to create encode reader: %w", err)
//	}
//	r = readfile.NewStripNewline(r, info.LineTerminator)
//
//	if info.Multiline != nil {
//		r, err = multiline.New(r, "\n", int(info.MaxBytes), info.Multiline)
//		if err != nil {
//			return fmt.Errorf("error setting up multiline: %v", err)
//		}
//	}
//
//	r = readfile.NewLimitReader(r, int(info.MaxBytes))
//
//	var offset int64
//	for {
//		message, err := r.Next()
//		if err == io.EOF {
//			// No more lines
//			break
//		}
//		if err != nil {
//			return fmt.Errorf("error reading message: %w", err)
//		}
//		event := createEvent(string(message.Content), offset, info, objectHash, s3Ctx)
//		offset += int64(message.Bytes)
//		if err = c.forwardEvent(event); err != nil {
//			return fmt.Errorf("forwardEvent failed: %w", err)
//		}
//	}
//	return nil
//}

func createEvent(message string, offset int64, obj s3EventV2, objectHash string) beat.Event {
	event := beat.Event{
		Timestamp: time.Now().UTC(),
		Fields: common.MapStr{
			"message": message,
			"log": common.MapStr{
				"offset": offset,
				"file": common.MapStr{
					"path": constructObjectURL(obj),
				},
			},
			"aws": common.MapStr{
				"s3": common.MapStr{
					"bucket": common.MapStr{
						"name": obj.S3.Bucket.Name,
						"arn":  obj.S3.Bucket.ARN},
					"object": common.MapStr{
						"key": obj.S3.Object.Key,
					},
				},
			},
			"cloud": common.MapStr{
				"provider": "aws",
				"region":   obj.AWSRegion,
			},
		},
	}
	event.SetID(objectID(objectHash, offset))

	return event
}

func objectID(objectHash string, offset int64) string {
	return fmt.Sprintf("%s-%012d", objectHash, offset)
}

func constructObjectURL(obj s3EventV2) string {
	return "https://" + obj.S3.Bucket.Name + ".s3-" + obj.AWSRegion + ".amazonaws.com/" + obj.S3.Bucket.Name
}

// s3ObjectHash returns a short sha256 hash of the bucket arn + object key name.
func s3ObjectHash(obj s3EventV2) string {
	h := sha256.New()
	h.Write([]byte(obj.S3.Bucket.ARN))
	h.Write([]byte(obj.S3.Object.Key))
	prefix := hex.EncodeToString(h.Sum(nil))
	return prefix[:10]
}

// isStreamGzipped determines whether the given stream of bytes (encapsulated in a buffered reader)
// represents gzipped content or not. A buffered reader is used so the function can peek into the byte
// stream without consuming it. This makes it convenient for code executed after this function call
// to consume the stream if it wants.
func isStreamGzipped(r *bufio.Reader) (bool, error) {
	// Why 512? See https://godoc.org/net/http#DetectContentType
	buf, err := r.Peek(512)
	if err != nil && err != io.EOF {
		return false, err
	}

	switch http.DetectContentType(buf) {
	case "application/x-gzip", "application/zip":
		return true, nil
	default:
		return false, nil
	}
}
