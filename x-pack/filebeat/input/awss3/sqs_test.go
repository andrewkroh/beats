// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package awss3

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gofrs/uuid"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/v7/libbeat/logp"
)

//go:generate mockgen -source=sqs.go -destination=mock_sqs_test.go -package awss3 -mock_names=sqsAPI=MockSQSAPI,sqsProcessor=MockSQSProcessor sqsAPI,sqsProcessor
//go:generate mockgen -source=sqs_s3_event.go -destination=mock_sqs_s3_event_test.go -package awss3 -mock_names=s3ObjectHandler=MockS3ObjectHandler s3ObjectHandler
//go:generate mockgen -source=s3.go -destination=mock_s3_test.go -package awss3 -mock_names=s3API=MockS3API s3API
//go:generate mockgen -destination=mock_publisher_test.go -package=awss3 -mock_names=Client=MockBeatClient github.com/elastic/beats/v7/libbeat/beat Client

const testTimeout = 10 * time.Second

var errFakeConnectivityFailure = errors.New("fake connectivity failure")

func TestSQSReceiver(t *testing.T) {
	logp.TestingSetup()
	const maxMessages = 5

	t.Run("ReceiveMessage success", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		ctrl, ctx := gomock.WithContext(ctx, t)
		defer ctrl.Finish()
		mockAPI := NewMockSQSAPI(ctrl)
		mockMsgHandler := NewMockSQSProcessor(ctrl)
		msg := newSQSMessage(newS3Event("log.json"))

		gomock.InOrder(
			// Initial ReceiveMessage for maxMessages.
			mockAPI.EXPECT().
				ReceiveMessage(gomock.Any(), gomock.Eq(maxMessages)).
				Times(1).
				DoAndReturn(func(_ context.Context, _ int) ([]sqs.Message, error) {
					// Return single message.
					return []sqs.Message{msg}, nil
				}),

			// Follow up ReceiveMessages for either maxMessages-1 or maxMessages
			// depending on how long processing of previous message takes.
			mockAPI.EXPECT().
				ReceiveMessage(gomock.Any(), gomock.Any()).
				Times(1).
				DoAndReturn(func(_ context.Context, _ int) ([]sqs.Message, error) {
					// Stop the test.
					cancel()
					return nil, nil
				}),
		)

		// Expect the one message returned to have been processed.
		mockMsgHandler.EXPECT().
			ProcessSQS(gomock.Any(), gomock.Eq(&msg)).
			Times(1).
			Return(nil)

		// Execute SQSReceiver and verify calls/state.
		receiver := newSQSReader(logp.NewLogger(inputName), mockAPI, maxMessages, mockMsgHandler)
		err := receiver.Receive(ctx)
		assert.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, but got %v", err)
		assert.Equal(t, maxMessages, receiver.workerSem.available)
	})

	t.Run("retry after ReceiveMessage error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), sqsRetryDelay+testTimeout)
		defer cancel()

		ctrl, ctx := gomock.WithContext(ctx, t)
		defer ctrl.Finish()
		mockAPI := NewMockSQSAPI(ctrl)
		mockMsgHandler := NewMockSQSProcessor(ctrl)

		gomock.InOrder(
			// Initial ReceiveMessage gets an error.
			mockAPI.EXPECT().
				ReceiveMessage(gomock.Any(), gomock.Eq(maxMessages)).
				Times(1).
				DoAndReturn(func(_ context.Context, _ int) ([]sqs.Message, error) {
					return nil, errFakeConnectivityFailure
				}),
			// After waiting for sqsRetryDelay, it retries.
			mockAPI.EXPECT().
				ReceiveMessage(gomock.Any(), gomock.Eq(maxMessages)).
				Times(1).
				DoAndReturn(func(_ context.Context, _ int) ([]sqs.Message, error) {
					cancel()
					return nil, nil
				}),
		)

		// Execute SQSReceiver and verify calls/state.
		receiver := newSQSReader(logp.NewLogger(inputName), mockAPI, maxMessages, mockMsgHandler)
		err := receiver.Receive(ctx)
		assert.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, but got %v", err)
		assert.Equal(t, maxMessages, receiver.workerSem.available)
	})
}

func newSQSMessage(events ...s3EventV2) sqs.Message {
	body, err := json.Marshal(s3EventsV2{Records: events})
	if err != nil {
		panic(err)
	}

	hash := sha256.Sum256(body)
	id, _ := uuid.FromBytes(hash[:16])
	messageID := id.String()
	receipt := "receipt-" + messageID
	bodyStr := string(body)

	return sqs.Message{
		Body:          &bodyStr,
		MessageId:     &messageID,
		ReceiptHandle: &receipt,
	}
}

func newS3Event(key string) s3EventV2 {
	record := s3EventV2{
		AWSRegion:   "us-east-1",
		EventSource: "aws:s3",
		EventName:   "ObjectCreated:Put",
	}
	record.S3.Bucket.Name = "foo"
	record.S3.Bucket.ARN = "arn:aws:s3:::foo"
	record.S3.Object.Key = key
	return record
}
