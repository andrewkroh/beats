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

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/processors/script/javascript"

	_ "github.com/elastic/beats/libbeat/processors/script/javascript/module/require"
)

func testEvent() *beat.Event {
	return &beat.Event{
		Fields: common.MapStr{
			"source": common.MapStr{
				"ip": "192.0.2.1",
			},
			"message": "key=hello",
		},
	}
}

func TestNewProcessorDNS(t *testing.T) {
	const script = `
var processor = require('processor');

var dns = new processor.DNS({
    type: "reverse",
    fields: {
        "source.ip": "source.domain",
        "destination.ip": "destination.domain"
    },
    tag_on_failure: ["_dns_reverse_lookup_failed"],
});

function process(evt) {
	dns.Run(evt);
    if (evt.Get().tags[0] !== "_dns_reverse_lookup_failed") {
        throw "missing tag"
    }
}
`

	logp.TestingSetup()
	p, err := javascript.NewFromConfig(javascript.Config{Code: script}, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Run(testEvent())
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewProcessorDissect(t *testing.T) {
	const script = `
var processor = require('processor');

var chopLog = new processor.Dissect({
    tokenizer: "key=%{key}",
    field: "message",
});

function process(evt) {
    chopLog.Run(evt);
}
`

	logp.TestingSetup()
	p, err := javascript.NewFromConfig(javascript.Config{Code: script}, nil)
	if err != nil {
		t.Fatal(err)
	}

	evt, err := p.Run(testEvent())
	if err != nil {
		t.Fatal(err)
	}

	key, _ := evt.GetValue("dissect.key")
	assert.Equal(t, "hello", key)
}
