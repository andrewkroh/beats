// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mage

import (
	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage/target/common"
	"github.com/elastic/beats/dev-tools/mage/target/dashboards"
	"github.com/elastic/beats/dev-tools/mage/target/docs"

	"github.com/elastic/beats/dev-tools/mage/target/build"

	"github.com/elastic/beats/dev-tools/mage"
)

func init() {
	common.RegisterCheckDeps(Update.All)

	dashboards.RegisterImportDeps(build.Build, Update.Dashboards)

	docs.RegisterDeps(Update.FieldDocs)
}

var (
	// SelectLogic configures the types of project logic to use (OSS vs X-Pack).
	SelectLogic mage.ProjectType
)

type Update mg.Namespace

// All updates all generated content.
func (Update) All() {
	mg.Deps(Update.Fields, Update.Dashboards, Update.Config, Update.FieldDocs)
}

func (Update) Config() error {
	return config()
}

// Dashboards collects all the dashboards and generates index patterns.
func (Update) Dashboards() error {
	mg.Deps(fb.FieldsYML)
	return mage.KibanaDashboards()
}

func (Update) Fields() {
	mg.Deps(fb.All)
}

func (Update) FieldDocs() error {
	mg.Deps(fb.FieldsAllYML)
	return mage.Docs.FieldDocs(mage.FieldsAllYML)
}
