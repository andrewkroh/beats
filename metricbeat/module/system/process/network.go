package process

import (
	"strings"

	"github.com/elastic/beats/libbeat/common"
)

func (m *MetricSet) getNetworkInterfaceStats(pid int) (common.MapStr, error) {
	proc, err := m.procfs.NewProc(pid)
	if err != nil {
		return nil, err
	}

	ns, err := proc.NewNamespaces()
	if err != nil {
		return nil, err
	}

	// Only report network metrics if they differ from the overall hosts. We
	// check if the process is in a different network namespace.
	if net, found := ns["net"]; !found || net.Inode == m.hostNetNamespace {
		return nil, nil
	}

	networkStats, err := proc.NewNetDev()
	if err != nil {
		return nil, err
	}

	if len(m.interfaces) > 0 {
		// Filter network devices by name.
		for i, stats := range networkStats {
			name := strings.ToLower(stats.Name)
			if _, include := m.interfaces[name]; !include {
				networkStats = append(networkStats[:i], networkStats[i+1:]...)
			}
		}
	}

	// Report metrics in aggregate (to avoid having an array).
	total := networkStats.Total()

	return common.MapStr{
		"name": total.Name,
		"in": common.MapStr{
			"errors":  total.RxErrors,
			"dropped": total.RxDropped,
			"bytes":   total.RxBytes,
			"packets": total.RxPackets,
		},
		"out": common.MapStr{
			"errors":  total.TxErrors,
			"dropped": total.TxDropped,
			"packets": total.TxPackets,
			"bytes":   total.TxBytes,
		},
	}, nil
}
