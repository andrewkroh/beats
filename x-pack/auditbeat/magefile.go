// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build mage

package main

import (
	"github.com/elastic/beats/dev-tools/mage"

	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/common"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/build"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/pkg"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/dashboard"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/unittest"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/integtest"
	// mage:import
	auditbeat "github.com/elastic/beats/auditbeat/scripts/mage"
)

func init() {
	auditbeat.SelectLogic = mage.XPackProject

	mage.BeatDescription = "Audit the activities of users and processes on your system."
	mage.BeatLicense = "Elastic License"
}
