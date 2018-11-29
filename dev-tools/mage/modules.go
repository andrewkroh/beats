package mage

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

var modulesDConfigTemplate = `
# Module: {{.Module}}
# Docs: https://www.elastic.co/guide/en/beats/{{.BeatName}}/{{ beat_doc_branch }}/{{.BeatName}}-module-{{.Module}}.html

{{.Config}}`[1:]

func GenerateDirModulesD() error {
	if err := os.RemoveAll("modules.d"); err != nil {
		return err
	}

	shortConfigs, err := filepath.Glob("module/*/_meta/config.yml")
	if err != nil {
		return err
	}

	for _, f := range shortConfigs {
		parts := strings.Split(filepath.ToSlash(f), "/")
		if len(parts) < 2 {
			continue
		}
		moduleName := parts[1]

		config, err := ioutil.ReadFile(f)
		if err != nil {
			return err
		}

		data, err := Expand(modulesDConfigTemplate, map[string]interface{}{
			"Module": moduleName,
			"Config": string(config),
		})
		if err != nil {
			return err
		}

		target := filepath.Join("modules.d", moduleName+".yml.disabled")
		err = ioutil.WriteFile(createDir(target), []byte(data), 0644)
		if err != nil {
			return err
		}
	}
	return nil
}
