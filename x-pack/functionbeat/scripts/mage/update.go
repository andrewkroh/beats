// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mage

import (
	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage/target/build"

	"github.com/elastic/beats/dev-tools/mage"
)

// Check runs fmt and update then returns an error if any modifications are found.
func Check() {
	mg.SerialDeps(mage.Format, Update, mage.Check)
}

// Dashboards collects all the dashboards and generates index patterns.
func Dashboards() error {
	mg.Deps(Fields)
	return mage.KibanaDashboards()
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
	mg.SerialDeps(Fields, Dashboards, Config)
}

func includeList() error {
	return mage.GenerateIncludeListGo(nil, nil)
}
