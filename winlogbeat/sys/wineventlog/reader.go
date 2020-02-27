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

// +build windows

package wineventlog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"text/template"
	"time"
	"unsafe"

	"github.com/cespare/xxhash"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
	"golang.org/x/sys/windows"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/winlogbeat/sys"
)

// https://docs.microsoft.com/en-us/dotnet/api/system.diagnostics.eventing.reader.standardeventkeywords?view=netframework-4.8
const (
	keywordClassic = 0x80000000000000
)

type Renderer struct {
	metadataCache map[string]*publisherMetadataStore
	systemContext EvtHandle // Render context for system values.
	userContext   EvtHandle // Render context for user values (event data).
	log           *logp.Logger
}

func NewRenderer() (*Renderer, error) {
	systemContext, err := _EvtCreateRenderContext(0, 0, EvtRenderContextSystem)
	if err != nil {
		return nil, err
	}
	userContext, err := _EvtCreateRenderContext(0, 0, EvtRenderContextUser)
	if err != nil {
		return nil, err
	}
	return &Renderer{
		metadataCache: map[string]*publisherMetadataStore{},
		systemContext: systemContext,
		userContext:   userContext,
		log:           logp.NewLogger("renderer"),
	}, nil
}

func (r *Renderer) Close() error {
	return multierr.Combine(
		r.systemContext.Close(),
		r.userContext.Close(),
	)
}

func (r *Renderer) Render(handle EvtHandle) (*sys.Event, error) {
	event := &sys.Event{}

	if err := r.renderSystem(handle, event); err != nil {
		return nil, errors.Wrap(err, "failed to render system properties")
	}

	var errs []error

	// This always returns a non-nil value (even on error).
	md, err := r.getPublisherMetadata(event.Provider.Name)
	if err != nil {
		errs = append(errs, err)
	}

	eventData, fingerprint, err := r.renderUser(handle, event)
	if err != nil {
		errs = append(errs, errors.Wrap(err, "failed to render event data"))
	}

	r.log.Infow("event fingerprint", "event.code", event.EventIdentifier.ID, "fingerprint", fingerprint)

	// Load cached event metadata or try to bootstrap it from the event's XML.
	eventMetadata := md.getEventMetadata(uint16(event.EventIdentifier.ID), fingerprint, handle)

	// Associate raw system properties to names (e.g. level=2 to Error).
	enrichRawValuesWithNames(md, event)

	if err = r.addEventData(eventMetadata, eventData, event); err != nil {
		errs = append(errs, err)
	}

	if event.Message, err = r.formatMessage(md, handle, event, eventData); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return event, multierr.Combine(errs...)
	}
	return event, nil
}

func (r *Renderer) getPublisherMetadata(publisher string) (*publisherMetadataStore, error) {
	var err error
	md, found := r.metadataCache[publisher]
	if !found {
		md, err = newPublisherMetadataStore(NilHandle, publisher, r.log)
		if err != nil {
			md = newEmptyPublisherMetadataStore(publisher, r.log)
		}
		r.metadataCache[publisher] = md
	}

	return md, err
}

func (r *Renderer) renderSystem(handle EvtHandle, event *sys.Event) error {
	bb, propertyCount, err := r.render(r.systemContext, handle)
	if err != nil {
		return errors.Wrap(err, "failed to get system values")
	}
	defer bb.free()

	for i := 0; i < int(propertyCount); i++ {
		property := EvtSystemPropertyID(i)
		offset := i * int(sizeofEvtVariant)
		evtVar := (*EvtVariant)(unsafe.Pointer(&bb.buf[offset]))

		data, err := evtVar.Data(bb.buf)
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
			event.KeywordsRaw = sys.HexInt64(data.(hexInt64))
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
			sid := data.(*windows.SID)
			event.User.Identifier, _ = sid.String()
			var accountType uint32
			event.User.Name, event.User.Domain, accountType, _ = sid.LookupAccount("")
			event.User.Type = sys.SIDType(accountType)
		case EvtSystemVersion:
			event.Version = sys.Version(data.(uint8))
		}
	}
	return nil
}

func (r *Renderer) renderUser(handle EvtHandle, event *sys.Event) ([]interface{}, uint64, error) {
	bb, propertyCount, err := r.render(r.userContext, handle)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to get user values")
	}
	defer bb.free()

	if propertyCount == 0 {
		return nil, 0, nil
	}

	// Fingerprint the argument types to help ensure we match these values with
	// the correct event data parameter names.
	argumentHash := xxhash.New()
	binary.Write(argumentHash, binary.LittleEndian, propertyCount)

	parameters := make([]interface{}, propertyCount)
	for i := 0; i < propertyCount; i++ {
		offset := i * int(sizeofEvtVariant)
		evtVar := (*EvtVariant)(unsafe.Pointer(&bb.buf[offset]))
		binary.Write(argumentHash, binary.LittleEndian, uint32(evtVar.Type))

		parameters[i], err = evtVar.Data(bb.buf)
		if err != nil {
			r.log.Warnw("Failed to read event parameter.",
				"provider", event.Provider.Name,
				"event_id", event.EventIdentifier.ID,
				"parameter_index", i,
				"parameter_type", evtVar.Type.String(),
				"error", err,
			)
		}
	}
	return parameters, argumentHash.Sum64(), nil
}

func (r *Renderer) render(context EvtHandle, eventHandle EvtHandle) (*byteBuffer, int, error) {
	var bufferUsed, propertyCount uint32
	err := _EvtRender(context, eventHandle, EvtRenderEventValues, 0, nil, &bufferUsed, &propertyCount)
	if err != nil && err != ERROR_INSUFFICIENT_BUFFER {
		return nil, 0, errors.Errorf("expected ERROR_INSUFFICIENT_BUFFER but got %v", err)
	}
	if propertyCount == 0 {
		return nil, 0, nil
	}

	bb := newByteBuffer()
	bb.SetLength(int(bufferUsed))
	err = _EvtRender(context, eventHandle, EvtRenderEventValues, uint32(len(bb.buf)), &bb.buf[0], &bufferUsed, &propertyCount)
	if err != nil {
		bb.free()
		return nil, 0, errors.Wrap(err, "failed to get values")
	}

	return bb, int(propertyCount), nil
}

func (r *Renderer) addEventData(evtMeta *eventMetadata, values []interface{}, event *sys.Event) error {
	if len(values) == 0 {
		return nil
	}

	if evtMeta == nil {
		r.log.Warnw("Event metadata not found.",
			"provider", event.Provider.Name,
			"event_id", event.EventIdentifier.ID)
	} else if len(values) != len(evtMeta.EventData) {
		r.log.Warnw("The number of event data parameters doesn't match the number "+
			"of parameters in the template.",
			"provider", event.Provider.Name,
			"event_id", event.EventIdentifier.ID,
			"event_parameter_count", len(values),
			"template_parameter_count", len(evtMeta.EventData),
			"template_version", evtMeta.Version,
			"event_version", event.Version)
	}

	// Fallback to paramN naming when the value does not exist in event data.
	// This can happen for legacy providers without manifests. This can also
	// happen if the installed provider manifest doesn't match the version that
	// produced the event (forwarded events, reading from evtx, or software was
	// updated). If software was updated it could also be that this cached
	// template is no longer valid.
	paramName := func(idx int) string {
		if evtMeta != nil && idx < len(evtMeta.EventData) {
			return evtMeta.EventData[idx].Name
		}
		return "param" + strconv.Itoa(idx)
	}

	for i, v := range values {
		var strVal string
		switch t := v.(type) {
		case string:
			strVal = t
		case *windows.SID:
			strVal, _ = t.String()
		default:
			strVal = fmt.Sprintf("%v", v)
		}

		event.EventData.Pairs = append(event.EventData.Pairs, sys.KeyValue{
			Key:   paramName(i),
			Value: strVal,
		})
	}

	return nil
}

func (r *Renderer) formatMessage(publisherMeta *publisherMetadataStore, eventHandle EvtHandle, event *sys.Event, values []interface{}) (string, error) {
	eventID := uint16(event.EventIdentifier.ID)
	data := publisherMeta.Events[eventID]

	if data == nil {
		// Try to get the raw message string using the event handle.
		msg, err := getMessageStringFromHandle(publisherMeta.Metadata, eventHandle, insertStrings.EvtVariants[:])
		if err != nil {
			return "", err
		}
		eventMetadata := &eventMetadata{
			EventID: eventID,
		}
		if err = eventMetadata.setMessage(msg); err != nil {
			return "", err
		}
		publisherMeta.Events[eventID] = eventMetadata
		data = eventMetadata

		r.log.Debugw("Got previously unknown message template.",
			"provider", publisherMeta.Metadata.Name,
			"event_id", eventID,
			"message", msg)
	}

	if data != nil {
		if data.MsgStatic != "" {
			return data.MsgStatic, nil
		} else if data.MsgTemplate != nil {
			return r.formatMessageFromTemplate(data.MsgTemplate, values)
		}
	}

	// Fallback to the traditional EvtFormatMessage mechanism.
	r.log.Debugf("Falling back to EvtFormatMessage for event ID %d.", event.EventIdentifier.ID)
	return getMessageString(publisherMeta.Metadata, eventHandle, 0, nil)
}

func (r *Renderer) formatMessageFromTemplate(msgTmpl *template.Template, values []interface{}) (string, error) {
	buf := bytes.NewBuffer(nil)
	err := msgTmpl.Execute(buf, values)
	if err != nil {
		return "", errors.Wrapf(err, "failed to execute template with data=%v template=%v", spew.Sdump(values), msgTmpl.Root.String())
	}
	return buf.String(), nil
}

// enrichRawValuesWithNames adds the names associated with the raw system
// property values. It enriches the event with keywords, opcode, level, and
// task. The search order is defined in the EvtFormatMessage documentation.
func enrichRawValuesWithNames(publisherMeta *publisherMetadataStore, event *sys.Event) {
	// Keywords. Each bit in the value can represent a keyword.
	rawKeyword := int64(event.KeywordsRaw)
	isClassic := keywordClassic&rawKeyword > 0
	for mask, keyword := range winMeta.Keywords {
		if rawKeyword&mask > 0 {
			event.Keywords = append(event.Keywords, keyword)
			rawKeyword -= mask
		}
	}
	for mask, keyword := range publisherMeta.Keywords {
		if rawKeyword&mask > 0 {
			event.Keywords = append(event.Keywords, keyword)
			rawKeyword -= mask
		}
	}

	// Opcode (search in winmeta first).
	var found bool
	if !isClassic {
		event.Opcode, found = winMeta.Opcodes[event.OpcodeRaw]
		if !found {
			event.Opcode = publisherMeta.Opcodes[event.OpcodeRaw]
		}
	}

	// Level (search in winmeta first).
	event.Level, found = winMeta.Levels[event.LevelRaw]
	if !found {
		event.Level, found = publisherMeta.Levels[event.LevelRaw]
	}

	// Task (fall-back to winmeta if not found).
	event.Task, found = publisherMeta.Tasks[event.TaskRaw]
	if !found {
		event.Task = winMeta.Tasks[event.TaskRaw]
	}
}
