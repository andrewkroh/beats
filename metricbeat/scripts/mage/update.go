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
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage"
	"github.com/elastic/beats/dev-tools/mage/target/build"
	"github.com/elastic/beats/dev-tools/mage/target/common"
	"github.com/elastic/beats/dev-tools/mage/target/dashboards"
	"github.com/elastic/beats/dev-tools/mage/target/docs"
	"github.com/elastic/beats/dev-tools/mage/target/integtest"
	"github.com/elastic/beats/dev-tools/mage/target/unittest"
)

func init() {
	common.RegisterCheckDeps(Update.All)

	dashboards.RegisterImportDeps(build.Build, Update.Dashboards)

	docs.RegisterDeps(Update.FieldDocs, Update.ModuleDocs)

	unittest.RegisterPythonTestDeps(Update.Fields)

	integtest.RegisterPythonTestDeps(Update.Fields, Update.Dashboards)
}

var (
	// SelectLogic configures the types of project logic to use (OSS vs X-Pack).
	SelectLogic mage.ProjectType
)

type Update mg.Namespace

func (Update) All() {
	mg.Deps(Update.Fields, Update.Config, Update.Dashboards,
		Update.Includes, Update.ModulesD)
}

func (Update) Config() error {
	return config()
}

// Dashboards collects all the dashboards and generates index patterns.
func (Update) Dashboards() error {
	mg.Deps(fb.FieldsYML)
	switch SelectLogic {
	case mage.OSSProject:
		return mage.KibanaDashboards(mage.OSSBeatDir("module"))
	case mage.XPackProject:
		return mage.KibanaDashboards(mage.OSSBeatDir("module"),
			mage.XPackBeatDir("module"))
	default:
		panic(mage.ErrUnknownProjectType)
	}
}

func (Update) Fields() {
	mg.Deps(fb.All)
}

func (Update) Includes() error {
	mg.Deps(Update.Fields)
	return mage.GenerateIncludeListGo(nil, []string{"module"})
}

func (Update) ModulesD() error {
	// Only generate modules.d if there is a module dir. Newly generated
	// beats based on Metricbeat initially do not have a module dir.
	if _, err := os.Stat("module"); err == nil {
		return mage.GenerateDirModulesD(mage.EnableModule("system"))
	}
	return nil
}

// FieldDocs generates docs/fields.asciidoc containing all fields (including x-pack).
func (Update) FieldDocs() error {
	mg.Deps(fb.FieldsAllYML)
	return mage.Docs.FieldDocs(mage.FieldsAllYML)
}

// ModuleDocs collects documentation from modules (both OSS and X-Pack).
func (Update) ModuleDocs() error {
	ve, err := mage.PythonVirtualenv()
	if err != nil {
		return err
	}

	python, err := mage.LookVirtualenvPath(ve, "python")
	if err != nil {
		return err
	}

	if err = os.RemoveAll(mage.OSSBeatDir("docs/modules")); err != nil {
		return err
	}
	if err = os.MkdirAll(mage.OSSBeatDir("docs/modules"), 0755); err != nil {
		return err
	}

	// TODO: Port this script to Go.

	// Warning: This script does NOT work outside of the OSS filebeat directory
	// because it was not written in a portable manner.
	return runIn(mage.OSSBeatDir(), python,
		mage.OSSBeatDir("scripts/docs_collector.py"),
		"--beat", mage.BeatName)
}

func runIn(dir, cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Dir = dir
	c.Env = os.Environ()
	c.Stderr = os.Stderr
	if mg.Verbose() {
		c.Stdout = os.Stdout
	}
	c.Stdin = os.Stdin
	log.Printf("exec: (pwd=%v) %v %v", dir, cmd, strings.Join(args, " "))
	return c.Run()
}
