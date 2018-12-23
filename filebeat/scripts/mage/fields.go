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
	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage"
)

// Fields generates fields.yml and fields.go files for the Beat.
func Fields() {
	switch SelectLogic {
	case mage.OSSProject:
		mg.Deps(libbeatAndBeatCommonFieldsGo, moduleFieldsGo)
		mg.Deps(ossFieldsYML)
	case mage.XPackProject:
		mg.Deps(xpackFieldsYML, moduleFieldsGo, inputFieldsGo)
	}
}

// libbeatAndBeatCommonFieldsGo generates a fields.go containing both
// libbeat and Auditbeat's common fields.
func libbeatAndBeatCommonFieldsGo() error {
	if err := mage.GenerateFieldsYAML(); err != nil {
		return err
	}
	return mage.GenerateAllInOneFieldsGo()
}

// ossFieldsYML generates the fields.yml file containing all fields.
func ossFieldsYML() error {
	return mage.GenerateFieldsYAML("module")
}

// fieldsYML generates the fields.yml file containing all fields.
func xpackFieldsYML() error {
	return mage.GenerateFieldsYAML(mage.OSSBeatDir("module"), "module", "input")
}

// moduleFieldsGo generates a fields.go for each module.
func moduleFieldsGo() error {
	return mage.GenerateModuleFieldsGo("module")
}

func inputFieldsGo() error {
	return mage.GenerateModuleFieldsGo("input")
}
