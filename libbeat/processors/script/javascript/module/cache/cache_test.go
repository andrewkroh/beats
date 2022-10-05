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

package cache

import (
	"github.com/elastic/beats/v7/libbeat/processors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/processors/script/javascript"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent-libs/mapstr"

	// Register require module.
	_ "github.com/elastic/beats/v7/libbeat/processors/script/javascript/module/require"
)

func TestCache(t *testing.T) {
	const script = `
var cache = require('cache');

var persistentCache;

function register(scriptParams) {
	persistentCache = cache.New(scriptParams);
	persistentCache.Put("mykey", "foo");
}

function process(evt) {
	evt.Put("cached-value", persistentCache.Get("mykey"));
	evt.Put("missing-value", persistentCache.Get("not-mykey"));

	persistentCache.Delete("mykey");
	persistentCache.Delete("not-mykey");
}
`

	logp.TestingSetup()
	p, err := javascript.NewFromConfig(javascript.Config{
		MaxCachedSessions: 1,
		Source:            script,
		Params: map[string]interface{}{
			"id": "test-cache",
			"bolt": map[string]interface{}{
				"path": filepath.Join(t.TempDir(), "test.db"),
			},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	runtime.GC()

	evt, err := p.Run(&beat.Event{Fields: mapstr.M{"message": "hello world!"}})
	if err != nil {
		t.Fatal(err)
	}

	t.Log(evt.Fields)

	processors.Close(p)
	p = nil
	runtime.GC()
}
