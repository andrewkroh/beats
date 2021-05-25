// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package awss3

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/elastic/go-concert/timed"
)

func TestSQSS3EventProcessor(t *testing.T) {
	logp.TestingSetup()

	msg := newSQSMessage(newS3Event("log.json"))

	t.Run("s3 events are processed and sqs msg is deleted", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		ctrl, ctx := gomock.WithContext(ctx, t)
		defer ctrl.Finish()
		mockAPI := NewMockSQSAPI(ctrl)
		mockS3Handler := NewMockS3ObjectHandler(ctrl)

		gomock.InOrder(
			mockS3Handler.EXPECT().ProcessS3Object(gomock.Any(), gomock.Any()).Return(nil),
			mockAPI.EXPECT().DeleteMessage(gomock.Any(), gomock.Eq(&msg)).Return(nil),
		)

		p := newSQSS3EventProcessor(logp.NewLogger(inputName), mockAPI, time.Minute, mockS3Handler)
		require.NoError(t, p.ProcessSQS(ctx, &msg))
	})

	t.Run("invalid SQS JSON body does not retry", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		ctrl, ctx := gomock.WithContext(ctx, t)
		defer ctrl.Finish()
		mockAPI := NewMockSQSAPI(ctrl)
		mockS3Handler := NewMockS3ObjectHandler(ctrl)

		invalidBodyMsg := newSQSMessage(newS3Event("log.json"))
		body := *invalidBodyMsg.Body
		body = body[10:]
		invalidBodyMsg.Body = &body

		gomock.InOrder(
			mockAPI.EXPECT().DeleteMessage(gomock.Any(), gomock.Eq(&invalidBodyMsg)).Return(nil),
		)

		p := newSQSS3EventProcessor(logp.NewLogger(inputName), mockAPI, time.Minute, mockS3Handler)
		err := p.ProcessSQS(ctx, &invalidBodyMsg)
		t.Log(err)
		require.Error(t, err)
	})

	t.Run("zero S3 events in body", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		ctrl, ctx := gomock.WithContext(ctx, t)
		defer ctrl.Finish()
		mockAPI := NewMockSQSAPI(ctrl)
		mockS3Handler := NewMockS3ObjectHandler(ctrl)

		emptyRecordsMsg := newSQSMessage()

		gomock.InOrder(
			mockAPI.EXPECT().DeleteMessage(gomock.Any(), gomock.Eq(&emptyRecordsMsg)).Return(nil),
		)

		p := newSQSS3EventProcessor(logp.NewLogger(inputName), mockAPI, time.Minute, mockS3Handler)
		require.NoError(t, p.ProcessSQS(ctx, &emptyRecordsMsg))
	})

	t.Run("visibility is extended after half expires", func(t *testing.T) {
		const visibilityTimeout = time.Second

		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		ctrl, ctx := gomock.WithContext(ctx, t)
		defer ctrl.Finish()
		mockAPI := NewMockSQSAPI(ctrl)
		mockS3Handler := NewMockS3ObjectHandler(ctrl)

		mockAPI.EXPECT().ChangeMessageVisibility(gomock.Any(), gomock.Eq(&msg), gomock.Eq(visibilityTimeout)).AnyTimes().Return(nil)

		gomock.InOrder(
			mockS3Handler.EXPECT().ProcessS3Object(gomock.Any(), gomock.Any()).
				Do(func(ctx context.Context, v2 s3EventV2) {
					timed.Wait(ctx, 5*visibilityTimeout)
				}).Return(nil),
			mockAPI.EXPECT().DeleteMessage(gomock.Any(), gomock.Eq(&msg)).Return(nil),
		)

		p := newSQSS3EventProcessor(logp.NewLogger(inputName), mockAPI, visibilityTimeout, mockS3Handler)
		require.NoError(t, p.ProcessSQS(ctx, &msg))
	})

	t.Run("visibility timeout set to 0 on retryable failure", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		ctrl, ctx := gomock.WithContext(ctx, t)
		defer ctrl.Finish()
		mockAPI := NewMockSQSAPI(ctrl)
		mockS3Handler := NewMockS3ObjectHandler(ctrl)

		gomock.InOrder(
			mockS3Handler.EXPECT().ProcessS3Object(gomock.Any(), gomock.Any()).Return(errors.New("fake connectivity problem")),
			mockAPI.EXPECT().ChangeMessageVisibility(gomock.Any(), gomock.Eq(&msg), gomock.Eq(time.Duration(0))).Return(nil),
		)

		p := newSQSS3EventProcessor(logp.NewLogger(inputName), mockAPI, time.Minute, mockS3Handler)
		err := p.ProcessSQS(ctx, &msg)
		t.Log(err)
		require.Error(t, err)
	})
}

func TestSqsProcessor_getS3Notifications(t *testing.T) {
	logp.TestingSetup()

	p := newSQSS3EventProcessor(logp.NewLogger(inputName), nil, time.Minute, nil)

	t.Run("s3 key is url unescaped", func(t *testing.T) {
		msg := newSQSMessage(newS3Event("Happy+Face.jpg"))

		events, err := p.getS3Notifications(*msg.Body)
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "Happy Face.jpg", events[0].S3.Object.Key)
	})

	t.Run("non-ObjectCreated event types are ignored", func(t *testing.T) {
		event := newS3Event("HappyFace.jpg")
		event.EventName = "ObjectRemoved:Delete"
		msg := newSQSMessage(event)

		events, err := p.getS3Notifications(*msg.Body)
		require.NoError(t, err)
		assert.Len(t, events, 0)
	})
}
