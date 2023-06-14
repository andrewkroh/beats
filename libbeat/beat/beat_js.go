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

package beat

import (
	"github.com/elastic/beats/v7/libbeat/common/reload"
	"github.com/elastic/beats/v7/libbeat/management"
	"github.com/elastic/elastic-agent-libs/api"
	"github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/keystore"
)

// Beat contains the basic beat data and the publisher client used to publish
// events.
type Beat struct {
	Info      Info     // beat metadata.
	Publisher Pipeline // Publisher pipeline

	InSetupCmd bool // this is set to true when the `setup` command is called

	OverwritePipelinesCallback OverwritePipelinesCallback // ingest pipeline loader callback
	// XXX: remove Config from public interface.
	//      It's currently used by filebeat modules to setup the Ingest Node
	//      pipeline and ML jobs.
	Config *BeatConfig // Common Beat configuration data.

	// OutputConfigReloader may be set by a Creator to watch for output config changes.
	//
	// This reloader is called in addition to libbeat's internal output reloader, which
	// is responsible for reconfiguring Publisher.
	OutputConfigReloader reload.Reloadable

	BeatConfig *config.C // The beat's own configuration section

	Fields []byte // Data from fields.yml

	Manager management.Manager // manager

	Keystore keystore.Keystore

	//Instrumentation instrumentation.Instrumentation // instrumentation holds an APM agent for capturing and reporting traces

	API *api.Server // API server. This is nil unless the http endpoint is enabled.
}
