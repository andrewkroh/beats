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
	"google.golang.org/api/option"

	"github.com/elastic/beats/filebeat/channel"
	"github.com/elastic/beats/filebeat/input"
	"github.com/elastic/beats/filebeat/util"
	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/version"
)

const (
	inputName = "google-pubsub"
)

func init() {
	err := input.Register(inputName, NewInput)
	if err != nil {
		panic(err)
	}
}

type pubsubInput struct {
	config

	log    *logp.Logger
	outlet channel.Outleter

	stopOnce sync.Once
	done     chan struct{}
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
		outlet: out,
		done:   make(chan struct{}),
	}

	go func() {
		<-out.Done()
		in.log.Debug("Stopping input because Outlet is done.")
		in.Stop()
	}()

	in.log.Info("Initialized Google Pub/Sub input.")
	return in, nil
}

func (in *pubsubInput) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-in.done
		in.log.Debug("Cancelling Google Pub/Sub client receive because input in stopping.")
		cancel()
	}()

	in.log.Info("Using credentials_file:", in.CredentialsFile)
	client, err := pubsub.NewClient(ctx, in.ProjectID,
		option.WithCredentialsFile(in.CredentialsFile),
		option.WithUserAgent(fmt.Sprintf("Elastic Filebeat/%s (%s; %s; %s; %s)", version.GetDefaultVersion(), runtime.GOOS, runtime.GOARCH, version.Commit(), version.BuildTime())))
	if err != nil {
		in.log.Error(err)
		in.Stop()
		return
	}
	defer client.Close()

	sub := client.Subscription(in.Subscription)

	// Pub/Sub message IDs are unique with a topic so add project+topic to
	// make them more unique.
	h := sha256.New()
	h.Write([]byte(in.ProjectID))
	h.Write([]byte(in.Topic))
	prefix := hex.EncodeToString(h.Sum(nil))
	prefix = prefix[:10]

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
			//msg.Ack()
		} else {
			msg.Nack()
		}
	})
	if err != nil {
		in.log.Error(err)
	}
}

func (in *pubsubInput) Stop() {
	in.stopOnce.Do(func() {
		close(in.done)
	})
}

func (in *pubsubInput) Wait() {
	in.Stop()
}
