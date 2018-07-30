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

package javascript

import (
	"io/ioutil"

	"github.com/pkg/errors"

	"github.com/robertkrimen/otto"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/paths"
	"github.com/elastic/beats/libbeat/processors"
)

type jsProcessor struct {
	File   string `config:"file"`
	Script string `config:"script"`

	vm  *otto.Otto
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
	p.log = logp.NewLogger("processor.script.js")

	p.vm = otto.New()

	if err := registerHelperProcessors(p.log, p.vm); err != nil {
		return errors.Wrap(err, "failed to register helper processor constructors in JS VM")
	}

	if _, err := p.vm.Run(p.Script); err != nil {
		return errors.Wrap(err, "failed to compile script")
	}

	evt, err := newJSEvent(p.log, p.vm)
	if err != nil {
		return errors.Wrap(err, "failed to create JS event wrapper")
	}
	p.evt = evt

	return nil
}

func registerHelperProcessors(log *logp.Logger, vm *otto.Otto) error {
	// TODO: WIP

	return vm.Set("NewDNSProcessor", func(call otto.FunctionCall) otto.Value {
		if !call.Argument(0).IsObject() {
			return otto.UndefinedValue()
		}

		config, _ := call.Argument(0).Export()
		log.Debugf("NewDNSProcessor config: %T, %v", config, config)

		dnsProc, _ := call.Otto.Object("({})")
		return dnsProc.Value()
	})
}

func (p *jsProcessor) Run(event *beat.Event) (*beat.Event, error) {
	_, err := p.vm.Call("process", nil, p.evt.Wrap(event))
	if err != nil {
		return event, errors.Wrap(err, "failed invoking javascript 'process' function")
	}

	if p.evt.dropped {
		return nil, nil
	}

	return event, nil
}

func (p *jsProcessor) String() string {
	return "script=[type=javascript]"
}
