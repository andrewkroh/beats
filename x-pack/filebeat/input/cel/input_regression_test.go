package cel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	v2 "github.com/elastic/beats/v7/filebeat/input/v2"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/version"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent-libs/monitoring"
	"github.com/elastic/elastic-agent-libs/transport/httpcommon"
	"github.com/gofrs/uuid/v5"
)

type mockPublisher struct {
	events []beat.Event
}

func (p *mockPublisher) Publish(event beat.Event, cursor interface{}) error {
	p.events = append(p.events, event)
	return nil
}

func TestProgramRunSuccessMetricRegression(t *testing.T) {
	// Setup test server that returns invalid JSON to cause evaluation error (isDegraded=true)
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer testServer.Close()

	// Configure input
	u, _ := url.Parse(testServer.URL)
	maxExecutions := 5
	maxAttempts := 1
	waitMin := time.Millisecond
	waitMax := time.Millisecond
	enabled := false

	cfg := config{
		Interval:      100 * time.Millisecond,
		Program:       `bytes(get(state.url).Body).as(body,{"events":[body.decode_json()]})`,
		MaxExecutions: &maxExecutions,
		Resource: &ResourceConfig{
			URL: &urlConfig{URL: u},
			Retry: retryConfig{
				MaxAttempts: &maxAttempts,
				WaitMin:     &waitMin,
				WaitMax:     &waitMax,
			},
			Transport: httpcommon.DefaultHTTPTransportSettings(),
		},
		FailureDump: &dumpConfig{Enabled: &enabled, Filename: ""},
	}

	src := &source{cfg: cfg}
	log := logp.NewLogger("cel_test")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentID := uuid.Must(uuid.NewV4())
	env := v2.Context{
		Logger:          log,
		MetricsRegistry: monitoring.NewRegistry(),
		Cancelation:     ctx,
		IDWithoutName:   "test-id",
		Agent:           beat.Info{Version: version.GetDefaultVersion(), ID: agentID},
	}

	inp := input{}
	pub := &mockPublisher{}
	done := make(chan struct{})

	// Run input in background
	go func() {
		// Run blocks until context is cancelled
		_ = inp.run(env, src, nil, pub, &env)
		close(done)
	}()

	// Wait for enough time for metrics to be generated
	// We expect multiple executions in 1 second given 100ms interval
	time.Sleep(2 * time.Second)
	cancel()
	<-done

	snapshot := monitoring.CollectStructSnapshot(env.MetricsRegistry, monitoring.Full, false)
	if got := getUintMetric(t, snapshot, "cel_executions"); got == 0 {
		t.Error("cel_executions should be greater than 0")
	}

	// The bug was that success was counted even when there was an evaluation error.
	// With the fix, success should NOT be counted when isDegraded is true.
	// We expect the success counter to remain at 0.
	if got := getUintMetric(t, snapshot, "cel_success_executions"); got != 0 {
		t.Errorf("cel_success_executions = %d, want 0", got)
	}
}
