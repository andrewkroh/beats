package ecs

import (
	_ "embed"

	"github.com/elastic/beats/v7/libbeat/asset"
)

//go:embed _meta/fields.ecs.yml
// fieldsECS contains fields data from _meta/fields.ecs.yml.
var fieldsECS string

func MustRegisterFields(beat string) {
	if err := asset.RegisterFields(beat, "ecs", asset.ECSFieldsPri, fieldsECS); err != nil {
		panic(err)
	}
}
