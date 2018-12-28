// +build mage

package main

import (
	"fmt"
	"github.com/elastic/beats/dev-tools/mage"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/pkg/errors"
	"path/filepath"
	"regexp"
)

var (
	beats = []string{
		"auditbeat",
		"filebeat",
		"metricbeat",
	}

	kubectlCmd = sh.RunCmd("kubectl")
)

func Clean() error {
	return mage.Clean([]string{
		"*beat-kubernetes.yaml",
	})
}

func IntegTest() error {
	if err := haveKubernetes(); err != nil {
		fmt.Println(">> integTest: kubernetes testing (SKIPPED - kubernetes unavailable)")
		return nil
	}

	for _, beat := range beats {
		manifest := beat +"-kubernetes.yaml"
		if err := sh.RunV("kubectl", "create", "-f", manifest); err != nil {
			return errors.Wrapf(err, "failed deploying %v to kubernetes", manifest)
		}
	}
	return nil
}

func Update() error {
	mg.Deps(Clean)

	version, err := mage.BeatQualifiedVersion()
	if err != nil {
		return err
	}

	for _, beat := range beats {
		in := filepath.Join(beat, beat+"-*.yaml")
		out := beat +"-kubernetes.yaml"

		inputs, err := mage.FindFiles(in)
		if err != nil {
			return err
		}

		if err = mage.FileConcat(out, 0644, inputs...); err != nil {
			return err
		}

		if err = mage.FindReplace(out, regexp.MustCompile(`%VERSION%`), version); err != nil {
			return err
		}
	}
	return nil
}

func haveKubernetes() error {
	err := kubectlCmd("version")
	return errors.Wrap(err, "kubernetes is not available")
}
