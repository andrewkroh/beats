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

package dns

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/processors"
)

const logName = "processor.dns"

func init() {
	processors.RegisterPlugin("dns", newDNSProcessor)
}

type Processor struct {
	Config
	resolver PTRResolver
	log      *logp.Logger
}

func newDNSProcessor(cfg *common.Config) (processors.Processor, error) {
	var c Config
	if err := cfg.Unpack(&c); err != nil {
		return nil, errors.Wrap(err, "fail to unpack the dns configuration")
	}

	log := logp.NewLogger(logName)

	resolver, err := NewMiekgResolver(0, nil)
	if err != nil {
		return nil, err
	}

	cache, err := NewCachingResolver(c.CacheConfig, resolver)
	if err != nil {
		return nil, err
	}

	return &Processor{
		Config:   c,
		resolver: cache,
		log:      log,
	}, nil
}

func (p *Processor) Run(event *beat.Event) (*beat.Event, error) {
	for field, target := range p.Config.reverseFlat {
		p.processField(field, target, event)
	}
	return event, nil
}

func (p *Processor) processField(source, target string, event *beat.Event) error {
	v, err := event.GetValue(source)
	if err != nil {
		return nil
	}

	maybeIP, ok := v.(string)
	if !ok {
		return nil
	}

	name, err := p.resolver.LookupPTR(maybeIP)
	if err != nil {
		return nil
	}

	_, err = event.PutValue(target, name.Host)
	return err
}

func (p Processor) String() string {
	return fmt.Sprintf("dns=[reverse=[%v]]", p.Config.reverseFlat)
}
