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

package filepath

import (
	"path/filepath"

	"github.com/yuin/gopher-lua"
)

const ModuleName = "filepath"

func Loader(L *lua.LState) int {
	// register functions to the table
	mod := L.SetFuncs(L.NewTable(), exports)
	// register other stuff
	L.SetField(mod, "name", lua.LString(ModuleName))

	// returns the module
	L.Push(mod)
	return 1
}

var exports = map[string]lua.LGFunction{
	"base": base,
	"dir":  dir,
	"ext":  ext,
}

func dir(L *lua.LState) int {
	if L.GetTop() != 1 {
		L.ArgError(1, "path arg expected")
		return 0
	}

	path := L.CheckString(1)
	rtn := filepath.Dir(path)
	L.Push(lua.LString(rtn))
	return 1
}

func base(L *lua.LState) int {
	if L.GetTop() != 1 {
		L.ArgError(1, "path arg expected")
		return 0
	}

	path := L.CheckString(1)
	rtn := filepath.Base(path)
	L.Push(lua.LString(rtn))
	return 1
}

func ext(L *lua.LState) int {
	if L.GetTop() != 1 {
		L.ArgError(1, "path arg expected")
		return 0
	}

	path := L.CheckString(1)
	rtn := filepath.Ext(path)
	L.Push(lua.LString(rtn))
	return 1
}
