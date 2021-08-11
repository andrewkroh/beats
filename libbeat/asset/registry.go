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

package asset

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"io/ioutil"
	"math"
	"regexp"
	"sort"
	"strings"
)

const datasetFieldIndent = 8

type Priority int32

const (
	Highest          Priority = 1
	ECSFieldsPri     Priority = 5
	LibbeatFieldsPri Priority = 10
	BeatFieldsPri    Priority = 50
	ModuleFieldsPri  Priority = 100
	Lowest           Priority = math.MaxInt32
)

// fieldsRegistry contains the contents of fields.yml files.
//
// This is a mapping of:
//     beatName -> asset.Priority -> fieldSetName -> string
//
// As each entry is an array of strings, multiple fields.yml can be added under one path.
// This can become useful as we don't have to generate anymore the fields.yml but can
// package each local fields.yml from things like processors.
var fieldsRegistry = map[string]map[int]map[string][]string{}

// moduleDatasetRegistry contains the contents of fields.yml files for module datasets.
// This is a mapping of:
//   beatName -> moduleName -> dataSetName -> string
var moduleDatasetRegistry = map[string]map[string]map[string]string{}

// RegisterFields sets the fields for a given beat and asset name.
func RegisterFields(beat, name string, p Priority, fields string) error {
	if _, ok := fieldsRegistry[beat]; !ok {
		fieldsRegistry[beat] = map[int]map[string][]string{}
	}

	priority := int(p)
	if _, ok := fieldsRegistry[beat][priority]; !ok {
		fieldsRegistry[beat][priority] = map[string][]string{}
	}

	fieldsRegistry[beat][priority][name] = append(fieldsRegistry[beat][priority][name], fields)
	return nil
}

// RegisterModuleDatasetFields sets the fields for a given beat and asset name
func RegisterModuleDatasetFields(beat, module, dataset string, fields string) error {
	// The final fields data is just a simple blob of concatenated YAML
	// data. For datasets to be "children" of the module they must be
	// indented.
	fields = indent(datasetFieldIndent, fields)

	if _, ok := moduleDatasetRegistry[beat]; !ok {
		moduleDatasetRegistry[beat] = map[string]map[string]string{}
	}

	if _, ok := moduleDatasetRegistry[beat][module]; !ok {
		moduleDatasetRegistry[beat][module] = map[string]string{}
	}

	moduleDatasetRegistry[beat][module][dataset] = fields
	return nil
}

var nonWhitespaceRegex = regexp.MustCompile(`(?m)(^.*\S.*$)`)

// indent pads all non-whitespace lines with the number of spaces specified.
func indent(spaces int, content string) string {
	pad := strings.Repeat(" ", spaces)
	return nonWhitespaceRegex.ReplaceAllString(content, pad+"$1")
}

func getModuleDatasetsFields(beat, module string) []string {
	moduleDatasets := moduleDatasetRegistry[beat][module]

	var datasets []string
	for datasetName := range moduleDatasets {
		datasets = append(datasets, datasetName)
	}
	sort.Strings(datasets)

	var fields []string
	for _, datasetName := range datasets {
		fields = append(fields, moduleDatasets[datasetName])
	}

	return fields
}

// GetFields returns a byte array containing all fields for the specified beat.
func GetFields(beat string) ([]byte, error) {
	// Get all priorities and sort them.
	beatRegistry := fieldsRegistry[beat]
	priorities := make([]int, 0, len(beatRegistry))
	for p := range beatRegistry {
		priorities = append(priorities, p)
	}
	sort.Ints(priorities)

	var fields []byte
	for _, priority := range priorities {
		priorityRegistry := beatRegistry[priority]

		// Sort all entries with same priority alphabetically
		entries := make([]string, 0, len(priorityRegistry))
		for e := range priorityRegistry {
			entries = append(entries, e)
		}
		sort.Strings(entries)

		for _, fieldSetName := range entries {
			list := priorityRegistry[fieldSetName]

			// Add in dataset fields if this is a module.
			if ModuleFieldsPri == Priority(priority) {
				list = append(list, getModuleDatasetsFields(beat, fieldSetName)...)
			}

			for _, data := range list {
				fields = append(fields, []byte(data)...)
			}
		}
	}
	return fields, nil
}

// SetFields sets the fields for a given beat and asset name.
//
// Deprecated: Switch to go:embed and the Register* methods.
func SetFields(beat, name string, p Priority, assetProducer func() string) error {
	data, err := DecodeData(assetProducer())
	if err != nil {
		return err
	}
	return RegisterFields(beat, name, p, string(data))
}

// EncodeData compresses the data with zlib and base64 encodes it
//
// Deprecated: Switch to go:embed and the Register* methods.
func EncodeData(data string) (string, error) {
	var zlibBuf bytes.Buffer
	writer := zlib.NewWriter(&zlibBuf)
	_, err := writer.Write([]byte(data))
	if err != nil {
		return "", err
	}
	err = writer.Close()
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(zlibBuf.Bytes()), nil
}

// DecodeData base64 decodes the data and uncompresses it.
//
// Deprecated: Switch to go:embed and the Register* methods.
func DecodeData(data string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}

	b := bytes.NewReader(decoded)
	r, err := zlib.NewReader(b)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return ioutil.ReadAll(r)
}
