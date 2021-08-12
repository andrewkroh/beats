// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Code generated by go_embed_fields - DO NOT EDIT.

package fortinet

import (
	_ "embed"

	"github.com/elastic/beats/v7/libbeat/asset"
)

func init() {
	if err := asset.RegisterFields("filebeat", "fortinet", asset.ModuleFieldsPri, fieldsModuleFortinet); err != nil {
		panic(err)
	}
}

//go:embed _meta/fields.yml
// fieldsModuleFortinet contains fields data from _meta/fields.yml.
var fieldsModuleFortinet string
