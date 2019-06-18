// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package googlepubsub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/pkg/errors"
	"google.golang.org/api/option"

	"github.com/elastic/beats/filebeat/channel"
	"github.com/elastic/beats/filebeat/input"
	"github.com/elastic/beats/filebeat/util"
	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/common/atomic"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/version"
)

const (
	inputName = "google-pubsub"
)

func init() {
	err := input.Register(inputName, NewInput)
	if err != nil {
		panic(errors.Wrap(err, "failed to register google-pubsub input"))
	}
}

type pubsubInput struct {
	config

	log    *logp.Logger
	outlet channel.Outleter

	stopOnce sync.Once
	done     chan struct{}
	wg       sync.WaitGroup

	ackedCount *atomic.Uint32
}

// NewInput creates a new Google Cloud Pub/Sub input that consumes events from
// a topic subscription.
func NewInput(
	cfg *common.Config,
	outlet channel.Connector,
	inputCtx input.Context,
) (input.Input, error) {
	// Extract and validate the input's configuration.
	var conf config
	if err := cfg.Unpack(&conf); err != nil {
		return nil, err
	}

	// Build outlet for events.
	out, err := outlet(cfg, inputCtx.DynamicFields)
	if err != nil {
		return nil, err
	}

	in := &pubsubInput{
		config: conf,
		log: logp.NewLogger("google.pubsub").With(
			"pubsub_project", conf.ProjectID,
			"pubsub_topic", conf.Topic,
			"pubsub_subscription", conf.Subscription),
		outlet:     out,
		done:       make(chan struct{}),
		ackedCount: atomic.NewUint32(0),
	}

	// Relay done signal from the input context.
	go func() {
		<-inputCtx.Done
		in.nonBlockingStop()
	}()

	in.log.Info("Initialized Google Pub/Sub input.")
	return in, nil
}

func (in *pubsubInput) Run() {
	defer in.nonBlockingStop()

	in.wg.Add(1)
	defer in.wg.Done()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-in.done
		in.log.Debug("Cancelling Google Pub/Sub client context because input is stopping.")
		cancel()
	}()

	userAgent := fmt.Sprintf("Elastic Filebeat/%s (%s; %s; %s; %s)",
		version.GetDefaultVersion(), runtime.GOOS, runtime.GOARCH,
		version.Commit(), version.BuildTime())

	// TODO: add a way to embed json credentials into config file
	client, err := pubsub.NewClient(ctx, in.ProjectID,
		option.WithCredentialsFile(in.CredentialsFile),
		option.WithUserAgent(userAgent),
	)
	if err != nil {
		in.log.Error(err)
		return
	}
	defer client.Close()

	// Pub/Sub message IDs are unique within a topic so add bytes derived from
	// project+topic to make the ID more unique.
	h := sha256.New()
	h.Write([]byte(in.ProjectID))
	h.Write([]byte(in.Topic))
	prefix := hex.EncodeToString(h.Sum(nil))
	prefix = prefix[:10]

	sub := client.Subscription(in.Subscription)
	sub.ReceiveSettings.NumGoroutines = runtime.GOMAXPROCS(0)
	sub.ReceiveSettings.MaxOutstandingMessages = 50

	err = sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		event := beat.Event{
			Timestamp: msg.PublishTime.UTC(),
			Fields: common.MapStr{
				"event": common.MapStr{
					"id":      prefix + "-" + msg.ID,
					"created": time.Now().UTC(),
				},
				"message": string(msg.Data),
			},
		}
		if len(msg.Attributes) > 0 {
			event.Fields.Put("labels", msg.Attributes)
		}

		if ok := in.outlet.OnEvent(&util.Data{Event: event}); ok {
			// TODO: Does OnEvent success signal end-to-end ACK?
			msg.Ack()
			in.ackedCount.Inc()
		} else {
			in.log.Debug("OnEvent has failed. Stopping input.")
			msg.Nack()
			in.nonBlockingStop()
		}
	})
	if err != nil {
		in.log.Error(err)
		return
	}
}

func (in *pubsubInput) Stop() {
	in.nonBlockingStop()
	in.wg.Wait()
	in.log.Debugw("Pub/Sub input is stopped.", "pubsub_acked", in.ackedCount.Load())
}

func (in *pubsubInput) nonBlockingStop() {
	in.stopOnce.Do(func() {
		close(in.done)
		in.log.Debug("Pub/Sub input is stopping.")
	})
}

func (in *pubsubInput) Wait() {
	in.Stop()
}
