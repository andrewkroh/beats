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
	"strconv"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"go.uber.org/multierr"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/winlogbeat/sys"
)

var (
	eventDataNameTransform = strings.NewReplacer(" ", "_")

	eventMessageTemplateFuncs = template.FuncMap{
		"eventParam": eventParam,
	}
)

type publisherMetadataStore struct {
	Metadata *PublisherMetadata
	Keywords map[int64]string
	Opcodes  map[uint8]string
	Levels   map[uint8]string
	Tasks    map[uint16]string
	Events   map[uint16]*eventMetadata

	log *logp.Logger
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

func newEmptyPublisherMetadataStore(provider string, log *logp.Logger) *publisherMetadataStore {
	return &publisherMetadataStore{
		Keywords: map[int64]string{},
		Opcodes:  map[uint8]string{},
		Levels:   map[uint8]string{},
		Tasks:    map[uint16]string{},
		Events:   map[uint16]*eventMetadata{},
		log:      log.With("publisher", provider, "empty", true),
	}
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

	s.Events = map[uint16]*eventMetadata{}
	for itr.Next() {
		evt, err := newEventMetadataFromPublisherMetadata(itr, s.Metadata)
		if err != nil {
			s.log.Warn("Failed to read metadata from publisher for event.",
				"event.code", evt.EventID,
				"error", err)
			continue
		}
		s.Events[evt.EventID] = evt
	}
	return itr.Err()
}

func (s *publisherMetadataStore) getEventMetadata(eventID uint16) *eventMetadata {
	return s.Events[eventID]
}

func (s *publisherMetadataStore) addEventMetadata(eventHandle EvtHandle) *eventMetadata {
	em, err := newEventMetadataFromEventHandle(s.Metadata, eventHandle)
	if err != nil {
		return nil
	}
	s.Events[em.EventID] = em
	return em
}

// XXX: The publisherMetadataStore might need to offer a way to store multiple
// eventMetadata values for an event ID. Logs might contain multiple "versions"
// for a single event ID if software is updated, logs are collected via WEC, or
// logs are read from an .evtx file.
//
// But the version field is not the way to identify different "versions". Some
// kind of signature based on the message template, number of values, types of
// values, raw keyword/level/opcode/task could be used.

type eventMetadata struct {
	EventID     uint16
	Version     uint8              // Event format version.
	MsgStatic   string             // Used when the message has no parameters.
	MsgTemplate *template.Template // Template that expects an array of values as its data.
	EventData   []EventData        // Names of parameters from XML template.
}

// newEventMetadataFromEventHandle collects metadata about an event type using
// the handle of an event.
func newEventMetadataFromEventHandle(publisher *PublisherMetadata, eventHandle EvtHandle) (*eventMetadata, error) {
	xml, err := getEventXML(publisher, eventHandle)
	if err != nil {
		return nil, err
	}

	// By parsing the XML we can get the names of the parameters even if the
	// publisher metadata is unavailable or is out of sync with the events.
	event, err := sys.UnmarshalEventXML([]byte(xml))
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal XML")
	}

	em := &eventMetadata{
		EventID: uint16(event.EventIdentifier.ID),
		Version: uint8(event.Version),
	}
	for _, pair := range event.EventData.Pairs {
		em.EventData = append(em.EventData, EventData{Name: pair.Key})
	}

	// The message template is only available from the publisher metadata. This
	// message template may not match up with the event data we got from the
	// event's XML, but it's the only option available. Even forwarded events
	// with "RenderedText" won't help because their messages are already
	// rendered.
	if publisher != nil {
		msg, err := getMessageStringFromHandle(publisher, eventHandle, insertStrings.EvtVariants[:])
		if err != nil {
			return nil, err
		}
		em.setMessage(msg)
	}

	return em, nil
}

// newEventMetadataFromPublisherMetadata collects metadata about an event type
// using the publisher metadata.
func newEventMetadataFromPublisherMetadata(itr *EventMetadataIterator, publisher *PublisherMetadata) (*eventMetadata, error) {
	em := &eventMetadata{}
	err := multierr.Combine(
		em.initEventID(itr),
		em.initVersion(itr),
		em.initEventDataTemplate(itr),
		em.initEventMessage(itr, publisher),
	)
	if err != nil {
		return nil, err
	}
	return em, nil
}

func (em *eventMetadata) initEventID(itr *EventMetadataIterator) error {
	id, err := itr.EventID()
	if err != nil {
		return err
	}
	// The upper 16 bits are the qualifier and lower 16 are the ID.
	em.EventID = uint16(0xFFFF & id)
	return nil
}

func (em *eventMetadata) initVersion(itr *EventMetadataIterator) error {
	version, err := itr.Version()
	if err != nil {
		return err
	}
	em.Version = uint8(version)
	return nil
}

func (em *eventMetadata) initEventDataTemplate(itr *EventMetadataIterator) error {
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

	em.EventData = tmpl.Data
	return nil
}

func (em *eventMetadata) initEventMessage(itr *EventMetadataIterator, publisher *PublisherMetadata) error {
	messageID, err := itr.MessageID()
	if err != nil {
		return err
	}

	msg, err := getMessageString(publisher, NilHandle, messageID, insertStrings.EvtVariants[:])
	if err != nil {
		return err
	}

	return em.setMessage(msg)
}

func (em *eventMetadata) setMessage(msg string) error {
	msg = sys.RemoveWindowsLineEndings(msg)
	tmplID := strconv.Itoa(int(em.EventID))
	tmpl, err := template.New(tmplID).Funcs(eventMessageTemplateFuncs).Parse(msg)
	if err != nil {
		return err
	}

	// One node means there were no parameters so this will optimize that case
	// by using a static string rather than a text/template.
	if len(tmpl.Root.Nodes) == 1 {
		em.MsgStatic = msg
	} else {
		em.MsgTemplate = tmpl
	}
	return nil
}

// --- Template Funcs

// eventParam return an event data value inside a text/template.
func eventParam(items []interface{}, paramNumber int) (interface{}, error) {
	// Windows parameter values start at %1 so adjust index value by -1.
	index := paramNumber - 1
	if index < len(items) {
		return items[index], nil
	}
	// Windows Event Viewer leaves the original placeholder (e.g. %22) in the
	// rendered message when no value provided.
	return "%" + strconv.Itoa(paramNumber), nil
}
