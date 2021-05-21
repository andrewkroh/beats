package awss3

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/elastic/go-concert/timed"
	"sync"
	"time"
)

const (
	sqsRetryDelay = 10 * time.Second
)

type sqsReader interface {
	ReceiveMessage(ctx context.Context, maxMessages int) ([]sqs.Message, error)
}

type sqsReceiver struct {
	maxMessagesInflight int

	workerSem *sem
	sqs       sqsReader
	log       *logp.Logger
	parentCtx context.Context
}

func (r *sqsReceiver) ReadLoop() {
	for r.parentCtx.Err() != nil {
		workers := r.workerSem.Acquire(5)

		msgs, err := r.sqs.ReceiveMessage(context.TODO(), workers)
		if err != nil {
			r.workerSem.Release(workers)
			r.log.Warn(err)
			// Throttle retries.
			timed.Wait(r.parentCtx, sqsRetryDelay)
			continue
		}

		for _, msg := range msgs {
			go func(m sqs.Message) {
				defer r.workerSem.Release(1)
				// process SQS message
			}(msg)
		}
	}
}

type sqsProcessor {}

func (p *sqsProcessor) process(msg *sqs.Message) error {

}

type sem struct {
	mutex *sync.Mutex
	cond sync.Cond
	available int
}

func newSem(n int) *sem {
	var m sync.Mutex
	return &sem{
		available: n,
		mutex: &m,
		cond: sync.Cond{
			L: &m,
		},
	}
}

func (s *sem) Acquire(n int) int {
	if n <= 0 {
		return 0
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.available == 0 {
		s.cond.Wait()
	}

	if n >= s.available {
		rtn := s.available
		s.available = 0
		return rtn
	}

	s.available -= n
	return n
}

func (s *sem) Release(n int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.cond.Signal()
	s.available += n
}
