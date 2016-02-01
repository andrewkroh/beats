package beat

import (
	"expvar"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/cfgfile"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/publisher"
	"github.com/elastic/beats/winlogbeat/checkpoint"
	"github.com/elastic/beats/winlogbeat/config"
	"github.com/elastic/beats/winlogbeat/eventlog"
)

// Metrics that can retrieved through the expvar web interface. Metrics must be
// enable through configuration in order for the web service to be started.
var (
	publishedEvents = expvar.NewMap("publishedEvents")
	ignoredEvents   = expvar.NewMap("ignoredEvents")
)

func init() {
	expvar.Publish("uptime", expvar.Func(uptime))
}

// Debug logging functions for this package.
var (
	debugf    = logp.MakeDebug("winlogbeat")
	detailf   = logp.MakeDebug("winlogbeat_detail")
	memstatsf = logp.MakeDebug("memstats")
)

// Time the application was started.
var startTime = time.Now().UTC()

type log struct {
	config.EventLogConfig
	eventLog eventlog.EventLog
}

type Winlogbeat struct {
	beat       *beat.Beat             // Common beat information.
	config     *config.Settings       // Configuration settings.
	eventLogs  []log                  // List of all event logs being monitored.
	done       chan struct{}          // Channel to initiate shutdown of main event loop.
	client     publisher.Client       // Interface to publish event.
	checkpoint *checkpoint.Checkpoint // Persists event log state to disk.
}

// New returns a new Winlogbeat.
func New() *Winlogbeat {
	return &Winlogbeat{}
}

func (eb *Winlogbeat) Config(b *beat.Beat) error {
	// Read configuration.
	err := cfgfile.Read(&eb.config, "")
	if err != nil {
		return fmt.Errorf("Error reading configuration file. %v", err)
	}

	// Validate configuration.
	err = eb.config.Winlogbeat.Validate()
	if err != nil {
		return fmt.Errorf("Error validating configuration file. %v", err)
	}
	debugf("Configuration validated. config=%v", eb.config)

	// Registry file grooming.
	if eb.config.Winlogbeat.RegistryFile == "" {
		eb.config.Winlogbeat.RegistryFile = config.DefaultRegistryFile
	}
	eb.config.Winlogbeat.RegistryFile, err = filepath.Abs(
		eb.config.Winlogbeat.RegistryFile)
	if err != nil {
		return fmt.Errorf("Error getting absolute path of registry file %s. %v",
			eb.config.Winlogbeat.RegistryFile, err)
	}
	logp.Info("State will be read from and persisted to %s",
		eb.config.Winlogbeat.RegistryFile)

	return nil
}

func (eb *Winlogbeat) Setup(b *beat.Beat) error {
	eb.beat = b
	eb.client = b.Events
	eb.done = make(chan struct{})

	var err error
	eb.checkpoint, err = checkpoint.NewCheckpoint(
		eb.config.Winlogbeat.RegistryFile, 10, 5*time.Second)
	if err != nil {
		return err
	}

	if eb.config.Winlogbeat.Metrics.BindAddress != "" {
		bindAddress := eb.config.Winlogbeat.Metrics.BindAddress
		sock, err := net.Listen("tcp", bindAddress)
		if err != nil {
			return err
		}
		go func() {
			logp.Info("Metrics hosted at http://%s/debug/vars", bindAddress)
			err := http.Serve(sock, nil)
			if err != nil {
				logp.Warn("Unable to launch HTTP service for metrics. %v", err)
				return
			}
		}()
	}

	return nil
}

func (eb *Winlogbeat) Run(b *beat.Beat) error {
	persistedState := eb.checkpoint.States()

	// Initialize metrics.
	publishedEvents.Add("total", 0)
	publishedEvents.Add("failures", 0)
	ignoredEvents.Add("total", 0)

	// TODO: If no event_logs are specified in the configuration, use the
	// Windows registry to discover the available event logs.
	eb.eventLogs = make([]log, 0, len(eb.config.Winlogbeat.EventLogs))
	for _, eventLogConfig := range eb.config.Winlogbeat.EventLogs {
		debugf("Initializing EventLog[%s]", eventLogConfig.Name)

		eventLog, err := eventlog.New(eventlog.Config{
			Name: eventLogConfig.Name,
			API:  eventLogConfig.API,
		})
		if err != nil {
			return fmt.Errorf("Failed to create new event log for %s. %v",
				eventLogConfig.Name, err)
		}

		// Initialize per event log metrics.
		publishedEvents.Add(eventLogConfig.Name, 0)
		ignoredEvents.Add(eventLogConfig.Name, 0)

		eb.eventLogs = append(eb.eventLogs, log{
			EventLogConfig: eventLogConfig,
			eventLog:       eventLog,
		})
	}

	var wg sync.WaitGroup
	for _, log := range eb.eventLogs {
		state, _ := persistedState[log.Name]
		ignoreOlder, _ := config.IgnoreOlderDuration(log.IgnoreOlder)

		// Start a goroutine for each event log.
		wg.Add(1)
		go eb.processEventLog(&wg, log.eventLog, state, ignoreOlder)
	}

	wg.Wait()
	eb.checkpoint.Shutdown()
	return nil
}

func (eb *Winlogbeat) Cleanup(b *beat.Beat) error {
	logp.Info("Dumping runtime metrics...")
	expvar.Do(func(kv expvar.KeyValue) {
		logf := logp.Info
		if kv.Key == "memstats" {
			logf = memstatsf
		}

		logf("%s=%s", kv.Key, kv.Value.String())
	})
	return nil
}

func (eb *Winlogbeat) Stop() {
	logp.Info("Stopping Winlogbeat")
	if eb.done != nil {
		close(eb.done)
	}
}

func (eb *Winlogbeat) processEventLog(
	wg *sync.WaitGroup,
	api eventlog.EventLog,
	state checkpoint.EventLogState,
	ignoreOlder time.Duration,
) {
	defer wg.Done()

	err := api.Open(state.RecordNumber)
	if err != nil {
		logp.Warn("EventLog[%s] Open() error. No events will be read from "+
			"this source. %v", api.Name(), err)
		return
	}
	defer func() {
		err := api.Close()
		if err != nil {
			logp.Warn("EventLog[%s] Close() error. %v", api.Name(), err)
			return
		}
	}()

	debugf("EventLog[%s] opened successfully", api.Name())

	records, errs := api.ReadPipeline(eb.done)
loop:
	for {
		select {
		case <-eb.done:
			break loop
		case err = <- errs:
			logp.Warn("EventLog[%s] Read() error: %v", api.Name(), err)
			break loop
		case lr := <- records:
			if ignoreOlder != 0 && time.Since(lr.TimeGenerated) > ignoreOlder {
				detailf("EventLog[%s] ignore_older filter dropping event: %s",
					api.Name(), lr.String())
				ignoredEvents.Add("total", 1)
				ignoredEvents.Add(api.Name(), 1)
				continue
			}

			ok := eb.client.PublishEvent(lr.ToMapStr(), publisher.Guaranteed)
			if ok {
				publishedEvents.Add("total", 1)
				publishedEvents.Add(api.Name(), 1)
			} else {
				logp.Warn("EventLog[%s] Failed to publish %d events",
					api.Name(), 1)
				publishedEvents.Add("failures", 1)
			}

			eb.checkpoint.Persist(api.Name(), lr.RecordNumber, lr.TimeGenerated.UTC())
		}
	}
}

// uptime returns a map of uptime related metrics.
func uptime() interface{} {
	now := time.Now().UTC()
	uptimeDur := now.Sub(startTime)

	return map[string]interface{}{
		"start_time":  startTime,
		"uptime":      uptimeDur.String(),
		"uptime_ms":   fmt.Sprintf("%d", uptimeDur.Nanoseconds()/int64(time.Microsecond)),
		"server_time": now,
	}
}
