// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build mage

package main

import (
	"fmt"

	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage"

	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/common"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/build"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/test"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/unittest"
	// TODO:import - Not imported because we are skipping integTest.
	"github.com/elastic/beats/dev-tools/mage/target/integtest"
)

func init() {
	integtest.RegisterGoTestDeps(Fields)
	integtest.RegisterPythonTestDeps(Fields)
}

// Check checks that source code is formatted, vetted, and up-to-date.
func Check() {
	mg.SerialDeps(mage.Format, mage.Check)
}

// Fields generates a fields.yml for the Beat.
func Fields() error {
	return mage.GenerateFieldsYAML(mage.OSSBeatDir("processors"))
}

func IntegTest() {
	fmt.Println(">> integTest: Skipped due to https://github.com/elastic/beats/issues/9597.")
}
