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
	"github.com/pkg/errors"

	"github.com/elastic/beats/dev-tools/mage/target/build"

	"github.com/elastic/beats/dev-tools/mage"
)

// Check runs fmt and update then returns an error if any modifications are found.
func Check() {
	mg.SerialDeps(mage.Format, Update, mage.Check)
}

// Dashboards collects all the dashboards and generates index patterns.
func Dashboards() error {
	switch SelectLogic {
	case mage.OSSProject:
		return mage.KibanaDashboards("module")
	case mage.XPackProject:
		return mage.KibanaDashboards(mage.OSSBeatDir("module"), "module")
	default:
		panic(errors.Errorf("invalid SelectLogic value"))
	}
}

// DashboardsImport imports all dashboards to Kibana.
//
// Optional environment variables:
// - KIBANA_URL: URL of Kibana
func DashboardsImport() error {
	return mage.ImportDashboards(build.Build, Dashboards)
}

// Update is an alias for running fields, dashboards, config, includes, docs.
func Update() {
	mg.SerialDeps(updateWithoutDocs, Docs)
}

func updateWithoutDocs() {
	mg.SerialDeps(Fields, Dashboards, Config, includeList, modulesD)
}

func includeList() error {
	return mage.GenerateIncludeListGo([]string{"input/*"}, []string{"module"})
}

func modulesD() error {
	return mage.GenerateDirModulesD()
}
