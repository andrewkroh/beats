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
	"compress/zlib"
	"encoding/base64"
	"flag"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"regexp"
	"strings"
	"text/template"
)

const datasetFieldIndent = 8

var (
	fieldsYAMLFile string
	beatName       string
	name           string
	fieldsType     string
	pkg            string
	module         string
	templateFile   string
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
	flag.StringVar(&templateFile, "template", "", "Path to fields.go.tmpl file")
	flag.StringVar(&output, "o", "", "Name of output file")
	flag.BoolVar(&verbose, "v", false, "verbose mode")
}

type templateData struct {
	SourceFile         string
	Package            string
	Beat               string
	Priority           int
	AssetName          string
	Module             string // Parent module name when fieldsType=dataset.
	ProviderGoFuncName string
	Data               string
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

	fieldsBytes, err := readFieldsYml(fieldsYAMLFile)
	if err != nil {
		log.Fatal(err)
	}

	tmpl, err := loadTemplate(templateFile)
	if err != nil {
		log.Fatal(err)
	}

	if "dataset" == fieldsType {
		// The final fields data is just a simple blob of concatenated YAML
		// data. For datasets to be "children" of the module they must be
		// indented.
		fieldsBytes = indent(datasetFieldIndent, fieldsBytes)
	}

	tmplData := templateData{
		SourceFile:         fieldsYAMLFile,
		Package:            pkg,
		Beat:               beatName,
		AssetName:          name,
		Module:             module, // Only used for type=dataset.
		Priority:           priority,
		ProviderGoFuncName: "Asset" + goTypeName(name),
		Data:               string(fieldsBytes),
	}
	renderedTemplateBytes, err := renderTemplate(tmpl, tmplData)
	if err != nil {
		log.Fatal(err)
	}

	if err = ioutil.WriteFile(output, renderedTemplateBytes, 0644); err != nil {
		log.Fatalln("Failed to create output file", err)
	}
}

func readFieldsYml(path string) ([]byte, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read fields YAML file %q: %w", path, err)
	}

	// Depending on OS or tools configuration, files can contain carriages (\r),
	// what leads to different results, remove them before encoding.
	data = bytes.Replace(data, []byte("\r"), []byte(""), -1)

	return data, nil
}

func loadTemplate(path string) (*template.Template, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("fields.go.tmpl").
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"zlibCompress": zlibCompress,
		}).
		Parse(string(data))
	if err != nil {
		return nil, err
	}

	return tmpl, nil
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

func assetPriority(fieldsType string) (int, error) {
	switch fieldsType {
	case "ecs":
		return 5, nil
	case "libbeat":
		return 10, nil
	case "beat":
		return 50, nil
	case "module", "dataset":
		return 100, nil
	default:
		return 0, fmt.Errorf("invalid type %q", fieldsType)
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

func zlibCompress(data string) (string, error) {
	buf := new(bytes.Buffer)
	writer := zlib.NewWriter(buf)

	if _, err := writer.Write([]byte(data)); err != nil {
		return "", fmt.Errorf("failed to zlib compress: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to zlib compress: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

var nonWhitespaceRegex = regexp.MustCompile(`(?m)(^.*\S.*$)`)

// indent pads all non-whitespace lines with the number of spaces specified.
func indent(spaces int, content []byte) []byte {
	pad := strings.Repeat(" ", spaces)
	return nonWhitespaceRegex.ReplaceAll(content, []byte(pad+"$1"))
}
