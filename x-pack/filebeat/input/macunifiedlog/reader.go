// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

//go:build darwin && cgo
// +build darwin,cgo

package macunifiedlog

import (
	"context"
	"fmt"
	"time"

	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent-libs/mapstr"
	"github.com/elastic/go-unified-logging/oslog"

	cursor "github.com/elastic/beats/v7/filebeat/input/v2/input-cursor"
	"github.com/elastic/beats/v7/libbeat/beat"
)

type reader struct {
	entryChan <-chan oslog.Entry
	errChan   <-chan error
}

func newReader(ctx context.Context, log *logp.Logger, lastTimestamp time.Time, conf config) (*reader, error) {
	s, err := oslog.LocalStore()
	if err != nil {
		return nil, err
	}

	opts := []oslog.QueryOption{
		oslog.LiveTail(),
	}

	if !lastTimestamp.IsZero() {
		opts = append(opts, oslog.Since(lastTimestamp))
	} else if conf.IgnoreOlder > 0 {
		opts = append(opts, oslog.Since(time.Now().Add(-1*conf.IgnoreOlder)))
		log.Infof("Using ignore_older of %v.", conf.IgnoreOlder)
	}

	for _, pred := range conf.Predicates {
		opts = append(opts, oslog.WithPredicate(pred))
	}

	entryChan, errChan, err := s.QueryContext(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return &reader{
		entryChan: entryChan,
		errChan:   errChan,
	}, nil
}

func (r *reader) Read(ctx context.Context, p cursor.Publisher) error {
	// When reading a large range of events and using complex predicates
	// it can take the oslog enumerator a long time to stop. So this
	// loop is also watching the input's cancellation context to stop
	// immediately.
selectLoop:
	for {
		select {
		case entry, ok := <-r.entryChan:
			if !ok {
				break selectLoop
			}

			event := entryToEvent(entry)
			state := inputState{Timestamp: entry.Time().UnixNano()}

			if err := p.Publish(*event, state); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}

	// Check for an error:
	select {
	case err := <-r.errChan:
		return err
	default:
		return nil
	}
}

func entryToEvent(entry oslog.Entry) *beat.Event {
	var event *beat.Event

	switch v := entry.(type) {
	case *oslog.ActivityEntry:
		event = makeEvent(&v.BaseEntry, nil, nil)
		addProcess(event, &v.EntryFromProcess, v.ParentActivityIdentifier)
	case *oslog.BoundaryEntry:
		event = makeEvent(&v.BaseEntry, nil, nil)
	case *oslog.LogEntry:
		event = makeEvent(&v.BaseEntry, &v.EntryWithPayload, nil)
		addProcess(event, &v.EntryFromProcess, 0)
		event.Fields["log"] = mapstr.M{
			"level": v.Level.String(),
		}
	case *oslog.SignpostEntry:
		event = makeEvent(&v.BaseEntry, &v.EntryWithPayload, v)
	case *oslog.BaseEntry:
		event = makeEvent(v, nil, nil)
	default:
		panic(fmt.Errorf("unhandled log type: %T", v))
	}

	return event
}

func makeEvent(base *oslog.BaseEntry, payload *oslog.EntryWithPayload, signpost *oslog.SignpostEntry) *beat.Event {
	unifiedLogging := mapstr.M{
		"type": base.Type.String(),
	}

	if base.StoreCategory != 0 {
		unifiedLogging["store_category"] = base.StoreCategory.String()
	}

	if payload != nil {
		if payload.Category != "" {
			unifiedLogging["category"] = payload.Category
		}
		unifiedLogging["subsystem"] = payload.Subsystem
	}

	if signpost != nil && signpost.SignpostIdentifier > 0 && signpost.SignpostName != "" && signpost.SignpostType != 0 {
		m := mapstr.M{}
		if signpost.SignpostIdentifier > 0 {
			m["id"] = signpost.SignpostIdentifier
		}
		if signpost.SignpostName != "" {
			m["name"] = signpost.SignpostName
		}
		if signpost.SignpostType != 0 {
			m["type"] = signpost.SignpostType.String()
		}
		unifiedLogging["signpost"] = m
	}

	return &beat.Event{
		Timestamp: base.Timestamp,
		Fields: map[string]interface{}{
			"message":         base.Message,
			"unified_logging": unifiedLogging,
		},
	}
}

func addProcess(event *beat.Event, process *oslog.EntryFromProcess, ppid uint64) {
	p := mapstr.M{
		"name":       process.Process,
		"pid":        process.ProcessIdentifier,
		"executable": process.Sender,
		"thread": mapstr.M{
			"id": process.ThreadIdentifier,
		},
	}

	if ppid > 0 {
		p["parent"] = mapstr.M{
			"pid": ppid,
		}
	}

	event.Fields["process"] = p
}
