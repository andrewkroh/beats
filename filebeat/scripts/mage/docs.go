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
	"github.com/magefile/mage/sh"

	"github.com/elastic/beats/dev-tools/mage"
)

// Docs generates modules and field documentation.
func Docs() {
	switch SelectLogic {
	case mage.OSSProject:
		// Module() only works from the OSS dir because the python script is
		// not portable.
		mg.Deps(docs.Module, docs.Fields)
	case mage.XPackProject:
		mg.Deps(docs.Fields)
	}
}

type docs struct{}

// Module collects documentation from modules (both OSS and X-Pack).
func (docs) Module() error {
	ve, err := mage.PythonVirtualenv()
	if err != nil {
		return err
	}

	python, err := mage.LookVirtualenvPath(ve, "python")
	if err != nil {
		return err
	}

	// TODO: Port this script to Go.
	return sh.Run(python,
		mage.OSSBeatDir("scripts/docs_collector.py"),
		"--beat", mage.BeatName)
}

// Fields generates docs/fields.asciidoc containing all fields (including x-pack).
func (docs) Fields() error {
	inputs := []string{
		mage.OSSBeatDir("module"),
		mage.XPackBeatDir("module"),
		mage.OSSBeatDir("input"),
		mage.XPackBeatDir("input"),
	}
	output := mage.CreateDir(mage.OSSBeatDir("build/fields/fields.all.yml"))
	if err := mage.GenerateFieldsYAMLTo(output, inputs...); err != nil {
		return err
	}
	return mage.Docs.FieldDocs(output)
}
