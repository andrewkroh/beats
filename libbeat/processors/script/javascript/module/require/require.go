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

package require

import (
	"io/ioutil"

	"github.com/dop251/goja_nodejs/require"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/paths"
	"github.com/elastic/beats/libbeat/processors/script/javascript"
)

func init() {
	javascript.AddSessionHook("require", func(s javascript.Session) {
		reg := require.NewRegistryWithLoader(loadSource)
		reg.Enable(s.Runtime())
	})
}

// loadSource checks the file's permissions and resolves the path to the config
// path if it is relative.
func loadSource(path string) ([]byte, error) {
	path = paths.Resolve(paths.Config, path)

	if err := common.OwnerHasExclusiveWritePerms(path); err != nil {
		return nil, err
	}

	return ioutil.ReadFile(path)
}
