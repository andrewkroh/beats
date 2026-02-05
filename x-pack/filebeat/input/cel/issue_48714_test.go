// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cel

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elastic/beats/v7/filebeat/input/v2"
	"github.com/elastic/beats/v7/x-pack/filebeat/otel"
	conf "github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent-libs/monitoring"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type InMemoryExporter struct {
	mu      sync.Mutex
	Metrics []metricdata.ResourceMetrics
}

func (e *InMemoryExporter) Export(ctx context.Context, metrics *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Metrics = append(e.Metrics, *metrics)
	return nil
}

func (e *InMemoryExporter) Shutdown(ctx context.Context) error { return nil }
func (e *InMemoryExporter) ForceFlush(ctx context.Context) error { return nil }

func (e *InMemoryExporter) Temporality(k sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.DeltaTemporality
}

func (e *InMemoryExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(k)
}

func TestIssue48714(t *testing.T) {
	// Setup InMemory Exporter
	exporter := &InMemoryExporter{}
	otel.GetGlobalMetricsExporterFactory().SetGlobalMetricsExporter(exporter)
	defer otel.GetGlobalMetricsExporterFactory().SetGlobalMetricsExporter(nil)

	// Configure input
	configMap := map[string]interface{}{
		"interval": "10ms",
		// Program that returns a single error object in "events", which causes isDegraded=true
		// but flows to publication as an event.
		"program": `{"events": {"error": "simulated failure"}}`,
		"resource": map[string]interface{}{
			"url": "http://localhost",
		},
	}
	cfg := conf.MustNewConfigFrom(configMap)
	conf := defaultConfig()
	if err := cfg.Unpack(&conf); err != nil {
		t.Fatalf("failed to unpack config: %v", err)
	}

	src := &source{conf}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	v2Ctx := v2.Context{
		Logger:          logp.NewLogger("cel_test"),
		ID:              "test_id_issue_48714",
		IDWithoutName:   "test_id_issue_48714",
		Cancelation:     ctx,
		MetricsRegistry: monitoring.NewRegistry(),
	}

	// Mock publisher that always succeeds
	pub := &publisher{
		done: func() {},
	}

	// Run input in a goroutine
	errCh := make(chan error, 1)
	go func() {
		err := input{}.run(v2Ctx, src, nil, pub, &v2Ctx)
		errCh <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-errCh

	exporter.mu.Lock()
	defer exporter.mu.Unlock()

	if len(exporter.Metrics) == 0 {
		t.Fatal("No metrics collected")
	}

	var successCount int64

	for _, resourceMetrics := range exporter.Metrics {
		for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
			for _, metric := range scopeMetrics.Metrics {
				if metric.Name == "input.cel.periodic.program.run.success" {
					switch data := metric.Data.(type) {
					case metricdata.Sum[int64]:
						for _, dp := range data.DataPoints {
							successCount += dp.Value
						}
					}
				}
			}
		}
	}

	if successCount > 0 {
		t.Errorf("Expected 0 successes for failed program, got %d", successCount)
	}
}
