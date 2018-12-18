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

// Code generated by beats/dev-tools/cmd/asset/asset.go - DO NOT EDIT.

package cassandra

import (
	"github.com/elastic/beats/libbeat/asset"
)

func init() {
	if err := asset.SetFields("packetbeat", "cassandra", Asset); err != nil {
		panic(err)
	}
}

// Asset returns asset data
func Asset() string {
	return "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"
}
