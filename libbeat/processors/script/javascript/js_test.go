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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

func newEvent() *beat.Event {
	return &beat.Event{
		Fields: common.MapStr{
			"source": common.MapStr{
				"ip": "192.168.1.1",
			},
		},
	}
}

const testScript = `
var dns = NewDNSProcessor({
	hit_cache_ttl: "30s",
});
if (typeof dns == 'undefined') {
	throw "failed to initialize dns processor";
}

if [source][ip] in network(private) {

}

if (networkContain(
var rename = function(event, from, to) {
	var v = event.get(from);
	if (event.put(to, v)) {
		event.delete(from);
    }
};

function process(event) {
  console.log("Before rename: " + event.get("source.ip"));
  rename(event, "source.ip", "destination.ip");
  console.log("After rename: " + event.get("destination.ip"));
};
`

const putScript = `
function process(event) {
  event.put("hello", "world")
};
`

func TestJavaScriptProcessor(t *testing.T) {
	logp.TestingSetup()

	p := &jsProcessor{Script: putScript}
	if err := p.init(); err != nil {
		t.Fatal(err)
	}

	e, err := p.Run(newEvent())
	if err != nil {
		t.Fatalf("%+v", err)
	}

	v, _ := e.GetValue("hello")
	assert.Equal(t, v, "world")
}

func TestJSEvent(t *testing.T) {
	logp.TestingSetup()

	script := `
function process(event) {
	if (!event.put("source", 10)) {
		throw "failed to put 10";
	}
    var source = event.get("source");
    if (10 != source) {
		throw "source is not set to 10";
    }
	if (!event.delete("source")) {
		throw "failed to delete source";
	}
	if (!event.put("source", {"ip": "192.168.10.1"})) {
		throw "failed to put object";
	}
	if (!event.rename("source", "destination")) {
		throw "failed to rename source";
	}
};
`
	p := &jsProcessor{Script: script}
	if err := p.init(); err != nil {
		t.Fatal(err)
	}

	e, err := p.Run(newEvent())
	if err != nil {
		t.Fatalf("%+v", err)
	}

	assert.Equal(t, common.MapStr{
		"destination.ip": "192.168.10.1",
	}, e.Fields.Flatten())
}

func BenchmarkJavascriptProcessorRun(b *testing.B) {
	p := &jsProcessor{Script: putScript}
	if err := p.init(); err != nil {
		b.Fatal(err)
	}

	event := newEvent()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Run(event)
		if err != nil {
			b.Fatal(err)
		}
	}
}
