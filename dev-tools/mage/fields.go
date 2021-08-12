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

package mage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"

	"github.com/magefile/mage/sh"
)

const (
	// FieldsYML specifies the path to the file containing the field data for
	// the Beat (formerly this was ./fields.yml).
	FieldsYML = "build/fields/fields.yml"
	// FieldsYMLRoot specifies the filename of the project's root level
	// fields.yml file (this is being replaced by FieldsYML).
	FieldsYMLRoot = "fields.yml"
	// FieldsAllYML specifies the path to the file containing the field data for
	// the Beat from all license types. It's generally used for making documentation.
	FieldsAllYML = "build/fields/fields.all.yml"
)

// IncludeListOptions stores the options for IncludeList generation
type IncludeListOptions struct {
	ImportDirs       []string
	ModuleDirs       []string
	ModulesToExclude []string
	Outfile          string
	BuildTags        string
	Pkg              string
}

// DefaultIncludeListOptions initializes IncludeListOptions struct with default values
func DefaultIncludeListOptions() IncludeListOptions {
	return IncludeListOptions{
		ImportDirs:       nil,
		ModuleDirs:       []string{"module"},
		ModulesToExclude: nil,
		Outfile:          "include/list.go",
		BuildTags:        "",
		Pkg:              "include",
	}
}

// FieldsBuilder is the interface projects to implement for building field data.
type FieldsBuilder interface {
	// Generate all fields.go files.
	FieldsGo() error

	// Generate build/fields/fields.yml containing fields for the Beat. This
	// file may need be copied to fields.yml if tests depend on it, but those
	// tests should be updated.
	FieldsYML() error

	// Generate build/fields/fields.all.yml containing all possible fields
	// for all license types. (Used for field documentation.)
	FieldsAllYML() error

	All() // Build everything.
}

// GenerateFieldsYAML generates a fields.yml file for a Beat. This will include
// the common fields specified by libbeat, the common fields for the Beat,
// and any additional fields.yml files you specify.
//
// moduleDirs specifies additional directories to search for modules. The
// contents of each fields.yml will be included in the generated file.
func GenerateFieldsYAML(moduleDirs ...string) error {
	return generateFieldsYAML(OSSBeatDir(), "fields.yml", moduleDirs...)
}

// GenerateFieldsYAMLTo generates a YAML file containing the field definitions
// for the Beat. It's the same as GenerateFieldsYAML but with a configurable
// output file.
func GenerateFieldsYAMLTo(output string, moduleDirs ...string) error {
	return generateFieldsYAML(OSSBeatDir(), output, moduleDirs...)
}

func generateFieldsYAML(baseDir, output string, moduleDirs ...string) error {
	const globalFieldsCmdPath = "libbeat/scripts/cmd/global_fields/main.go"

	beatsDir, err := ElasticBeatsDir()
	if err != nil {
		return err
	}

	cmd := []string{"run",
		"-mod=readonly",
		filepath.Join(beatsDir, globalFieldsCmdPath),
		"-es_beats_path", beatsDir,
		"-beat_path", baseDir,
		"-out", CreateDir(output),
	}
	globalFieldsCmd := sh.RunCmd("go", cmd...)

	return globalFieldsCmd(moduleDirs...)
}

// GenerateAllInOneFieldsGo generates an all-in-one fields.go file.
func GenerateAllInOneFieldsGo() error {
	return GenerateFieldsGo("fields.yml", "include/fields.go")
}

// GenerateFieldsGo generates a .go file containing the fields.yml data.
func GenerateFieldsGo(fieldsYML, out string) error {
	const assetCmdPath = "dev-tools/cmd/asset/asset.go"

	beatsDir, err := ElasticBeatsDir()
	if err != nil {
		return err
	}

	cmd := []string{"run",
		"-mod=readonly",
		filepath.Join(beatsDir, assetCmdPath),
		"-pkg", "include",
		"-in", fieldsYML,
		"-out", CreateDir(out),
		"-license", toLibbeatLicenseName(BeatLicense),
		BeatName,
	}
	assetCmd := sh.RunCmd("go", cmd...)

	return assetCmd()
}

// GenerateModuleFieldsGo generates a fields.go file containing a copy of the
// each module's field.yml data in a format that can be embedded in Beat's
// binary.
func GenerateModuleFieldsGo(moduleDir string) error {
	const moduleFieldsCmdPath = "dev-tools/cmd/module_fields/module_fields.go"

	beatsDir, err := ElasticBeatsDir()
	if err != nil {
		return err
	}

	if !filepath.IsAbs(moduleDir) {
		moduleDir = CWD(moduleDir)
	}

	cmd := []string{"run",
		filepath.Join(beatsDir, moduleFieldsCmdPath),
		"-beat", BeatName,
		"-license", toLibbeatLicenseName(BeatLicense),
		moduleDir,
	}
	moduleFieldsCmd := sh.RunCmd("go", cmd...)

	return moduleFieldsCmd()
}

var (
	moduleFieldsYmlRegex = regexp.MustCompile(`(?m)([^/]+)/_meta/fields\.yml$`)
	datasetFieldsYmlRegex =  regexp.MustCompile(`(?m)([^/]+)/([^/]+)/_meta/fields\.yml$`)
)

func GenerateBeatFieldsEmbedGo() error {
	const goEmbedFieldsCmdPath = "dev-tools/cmd/go_embed_fields"

	beatsDir, err := ElasticBeatsDir()
	if err != nil {
		return err
	}
	goEmbedFields := sh.RunCmd("go", "run", "-mod=readonly", filepath.Join(beatsDir, goEmbedFieldsCmdPath))

	args := []string{
		"-i", "fields/_meta/fields.beat.yml",
		"-type", "beat",
		"-beat", BeatName,
		"-name", BeatName,
		"-pkg", "fields",
		"-o", filepath.Join("fields", "fields.beat.go"),
		"-license", toLibbeatLicenseName(BeatLicense),
	}

	if err = goEmbedFields(args...); err != nil {
		return err
	}

	os.Remove("include/fields.go")
	return nil
}

func GenerateFieldsEmbedGo(packageDirAndNamePairs ...string) error {
	for i := 0; i <= len(packageDirAndNamePairs)/2; i += 2 {
		dir, name := packageDirAndNamePairs[i], packageDirAndNamePairs[i+1]
		if err := generateFieldsEmbedGo(dir, name); err != nil {
			return fmt.Errorf("failed to generate fields.go for %v: %w", dir, err)
		}
	}
	return nil
}

func generateFieldsEmbedGo(packageDir, name string) error {
	const goEmbedFieldsCmdPath = "dev-tools/cmd/go_embed_fields"

	beatsDir, err := ElasticBeatsDir()
	if err != nil {
		return err
	}
	goEmbedFields := sh.RunCmd("go", "run", "-mod=readonly", filepath.Join(beatsDir, goEmbedFieldsCmdPath))
	goListPackageName := sh.OutCmd("go", "list", "-f={{.Name}}")

	dirAbs, err := filepath.Abs(filepath.Join(".", packageDir))
	if err != nil {
		return err
	}

	pkg, err := goListPackageName(dirAbs)
	if err != nil {
		return err
	}

	args := []string{
		"-i", filepath.Join(packageDir, "_meta/fields.yml"),
		"-type", "beat",
		"-beat", BeatName,
		"-name", name,
		"-pkg", pkg,
		"-o", filepath.Join(packageDir, "fields.go"),
		"-license", toLibbeatLicenseName(BeatLicense),
	}

	if err = goEmbedFields(args...); err != nil {
		return err
	}

	return nil
}

func GenerateModuleFieldsEmbedGo(moduleDir string) error {
	const goEmbedFieldsCmdPath = "dev-tools/cmd/go_embed_fields"

	beatsDir, err := ElasticBeatsDir()
	if err != nil {
		return err
	}
	goEmbedFields := sh.RunCmd("go", "run", "-mod=readonly", filepath.Join(beatsDir, goEmbedFieldsCmdPath))
	goListPackageName := sh.OutCmd("go", "list", "-f={{.Name}}")

	// Modules
	moduleFieldsYmlFiles, err := FindFiles(filepath.Join(moduleDir, "*/_meta/fields.yml"))
	if err != nil {
		return err
	}

	for _, fieldsFile := range moduleFieldsYmlFiles {
		matches := moduleFieldsYmlRegex.FindStringSubmatch(filepath.ToSlash(fieldsFile))
		if len(matches) != 2 {
			continue
		}
		moduleName := matches[1]

		os.Remove(filepath.Join(moduleDir, moduleName, "fields.go"))

		dirAbs, err := filepath.Abs(filepath.Join(".", moduleDir, moduleName))
		if err != nil {
			return err
		}

		pkg := moduleName
		if goFiles, err := filepath.Glob(filepath.Join(dirAbs, "*.go")); len(goFiles) > 0 {
			pkg, err = goListPackageName(dirAbs)
			if err != nil {
				return err
			}
		}

		args := []string{
			"-i", fieldsFile,
			"-type", "module",
			"-beat", BeatName,
			"-name", moduleName,
			"-pkg", pkg,
			"-o", filepath.Join(moduleDir, moduleName, "fields.go"),
			"-license", toLibbeatLicenseName(BeatLicense),
		}

		if err = goEmbedFields(args...); err != nil {
			return err
		}

	}

	// Module Datasets
	datasetFieldsYmlFiles, err := FindFiles(filepath.Join(moduleDir, "*/*/_meta/fields.yml"))
	if err != nil {
		return err
	}

	for _, fieldsFile := range datasetFieldsYmlFiles {
		matches := datasetFieldsYmlRegex.FindStringSubmatch(filepath.ToSlash(fieldsFile))
		if len(matches) != 3 {
			continue
		}
		moduleName := matches[1]
		datasetName := matches[2]

		dirAbs, err := filepath.Abs(filepath.Join(".", moduleDir, moduleName, datasetName))
		if err != nil {
			return err
		}

		pkg := datasetName
		if goFiles, err := filepath.Glob(filepath.Join(dirAbs, "*.go")); len(goFiles) > 0 {
			pkg, err = goListPackageName(dirAbs)
			if err != nil {
				return err
			}
		}

		args := []string{
			"-i", fieldsFile,
			"-type", "dataset",
			"-beat", BeatName,
			"-module", moduleName,
			"-name", datasetName,
			"-pkg", pkg,
			"-o", filepath.Join(moduleDir, moduleName, datasetName, "fields.go"),
			"-license", toLibbeatLicenseName(BeatLicense),
			"-v",
		}

		if err = goEmbedFields(args...); err != nil {
			return err
		}
	}

	return nil
}

// GenerateModuleIncludeListGo generates an include/list.go file containing
// a import statement for each module and dataset.
func GenerateModuleIncludeListGo() error {
	return GenerateIncludeListGo(DefaultIncludeListOptions())
}

// GenerateIncludeListGo generates an include/list.go file containing imports
// for the packages that match the paths (or globs) in importDirs (optional)
// and moduleDirs (optional).
func GenerateIncludeListGo(options IncludeListOptions) error {
	const moduleIncludeListCmdPath = "dev-tools/cmd/module_include_list/module_include_list.go"

	beatsDir, err := ElasticBeatsDir()
	if err != nil {
		return err
	}

	cmd := []string{"run",
		filepath.Join(beatsDir, moduleIncludeListCmdPath),
		"-license", toLibbeatLicenseName(BeatLicense),
		"-out", options.Outfile, "-buildTags", options.BuildTags,
		"-pkg", options.Pkg,
	}

	includeListCmd := sh.RunCmd("go", cmd...)

	var args []string
	for _, dir := range options.ImportDirs {
		if !filepath.IsAbs(dir) {
			dir = CWD(dir)
		}
		args = append(args, "-import", dir)
	}
	for _, dir := range options.ModuleDirs {
		if !filepath.IsAbs(dir) {
			dir = CWD(dir)
		}
		args = append(args, "-moduleDir", dir)
	}
	for _, dir := range options.ModulesToExclude {
		if !filepath.IsAbs(dir) {
			dir = CWD(dir)
		}
		args = append(args, "-moduleExcludeDirs", dir)
	}
	return includeListCmd(args...)
}

// toLibbeatLicenseName translates the license type used in packages to
// the identifiers used by github.com/elastic/beatslibbeat/licenses.
func toLibbeatLicenseName(name string) string {
	switch name {
	case "ASL 2.0":
		return "ASL2"
	case "Elastic License":
		return "Elastic"
	default:
		panic(errors.Errorf("invalid license name '%v'", name))
	}
}
