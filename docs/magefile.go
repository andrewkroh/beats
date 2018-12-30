// +build mage

package main

import "github.com/elastic/beats/dev-tools/mage"

func Docs() error {
	return mage.Docs.AsciidocBook(
		mage.DocsName("dev-guide"),
		mage.DocsIndexFile("devguide/index.asciidoc"),
	)
}

func Clean() error {
	return mage.Clean([]string{
		"build",
	})
}
