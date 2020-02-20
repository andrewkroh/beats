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
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/winlogbeat/sys"
)

func TestTemplateFunc(t *testing.T) {
	tmpl := template.Must(template.New("").
		Funcs(template.FuncMap{"eventParam": eventParam}).
		Parse(`Hello {{ eventParam $ 1 }}! Foo {{ eventParam $ 2 }}.`))

	buf := new(bytes.Buffer)
	err := tmpl.Execute(buf, []interface{}{"world"})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "Hello world! Foo %2.", buf.String())
}

func TestTemplateNodes(t *testing.T) {
	tmpl := template.Must(template.New("").
		Funcs(template.FuncMap{"eventParam": eventParam}).
		Parse(`Hello {{ eventParam $ 1 }}! Foo {{ eventParam $ 2 }}.`))

	t.Log(len(tmpl.Root.Nodes))
	t.Log(tmpl.Root.Nodes[0].String())
}

func TestRenderSysmon9(t *testing.T) {
	logp.TestingSetup()

	logHandle := openLog(t, "../../../x-pack/winlogbeat/module/sysmon/test/testdata/sysmon-9.01.evtx")

	defer logHandle.Close()

	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	events := readEvents(t, logHandle, r)
	assert.NotEmpty(t, events)

	if t.Failed() {
		logEventsAsJSON(t, events)
	}
}

func TestRenderSecurityEventID4752(t *testing.T) {
	logp.TestingSetup()

	logHandle := openLog(t, "../../../x-pack/winlogbeat/module/security/test/testdata/4752.evtx")
	defer logHandle.Close()

	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	events := readEvents(t, logHandle, r)
	if !assert.Len(t, events, 1) {
		return
	}
	e := events[0]

	assert.EqualValues(t, 4752, e.EventIdentifier.ID)
	assert.Equal(t, "Microsoft-Windows-Security-Auditing", e.Provider.Name)
	assertEqualIgnoreCase(t, "{54849625-5478-4994-a5ba-3e3b0328c30d}", e.Provider.GUID)
	assert.Equal(t, "DC_TEST2k12.TEST.SAAS", e.Computer)
	assert.Equal(t, "Security", e.Channel)
	assert.EqualValues(t, 3707686, e.RecordID)

	assert.Equal(t, e.Keywords, []string{"Audit Success"})

	assert.EqualValues(t, 0, e.OpcodeRaw)
	assert.Equal(t, "Info", e.Opcode)

	assert.EqualValues(t, 0, e.LevelRaw)
	assert.Equal(t, "Information", e.Level)

	assert.EqualValues(t, 13827, e.TaskRaw)
	assert.Equal(t, "Distribution Group Management", e.Task)

	assert.EqualValues(t, 492, e.Execution.ProcessID)
	assert.EqualValues(t, 1076, e.Execution.ThreadID)
	assert.Len(t, e.EventData.Pairs, 10)

	assert.NotEmpty(t, e.Message)

	if t.Failed() {
		logEventsAsJSON(t, events)
	}
}

func TestRenderApplicationWindowsErrorReporting1001(t *testing.T) {
	logp.TestingSetup()

	logHandle := openLog(t, "testdata/application-windows-error-reporting.evtx")
	defer logHandle.Close()

	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	events := readEvents(t, logHandle, r)
	if !assert.Len(t, events, 1) {
		return
	}
	e := events[0]

	assert.EqualValues(t, 1001, e.EventIdentifier.ID)
	assert.Equal(t, "Windows Error Reporting", e.Provider.Name)
	assert.Empty(t, e.Provider.GUID)
	assert.Equal(t, "vagrant", e.Computer)
	assert.Equal(t, "Application", e.Channel)
	assert.EqualValues(t, 420107, e.RecordID)

	assert.Equal(t, e.Keywords, []string{"Classic"})

	assert.EqualValues(t, 0, e.OpcodeRaw)
	assert.Equal(t, "", e.Opcode)

	assert.EqualValues(t, 4, e.LevelRaw)
	assert.Equal(t, "Information", e.Level)

	assert.EqualValues(t, 0, e.TaskRaw)
	assert.Equal(t, "None", e.Task)

	assert.EqualValues(t, 0, e.Execution.ProcessID)
	assert.EqualValues(t, 0, e.Execution.ThreadID)
	assert.Len(t, e.EventData.Pairs, 23)

	assert.NotEmpty(t, e.Message)

	if t.Failed() {
		logEventsAsJSON(t, events)
	}
}

func openLog(t *testing.T, log string) EvtHandle {
	var flags EvtQueryFlag = EvtQueryReverseDirection

	if info, err := os.Stat(log); err == nil && info.Mode().IsRegular() {
		flags |= EvtQueryFilePath
	} else {
		flags |= EvtQueryChannelPath
	}

	h, err := EvtQuery(NilHandle, log, "", flags)
	if err != nil {
		t.Fatal("failed to open log", log, err)
	}
	return h
}

func assertEqualIgnoreCase(t *testing.T, expected, actual string) {
	t.Helper()
	assert.Equal(t,
		strings.ToLower(expected),
		strings.ToLower(actual),
	)
}

func readEvents(t *testing.T, logHandle EvtHandle, renderer *Renderer) []*sys.Event {
	t.Helper()

	var events []*sys.Event
	for {
		handles, err := EventHandles(logHandle, 50)
		if err != nil {
			if err == ERROR_NO_MORE_ITEMS {
				break
			}
			t.Fatal(err)
		}

		for _, h := range handles {
			evt, err := renderer.Render(h)
			h.Close()
			if err != nil {
				t.Fatalf("Render failed: %+v", err)
			}

			events = append(events, evt)
		}
	}
	return events
}

func logEventsAsJSON(t testing.TB, events []*sys.Event) {
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
}

//<TimeCreated SystemTime="2019-12-19T08:21:23.644422500Z" />
//<EventRecordID>3707686</EventRecordID>
//<Correlation />
//<Execution ProcessID="492" ThreadID="1076" />
//<Channel>Security</Channel>
//<Computer>DC_TEST2k12.TEST.SAAS</Computer>
//<Security />
//</System>
//- <EventData>
//<Data Name="MemberName">CN=Administrator,CN=Users,DC=TEST,DC=SAAS</Data>
//<Data Name="MemberSid">S-1-5-21-1717121054-434620538-60925301-500</Data>
//<Data Name="TargetUserName">testglobal1</Data>
//<Data Name="TargetDomainName">TEST</Data>
//<Data Name="TargetSid">S-1-5-21-1717121054-434620538-60925301-2904</Data>
//<Data Name="SubjectUserSid">S-1-5-21-1717121054-434620538-60925301-2794</Data>
//<Data Name="SubjectUserName">at_adm</Data>
//<Data Name="SubjectDomainName">TEST</Data>
//<Data Name="SubjectLogonId">0x2e67800</Data>
//<Data Name="PrivilegeList">-</Data>
//</EventData>
//</Event>
