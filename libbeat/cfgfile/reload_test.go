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

package cfgfile

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/paths"
)

func TestReloaderPathVariableExpansion(t *testing.T) {
	paths.Paths = &paths.Path{Config: "/config"}

	confFile := filepath.Join(t.TempDir(), "conf.yml")
	yamlData := `
- module: foo
  var.credentials: '${path.config}/key.json'
`
	require.NoError(t, ioutil.WriteFile(confFile, []byte(yamlData), 0644))

	r := &Reloader{}
	confs, err := r.loadConfigs([]string{confFile})
	require.NoError(t, err)
	require.Len(t, confs, 1)
	conf := confs[0]

	hash, err := HashConfig(conf.Config)
	require.NoError(t, err)
	require.NotZero(t, hash)

	m := common.MapStr{}
	err = conf.Config.Unpack(&m)
	require.NoError(t, err)

	v, err := m.GetValue("var.credentials")
	require.NoError(t, err)
	require.Equal(t, "/config/key.json", v)
}
