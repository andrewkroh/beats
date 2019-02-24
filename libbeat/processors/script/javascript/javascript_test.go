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

package javascript_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/processors/script/javascript"

	_ "github.com/elastic/beats/libbeat/processors/script/javascript/module"
)

func TestConsole(t *testing.T) {
	const script = `
        var console = require('console');

		function process(event) {
			console.log("console log");
			console.warn("console warn");
			console.error("console error");
 			return event;
		}
	`
	e := &beat.Event{Fields: common.MapStr{"hello": "world"}}

	logp.DevelopmentSetup(logp.ToObserverOutput())
	p, err := javascript.NewFromConfig(javascript.Config{
		Code: script,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Run(e)
	if err != nil {
		t.Fatal(err)
	}

	// Validate that the console messages were logged with correct levels.
	logs := logp.ObserverLogs()
	assert.EqualValues(t, logp.DebugLevel, logs.FilterMessage("console log").All()[0].Level)
	assert.EqualValues(t, logp.WarnLevel, logs.FilterMessage("console warn").All()[0].Level)
	assert.EqualValues(t, logp.ErrorLevel, logs.FilterMessage("console error").All()[0].Level)
}
