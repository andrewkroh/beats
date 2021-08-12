// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Code generated by go_embed_fields - DO NOT EDIT.

package gsuite

import (
	_ "embed"

	"github.com/elastic/beats/v7/libbeat/asset"
)

func init() {
	if err := asset.RegisterModuleDatasetFields("filebeat", "gsuite", "login", fieldsDatasetLogin); err != nil {
		panic(err)
	}
}

//go:embed _meta/fields.yml
// fieldsDatasetLogin contains fields data from _meta/fields.yml.
var fieldsDatasetLogin string
