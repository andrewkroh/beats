// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mage

import (
	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage"
)

// Fields generates fields.yml and fields.go files for the Beat.
func Fields() {
	mg.SerialDeps(libbeatAndBeatCommonFieldsGo)
}

// libbeatAndBeatCommonFieldsGo generates a fields.go containing both
// libbeat and Auditbeat's common fields.
func libbeatAndBeatCommonFieldsGo() error {
	if err := mage.GenerateFieldsYAML(); err != nil {
		return err
	}
	return mage.GenerateAllInOneFieldsGo()
}
