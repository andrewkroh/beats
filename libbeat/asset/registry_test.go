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

package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFields(t *testing.T) {
	resetRegistry()

	data := "hello world"
	d, err := EncodeData(data)
	require.NoError(t, err)

	f := func() string {
		return d
	}

	SetFields("test", "foo", 1, f)
	newData, err := GetFields("test")
	require.NoError(t, err)
	assert.Equal(t, data, string(newData))
}

func TestGetFieldsForModule(t *testing.T) {
	resetRegistry()

	SetFields("brewbeat", "brewing", ModuleFieldsPri, func() string {
		data, _ := EncodeData("module: brewing\n")
		return data
	})

	SetModuleDatasetFields("brewbeat", "brewing", "mashing", func() string {
		data, _ := EncodeData("dataset: mashing")
		return data
	})

	fields, err := GetFields("brewbeat")
	require.NoError(t, err)
	assert.Equal(t, string(fields), `module: brewing
dataset: mashing`)
}

func resetRegistry() {
	fieldsRegistry = map[string]map[int]map[string][]func() string{}
	moduleDatasetRegistry = map[string]map[string]map[string]func() string{}
}
