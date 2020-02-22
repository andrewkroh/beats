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

package wineventlog

import (
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/elastic/beats/libbeat/logp"
)

func TestPublisherMetadataStore(t *testing.T) {
	logp.TestingSetup()

	s, err := newPublisherMetadataStore(NilHandle, "Microsoft-Windows-Sysmon", logp.NewLogger("metadata"))
	if err != nil {
		t.Fatal(err)
	}

	logHandle := openLog(t, sysmon9File, "1")
	defer logHandle.Close()

	handles, err := EventHandles(logHandle, 32)
	if err != nil {
		t.Fatal(err)
	}

	h := handles[0]
	defer h.Close()

	em, err := newEventMetadataFromEventHandle(s.Metadata, h)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(spew.Sdump(em))
}
