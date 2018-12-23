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
	filebeat "github.com/elastic/beats/filebeat/scripts/mage"
)

func init() {
	filebeat.SelectLogic = mage.XPackProject

	mage.BeatDescription = "Filebeat sends log files to Logstash or directly to Elasticsearch."
	mage.BeatLicense = "Elastic License"
}
