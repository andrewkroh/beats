package golang

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/metricbeat/mb"
	"github.com/elastic/beats/metricbeat/mb/parse"
	"github.com/pkg/errors"
)

var hostParser = parse.URLHostParserBuilder{DefaultScheme: "http", PathConfigKey: "path", DefaultPath: "/debug/vars"}.Build()

func init() {
	// Register the MetricSetFactory function for the "expvar" MetricSet.
	if err := mb.Registry.AddMetricSet("golang", "expvar", NewMetricSet, hostParser); err != nil {
		panic(err)
	}
}

type MetricSet struct {
	mb.BaseMetricSet
	client *http.Client // HTTP client that is reused across requests.
}

func NewMetricSet(base mb.BaseMetricSet) (mb.MetricSet, error) {
	logp.Info("golang-expvar url=", base.HostData().SanitizedURI)

	return &MetricSet{
		BaseMetricSet: base,
		client:        &http.Client{Timeout: base.Module().Config().Timeout},
	}, nil
}

func (m *MetricSet) Fetch() (common.MapStr, error) {
	req, err := http.NewRequest("GET", m.HostData().SanitizedURI, nil)
	if m.HostData().User != "" || m.HostData().Password != "" {
		req.SetBasicAuth(m.HostData().User, m.HostData().Password)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making http request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ready response body")
	}

	var stats Stats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal golang memstats")
	}

	return common.MapStr{
		"command":  strings.Join(stats.Command, " "),
		"memstats": stats.MemStats,
	}, nil
}

type Stats struct {
	Command  []string      `json:"cmdline"`
	MemStats common.MapStr `json:"memstats"`
}
