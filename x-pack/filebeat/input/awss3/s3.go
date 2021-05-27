// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package awss3

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/errors"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common/acker"
	"github.com/elastic/beats/v7/libbeat/logp"
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
	publisher beat.Client
	s3        s3API
	log       *logp.Logger
}

func newS3ObjectProcessor(log *logp.Logger, s3 s3API, publisher beat.Client) *s3ObjectProcessor {
	return &s3ObjectProcessor{log: log, s3: s3, publisher: publisher}
}

func (p *s3ObjectProcessor) ProcessS3Object(ctx context.Context, ack *eventACKTracker, obj s3EventV2) error {
	log := p.log.With("bucket", obj.S3.Bucket.Name, "s3_object", obj.S3.Object.Key)
	log.Debug("Processing object.")
	defer log.Debug("Processing complete.")

	// Request object (download).
	contentType, body, err := p.download(ctx, obj)
	if err != nil {
		return errors.Wrap(err, "failed to get s3 object")
	}
	defer body.Close()

	// Process object content stream.
	// TODO: Make a map[string]reader for content-types.
	switch {
	case contentType == "application/json":
		err = p.readJSON(ctx, ack, body)
	default:
		err = p.readFile(ctx, ack, body)
	}
	if err != nil {
		return err
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

func (p *s3ObjectProcessor) readJSON(ctx context.Context, ack *eventACKTracker, r io.Reader) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()

	for dec.More() && ctx.Err() == nil {
		offset := dec.InputOffset()

		var item json.RawMessage
		if err := dec.Decode(&item); err != nil {
			return err
		}

		data, _ := item.MarshalJSON()

		p.publish(ack, &beat.Event{
			Private: nil,
			Fields: map[string]interface{}{
				"message": string(data),
				"offset":  offset,
			},
		})
	}

	return nil
}

func (p *s3ObjectProcessor) readFile(ctx context.Context, ack *eventACKTracker, r io.Reader) error {
	s := bufio.NewScanner(r)
	for s.Scan() && ctx.Err() == nil {
		p.publish(ack, &beat.Event{
			Private: nil,
			Fields: map[string]interface{}{
				"message": s.Text(),
			},
		})
	}
	return s.Err()
}

func (p *s3ObjectProcessor) publish(ack *eventACKTracker, event *beat.Event) {
	ack.Add(1)
	event.Private = ack
	p.publisher.Publish(*event)
}
