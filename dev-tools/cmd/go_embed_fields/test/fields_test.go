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

package fields_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/elastic/beats/v7/libbeat/asset"
	"github.com/elastic/beats/v7/libbeat/mapping"
)

//go:generate go run .. -i fields.processor.yml -type beat -beat brewbeat -name add_tank_metadata -o fields.processor.go -pkg fields
//go:generate go run ..	-i fields.module.yml -type module -beat brewbeat -name brew -o fields.module.go -pkg fields
//go:generate go run .. -i fields.dataset.yml -type dataset -beat brewbeat -module brew -name mash -o fields.dataset.go -pkg fields

func TestGetFields(t *testing.T) {
	matches, err := filepath.Glob("fields.*.go")
	require.NoError(t, err)
	if len(matches) == 0 {
		t.Skip("You must run 'go generate' before running this test.")
	}

	yml, err := asset.GetFields("brewbeat")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Fields YAML:\n%v", string(yml))

	fields, err := mapping.LoadFields(yml)
	if err != nil {
		t.Fatal(err)
	}

	keys := fields.GetKeys()
	sort.Strings(keys)
	t.Logf("Fields: [%v]", strings.Join(keys, ", "))

	assert.Equal(t, []string{
		"brew.batch_id",
		"brew.mash.temperature",
		"tank.capacity",
		"tank.temperature",
	}, keys)
}
