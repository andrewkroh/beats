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
	"context"

	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage"
)

// UnitTest executes the unit tests (Go and Python).
func UnitTest() {
	mg.SerialDeps(GoUnitTest, PythonUnitTest)
}

// GoTestUnit alias for goUnitTest (needed by dev-tools/jenkins_ci.ps1).
// Deprecated: Remove this after the Jenkins script is fixed.
func GoTestUnit(ctx context.Context) error {
	return GoUnitTest(ctx)
}

// GoUnitTest executes the Go unit tests.
// Use TEST_COVERAGE=true to enable code coverage profiling.
// Use RACE_DETECTOR=true to enable the race detector.
func GoUnitTest(ctx context.Context) error {
	mg.Deps(Fields)
	return mage.GoTest(ctx, mage.DefaultGoTestUnitArgs())
}

// PythonUnitTest executes the python system tests.
func PythonUnitTest() error {
	mg.SerialDeps(Fields, mage.BuildSystemTestBinary)
	return mage.PythonNoseTest(mage.DefaultPythonTestUnitArgs())
}

// IntegTest executes integration tests (it uses Docker to run the tests).
func IntegTest() {
	mage.AddIntegTestUsage()
	defer mage.StopIntegTestEnv()
	mg.SerialDeps(GoIntegTest, PythonIntegTest)
}

// GoIntegTest executes the Go integration tests.
// Use TEST_COVERAGE=true to enable code coverage profiling.
// Use RACE_DETECTOR=true to enable the race detector.
func GoIntegTest(ctx context.Context) error {
	return mage.RunIntegTest("goIntegTest", func() error {
		return mage.GoTest(ctx, mage.DefaultGoTestIntegrationArgs())
	})
}

// PythonIntegTest executes the python system tests in the integration environment (Docker).
func PythonIntegTest(ctx context.Context) error {
	if !mage.IsInIntegTestEnv() {
		mg.SerialDeps(Fields, Dashboards)
	}
	return mage.RunIntegTest("pythonIntegTest", func() error {
		mg.Deps(mage.BuildSystemTestBinary)
		return mage.PythonNoseTest(mage.DefaultPythonTestIntegrationArgs())
	})
}
