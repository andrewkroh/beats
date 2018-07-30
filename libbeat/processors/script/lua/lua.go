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

package lua

import (
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/yuin/gopher-lua"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/paths"
	"github.com/elastic/beats/libbeat/processors"
	"github.com/elastic/beats/libbeat/processors/script/lua/filepath"
)

type luaProcessor struct {
	File   string `config:"file"`
	Script string `config:"script"`

	state *lua.LState
	log   *logp.Logger
}

func NewProcessorFromConfig(c *common.Config) (processors.Processor, error) {
	p := &luaProcessor{}
	if err := c.Unpack(p); err != nil {
		return nil, err
	}

	if p.File == "" && p.Script == "" {
		return nil, errors.Errorf("a lua script must be defined via 'file' or inline using 'script'")
	}

	if p.File != "" {
		file := paths.Resolve(paths.Config, p.File)
		data, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read script file %v", file)
		}
		p.Script = string(data)
	}

	if err := p.init(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *luaProcessor) init() error {
	p.log = logp.NewLogger("processor.script.lua")

	// Open a subset of modules.
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	for _, pair := range []struct {
		n string
		f lua.LGFunction
	}{
		{lua.LoadLibName, lua.OpenPackage}, // Must be first
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
	} {
		if err := L.CallByParam(lua.P{
			Fn:      L.NewFunction(pair.f),
			NRet:    0,
			Protect: true,
		}, lua.LString(pair.n)); err != nil {
			return err
		}
	}
	L.PreloadModule(filepath.ModuleName, filepath.Loader)

	if err := L.DoString(p.Script); err != nil {
		return errors.Wrap(err, "failure loading lua script")
	}

	p.state = L
	return nil
}

func (p *luaProcessor) Run(event *beat.Event) (*beat.Event, error) {
	luaEvent := &LuaEvent{event}

	// Call process(event).
	if err := p.state.CallByParam(lua.P{
		Fn:      p.state.GetGlobal("process"),
		NRet:    1,
		Protect: true,
	}, luaEvent.getLValue(p.state)); err != nil {
		return nil, errors.Wrap(err, "failed in lua script while invoking process()")
	}

	// Get the return value (if any).
	ret := p.state.Get(-1)
	if ret != lua.LNil {
		// Remove received value.
		p.log.Debugf("lua script returned %v", ret)
	}
	p.state.Pop(1)

	return luaEvent.event, nil
}

func (p *luaProcessor) String() string {
	return "script=[type=lua]"
}
