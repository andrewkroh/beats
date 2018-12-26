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

package mage

import (
	"github.com/elastic/beats/dev-tools/mage"
)

// SelectLogic configures the types of project logic to use (OSS vs X-Pack).
var SelectLogic mage.ProjectType

// Config generates short/reference/docker configs.
func Config() error {
	return mage.Config(mage.ShortConfigType|mage.ReferenceConfigType, configFileParams(), ".")
}

func configFileParams() mage.ConfigFileParams {
	return mage.ConfigFileParams{
		ShortParts: []string{
			mage.OSSBeatDir("_meta/beat.yml"),
			mage.LibbeatDir("_meta/config.yml"),
		},
		ReferenceParts: []string{
			mage.OSSBeatDir("_meta/beat.reference.yml"),
			mage.LibbeatDir("_meta/config.reference.yml"),
		},
		DockerParts: []string{
			mage.OSSBeatDir("_meta/beat.docker.yml"),
			mage.LibbeatDir("_meta/config.docker.yml"),
		},
		ExtraVars: map[string]interface{}{
			"GOOS": "windows",
		},
	}
}
