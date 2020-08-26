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
	"sort"
)

// fieldsRegistry contains the contents of fields.yml files.
// This is a mapping of:
//   beatName -> asset.Priority -> fieldSetName -> []hexEncodedZlibBytesFunc
// As each entry is an array of bytes multiple fields.yml can be added under one path.
// This can become useful as we don't have to generate anymore the fields.yml but can
// package each local fields.yml from things like processors.
var fieldsRegistry = map[string]map[int]map[string][]func() string{}

// moduleDatasetRegistry contains the contents of fields.yml files for module datasets.
// This is a mapping of:
//   beatName -> moduleName -> dataSetName -> hexEncodedZlibBytesFunc
var moduleDatasetRegistry = map[string]map[string]map[string]func() string{}

// SetFields sets the fields for a given beat and asset name
func SetFields(beat, name string, p Priority, assetProducer func() string) error {
	if _, ok := fieldsRegistry[beat]; !ok {
		fieldsRegistry[beat] = map[int]map[string][]func() string{}
	}

	priority := int(p)
	if _, ok := fieldsRegistry[beat][priority]; !ok {
		fieldsRegistry[beat][priority] = map[string][]func() string{}
	}

	fieldsRegistry[beat][priority][name] = append(fieldsRegistry[beat][priority][name], assetProducer)
	return nil
}

// SetModuleDatasetFields sets the fields for a given beat and asset name
func SetModuleDatasetFields(beat, module, dataset string, assetProducer func() string) error {
	if _, ok := moduleDatasetRegistry[beat]; !ok {
		moduleDatasetRegistry[beat] = map[string]map[string]func() string{}
	}

	if _, ok := moduleDatasetRegistry[beat][module]; !ok {
		moduleDatasetRegistry[beat][module] = map[string]func() string{}
	}

	moduleDatasetRegistry[beat][module][dataset] = assetProducer
	return nil
}

func getModuleDatasetsFields(beat, module string) []func() string {
	moduleDatasets := moduleDatasetRegistry[beat][module]

	var datasets []string
	for datasetName := range moduleDatasets {
		datasets = append(datasets, datasetName)
	}
	sort.Strings(datasets)

	var assetProducers []func() string
	for _, datasetName := range datasets {
		assetProducers = append(assetProducers, moduleDatasets[datasetName])
	}

	return assetProducers
}

// GetFields returns a byte array contains all fields for the given beat
func GetFields(beat string) ([]byte, error) {
	// Get all priorities and sort them
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

			for _, assetProducer := range list {
				output, err := DecodeData(assetProducer())
				if err != nil {
					return nil, err
				}

				fields = append(fields, output...)
			}
		}
	}
	return fields, nil
}

// EncodeData compresses the data with zlib and base64 encodes it.
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

// DecodeData decodes base64 encoded zlib compressed data.
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
