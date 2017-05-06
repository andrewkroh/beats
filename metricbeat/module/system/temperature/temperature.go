// +build darwin,cgo

package temperature

import (
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/metricbeat/mb"
	"github.com/elastic/beats/metricbeat/mb/parse"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/host"
)

func init() {
	if err := mb.Registry.AddMetricSet("system", "temperature", New, parse.EmptyHostParser); err != nil {
		panic(err)
	}
}

type MetricSet struct {
	mb.BaseMetricSet
}

func New(base mb.BaseMetricSet) (mb.MetricSet, error) {
	return &MetricSet{base}, nil
}

func (m *MetricSet) Fetch() (common.MapStr, error) {
	samples, err := host.SensorsTemperatures()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sensor temperatures")
	}

	event := common.MapStr{}
	for _, sample := range samples {
		event[sample.SensorKey] = sample.Temperature
	}

	return event, nil
}
