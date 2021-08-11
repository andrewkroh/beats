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

package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/elastic/beats/v7/licenses"

	"gopkg.in/yaml.v2"
)

const datasetFieldIndent = 8

var (
	//go:embed fields.go.tmpl
	fieldsGoTemplate string

	tmpl = template.Must(template.New("fields.go.tmpl").Option("missingkey=error").Parse(fieldsGoTemplate))
)

var (
	fieldsYAMLFile string
	beatName       string
	name           string
	fieldsType     string
	pkg            string
	module         string
	license        string
	output         string
	verbose        bool
)

func init() {
	flag.StringVar(&fieldsYAMLFile, "i", "", "fields.yml source file")
	flag.StringVar(&beatName, "beat", "", "Name of the Beat")
	flag.StringVar(&name, "name", "", "Asset name")
	flag.StringVar(&fieldsType, "type", "", "Type of the fields (relates to asset.Priority)")
	flag.StringVar(&pkg, "pkg", "", "Package name for generated .go file")
	flag.StringVar(&module, "module", "", "Name of the parent module (for type=dataset only)")
	flag.StringVar(&license, "license", "ASL2", "License header for generated file.")
	flag.StringVar(&output, "o", "", "Name of output file")
	flag.BoolVar(&verbose, "v", false, "verbose mode")
}

type templateData struct {
	License           string
	SourceFile        string
	Package           string
	Beat              string
	Priority          string
	AssetName         string
	Module            string // Parent module name when fieldsType=dataset.
	EmbedVariableName string
}

func main() {
	log.SetPrefix("go_embed_fields: ")
	flag.Parse()

	if verbose {
		log.Printf("Generating %q from %q with beat=%v, asset=%v, type=%v, "+
			"package=%v, module=%v.",
			output, fieldsYAMLFile, beatName, name, fieldsType, pkg, module)
	}

	priority, err := assetPriority(fieldsType)
	if err != nil {
		log.Fatal(err)
	}

	if err = validateYAML(fieldsYAMLFile); err != nil {
		log.Fatal(err)
	}

	relativeYALMFile, err := filepath.Rel(filepath.Dir(output), fieldsYAMLFile)
	if err != nil {
		log.Fatal(err)
	}

	license, err := licenses.Find(license)
	if err != nil {
		log.Fatalf("Invalid license specifier: %v", err)
	}

	tmplData := templateData{
		License:           license,
		SourceFile:        filepath.ToSlash(relativeYALMFile),
		Package:           pkg,
		Beat:              beatName,
		AssetName:         name,
		Module:            module, // Only used for type=dataset.
		Priority:          priority,
		EmbedVariableName: "fields" + strings.Title(fieldsType) + goTypeName(name),
	}
	renderedTemplateBytes, err := renderTemplate(tmpl, tmplData)
	if err != nil {
		log.Fatal(err)
	}

	if err = ioutil.WriteFile(output, renderedTemplateBytes, 0644); err != nil {
		log.Fatalln("Failed to create output file", err)
	}
}

func validateYAML(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read fields YAML file %q: %w", path, err)
	}

	// Simple check that this contains valid YAML.
	var v interface{}
	if err = yaml.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("failed to unmarshal YAML from %q: %w", path, err)
	}

	return nil
}

func renderTemplate(tmpl *template.Template, data templateData) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	gofmtBytes, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to gofmt generated template: %w", err)
	}

	return gofmtBytes, nil
}

func assetPriority(fieldsType string) (string, error) {
	switch fieldsType {
	case "ecs":
		return "asset.ECSFieldsPri", nil
	case "libbeat":
		return "asset.LibbeatFieldsPri", nil
	case "beat":
		return "asset.BeatFieldsPri", nil
	case "module", "dataset":
		return "asset.ModuleFieldsPri", nil
	default:
		return "", fmt.Errorf("invalid type %q", fieldsType)
	}
}

// goTypeName removes special characters ('_', '.', '@') and returns a
// camel-cased name.
func goTypeName(name string) string {
	var b strings.Builder
	for _, w := range strings.FieldsFunc(name, isSeparator) {
		b.WriteString(strings.Title(w))
	}
	return b.String()
}

// isSeparate returns true if the character is a field name separator. This is
// used to detect the separators in fields like ephemeral_id or instance.name.
func isSeparator(c rune) bool {
	switch c {
	case '.', '_', '/':
		return true
	case '@':
		// This effectively filters @ from field names.
		return true
	default:
		return false
	}
}
