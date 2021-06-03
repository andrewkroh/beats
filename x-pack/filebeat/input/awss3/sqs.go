// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package awss3

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/elastic/go-concert/timed"
)

const (
	sqsRetryDelay = 10 * time.Second
)

type sqsAPI interface {
	sqsReceiver
	sqsDeleter
	sqsVisibilityChanger
}

type sqsReceiver interface {
	ReceiveMessage(ctx context.Context, maxMessages int) ([]sqs.Message, error)
}

type sqsDeleter interface {
	DeleteMessage(ctx context.Context, msg *sqs.Message) error
}

type sqsVisibilityChanger interface {
	ChangeMessageVisibility(ctx context.Context, msg *sqs.Message, timeout time.Duration) error
}

type sqsProcessor interface {
	ProcessSQS(ctx context.Context, msg *sqs.Message) error
}

type sqsReader struct {
	maxMessagesInflight int
	workerSem           *sem
	sqs                 sqsAPI
	msgHandler          sqsProcessor
	log                 *logp.Logger
}

func newSQSReader(log *logp.Logger, sqs sqsAPI, maxMessagesInflight int, msgHandler sqsProcessor) *sqsReader {
	return &sqsReader{
		maxMessagesInflight: maxMessagesInflight,
		workerSem:           newSem(maxMessagesInflight),
		sqs:                 sqs,
		msgHandler:          msgHandler,
		log:                 log,
	}
}

func (r *sqsReader) Receive(ctx context.Context) error {
	// This loop tries to keep the workers busy as much as possible while
	// honoring the max message cap as opposed to a simpler loop that receives
	// N messages, waits for them all to finish, then requests N more messages.
	var workerWg sync.WaitGroup
	for ctx.Err() == nil {
		// Determine how many SQS workers are available.
		workers, err := r.workerSem.AcquireContext(r.maxMessagesInflight, ctx)
		if err != nil {
			break
		}

		// Receive (at most) as many SQS messages as there are workers.
		msgs, err := r.sqs.ReceiveMessage(ctx, workers)
		if err != nil {
			r.workerSem.Release(workers)
			r.log.Warnw("SQS ReceiveMessage returned an error. Will retry after a short delay.", "error", err)

			// Throttle retries.
			timed.Wait(ctx, sqsRetryDelay)
			continue
		}

		// Release unused workers.
		r.workerSem.Release(workers - len(msgs))

		// Process each SQS message asynchronously with a goroutine.
		r.log.Debugf("Received %v SQS messages.", len(msgs))
		workerWg.Add(len(msgs))
		for _, msg := range msgs {
			go func(msg sqs.Message) {
				defer r.workerSem.Release(1)
				defer workerWg.Done()

				if err := r.msgHandler.ProcessSQS(ctx, &msg); err != nil {
					r.log.Warnw("Failed processing SQS message.", "error", err, "message_id", *msg.MessageId)
				}
			}(msg)
		}
	}

	// Wait for all workers to finish.
	workerWg.Wait()

	return ctx.Err()
}
