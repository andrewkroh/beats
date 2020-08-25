// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// +build integration

package status

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"

	"github.com/elastic/beats/v7/docker/containertest"
	mbtest "github.com/elastic/beats/v7/metricbeat/mb/testing"
	"github.com/elastic/beats/v7/metricbeat/module/kibana/mtest"
)

// NOTE: This is PoC demo code and should be improved.
func TestMain(m *testing.M) {
	os.Exit(containertest.RunAndCleanup(m))
}

func TestFetch(t *testing.T) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatal(err)
	}
	pool.MaxWait = 3 * time.Minute

	esOptions := &dockertest.RunOptions{
		Name:       "elasticsearch",
		Repository: "bazel/elasticsearch",
		Tag:        "latest",
		PortBindings: map[docker.Port][]docker.PortBinding{
			"9200/tcp": []docker.PortBinding{{HostIP: "127.0.0.1", HostPort: "0"}},
		},
		Env: []string{
			"ES_JAVA_OPTS=-Xms1g -Xmx1g",
			"network.host=",
			"transport.host=127.0.0.1",
			"http.host=0.0.0.0",
			"xpack.security.enabled=false",
			"indices.id_field_data.enabled=true",
		},
	}
	es, err := pool.RunWithOptions(esOptions)
	if err != nil {
		t.Fatalf("Could not start resource: %s", err)
	}
	containertest.RunAtExit(func() {
		pool.Purge(es)
	})
	defer pool.Purge(es)

	kibanaOptions := &dockertest.RunOptions{
		Name:       "kibana",
		Repository: "bazel/kibana",
		Tag:        "latest",
		PortBindings: map[docker.Port][]docker.PortBinding{
			"5601/tcp": []docker.PortBinding{{HostIP: "127.0.0.1", HostPort: "0"}},
		},
		Links: []string{
			es.Container.Name,
		},
	}
	kibana, err := pool.RunWithOptions(kibanaOptions)
	if err != nil {
		t.Fatalf("Could not start resource: %s", err)
	}
	containertest.RunAtExit(func() {
		pool.Purge(kibana)
	})
	defer pool.Purge(kibana)

	endpoint := fmt.Sprintf("localhost:%s", kibana.GetPort("5601/tcp"))
	t.Log("Kibana address:", endpoint)

	err = pool.Retry(func() error {
		r, err := http.Get("http://" + endpoint + "/api/status")
		if err != nil {
			return err
		}
		defer r.Body.Close()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return err
		}
		if r.StatusCode == http.StatusOK && bytes.Contains(body, []byte(`{"overall":{"state":"green"`)) && bytes.Contains(body, []byte(`"metrics"`)) {
			return nil
		}
		return fmt.Errorf("http status: %d", r.StatusCode)
	})
	if err != nil {
		t.Fatal("Timeout waiting for Kibana", err)
	}

	f := mbtest.NewReportingMetricSetV2Error(t, mtest.GetConfig("status", endpoint, false))
	events, errs := mbtest.ReportingFetchV2Error(f)

	require.Empty(t, errs)
	require.NotEmpty(t, events)

	t.Logf("%s/%s event: %+v", f.Module().Name(), f.Name(),
		events[0].BeatEvent("kibana", "status").Fields.StringToPrint())
}
