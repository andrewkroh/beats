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

	conf "github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/logp"

	input "github.com/elastic/beats/v7/filebeat/input/v2"
	cursor "github.com/elastic/beats/v7/filebeat/input/v2/input-cursor"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/feature"
)

const inputName = "macos-unified-logging"

// Plugin creates a new macos-unified-logging input plugin for creating a
// stateful input.
func Plugin(log *logp.Logger, store cursor.StateStore) input.Plugin {
	return input.Plugin{
		Name:       inputName,
		Stability:  feature.Experimental,
		Deprecated: false,
		Info:       inputName + " input",
		Doc:        "The " + inputName + " input collects logs from macOS unified logging system.",
		Manager: &cursor.InputManager{
			Logger:     log,
			StateStore: store,
			Type:       inputName,
			Configure:  configureInput,
		},
	}
}

type cursorIDSource string

func (s cursorIDSource) Name() string { return string(s) }

func configureInput(cfg *conf.C) ([]cursor.Source, cursor.Input, error) {
	var config config
	if err := cfg.Unpack(&config); err != nil {
		return nil, nil, fmt.Errorf("failed configuring "+inputName+" input: %w", err)
	}

	sources := []cursor.Source{
		cursorIDSource(config.ID),
	}

	ul := &unifiedLogging{
		config: config,
	}

	return sources, ul, nil
}

type unifiedLogging struct {
	config       config
	initialState inputState
}

type inputState struct {
	Timestamp int64 `config:"timestamp"` // Unix epoch in nanoseconds.
}

func (s inputState) Time() time.Time {
	if s.Timestamp == 0 {
		return time.Time{}
	}
	return time.Unix(0, s.Timestamp)
}

var _ cursor.Input = (*unifiedLogging)(nil)

func (*unifiedLogging) Name() string {
	return inputName
}

func (u *unifiedLogging) Test(source cursor.Source, testCtx input.TestContext) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := newReader(ctx, testCtx.Logger, time.Time{}, u.config)
	if err != nil {
		return err
	}

	return nil
}

func (u *unifiedLogging) Run(inputCtx input.Context, source cursor.Source, c cursor.Cursor, publisher cursor.Publisher) error {
	log := inputCtx.Logger
	log.Info("Starting " + inputName + " input")
	defer log.Info(inputName + " input stopped")

	if !c.IsNew() {
		if err := c.Unpack(&u.initialState); err != nil {
			return fmt.Errorf("failed to unpack state for input with id=%v", inputCtx.ID)
		}
		log.Debugw("Found cursor state",
			"cursor_timestamp", common.Time(u.initialState.Time()))
	} else {
		log.Debug("No existing cursor state found.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel when cancellation is signaled.
	go func() {
		<-inputCtx.Cancelation.Done()
		cancel()
	}()

	r, err := newReader(ctx, log, u.initialState.Time(), u.config)
	if err != nil {
		return err
	}

	return r.Read(ctx, publisher)
}
