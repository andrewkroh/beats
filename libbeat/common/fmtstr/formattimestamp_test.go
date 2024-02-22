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

package fmtstr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/elastic-agent-libs/mapstr"
)

func TestTimestampFormatString(t *testing.T) {
	tests := []struct {
		title        string
		format       string
		staticFields mapstr.M
		timestamp    time.Time
		expected     string
	}{
		{
			"empty string",
			"",
			nil,
			time.Time{},
			"",
		},
		{
			"no fields configured",
			"format string",
			nil,
			time.Time{},
			"format string",
		},
		{
			"expand field",
			"%{[key]}",
			mapstr.M{"key": "value"},
			time.Time{},
			"value",
		},
		{
			"expand with default",
			"%{[key]:default}",
			nil,
			time.Time{},
			"default",
		},
		{
			"expand nested field",
			"%{[nested.key]}",
			mapstr.M{"nested": mapstr.M{"key": "value"}},
			time.Time{},
			"value",
		},
		{
			"test timestamp formatter",
			"%{[key]}: %{+YYYY.MM.dd}",
			mapstr.M{"key": "timestamp"},
			time.Date(2015, 5, 1, 20, 12, 34, 0, time.UTC),
			"timestamp: 2015.05.01",
		},
		{
			"test timestamp formatter",
			"%{[@timestamp]}: %{+YYYY.MM.dd}",
			mapstr.M{"key": "timestamp"},
			time.Date(2015, 5, 1, 20, 12, 34, 0, time.UTC),
			"2015-05-01T20:12:34.000Z: 2015.05.01",
		},
		{
			"test windows path with no format",
			`C:\Users\jenkins\workspace\run\\test_airflow.Test.test_server507/output\`,
			nil,
			time.Time{},
			`C:\Users\jenkins\workspace\run\\test_airflow.Test.test_server507/output\`,
		},
		{
			"test posix path",
			"/var/log/%{[agent][id]}-%{+YYYY_MM_dd-HH_mm_ss}",
			mapstr.M{"agent": mapstr.M{"id": "08072bc2-2abd-4bab-896d-814ae9ac5518"}},
			time.Date(2015, 5, 1, 20, 12, 34, 0, time.UTC),
			"/var/log/08072bc2-2abd-4bab-896d-814ae9ac5518-2015_05_01-20_12_34",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.title, func(t *testing.T) {
			efs, err := CompileEvent(test.format)
			if err != nil {
				t.Error(err)
				return
			}

			fs, err := NewTimestampFormatString(efs, test.staticFields)
			if err != nil {
				t.Error(err)
				return
			}

			actual, err := fs.Run(test.timestamp)

			assert.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}
