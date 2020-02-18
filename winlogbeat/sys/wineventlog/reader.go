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
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unsafe"

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

var (
	eventDataNameTransform = strings.NewReplacer(" ", "_")

	// TODO: This does not handle escape sequences.
	// https://docs.microsoft.com/en-us/windows/win32/eventlog/message-text-files
	messageParamRegex = regexp.MustCompile(`%(\d+)`)
)

type publisherMetadataStore struct {
	Metadata *PublisherMetadata
	Keywords map[int64]string
	Opcodes  map[uint8]string
	Levels   map[uint8]string
	Tasks    map[uint16]string
	Events   map[uint32]*eventMetadataStore

	log *logp.Logger
}

type eventMetadataStore struct {
	EventID     uint32
	MsgStatic   string             // Used when the message has no parameters.
	MsgTemplate *template.Template // Template that expects an array of values as its data.
	EventData   []EventData        // Names of parameters from XML template.
}

func newPublisherMetadataStore(session EvtHandle, provider string, log *logp.Logger) (*publisherMetadataStore, error) {
	md, err := NewPublisherMetadata(session, provider)
	if err != nil {
		return nil, err
	}
	store := &publisherMetadataStore{Metadata: md, log: log.With("publisher", provider)}

	// Query the provider metadata to build an in-memory cache of the
	// information to optimize event reading.
	err = multierr.Combine(
		store.initKeywords(),
		store.initOpcodes(),
		store.initLevels(),
		store.initTasks(),
		store.initEvents(),
	)
	if err != nil {
		return nil, err
	}

	return store, nil
}

func (s *publisherMetadataStore) initKeywords() error {
	keywords, err := s.Metadata.Keywords()
	if err != nil {
		return err
	}

	s.Keywords = make(map[int64]string, len(keywords))
	for _, keywordMeta := range keywords {
		// TODO: Choose between Name and Message. Preference?
		s.Keywords[int64(keywordMeta.Mask)] = keywordMeta.Name
	}
	return nil
}

func (s *publisherMetadataStore) initOpcodes() error {
	opcodes, err := s.Metadata.Opcodes()
	if err != nil {
		return err
	}
	s.Opcodes = make(map[uint8]string, len(opcodes))
	for _, opcodeMeta := range opcodes {
		s.Opcodes[uint8(opcodeMeta.Mask)] = opcodeMeta.Message
	}
	return nil
}

func (s *publisherMetadataStore) initLevels() error {
	levels, err := s.Metadata.Levels()
	if err != nil {
		return err
	}

	s.Levels = make(map[uint8]string, len(levels))
	for _, levelMeta := range levels {
		s.Levels[uint8(levelMeta.Mask)] = levelMeta.Name
	}
	return nil
}

func (s *publisherMetadataStore) initTasks() error {
	tasks, err := s.Metadata.Tasks()
	if err != nil {
		return err
	}
	s.Tasks = make(map[uint16]string, len(tasks))
	for _, taskMeta := range tasks {
		s.Tasks[uint16(taskMeta.Mask)] = taskMeta.Message
	}
	return nil
}

func (s *publisherMetadataStore) initEvents() error {
	itr, err := s.Metadata.EventMetadataIterator()
	if err != nil {
		return err
	}
	defer itr.Close()

	s.Events = map[uint32]*eventMetadataStore{}
	for itr.Next() {
		var evt eventMetadataStore
		err = multierr.Combine(
			s.readEventID(itr, &evt),
			s.readEventDataTemplate(itr, &evt),
			s.readEventMessage(itr, &evt),
		)
		// TODO: Should we accumulate errors and return them?
		if err != nil {
			s.log.Warn("Failed to read data for event.",
				"error", err, "event.code", evt.EventID)
			continue
		}
		s.Events[evt.EventID] = &evt
	}
	return itr.Err()
}

func (s *publisherMetadataStore) readEventID(itr *EventMetadataIterator, evt *eventMetadataStore) error {
	var err error
	evt.EventID, err = itr.EventID()
	return err
}

func (*publisherMetadataStore) readEventDataTemplate(itr *EventMetadataIterator, evt *eventMetadataStore) error {
	xml, err := itr.Template()
	if err != nil {
		return err
	}
	// Some events do not have templates.
	if xml == "" {
		return nil
	}

	tmpl := &EventTemplate{}
	if err = tmpl.Unmarshal([]byte(xml)); err != nil {
		return err
	}

	for _, kv := range tmpl.Data {
		kv.Name = eventDataNameTransform.Replace(kv.Name)
	}

	evt.EventData = tmpl.Data
	return nil
}

func eventParam(items []interface{}, paramNumber int) (interface{}, error) {
	index := paramNumber - 1
	if index < len(items) {
		return items[index], nil
	}
	// Windows Event Viewer leaves the original placeholder (e.g. %22) in the
	// rendered message when no value provided.
	return "%" + strconv.Itoa(paramNumber), nil
}

var eventMessageTemplateFuncs = template.FuncMap{
	"eventParam": eventParam,
}

func (s *publisherMetadataStore) readEventMessage(itr *EventMetadataIterator, evt *eventMetadataStore) error {
	msg, err := itr.Message()
	if err != nil {
		return err
	}

	// Replace all [^%]%n values with parameters.
	replaced := messageParamRegex.ReplaceAllString(msg, `{{ eventParam $ $1 }}`)

	if replaced == msg {
		evt.MsgStatic = sys.RemoveWindowsLineEndings(msg)
		return nil
	}

	replaced = sys.RemoveWindowsLineEndings(replaced)
	id := s.Metadata.Name + "/" + strconv.Itoa(int(evt.EventID))
	evt.MsgTemplate, err = template.New(id).Funcs(eventMessageTemplateFuncs).Parse(replaced)
	return err
}

func (evt *eventMetadataStore) addMessage(provider, msg string) error {
	id := provider + "/" + strconv.Itoa(int(evt.EventID))
	msg = sys.RemoveWindowsLineEndings(msg)
	tmpl, err := template.New(id).Funcs(eventMessageTemplateFuncs).Parse(msg)
	if err != nil {
		return err
	}

	// One node means there were no parameters optimize by having a static string.
	if len(tmpl.Root.Nodes) == 1 {
		evt.MsgStatic = msg
	} else {
		evt.MsgTemplate = tmpl
	}
	return nil
}

type Renderer struct {
	metadataCache map[string]*publisherMetadataStore
	systemContext EvtHandle // Render context for system values.
	userContext   EvtHandle // Render context for user values (event data).
	log           *logp.Logger
	buf           []byte
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
		_EvtClose(r.systemContext),
		_EvtClose(r.userContext),
	)
}

func (r *Renderer) Render(handle EvtHandle) (*sys.Event, error) {
	var event sys.Event

	if err := r.renderSystem(handle, &event); err != nil {
		return nil, errors.Wrap(err, "failed to render system properties")
	}

	// TODO: We might not be able to or want to read the local publisher
	// metadata for forwarded events.
	md, err := r.getPublisherMetadata(event.Provider.Name)
	if err != nil {
		return nil, err
	}

	// Associate numeric system properties to names (e.g. level 2 to Error).
	r.enrichRawValuesWithNames(md, &event)

	eventData, err := r.renderUser(handle, &event)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to render event data")
	}

	if err = r.addEventData(md, eventData, &event); err != nil {
		return nil, err
	}

	msg, err := r.formatMessage(md, handle, &event, eventData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to render message")
	}
	event.Message = msg

	return &event, nil
}

func (r *Renderer) getPublisherMetadata(publisher string) (*publisherMetadataStore, error) {
	md, found := r.metadataCache[publisher]
	if !found {
		var err error
		md, err = newPublisherMetadataStore(NilHandle, publisher, r.log)
		if err != nil {
			return nil, err
		}
		r.metadataCache[publisher] = md
	}

	return md, nil
}

func (r *Renderer) renderSystem(handle EvtHandle, event *sys.Event) error {
	buf, propertyCount, err := r.render(r.systemContext, handle)
	if err != nil {
		return errors.Wrap(err, "failed to get system values")
	}

	for i := 0; i < int(propertyCount); i++ {
		property := EvtSystemPropertyID(i)
		offset := i * int(sizeofEvtVariant)
		evtVar := (*EvtVariant)(unsafe.Pointer(&buf[offset]))

		data, err := evtVar.Data(buf)
		r.log.Debugf("name=%v, type=%v, is_array=%v, data=%v, data_error=%v",
			property, evtVar.Type, evtVar.Type.IsArray(), data, err)
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
			event.KeywordsRaw = data.(int64)
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
	return nil
}

func (r *Renderer) renderUser(handle EvtHandle, event *sys.Event) ([]interface{}, error) {
	buf, propertyCount, err := r.render(r.userContext, handle)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user values")
	}
	if propertyCount == 0 {
		return nil, nil
	}

	parameters := make([]interface{}, propertyCount)
	for i := 0; i < propertyCount; i++ {
		offset := i * int(sizeofEvtVariant)
		evtVar := (*EvtVariant)(unsafe.Pointer(&buf[offset]))

		parameters[i], err = evtVar.Data(buf)
		if err != nil {
			r.log.Warnw("Failed to read event parameter.",
				"provider", event.Provider.Name,
				"event_id", event.EventIdentifier.ID,
				"parameter_index", i,
				"parameter_type", evtVar.Type.String(),
			)
		}
	}
	return parameters, nil
}

func (r *Renderer) render(context EvtHandle, eventHandle EvtHandle) ([]byte, int, error) {
	var bufferUsed, propertyCount uint32
	err := _EvtRender(context, eventHandle, EvtRenderEventValues, 0, nil, &bufferUsed, &propertyCount)
	if err != nil && err != ERROR_INSUFFICIENT_BUFFER {
		return nil, 0, errors.Errorf("expected ERROR_INSUFFICIENT_BUFFER but got %v", err)
	}
	if propertyCount == 0 {
		return nil, 0, nil
	}

	buf := r.getBuf(bufferUsed)
	err = _EvtRender(context, eventHandle, EvtRenderEventValues, bufferUsed, &buf[0], &bufferUsed, &propertyCount)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to get values")
	}

	return buf, int(propertyCount), nil
}

func (r *Renderer) getBuf(length uint32) []byte {
	if cap(r.buf) < int(length) {
		r.buf = make([]byte, length)
	}
	r.buf = r.buf[0:length]
	return r.buf
}

func (r *Renderer) addEventData(publisherMeta *publisherMetadataStore, values []interface{}, event *sys.Event) error {
	if len(values) == 0 {
		return nil
	}

	eventID := event.EventIdentifier.ID
	eventMetadata := publisherMeta.Events[eventID]

	if eventMetadata == nil {
		r.log.Warnw("Event metadata not found.", "event_id", eventID)
	} else if len(values) != len(eventMetadata.EventData) {
		r.log.Warnw("The number of event data parameters doesn't match the number "+
			"of parameters in the template.",
			"event_id", eventID,
			"event_parameter_count", len(values),
			"template_parameter_count", len(eventMetadata.EventData),
			"event_version", event.Version)
	}

	// Fallback to paramN naming when the value does not exist in event data.
	// This can happen for legacy providers without manifests. This can also
	// happen if the installed provider manifest doesn't match the version that
	// produced the event (forwarded events, reading from evtx, or software was
	// updated). If software was updated it could also be that this cached
	// template is no longer valid.
	paramName := func(idx int) string {
		if eventMetadata != nil && idx < len(eventMetadata.EventData) {
			return eventMetadata.EventData[idx].Name
		}
		return "param" + strconv.Itoa(idx)
	}

	for i, v := range values {
		strVal, ok := v.(string)
		if !ok {
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
	data := publisherMeta.Events[event.EventIdentifier.ID]

	if data == nil {
		// Try to get the raw message string using the event handle.
		msg, err := getMessageStringFromHandle(publisherMeta.Metadata, eventHandle)
		if err != nil {
			return "", err
		}
		eventMetadata := &eventMetadataStore{
			EventID: event.EventIdentifier.ID,
		}
		if err = eventMetadata.addMessage(event.Provider.Name, msg); err != nil {
			return "", err
		}
		publisherMeta.Events[event.EventIdentifier.ID] = eventMetadata
		data = eventMetadata

		// TODO: Parse msg into a template
		r.log.Debugw("Got previously unknown message template.", "message", msg)
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
	return r.evtFormatMessage(publisherMeta.Metadata, eventHandle, EvtFormatMessageEvent)
}

func (r *Renderer) formatMessageFromTemplate(msgTmpl *template.Template, values []interface{}) (string, error) {
	buf := bytes.NewBuffer(nil)
	err := msgTmpl.Execute(buf, values)
	if err != nil {
		return "", errors.Wrapf(err, "failed to execute template with data=%v template=%v", spew.Sdump(values), msgTmpl.Root.String())
	}
	return buf.String(), nil
}

func (r *Renderer) evtFormatMessage(metadata *PublisherMetadata, eventHandle EvtHandle, messageFlag EvtFormatMessageFlag) (string, error) {
	var bufferUsed uint32
	err := _EvtFormatMessage(metadata.Handle, eventHandle, 0, 0, 0, messageFlag, 0, nil, &bufferUsed)
	if err != ERROR_INSUFFICIENT_BUFFER {
		return "", errors.Errorf("expected ERROR_INSUFFICIENT_BUFFER but got: %v", err)
	}

	buf := r.getBuf(bufferUsed * 2)
	err = _EvtFormatMessage(metadata.Handle, eventHandle, 0, 0, 0, messageFlag, uint32(len(buf)/2), &buf[0], &bufferUsed)
	if err != nil {
		return "", errors.Wrapf(err, "failed in EvtFormatMessage")
	}

	s, _, err := sys.UTF16BytesToString(buf)
	return s, err
}

// enrichRawValuesWithNames adds the names associated with the raw system
// property values. It enriches the event with keywords, opcode, level, and
// task. The search order is defined in the EvtFormatMessage documentation.
func (r *Renderer) enrichRawValuesWithNames(publisherMeta *publisherMetadataStore, event *sys.Event) {
	// Keywords. Each bit in the value can represent a keyword.
	for mask, keyword := range winMeta.Keywords {
		if event.KeywordsRaw&mask > 0 {
			event.Keywords = append(event.Keywords, keyword)
		}
	}
	for mask, keyword := range publisherMeta.Keywords {
		if event.KeywordsRaw&mask > 0 {
			event.Keywords = append(event.Keywords, keyword)
		}
	}
	isClassic := keywordClassic&event.KeywordsRaw > 0

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

// winMeta contains the values are a common across Windows. These values are
// from winmeta.xml inside the Windows SDK.
var winMeta = &publisherMetadataStore{
	Keywords: map[int64]string{
		0:                "AnyKeyword",
		0x1000000000000:  "Response Time",
		0x4000000000000:  "WDI Diag",
		0x8000000000000:  "SQM",
		0x10000000000000: "Audit Failure",
		0x20000000000000: "Audit Success",
		0x40000000000000: "Correlation Hint",
		0x80000000000000: "Classic",
	},
	Opcodes: map[uint8]string{
		0: "Info",
		1: "Start",
		2: "Stop",
		3: "DCStart",
		4: "DCStop",
		5: "Extension",
		6: "Reply",
		7: "Resume",
		8: "Suspend",
		9: "Send",
	},
	Levels: map[uint8]string{
		0: "Information", // "Log Always", but Event Viewer shows Information.
		1: "Critical",
		2: "Error",
		3: "Warning",
		4: "Information",
		5: "Verbose",
	},
	Tasks: map[uint16]string{
		0: "None",
	},
}

func getMessageStringFromHandle(metadata *PublisherMetadata, eventHandle EvtHandle) (string, error) {
	const flags = EvtFormatMessageEvent
	var valuesCount = insertStrings.ValuesCount
	var values = insertStrings.ValuesPtr

	var bufferUsed uint32
	err := _EvtFormatMessage(metadata.Handle, eventHandle, 0, valuesCount, values, flags, 0, nil, &bufferUsed)
	if err != ERROR_INSUFFICIENT_BUFFER {
		return "", errors.Errorf("expected ERROR_INSUFFICIENT_BUFFER but got: %v", err)
	}

	buf := make([]byte, bufferUsed*2)
	err = _EvtFormatMessage(metadata.Handle, eventHandle, 0, valuesCount, values, flags, uint32(len(buf)/2), &buf[0], &bufferUsed)
	if err != nil {
		switch err {
		case windows.ERROR_EVT_UNRESOLVED_VALUE_INSERT:
		case windows.ERROR_EVT_UNRESOLVED_PARAMETER_INSERT:
		case windows.ERROR_EVT_MAX_INSERTS_REACHED:
		default:
			return "", err
		}
	}
	s, _, err := sys.UTF16BytesToString(buf)
	return s, err
}
