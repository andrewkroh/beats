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
	"os"
	"regexp"

	"github.com/magefile/mage/mg"

	"github.com/elastic/beats/dev-tools/mage"
)

func ConfigOSS()   { mg.Deps(ShortConfig, DockerConfig, ReferenceConfigOSS) }
func ConfigXPack() { mg.Deps(ShortConfig, DockerConfig, ReferenceConfigXPack) }

func ShortConfig() error {
	var configParts = []string{
		mage.OSSBeatDir("_meta/common.p1.yml"),
		mage.OSSBeatDir("_meta/common.p2.yml"),
		mage.LibbeatDir("_meta/config.yml"),
	}

	configFile := mage.BeatName + ".yml"
	mage.MustFileConcat(configFile, 0640, configParts...)
	mage.MustFindReplace(configFile, regexp.MustCompile("beatname"), mage.BeatName)
	mage.MustFindReplace(configFile, regexp.MustCompile("beat-index-prefix"), mage.BeatIndexPrefix)
	return nil
}

func DockerConfig() error {
	var configParts = []string{
		mage.OSSBeatDir("_meta/beat.docker.yml"),
		mage.LibbeatDir("_meta/config.docker.yml"),
	}

	configFile := mage.BeatName + ".docker.yml"
	mage.MustFileConcat(configFile, 0640, configParts...)
	mage.MustFindReplace(configFile, regexp.MustCompile("beatname"), mage.BeatName)
	mage.MustFindReplace(configFile, regexp.MustCompile("beat-index-prefix"), mage.BeatIndexPrefix)
	return nil
}

func ReferenceConfigOSS() error   { return referenceConfig(mage.OSSBeatDir("module")) }
func ReferenceConfigXPack() error { return referenceConfig(mage.OSSBeatDir("module"), "module") }

func referenceConfig(moduleDirs ...string) error {
	const modulesConfigYml = "build/config.modules.yml"
	err := mage.GenerateModuleReferenceConfig(modulesConfigYml, moduleDirs...)
	if err != nil {
		return err
	}
	defer os.Remove(modulesConfigYml)

	var configParts = []string{
		mage.OSSBeatDir("_meta/common.reference.p1.yml"),
		modulesConfigYml,
		mage.OSSBeatDir("_meta/common.reference.p2.yml"),
		mage.LibbeatDir("_meta/config.reference.yml"),
	}

	configFile := mage.BeatName + ".reference.yml"
	mage.MustFileConcat(configFile, 0640, configParts...)
	mage.MustFindReplace(configFile, regexp.MustCompile("beatname"), mage.BeatName)
	mage.MustFindReplace(configFile, regexp.MustCompile("beat-index-prefix"), mage.BeatIndexPrefix)
	return nil
}
