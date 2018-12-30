package docs

import (
	"github.com/elastic/beats/dev-tools/mage"
	"github.com/magefile/mage/mg"
)

var (
	docsDeps []interface{}
)

func RegisterDeps(deps ...interface{}) {
	docsDeps = append(docsDeps, deps...)
}

func Docs() error {
	mg.SerialDeps(docsDeps...)
	return mage.Docs.AsciidocBook()
}
