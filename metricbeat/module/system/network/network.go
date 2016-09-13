// +build darwin freebsd linux windows

package network

import (
	"fmt"
	"strings"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/metricbeat/mb"
	"github.com/elastic/beats/metricbeat/module/system"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/net"
)

var debugf = logp.MakeDebug("system-network")

func init() {
	if err := mb.Registry.AddMetricSet("system", "network", New); err != nil {
		panic(err)
	}
}

// MetricSet for fetching system network IO metrics.
type MetricSet struct {
	mb.BaseMetricSet
	interfaces map[string]struct{}
}

// New is a mb.MetricSetFactory that returns a new MetricSet.
func New(base mb.BaseMetricSet) (mb.MetricSet, error) {
	systemModule, ok := base.Module().(*system.Module)
	if !ok {
		return nil, fmt.Errorf("unexpected module type")
	}
	debugf("network io stats will be included for %v",
		systemModule.NetworkInterfaces)

	return &MetricSet{
		BaseMetricSet: base,
		interfaces:    systemModule.NetworkInterfaces,
	}, nil
}

// Fetch fetches disk IO metrics from the OS.
func (m *MetricSet) Fetch() ([]common.MapStr, error) {
	stats, err := net.IOCounters(true)
	if err != nil {
		return nil, errors.Wrap(err, "network io counters")
	}

	var events []common.MapStr
	if m.interfaces == nil {
		// Include all stats.
		for _, counters := range stats {
			events = append(events, ioCountersToMapStr(counters))
		}
	} else {
		// Select stats by interface name.
		for _, counters := range stats {
			name := strings.ToLower(counters.Name)
			if _, include := m.interfaces[name]; include {
				events = append(events, ioCountersToMapStr(counters))
				continue
			}
		}
	}

	return events, nil
}

func ioCountersToMapStr(counters net.IOCountersStat) common.MapStr {
	return common.MapStr{
		"name": counters.Name,
		"in": common.MapStr{
			"errors":  counters.Errin,
			"dropped": counters.Dropin,
			"bytes":   counters.BytesRecv,
			"packets": counters.PacketsRecv,
		},
		"out": common.MapStr{
			"errors":  counters.Errout,
			"dropped": counters.Dropout,
			"packets": counters.PacketsSent,
			"bytes":   counters.BytesSent,
		},
	}
}
