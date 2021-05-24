package awss3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/pkg/errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/stretchr/testify/assert"
)

var _ sqsAPI = (*mockSQS)(nil)

type mockSQS struct {
	t          testing.TB
	queue      []sqs.Message
	deleted    []string // receipt ID
	visibility []struct {
		receiptID string
		timeout   time.Duration
	}
	callback func()
}

func (m *mockSQS) pop() *sqs.Message {
	if len(m.queue) == 0 {
		return nil
	}
	msg := m.queue[len(m.queue)-1]
	m.queue = m.queue[:len(m.queue)-1]
	return &msg
}

func (m *mockSQS) push(msg *sqs.Message) {
	m.queue = append([]sqs.Message{*msg}, m.queue...)
}

func (m *mockSQS) ReceiveMessage(ctx context.Context, maxMessages int) ([]sqs.Message, error) {
	if len(m.queue) == 0 {
		time.Sleep(time.Second)
	}

	var out []sqs.Message
	for i := 0; i < maxMessages; i++ {
		msg := m.pop()
		if msg == nil {
			break
		}
		out = append(out, *msg)
	}

	m.t.Log("ReceiveMessage returning", len(out), "messages.")
	return out, nil
}

func (m *mockSQS) DeleteMessage(ctx context.Context, msg *sqs.Message) error {
	m.t.Log("DeleteMessage", *msg.ReceiptHandle)
	m.deleted = append(m.deleted, *msg.ReceiptHandle)
	m.callback()
	return nil
}

func (m *mockSQS) ChangeMessageVisibility(ctx context.Context, msg *sqs.Message, timeout time.Duration) error {
	m.t.Log("ChangeMessageVisibility", *msg.ReceiptHandle, timeout)
	m.visibility = append(m.visibility, struct {
		receiptID string
		timeout   time.Duration
	}{receiptID: *msg.ReceiptHandle, timeout: timeout})
	m.callback()
	return nil
}

func TestNewSQSReceiver(t *testing.T) {
	logp.TestingSetup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sqsAPI := &mockSQS{
		t: t,
		queue: []sqs.Message{
			newSQSMessage(),
		},
	}
	sqsAPI.callback = func() {
		if len(sqsAPI.deleted) == 1 {
			cancel()
		}
	}

	sqsReceiver := NewSQSReceiver(logp.NewLogger(inputName), sqsAPI, 1)
	err := sqsReceiver.Receive(ctx)
	assert.True(t, errors.Is(err, context.Canceled))

	assert.Len(t, sqsAPI.deleted, 1)

	t.Run("receive one", func(t *testing.T) {
	})
	t.Run("receive error and retry", func(t *testing.T) {
	})
}

func TestSqsProcessor_Process(t *testing.T) {
	t.Run("invalid SQS JSON body does not retry", func(t *testing.T) {
	})
	t.Run("zero S3 events in body", func(t *testing.T) {
	})
	t.Run("visibility is extended after half expires", func(t *testing.T) {
	})
	t.Run("all s3 objects are processed", func(t *testing.T) {
	})
	t.Run("message deleted on success", func(t *testing.T) {
	})
	t.Run("visibility timeout set to 0 on retryable failure", func(t *testing.T) {
	})
}

func TestSqsProcessor_getS3Notifications(t *testing.T) {
	t.Run("s3 key is url unescaped", func(t *testing.T) {
	})
	t.Run("non-ObjectCreated event types are ignored", func(t *testing.T) {
	})
}

func newSQSMessage() sqs.Message {
	body := `{
  "Records": [
    {
      "eventVersion": "2.1",
      "eventSource": "aws:s3",
      "awsRegion": "us-east-2",
      "eventTime": "2021-05-19T15:13:59.239Z",
      "eventName": "ObjectCreated:CompleteMultipartUpload",
      "userIdentity": {
        "principalId": "AWS:AIDASDJDMBHZUCUQNNAUT"
      },
      "requestParameters": {
        "sourceIPAddress": "192.168.50.2"
      },
      "responseElements": {
        "x-amz-request-id": "5GD3G05EWV2R8JA3",
        "x-amz-id-2": "5C6Ga4KyKbYprqSmLM1k1jnHLy4WwH3kab7Cv1Y6qnSR/AvXpP5wvCk3KzNV/0FsiU5O7zBaakprqg2IaI3qodUEXzd2dOsJ"
      },
      "s3": {
        "s3SchemaVersion": "1.0",
        "configurationId": "filebeat-notify-sqs",
        "bucket": {
          "name": "aws-logs",
          "ownerIdentity": {
            "principalId": "A2UXKVEZX9JVR"
          },
          "arn": "arn:aws:s3:::aws-logs"
        },
        "object": {
          "key": "xml/logs.xml",
          "size": 717530947,
          "eTag": "659b0e787594bd15ccbf28156a86db87-86",
          "sequencer": "0060A52B35612BC817"
        }
      }
    }
  ]
}
{
  "id": "24785670A85E824A",
  "queue_url": "https://sqs.us-east-2.amazonaws.com/144492464627/filebeat-test",
  "region": "us-east-2"
}`

	hash := sha256.Sum256([]byte(body))
	messageID := hex.EncodeToString(hash[:])
	receipt := "receipt-" + messageID

	return sqs.Message{
		Body:          &body,
		MessageId:     &messageID,
		ReceiptHandle: &receipt,
	}
}
