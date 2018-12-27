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

// +build mage

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/pkg/errors"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"go.uber.org/multierr"

	"github.com/elastic/beats/dev-tools/mage"
)

var (
	projects = projectList{
		{"libbeat", unitTest | integTest | osxTesting},
		{"auditbeat", packaging | update | unitTest | integTest | osxTesting},
		{"filebeat", packaging | update | unitTest | integTest | osxTesting},
		{"heartbeat", packaging | dashboards | update | unitTest | integTest | osxTesting},
		{"journalbeat", packaging | dashboards | update | unitTest},
		{"metricbeat", packaging | dashboards | update | unitTest | integTest | osxTesting},
		{"packetbeat", packaging | dashboards | update | unitTest | osxTesting},
		{"winlogbeat", packaging | dashboards | update | unitTest},
		{"x-pack/libbeat", unitTest | integTest},
		{"x-pack/auditbeat", packaging | dashboards | update | unitTest | integTest | osxTesting},
		{"x-pack/filebeat", packaging | dashboards | update | unitTest | integTest | osxTesting},
		{"x-pack/functionbeat", packaging | dashboards | update | unitTest | integTest},
		{"x-pack/heartbeat", packaging | dashboards | update },
		{"x-pack/journalbeat", packaging | dashboards | update | unitTest},
		{"x-pack/metricbeat", packaging | update | unitTest | integTest | osxTesting},
		{"x-pack/packetbeat", packaging | update | unitTest | osxTesting},
		{"x-pack/winlogbeat", packaging | update | unitTest},
		{"dev-tools/packaging/preference-pane", none},
	}

	Aliases = map[string]interface{}{
		"check":   Check.All,
		"fmt":     Check.Fmt,
		"package": Package.All,
		"update":  Update.All,
		"vet":     Check.Vet,
	}
)

type project struct {
	Dir   string
	Attrs attribute
}

func (p project) HasAttribute(a attribute) bool {
	return p.Attrs&a > 0
}

type attribute uint16

const (
	none   attribute = 0
	update attribute = 1 << iota
	dashboards
	packaging
	unitTest
	integTest
	osxTesting

	any attribute = math.MaxUint16
)

type projectList []project

func (l projectList) ForEach(attr attribute, f func(proj project) error) error {
	for _, proj := range l {
		if proj.Attrs&attr > 0 {
			if err := f(proj); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Targets ---

// DumpVariables writes the template variables and values to stdout.
func DumpVariables() error {
	return mage.DumpVariables()
}

type Check mg.Namespace

// Check checks that code is formatted and generated files are up-to-date.
func (Check) All() {
	mg.SerialDeps(Check.Fmt, Update.All, mage.Check)
}

// Fmt formats code and adds license headers.
func (Check) Fmt() {
	mg.Deps(mage.GoImports, mage.PythonAutopep8)
	mg.Deps(addLicenseHeaders)
}

// addLicenseHeaders adds ASL2 headers to .go files outside of x-pack and
// add Elastic headers to .go files in x-pack.
func addLicenseHeaders() error {
	fmt.Println(">> fmt - go-licenser: Adding missing headers")

	if err := sh.Run("go", "get", mage.GoLicenserImportPath); err != nil {
		return err
	}

	return multierr.Combine(
		sh.RunV("go-licenser", "-license", "ASL2", "-exclude", "x-pack"),
		sh.RunV("go-licenser", "-license", "Elastic", "x-pack"),
	)
}

func (Check) Vet() {
	mg.Deps(mage.GoVet)
}

var commonBeatTargets = []string{
	"check",
	"clean",
	"dumpVariables",
	"fields",
	"fmt",
	"build",
	"buildGoDaemon",
	"crossBuild",
	"crossBuildGoDaemon",
	"crossBuildGoDaemon",
	"golangCrossBuild",
}

func (Check) Targets() error {
	mageCmd := sh.OutCmd("mage", "-d")
	var errs []error
	projects.ForEach(any, func(proj project) error {
		fmt.Println("> check:targets:", proj.Dir)
		out, err := mageCmd(proj.Dir, "-l")
		if err != nil {
			return errors.Wrapf(err, "failed checking mage targets of project %v", proj.Dir)
		}
		targets, err := parseTargets(out)
		if err != nil {
			return errors.Wrapf(err, "failed parsing mage -l output of project %v", proj.Dir)
		}

		// Build list of expected targets based on attributes.
		expectedTargets := make([]string, len(commonBeatTargets))
		copy(expectedTargets, commonBeatTargets)
		if proj.HasAttribute(update) {
			expectedTargets = append(expectedTargets,
				"update", "config", "dashboards", "dashboardsImport",
				"dashboardExport")
		}
		if proj.HasAttribute(dashboards) {
			expectedTargets = append(expectedTargets, "update")
		}
		if proj.HasAttribute(packaging) {
			expectedTargets = append(expectedTargets, "package", "packageTest")
		}
		if proj.HasAttribute(unitTest) {
			expectedTargets = append(expectedTargets, "unitTest")
		}
		if proj.HasAttribute(integTest) {
			expectedTargets = append(expectedTargets, "integTest")
		}

		// Check for missing targets.
		var missing []string
		for _, target := range expectedTargets {
			if _, found := targets[target]; !found {
				missing = append(missing, target)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			err = errors.Errorf("failed checking mage targets of project "+
				"%v: missing [%v]", proj.Dir, strings.Join(missing, ", "))
			errs = append(errs, err)
		}
		return nil
	})

	return multierr.Combine(errs...)
}

func parseTargets(rawOutput string) (map[string]string, error) {
	targets := map[string]string{}
	s := bufio.NewScanner(bytes.NewBufferString(rawOutput))
	for s.Scan() {
		line := s.Text()
		if line == "Target:" {
			continue
		}
		if parts := strings.Fields(line); len(parts) > 0 {
			targets[parts[0]] = strings.Join(parts[1:], " ")
		}
	}
	return targets, s.Err()
}

type Update mg.Namespace

// All updates all Beats.
func (Update) All() error {
	mg.Deps(Update.Notice, Update.TravisCI)
	return projects.ForEach(update, func(proj project) error {
		fmt.Println("> update:all:", proj.Dir)
		return errors.Wrapf(mage.Mage(proj.Dir, "update"), "failed updating project %v", proj.Dir)
	})
}

// Fields updates the fields for each Beat.
func (Update) Fields() error {
	return projects.ForEach(update, func(proj project) error {
		fmt.Println("> update:fields:", proj.Dir)
		return errors.Wrapf(mage.Mage(proj.Dir, "fields"), "failed updating project %v", proj.Dir)
	})
}

// Dashboards updates the dashboards for each Beat.
func (Update) Dashboards() error {
	return projects.ForEach(dashboards, func(proj project) error {
		fmt.Println("> update:dashboards:", proj.Dir)
		return errors.Wrapf(mage.Mage(proj.Dir, "dashboards"), "failed updating project %v", proj.Dir)
	})
}

func (Update) Notice() error {
	ve, err := mage.PythonVirtualenv()
	if err != nil {
		return err
	}
	pythonPath, err := mage.LookVirtualenvPath(ve, "python")
	if err != nil {
		return err
	}
	return sh.RunV(pythonPath, filepath.Clean("dev-tools/generate_notice.py"), ".")
}

func (Update) TravisCI() error {
	var data TravisCITemplateData

	data.Jobs = append(data.Jobs, TravisCIJob{
		OS:    "linux",
		Stage: "check",
		Env: []string{
			"BUILD_CMD=" + strconv.Quote("mage"),
			"TARGETS=" + strconv.Quote("check"),
		},
	})

	projects.ForEach(any, func(proj project) error {
		if proj.HasAttribute(unitTest) || proj.HasAttribute(integTest) {
			var targets []string
			if proj.HasAttribute(unitTest) {
				targets = append(targets, "unitTest")
			}
			if proj.HasAttribute(integTest) {
				targets = append(targets, "integTest")
			}
			data.Jobs = append(data.Jobs, TravisCIJob{
				OS:    "linux",
				Stage: "test",
				Env: []string{
					"BUILD_CMD=" + strconv.Quote("mage -d "+filepath.ToSlash(proj.Dir)),
					"TARGETS=" + strconv.Quote(strings.Join(targets, " ")),
				},
			})
		}

		// We don't run the integTest which require Docker on OSX workers.
		if proj.HasAttribute(osxTesting) && proj.HasAttribute(unitTest) {
			data.Jobs = append(data.Jobs, TravisCIJob{
				OS:    "osx",
				Stage: "test",
				Env: []string{
					"BUILD_CMD=" + strconv.Quote("mage -d "+filepath.ToSlash(proj.Dir)),
					"TARGETS=" + strconv.Quote("unitTest"),
				},
			})
		}
		return nil
	})

	projects.ForEach(any, func(proj project) error {
		if !strings.HasSuffix(filepath.Base(proj.Dir), "beat") {
			return nil
		}

		data.Jobs = append(data.Jobs, TravisCIJob{
			OS:    "linux",
			Stage: "crosscompile",
			Env: []string{
				"BUILD_CMD=" + strconv.Quote("make -C "+proj.Dir),
				"TARGETS=" + strconv.Quote("gox"),
			},
		})
		return nil
	})

	elasticBeats, err := mage.ElasticBeatsDir()
	if err != nil {
		return err
	}

	t, err := template.ParseFiles(filepath.Join(elasticBeats, "dev-tools/ci/templates/travis.yml.tmpl"))
	if err != nil {
		return err
	}

	out, err := os.OpenFile(".travis.yml", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	return t.Execute(out, data)
}

type TravisCITemplateData struct {
	Jobs []TravisCIJob
}

type TravisCIJob struct {
	OS    string
	Env   []string
	Stage string
}

type Package mg.Namespace

// All packages all Beats and generates the dashboards zip package.
func (Package) All() {
	mg.SerialDeps(Package.Dashboards, Package.Beats)
}

// Dashboards packages the dashboards from all Beats into a zip file.
func (Package) Dashboards() error {
	mg.Deps(Update.Dashboards)

	version, err := mage.BeatQualifiedVersion()
	if err != nil {
		return err
	}

	spec := mage.PackageSpec{
		Name:     "beats-dashboards",
		Version:  version,
		Snapshot: mage.Snapshot,
		Files: map[string]mage.PackageFile{
			".build_hash.txt": mage.PackageFile{
				Content: "{{ commit }}\n",
			},
		},
		OutputFile: "build/distributions/dashboards/{{.Name}}-{{.Version}}{{if .Snapshot}}-SNAPSHOT{{end}}",
	}

	projects.ForEach(dashboards, func(proj project) error {
		beat := filepath.Base(proj.Dir)
		spec.Files[beat] = mage.PackageFile{
			Source: filepath.Join(beat, "_meta/kibana.generated"),
		}
		return nil
	})

	return mage.PackageZip(spec.Evaluate())
}

// Beats packages each Beat.
//
// Use SNAPSHOT=true to build snapshots.
// Use PLATFORMS to control the target platforms.
// Use VERSION_QUALIFIER to control the version qualifier.
func (Package) Beats() (err error) {
	// TODO: copy packages to build/distributions/{BeatName}.
	return projects.ForEach(packaging, func(proj project) error {
		fmt.Println("> package:beats:", proj.Dir)
		return errors.Wrapf(mage.Mage(proj.Dir, "package"), "failed packaging project %v", proj.Dir)
	})
}

type Test mg.Namespace

func (Test) All() error {
	// Assumes that projects support integTest is a subset of unitTest.
	return projects.ForEach(unitTest, func(proj project) error {
		fmt.Println("> test:all:", proj.Dir)
		target := "unitTest"
		if proj.Attrs&integTest > 0 {
			target += " integTest"
		}
		return errors.Wrapf(mage.Mage(proj.Dir, target), "failed testing project %v", proj.Dir)
	})
}

func (Test) Unit() error {
	return projects.ForEach(unitTest, func(proj project) error {
		fmt.Println("> test:unit:", proj.Dir)
		return errors.Wrapf(mage.Mage(proj.Dir, "unitTest"), "failed testing project %v", proj.Dir)
	})
}

func (Test) Integ() error {
	return projects.ForEach(integTest, func(proj project) error {
		fmt.Println("> test:integ:", proj.Dir)
		return errors.Wrapf(mage.Mage(proj.Dir, "integTest"), "failed testing project %v", proj.Dir)
	})
}

// TODO: Add targets for
// - check:misspell
// - docs
