// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package awss3

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/logp"
)

func newS3Object(t testing.TB, filename, contentType string) (s3EventV2, *s3.GetObjectResponse) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	r := bytes.NewReader(data)
	contentLen := int64(r.Len())
	resp := &s3.GetObjectResponse{
		GetObjectOutput: &s3.GetObjectOutput{
			Body:          ioutil.NopCloser(r),
			ContentLength: &contentLen,
			ContentType:   &contentType,
		},
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".gz":
		gzipEncoding := "gzip"
		resp.ContentEncoding = &gzipEncoding
	}

	return newS3Event(filename), resp
}

func TestS3ObjectProcessor(t *testing.T) {
	logp.TestingSetup()

	t.Run("download text/plain file", func(t *testing.T) {
		testProcessS3Object(t, "testdata/log.txt", "text/plain", 2)
	})

	t.Run("multiline content", func(t *testing.T) {
		testProcessS3Object(t, "testdata/multiline.txt", "text/plain", 2)
	})

	t.Run("application/json content-type", func(t *testing.T) {
		testProcessS3Object(t, "testdata/log.json", "application/json", 2)
	})

	t.Run("application/x-ndjson content-type", func(t *testing.T) {
		testProcessS3Object(t, "testdata/log.ndjson", "application/x-ndjson", 2)
	})

	t.Run("configured content-type", func(t *testing.T) {
		testProcessS3Object(t, "testdata/multiline.json", "application/octet-stream", 2)
	})

	t.Run("uncompress application/zip content", func(t *testing.T) {
		testProcessS3Object(t, "testdata/multiline.json.gz", "application/json", 2)
	})

	t.Run("unparsable json", func(t *testing.T) {
		testProcessS3ObjectError(t, "testdata/invalid.json", "application/json", 0)
	})

	t.Run("split array", func(t *testing.T) {
		testProcessS3Object(t, "testdata/events-array.json", "application/json", 2)
	})

	t.Run("split array error missing key", func(t *testing.T) {
		testProcessS3ObjectError(t, "testdata/events-array.json", "application/json", 2)
	})

	t.Run("events have a unique repeatable _id", func(t *testing.T) {
		// Hash of bucket ARN, object key, object versionId, and log offset.
		events := testProcessS3Object(t, "testdata/log.txt", "text/plain", 2)

		const idFieldName = "@metadata._id"
		for _, event := range events {
			v, _ := event.GetValue(idFieldName)
			if assert.NotNil(t, v, idFieldName+" is nil") {
				_id, ok := v.(string)
				if assert.True(t, ok, idFieldName+" is not a string") {
					assert.NotEmpty(t, _id, idFieldName+" is empty")
				}
			}
		}
	})

	t.Run("download error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		ctrl, ctx := gomock.WithContext(ctx, t)
		defer ctrl.Finish()
		mockS3API := NewMockS3API(ctrl)
		mockPublisher := NewMockBeatClient(ctrl)

		s3Event := newS3Event("log.txt")

		mockS3API.EXPECT().
			GetObject(gomock.Any(), gomock.Eq(s3Event.S3.Bucket.Name), gomock.Eq(s3Event.S3.Object.Key)).
			Return(nil, errFakeConnectivityFailure)

		s3ObjProc := newS3ObjectProcessor(logp.NewLogger(inputName), mockS3API, mockPublisher)
		ack := newEventACKTracker(ctx)
		err := s3ObjProc.ProcessS3Object(ctx, ack, s3Event)
		require.Error(t, err)
		assert.True(t, errors.Is(err, errFakeConnectivityFailure), "expected errFakeConnectivityFailure error")
	})
}

func testProcessS3Object(t testing.TB, file, contentType string, numEvents int) []beat.Event {
	return _testProcessS3Object(t, file, contentType, numEvents, false)
}

func testProcessS3ObjectError(t testing.TB, file, contentType string, numEvents int) []beat.Event {
	return _testProcessS3Object(t, file, contentType, numEvents, true)
}

func _testProcessS3Object(t testing.TB, file, contentType string, numEvents int, expectErr bool) []beat.Event {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	ctrl, ctx := gomock.WithContext(ctx, t)
	defer ctrl.Finish()
	mockS3API := NewMockS3API(ctrl)
	mockPublisher := NewMockBeatClient(ctrl)

	s3Event, s3Resp := newS3Object(t, file, contentType)
	var events []beat.Event
	gomock.InOrder(
		mockS3API.EXPECT().
			GetObject(gomock.Any(), gomock.Eq(s3Event.S3.Bucket.Name), gomock.Eq(s3Event.S3.Object.Key)).
			Return(s3Resp, nil),
		mockPublisher.EXPECT().
			Publish(gomock.Any()).
			Do(func(event beat.Event) { events = append(events, event) }).
			Times(numEvents),
	)

	s3ObjProc := newS3ObjectProcessor(logp.NewLogger(inputName), mockS3API, mockPublisher)
	ack := newEventACKTracker(ctx)
	err := s3ObjProc.ProcessS3Object(ctx, ack, s3Event)

	if !expectErr {
		require.NoError(t, err)
		assert.Equal(t, numEvents, len(events))
		assert.Equal(t, numEvents, ack.pendingACKs)
	} else {
		require.Error(t, err)
	}

	return events
}
