package iptables

import (
	"fmt"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/metricbeat/mb"

	"github.com/fd0/go-iptables"
	"github.com/pkg/errors"
)

func init() {
	if err := mb.Registry.AddMetricSet("linux", "iptables", New); err != nil {
		panic(err)
	}
}

// MetricSet for fetching system disk IO metrics.
type MetricSet struct {
	mb.BaseMetricSet
}

// New is a mb.MetricSetFactory that returns a new MetricSet.
func New(base mb.BaseMetricSet) (mb.MetricSet, error) {
	return &MetricSet{base}, nil
}

// Fetch fetches disk IO metrics from the OS.
func (m *MetricSet) Fetch() ([]common.MapStr, error) {
	const tableName = "filter"
	table, err := iptables.NewIPTables(tableName)
	if err != nil {
		return nil, errors.Wrapf(err, "new iptables for %s", table)
	}
	defer table.Close()

	fmt.Printf("ip4tables:\n----------\n")
	chains := table.Chains()
	fmt.Printf("chains: %v\n", chains)

	for _, chain := range chains {
		counter, err := table.Counter(chain)

		if table.BuiltinChain(chain) {
			if err != nil {
				return nil, err
			}

			fmt.Printf("%v: %d packets, %d bytes\n", chain, counter.Packets, counter.Bytes)
		} else {
			if err == nil {
				return nil, fmt.Errorf("got counter for a not builtin chain?")
			}
			fmt.Printf("%v\n", chain)
		}

		for i, rule := range table.Rules(chain) {
			fmt.Printf("    rule %d: %s\n", i, rule)
		}

		//err = table.Zero(chain)
		//if err != nil {
		//	return nil, errors.Wrapf(err, "error zeroing chain %s", chain, err)
		//}
	}

	return nil, nil
}
