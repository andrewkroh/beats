// +build darwin freebsd linux windows

package process

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/metricbeat/mb"
	"github.com/elastic/beats/metricbeat/module/system"

	"github.com/elastic/gosigar/cgroup"
	"github.com/elastic/gosigar/util"
	"github.com/pkg/errors"
)

var debugf = logp.MakeDebug("system.process")

func init() {
	if err := mb.Registry.AddMetricSet("system", "process", New); err != nil {
		panic(err)
	}
}

// MetricSet that fetches process metrics.
type MetricSet struct {
	mb.BaseMetricSet
	stats            *ProcStats
	cgroup           *cgroup.Reader
	procfs           *util.FS
	interfaces       map[string]struct{}
	hostNetNamespace uint32 // Inode of the current process'es net namespace.
}

// New creates and returns a new MetricSet.
func New(base mb.BaseMetricSet) (mb.MetricSet, error) {
	systemModule, ok := base.Module().(*system.Module)
	if !ok {
		return nil, fmt.Errorf("unexpected module type")
	}

	config := struct {
		Procs   []string `config:"processes"` // collect all processes by default
		Cgroups bool     `config:"cgroups"`
	}{
		Procs:   []string{".*"},
		Cgroups: false,
	}

	if err := base.Module().UnpackConfig(&config); err != nil {
		return nil, err
	}

	m := &MetricSet{
		BaseMetricSet: base,
		stats: &ProcStats{
			ProcStats: true,
			Procs:     config.Procs,
		},
		interfaces: systemModule.NetworkInterfaces,
	}

	err := m.stats.InitProcStats()
	if err != nil {
		return nil, err
	}

	if runtime.GOOS == "linux" && config.Cgroups {
		logp.Warn("EXPERIMENTAL: Cgroup is enabled for the system.process MetricSet.")
		m.cgroup, err = cgroup.NewReader(systemModule.HostFS, true)
		if err != nil {
			return nil, errors.Wrap(err, "error initializing cgroup reader")
		}

		rootfsMountpoint := "/"
		if systemModule.HostFS != "" {
			rootfsMountpoint = systemModule.HostFS
		}

		procfs, err := util.NewFS(filepath.Join(rootfsMountpoint, "proc"))
		if err != nil {
			return nil, errors.Wrap(err, "error initializing procfs reader")
		}

		self, err := procfs.Self()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get self process")
		}

		ns, err := self.NewNamespaces()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get namespace info for current process")
		}

		net, found := ns["net"]
		if !found {
			debugf("net namespace info wasn't found, process network stats " +
				"will not be reported")
			return nil, nil
		}

		m.procfs = &procfs
		m.hostNetNamespace = net.Inode
	}

	return m, nil
}

// Fetch fetches metrics for all processes. It iterates over each PID and
// collects process metadata, CPU metrics, and memory metrics.
func (m *MetricSet) Fetch() ([]common.MapStr, error) {
	procs, err := m.stats.GetProcStats()
	if err != nil {
		return nil, errors.Wrap(err, "process stats")
	}

	if m.cgroup != nil {
		for _, proc := range procs {
			pid, ok := proc["pid"].(int)
			if !ok {
				debugf("error converting pid to int for proc %+v", proc)
				continue
			}
			stats, err := m.cgroup.GetStatsForProcess(pid)
			if err != nil {
				debugf("error getting cgroups stats for pid=%d, %v", pid, err)
				continue
			}

			if statsMap := cgroupStatsToMap(stats); statsMap != nil {
				proc["cgroup"] = statsMap
			}

			if m.procfs == nil {
				continue
			}
			network, err := m.getNetworkInterfaceStats(pid)
			if err != nil {
				debugf("error getting process network stats for pid=%d, %v", pid, err)
				continue
			}

			if network != nil {
				proc["network"] = network
			}
		}
	}

	return procs, err
}
