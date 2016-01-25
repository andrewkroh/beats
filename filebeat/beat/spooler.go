package beat

import (
	"sync"
	"time"

	cfg "github.com/elastic/beats/filebeat/config"
	"github.com/elastic/beats/filebeat/input"
	"github.com/elastic/beats/libbeat/logp"
)

var debugf = logp.MakeDebug("spooler")

type Spooler struct {
	Channel chan *input.FileEvent

	// Config
	idleTimeout time.Duration
	spoolSize   uint64

	exit          chan struct{}             // Channel used to signal shutdown.
	nextFlushTime time.Time                 // Scheduled time of the next flush.
	publisher     chan<- []*input.FileEvent // Channel used to publish events.
	spool         []*input.FileEvent        // FileEvents being held by the Spooler.
	wg            sync.WaitGroup            // WaitGroup used to control the shutdown.
}

func NewSpooler(
	config cfg.FilebeatConfig,
	publisher chan<- []*input.FileEvent,
) *Spooler {
	spoolSize := config.SpoolSize
	if spoolSize <= 0 {
		spoolSize = cfg.DefaultSpoolSize
		debugf("Spooler will use the default spool_size of %d", spoolSize)
	}

	idleTimeout := config.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = cfg.DefaultIdleTimeout
		debugf("Spooler will use the default idle_timeout of %s", idleTimeout)
	}

	return &Spooler{
		Channel:       make(chan *input.FileEvent, 16),
		idleTimeout:   idleTimeout,
		spoolSize:     spoolSize,
		exit:          make(chan struct{}),
		nextFlushTime: time.Now().Add(idleTimeout),
		publisher:     publisher,
		spool:         make([]*input.FileEvent, 0, spoolSize),
	}
}

func (s *Spooler) Start() {
	s.wg.Add(1)
	go s.run()
}

// run runs the spooler.
// It heartbeats periodically. If the last flush was longer than
// 'IdleTimeoutDuration' time ago, then we'll force a flush to prevent us from
// holding on to spooled events for too long.
func (s *Spooler) run() {
	ticker := time.NewTicker(s.idleTimeout / 2)

	logp.Info("Starting spooler: spool_size: %v; idle_timeout: %s",
		s.spoolSize, s.idleTimeout)

loop:
	for {
		select {
		case <-s.exit:
			ticker.Stop()
			break loop
		case event := <-s.Channel:
			s.queue(event)
		case <-ticker.C:
			s.timedFlush()
		}
	}

	// Drain any events that remain in Channel.
	for e := range s.Channel {
		s.queue(e)
	}
	debugf("Flushing events from spooler at shutdown")
	s.flush()
	s.wg.Done()
}

// Stop stops this Spooler. This method blocks until all events have been
// flushed to the publisher.
func (s *Spooler) Stop() {
	logp.Info("Stopping spooler")

	// Stop accepting writes.
	close(s.Channel)

	// Signal to the run method that it should stop. Then wait for it to exit.
	close(s.exit)
	s.wg.Wait()

	debugf("Spooler has stopped")
}

func (s *Spooler) queue(event *input.FileEvent) {
	s.spool = append(s.spool, event)
	if len(s.spool) == cap(s.spool) {
		debugf("Flushing spooler because spooler full. Events flushed: %v", len(s.spool))
		s.flush()
	}
}

func (s *Spooler) timedFlush() {
	if time.Now().After(s.nextFlushTime) {
		debugf("Flushing spooler because of timeout. Events flushed: %v", len(s.spool))
		s.flush()
	}
}

// flush flushes all event and sends them to the publisher.
func (s *Spooler) flush() {
	if len(s.spool) > 0 {
		// copy buffer
		tmpCopy := make([]*input.FileEvent, len(s.spool))
		copy(tmpCopy, s.spool)

		// clear buffer
		s.spool = s.spool[:0]

		// send
		s.publisher <- tmpCopy
	}
	s.nextFlushTime = time.Now().Add(s.idleTimeout)
}
