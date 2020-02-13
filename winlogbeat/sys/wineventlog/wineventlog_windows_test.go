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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"

	"github.com/elastic/beats/winlogbeat/sys"
)

var sysmonEvtx string

func init() {
	var err error
	sysmonEvtx, err = filepath.Abs("testdata/sysmon-9.01.evtx")
	if err != nil {
		panic(err)
	}

	if _, err = os.Lstat(sysmonEvtx); err != nil {
		panic(err)
	}
}

func TestEvtOpenLog(t *testing.T) {
	h, err := EvtOpenLog(0, sysmonEvtx, EvtOpenFilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(h)
}

func TestEvtQuery(t *testing.T) {
	h, err := EvtQuery(0, sysmonEvtx, "", EvtQueryFilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(h)
}

func TestReadEvtx(t *testing.T) {
	// Open .evtx file.
	h, err := EvtQuery(0, sysmonEvtx, "", EvtQueryFilePath|EvtQueryReverseDirection)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(h)

	// Get handles to events.
	buf := make([]byte, 32*1024)
	out := new(bytes.Buffer)
	count := 0
	for {
		handles, err := EventHandles(h, 8)
		if err == ERROR_NO_MORE_ITEMS {
			t.Log(err)
			break
		}
		if err != nil {
			t.Fatal(err)
		}

		// Read events.
		for _, h := range handles {
			out.Reset()
			if err = RenderEventXML(h, buf, out); err != nil {
				t.Fatal(err)
			}
			Close(h)
			count++
		}
	}

	if count != 32 {
		t.Fatal("expected to read 32 events but got", count, "from", sysmonEvtx)
	}
}

func TestChannels(t *testing.T) {
	channels, err := Channels()
	if err != nil {
		t.Fatal(err)
	}
	assert.NotEmpty(t, channels)

	for _, c := range channels {
		ext := filepath.Ext(c)
		if ext != "" {
			t.Fatal(err)
		}
	}
}

func TestEvtRender(t *testing.T) {
	// Open .evtx file.
	logHandle, err := EvtQuery(0, sysmonEvtx, "", EvtQueryFilePath|EvtQueryReverseDirection)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(logHandle)

	handles, err := EventHandles(logHandle, 1)
	if !assert.NoError(t, err) || !assert.NotEmpty(t, handles) {
		return
	}
	h := handles[0]

	renderCtx, err := _EvtCreateRenderContext(0, 0, EvtRenderContextSystem)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(renderCtx)

	var bufferUsed, propertyCount uint32
	err = _EvtRender(renderCtx, h, EvtRenderEventValues, 0, nil, &bufferUsed, &propertyCount)
	if err != ERROR_INSUFFICIENT_BUFFER {
		t.Fatal("expected ERROR_INSUFFICIENT_BUFFER", err)
	}
	t.Log(bufferUsed, propertyCount)

	buf := make([]byte, bufferUsed)
	err = _EvtRender(renderCtx, h, EvtRenderEventValues, bufferUsed, &buf[0], &bufferUsed, &propertyCount)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("system context size=", sizeofEvtVariant)
	t.Log("property count=", propertyCount)

	var event sys.Event
	for i := 0; i < int(propertyCount); i++ {
		property := EvtSystemPropertyID(i)
		offset := i * int(sizeofEvtVariant)
		evtVar := (*EvtVariant)(unsafe.Pointer(&buf[offset]))

		data, err := evtVar.Data(buf)
		t.Logf("name=%v, type=%v, is_array=%v, data=%v, data_error=%v", property, evtVar.Type, evtVar.Type.IsArray(), data, err)
		if err != nil || data == nil {
			continue
		}

		switch property {
		case EvtSystemProviderName:
			event.Provider.Name = data.(string)
		case EvtSystemProviderGuid:
			event.Provider.GUID = data.(windows.GUID).String()
		case EvtSystemEventID:
			event.EventIdentifier.ID = uint32(data.(uint16))
		case EvtSystemQualifiers:
			event.EventIdentifier.Qualifiers = data.(uint16)
		case EvtSystemLevel:
			event.LevelRaw = data.(uint8)
		case EvtSystemTask:
			event.TaskRaw = data.(uint16)
		case EvtSystemOpcode:
			event.OpcodeRaw = data.(uint8)
		case EvtSystemKeywords:
			// TODO:
			event.Keywords = []string{strconv.FormatInt(data.(int64), 10)}
		case EvtSystemTimeCreated:
			event.TimeCreated.SystemTime = data.(time.Time)
		case EvtSystemEventRecordId:
			event.RecordID = data.(uint64)
		case EvtSystemActivityID:
			event.Correlation.ActivityID = data.(windows.GUID).String()
		case EvtSystemRelatedActivityID:
			event.Correlation.RelatedActivityID = data.(windows.GUID).String()
		case EvtSystemProcessID:
			event.Execution.ProcessID = data.(uint32)
		case EvtSystemThreadID:
			event.Execution.ThreadID = data.(uint32)
		case EvtSystemChannel:
			event.Channel = data.(string)
		case EvtSystemComputer:
			event.Computer = data.(string)
		case EvtSystemUserID:
			event.User.Identifier = data.(string)
		case EvtSystemVersion:
			event.Version = sys.Version(data.(uint8))
		}
	}

	readUserContext(t, h, &event)

	t.Logf("%#v", event)
}

func readUserContext(t *testing.T, h EvtHandle, event *sys.Event) {
	renderCtx, err := _EvtCreateRenderContext(0, 0, EvtRenderContextUser)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(renderCtx)

	var bufferUsed, propertyCount uint32
	err = _EvtRender(renderCtx, h, EvtRenderEventValues, 0, nil, &bufferUsed, &propertyCount)
	if err != ERROR_INSUFFICIENT_BUFFER {
		t.Fatal("expected ERROR_INSUFFICIENT_BUFFER", err)
	}
	t.Log(bufferUsed, propertyCount)

	buf := make([]byte, bufferUsed)
	err = _EvtRender(renderCtx, h, EvtRenderEventValues, bufferUsed, &buf[0], &bufferUsed, &propertyCount)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("user context size=", sizeofEvtVariant)
	t.Log("property count=", propertyCount)

	mds, err := DumpEventMetadata(event.Provider.Name)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < int(propertyCount); i++ {
		offset := i * int(sizeofEvtVariant)
		evtVar := (*EvtVariant)(unsafe.Pointer(&buf[offset]))

		name, err := mds.LookupParam(event.EventIdentifier.ID, i)
		if err != nil {
			t.Fatal(err)
		}

		data, err := evtVar.Data(buf)
		if err != nil {
			t.Log(err)
		}
		t.Logf("name=%v, type=%v, is_array=%v, data=%v", name, evtVar.Type, evtVar.Type.IsArray(), data)

		event.EventData.Pairs = append(event.EventData.Pairs, sys.KeyValue{Key: name, Value: fmt.Sprintf("%s", data)})
	}
}

func TestDumpEventMetadata(t *testing.T) {
	logp.TestingSetup()

	//mds, err := DumpEventMetadata("Windows Error Reporting")
	mds, err := DumpEventMetadata("Microsoft-Windows-Security-Auditing")
	if err != nil {
		t.Fatalf("%+v", err)
	}

	t.Logf("%#v", mds)
}

func TestRenderer(t *testing.T) {
	logp.TestingSetup(logp.WithLevel(logp.DebugLevel))

	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Open .evtx file.
	//logHandle, err := EvtOpenLog(0, "Security", EvtOpenChannelPath)
	//logHandle, err := EvtQuery(0, sysmonEvtx, "", EvtQueryFilePath|EvtQueryReverseDirection)
	logHandle, err := EvtQuery(0, "Application", "", EvtQueryChannelPath|EvtQueryReverseDirection)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(logHandle)

	for {
		handles, err := EventHandles(logHandle, 50)
		if err != nil {
			if err == ERROR_NO_MORE_ITEMS {
				return
			}
			t.Fatal(err)
		}

		for _, h := range handles {
			evt, err := r.Render(h)
			if err != nil {
				t.Fatalf("Render failed: %+v", err)
			}

			data, err := json.MarshalIndent(evt, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			_ = data
			//t.Logf("%s", string(data))
		}
	}
}

func TestGetProviderKeywords(t *testing.T) {
	logp.TestingSetup()

	pub, err := EvtOpenPublisherMetadata("Microsoft-Windows-PowerShell")
	if err != nil {
		t.Fatalf("%+v", err)
	}

	_, err = EvtGetPublisherMetadataProperty(pub, EvtPublisherMetadataKeywords)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	pub, err = EvtOpenPublisherMetadata("Microsoft-Windows-Security-Auditing")
	if err != nil {
		t.Fatalf("%+v", err)
	}

	_, err = EvtGetPublisherMetadataProperty(pub, EvtPublisherMetadataKeywords)
	if err != nil {
		t.Fatalf("%+v", err)
	}
}





// EvtOpenPublisherMetadata
// EvtOpenEventMetadataEnum
// for EvtNextEventMetadata
//     for each EvtEventMetadataPropertyIdEND
//         EvtGetEventMetadataProperty
//         print template
