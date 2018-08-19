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

package goja

import (
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/paths"
	"github.com/elastic/beats/libbeat/processors"
)

type jsProcessor struct {
	File   string `config:"file"`
	Script string `config:"script"`

	vm  *goja.Runtime
	evt *jsEvent
	log *logp.Logger
}

func NewProcessorFromConfig(c *common.Config) (processors.Processor, error) {
	p := &jsProcessor{}
	if err := c.Unpack(p); err != nil {
		return nil, err
	}

	if p.File == "" && p.Script == "" {
		return nil, errors.Errorf("a javascript script must be defined via 'file' or inline using 'script'")
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

func (p *jsProcessor) init() error {
	p.log = logp.NewLogger("processor.script.goja")

	p.vm = goja.New()

	new(require.Registry).Enable(p.vm)
	console.Enable(p.vm)

	var err error
	p.evt, err = newJSEvent(p.log, p.vm)
	if err != nil {
		return errors.Wrap(err, "failed to create event wrapper")
	}

	if _, err := p.vm.RunString(p.Script); err != nil {
		return errors.Wrap(err, "failed to compile script")
	}

	return nil
}

func (p *jsProcessor) Run(event *beat.Event) (*beat.Event, error) {
	var call goja.Callable
	if err := p.vm.ExportTo(p.vm.Get("process"), &call); err != nil {
		return nil, err
	}

	_, err := call(goja.Undefined(), p.evt.Wrap(event))
	if err != nil {
		return event, errors.Wrap(err, "failed invoking javascript 'process' function")
	}

	if p.evt.dropped {
		return nil, nil
	}

	return event, nil
}

func (p *jsProcessor) String() string {
	return "script=[type=goja]"
}
