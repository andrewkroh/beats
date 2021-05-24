package awss3

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/pkg/errors"
	"go.uber.org/multierr"

	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/elastic/go-concert/timed"
)

const (
	sqsRetryDelay = 10 * time.Second

	sqsVisibilityTimeout = 5 * time.Minute
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

type sqsReader struct {
	maxMessagesInflight int
	workerSem           *sem
	sqs                 sqsAPI
	log                 *logp.Logger
}

func NewSQSReceiver(log *logp.Logger, sqs sqsAPI, maxMessagesInflight int) *sqsReader {
	return &sqsReader{
		maxMessagesInflight: maxMessagesInflight,
		workerSem:           newSem(maxMessagesInflight),
		sqs:                 sqs,
		log:                 log,
	}
}

func (r *sqsReader) Receive(ctx context.Context) error {
	// This loop tries to keep the workers busy as much as possible while
	// honoring the max message cap as opposed to a simpler loop that receives
	// N messages, waits for them all to finish, then requests N more messages.
	for ctx.Err() == nil {
		// Determine how many SQS workers are available.
		workers, err := r.workerSem.AcquireContext(r.maxMessagesInflight, ctx)
		if err != nil {
			return err
		}

		// Receive (at most) as many SQS messages as there are workers.
		msgs, err := r.sqs.ReceiveMessage(ctx, workers)
		if err != nil {
			r.workerSem.Release(workers)
			r.log.Warn(err)

			// Throttle retries.
			timed.Wait(ctx, sqsRetryDelay)
			continue
		}

		// Release unused workers.
		r.workerSem.Release(workers - len(msgs))

		// Process each SQS message asynchronously with a goroutine.
		r.log.Debugf("Received %v SQS messages.", len(msgs))
		for _, msg := range msgs {
			go func(msg sqs.Message) {
				defer r.workerSem.Release(1)

				log := r.log.With("message_id", *msg.MessageId)
				msgHandler := newSQSProcessor(log, r.sqs, &msg)

				if err := msgHandler.Process(ctx); err != nil {
					log.Warnw("Failed processing SQS message.", "error", err)
				}
			}(msg)
		}
	}

	return ctx.Err()
}

type sqsProcessor struct {
	msg      *sqs.Message
	sqs      sqsAPI
	log      *logp.Logger
	warnOnce sync.Once
}

func newSQSProcessor(log *logp.Logger, sqs sqsAPI, msg *sqs.Message) *sqsProcessor {
	return &sqsProcessor{sqs: sqs, log: log, msg: msg}
}

func (p *sqsProcessor) Process(ctx context.Context) error {
	s3Events, err := p.getS3Notifications(isObjectCreatedEvents)
	if err != nil {
		return err
	}
	p.log.Debugf("SQS message contained %d S3 event notifications.", len(s3Events))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start SQS keepalive worker.
	go func() {
		t := time.NewTicker(sqsVisibilityTimeout / 2)
		defer t.Stop()

		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.log.Debug("Extending SQS message visibility timeout.")

			// Renew visibility.
			if err := p.sqs.ChangeMessageVisibility(ctx, p.msg, sqsVisibilityTimeout); err != nil {
				p.log.Warn("Failed to extend message visibility timeout.", "error", err)
			}
		}
	}()

	var errs []error
	for _, event := range s3Events {
		// Process S3 object (download, parse, create events).
		// TODO
		_ = event
	}

	if len(errs) > 0 {
		cancel()
		// Set visibility timeout to 0 to return item to queue.
		if err := p.sqs.ChangeMessageVisibility(ctx, p.msg, 0); err != nil {
			p.log.Debug("Failed to immediately return SQS message to queue by "+
				"setting visibility timeout to 0.", "error", err)
		}
		return errors.Wrap(multierr.Combine(err), "failed processing SQS message, returning message to queue")
	}

	// Delete Message
	return errors.Wrap(p.sqs.DeleteMessage(ctx, p.msg), "failed deleting message from SQS queue")
}

type s3Events struct {
	Records []s3Event `json:"Records"`
}

type s3Event struct {
	AWSRegion   string `json:"awsRegion"`
	EventName   string `json:"eventName"`
	EventSource string `json:"eventSource"`
	S3          struct {
		Bucket struct {
			Name string `json:"name"`
			ARN  string `json:"arn"`
		} `json:"bucket"`
		Object struct {
			Key string `json:"key"`
		} `json:"object"`
	} `json:"s3"`
}

func (p *sqsProcessor) getS3Notifications(predicate func(event s3Event) bool) ([]s3Event, error) {
	var events s3Events
	dec := json.NewDecoder(strings.NewReader(*p.msg.Body))
	if err := dec.Decode(&events); err != nil {
		return nil, fmt.Errorf("failed to decode SQS message body as an S3 notification: %w", err)
	}

	var out []s3Event
	for _, record := range events.Records {
		if !predicate(record) {
			p.warnOnce.Do(func() {
				p.log.Warnf("Received S3 notification for %q event type, but " +
					"only 'ObjectCreated:*' types are handled. It is recommended " +
					"that you update the S3 Event Notification configuration to " +
					"only include ObjectCreated event types to save resources.")
			})
			continue
		}

		// Unescape s3 key name. For example, convert "%3D" back to "=".
		key, err := url.QueryUnescape(record.S3.Object.Key)
		if err != nil {
			return nil, fmt.Errorf("url unescape failed for '%v': %w", record.S3.Object.Key, err)
		}
		record.S3.Object.Key = key

		out = append(out, record)
	}

	return out, nil
}

func isObjectCreatedEvents(event s3Event) bool {
	return event.EventSource == "aws:s3" && strings.HasPrefix(event.EventName, "ObjectCreated:")
}
