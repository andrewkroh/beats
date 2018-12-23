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
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"go.uber.org/multierr"

	"github.com/elastic/beats/dev-tools/mage"
)

var (
	projects = projectList{
		{"auditbeat", packaging | update | unitTest | integTest},
		{"dev-tools", none},
		{"filebeat", packaging | update | unitTest | integTest},
		{"heartbeat", packaging | dashboards | update | unitTest | integTest},
		{"journalbeat", packaging | dashboards | update | unitTest | integTest},
		{"metricbeat", packaging | dashboards | update | unitTest | integTest},
		{"packetbeat", packaging | dashboards | update | unitTest | integTest},
		{"winlogbeat", packaging | dashboards | update | unitTest | integTest},
		{"x-pack/auditbeat", packaging | dashboards | update | unitTest | integTest},
		{"x-pack/filebeat", packaging | dashboards | update | unitTest | integTest},
		{"x-pack/functionbeat", packaging | dashboards | update | unitTest | integTest},
		{"x-pack/libbeat", none},
		{"x-pack/metricbeat", packaging | update | unitTest | integTest},
	}

	Aliases = map[string]interface{}{
		"update":  Update.All,
		"package": Package.All,
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

// Check checks that code is formatted and generated files are up-to-date.
func Check() {
	mg.SerialDeps(Fmt, Update.All, mage.Check)
}

// Fmt formats code and adds license headers.
func Fmt() {
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

type Update mg.Namespace

// All updates all Beats.
func (Update) All() error {
	mg.Deps(Update.Notice)
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
	mg.Deps(Test.MageTargets)

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

func (Test) MageTargets() error {
	mageCmd := sh.OutCmd("mage", "-d")
	return projects.ForEach(any, func(proj project) error {
		fmt.Println("> test:mageTargets:", proj.Dir)
		out, err := mageCmd(proj.Dir, "-l")
		if err != nil {
			return errors.Wrapf(err, "failed testing mage targets of project %v", proj.Dir)
		}
		targets, err := parseTargets(out)
		if err != nil {
			return errors.Wrapf(err, "failed parsing mage -l output of project %v", proj.Dir)
		}

		// TODO: Reduce duplication and test more targets like build/fmt/check.
		var errs []error
		if proj.HasAttribute(update) {
			if _, found := targets["update"]; !found {
				errs = append(errs, fmt.Errorf("missing update target"))
			}
		}
		if proj.HasAttribute(dashboards) {
			if _, found := targets["dashboards"]; !found {
				errs = append(errs, fmt.Errorf("missing dashboards target"))
			}
		}
		if proj.HasAttribute(packaging) {
			if _, found := targets["package"]; !found {
				errs = append(errs, fmt.Errorf("missing package target"))
			}
		}
		if proj.HasAttribute(unitTest) {
			if _, found := targets["unitTest"]; !found {
				errs = append(errs, fmt.Errorf("missing unitTest target"))
			}
		}
		if proj.HasAttribute(integTest) {
			if _, found := targets["unitTest"]; !found {
				errs = append(errs, fmt.Errorf("missing unitTest target"))
			}
			if _, found := targets["integTest"]; !found {
				errs = append(errs, fmt.Errorf("missing integTest target"))
			}
		}
		return errors.Wrapf(multierr.Combine(errs...), "failed testing mage targets of project %v", proj.Dir)
	})
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

// TODO: Add targets for
// - check:misspell
// - docs
