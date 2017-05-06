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
		if sample.Temperature > 0 {
			friendlyName := name(sample.SensorKey)
			event[friendlyName] = sample.Temperature
		}
	}

	return event, nil
}

var darwinSensorNames = map[string]string{
	"TA0P": "ambient_air_0",
	"TA1P": "ambient_air_1",
	"TC0D": "cpu_0_diode",
	"TC0H": "cpu_0_heatsink",
	"TC0P": "cpu_0_proximity",
	"TB0T": "enclosure_base_0",
	"TB1T": "enclosure_base_1",
	"TB2T": "enclosure_base_2",
	"TB3T": "enclosure_base_3",
	"TG0D": "gpu_0_diode",
	"TG0H": "gpu_0_heatsink",
	"TG0P": "gpu_0_proximity",
	"TH0P": "hard_drive_bay",
	"TM0S": "memory_slot_0",
	"TM0P": "memory_slots_proximity",
	"TN0H": "northbridge",
	"TN0D": "northbridge_diode",
	"TN0P": "northbridge_proximity",
	"TI0P": "thunderbolt_0",
	"TI1P": "thunderbolt_1",
	"TW0P": "wireless_module",
}

func name(sensor string) string {
	n, found := darwinSensorNames[sensor]
	if found {
		return n
	}

	return sensor
}
