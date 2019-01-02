// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
// +build mage

package main

import (
	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage"

	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/common"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/build"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/pkg"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/dashboards"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/test"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/unittest"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/integtest"
	// mage:import
	metricbeat "github.com/elastic/beats/metricbeat/scripts/mage"
)

func init() {
	mage.BeatProjectType = mage.XPackProject

	mage.BeatLicense = "Elastic License"
}

// Update is an alias for update:all. This is a workaround for
// https://github.com/magefile/mage/issues/217.
func Update() { mg.Deps(metricbeat.Update.All) }
