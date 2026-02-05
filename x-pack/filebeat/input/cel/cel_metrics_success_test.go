// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cel

import (
	"context"
	"errors"
	"testing"
	"time"

	v2 "github.com/elastic/beats/v7/filebeat/input/v2"
	conf "github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent-libs/monitoring"
)

func TestSuccessMetricNotCountedWhenDegraded(t *testing.T) {
	t.Run("degraded", func(t *testing.T) {
		snapshot := runInputWithConfig(t, map[string]interface{}{
			"interval": 1,
			"program":  `{"events":[{"message":{"value": debug("divide by zero", 0/0)}}]}`,
			"state":    nil,
			"resource": map[string]interface{}{
				"url": "",
			},
		})

		if got := getUintMetric(t, snapshot, "cel_executions"); got != 1 {
			t.Fatalf("cel_executions = %d, want 1", got)
		}
		if got := getUintMetric(t, snapshot, "cel_success_executions"); got != 0 {
			t.Fatalf("cel_success_executions = %d, want 0", got)
		}
	})

	t.Run("success", func(t *testing.T) {
		snapshot := runInputWithConfig(t, map[string]interface{}{
			"interval": 1,
			"program":  `{"events":[{"message":"hello"}]}`,
			"state":    nil,
			"resource": map[string]interface{}{
				"url": "",
			},
		})

		if got := getUintMetric(t, snapshot, "cel_executions"); got != 1 {
			t.Fatalf("cel_executions = %d, want 1", got)
		}
		if got := getUintMetric(t, snapshot, "cel_success_executions"); got != 1 {
			t.Fatalf("cel_success_executions = %d, want 1", got)
		}
	})
}

func runInputWithConfig(t *testing.T, config map[string]interface{}) map[string]interface{} {
	t.Helper()

	cfg := conf.MustNewConfigFrom(config)
	conf := defaultConfig()
	conf.Redact = &redact{}
	if err := cfg.Unpack(&conf); err != nil {
		t.Fatalf("unexpected error unpacking config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	v2Ctx := v2.Context{
		Logger:          logp.NewLogger("cel_test"),
		ID:              "metrics_test",
		IDWithoutName:   "metrics_test",
		Cancelation:     ctx,
		MetricsRegistry: monitoring.NewRegistry(),
	}

	var client publisher
	client.done = func() {}

	err := input{}.run(v2Ctx, &source{conf}, nil, &client, &v2Ctx)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected error from running input: %v", err)
	}

	return monitoring.CollectStructSnapshot(v2Ctx.MetricsRegistry, monitoring.Full, false)
}

func getUintMetric(t *testing.T, snapshot map[string]interface{}, key string) uint64 {
	t.Helper()

	raw, ok := snapshot[key]
	if !ok {
		t.Fatalf("missing metric %q", key)
	}

	switch v := raw.(type) {
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case uint:
		return uint64(v)
	case int64:
		return uint64(v)
	case int32:
		return uint64(v)
	case int:
		return uint64(v)
	case float64:
		return uint64(v)
	default:
		t.Fatalf("unexpected metric type for %q: %T", key, raw)
	}

	return 0
}
