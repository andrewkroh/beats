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

package eventlog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/joeshaw/multierror"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/winlogbeat/checkpoint"
	win "github.com/elastic/beats/winlogbeat/sys/wineventlog"
)

const (
	// winEventLogExpApiName is the name used to identify the Windows Event Log API
	// as both an event type and an API.
	winEventLogExpAPIName = "wineventlog-experimental"
)

var winEventLogExpConfigKeys = common.MakeStringSet(append(commonConfigKeys,
	"batch_read_size", "ignore_older", "include_xml", "event_id", "forwarded",
	"level", "provider", "no_more_events")...)

type winEventLogExpConfig struct {
	ConfigCommon  `config:",inline"`
	BatchReadSize int                `config:"batch_read_size"` // Maximum number of events that Read will return.
	IncludeXML    bool               `config:"include_xml"`
	Forwarded     *bool              `config:"forwarded"`
	SimpleQuery   query              `config:",inline"`
	NoMoreEvents  NoMoreEventsAction `config:"no_more_events"` // Action to take when no more events are available - wait or stop.
}

// defaultWinEventLogExpConfig is the default configuration for new wineventlog readers.
var defaultWinEventLogExpConfig = winEventLogExpConfig{
	BatchReadSize: 512,
}

// Validate validates the winEventLogExpConfig data and returns an error describing
// any problems or nil.
func (c *winEventLogExpConfig) Validate() error {
	var errs multierror.Errors
	if c.Name == "" {
		errs = append(errs, fmt.Errorf("event log is missing a 'name'"))
	}

	return errs.Err()
}

// Validate that winEventLogExp implements the EventLog interface.
var _ EventLog = &winEventLogExp{}

// winEventLogExp implements the EventLog interface for reading from the Windows
// Event Log API.
type winEventLogExp struct {
	config       winEventLogExpConfig
	query        string
	channelName  string                   // Name of the channel from which to read.
	file         bool                     // Reading from file rather than channel.
	subscription win.EvtHandle            // Handle to the subscription.
	maxRead      int                      // Maximum number returned in one Read.
	lastRead     checkpoint.EventLogState // Record number of the last read event.
	log          *logp.Logger

	renderer *win.Renderer
}

// Name returns the name of the event log (i.e. Application, Security, etc.).
func (l *winEventLogExp) Name() string {
	return l.channelName
}

func (l *winEventLogExp) Open(state checkpoint.EventLogState) error {
	var bookmark win.EvtHandle
	var err error
	if len(state.Bookmark) > 0 {
		bookmark, err = win.CreateBookmarkFromXML(state.Bookmark)
	} else if state.RecordNumber > 0 {
		bookmark, err = win.CreateBookmarkFromRecordID(l.channelName, state.RecordNumber)
	}
	if err != nil {
		return err
	}
	defer win.Close(bookmark)

	if l.file {
		return l.openFile(state, bookmark)
	}
	return l.openChannel(bookmark)
}

func (l *winEventLogExp) openChannel(bookmark win.EvtHandle) error {
	// Using a pull subscription to receive events. See:
	// https://msdn.microsoft.com/en-us/library/windows/desktop/aa385771(v=vs.85).aspx#pull
	signalEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(signalEvent)

	var flags win.EvtSubscribeFlag
	if bookmark > 0 {
		flags = win.EvtSubscribeStartAfterBookmark
	} else {
		flags = win.EvtSubscribeStartAtOldestRecord
	}

	l.log.Debugw("Using subscription query.", "winlog.query", l.query)
	subscriptionHandle, err := win.Subscribe(
		0, // Session - nil for localhost
		signalEvent,
		"",       // Channel - empty b/c channel is in the query
		l.query,  // Query - nil means all events
		bookmark, // Bookmark - for resuming from a specific event
		flags)
	if err != nil {
		return err
	}

	l.subscription = subscriptionHandle
	return nil
}

func (l *winEventLogExp) openFile(state checkpoint.EventLogState, bookmark win.EvtHandle) error {
	path := l.channelName

	h, err := win.EvtQuery(0, path, "", win.EvtQueryFilePath|win.EvtQueryForwardDirection)
	if err != nil {
		return errors.Wrapf(err, "failed to get handle to event log file %v", path)
	}

	if bookmark > 0 {
		l.log.Debugf("Seeking to bookmark. timestamp=%v bookmark=%v",
			state.Timestamp, state.Bookmark)

		// This seeks to the last read event and strictly validates that the
		// bookmarked record number exists.
		if err = win.EvtSeek(h, 0, bookmark, win.EvtSeekRelativeToBookmark|win.EvtSeekStrict); err == nil {
			// Then we advance past the last read event to avoid sending that
			// event again. This won't fail if we're at the end of the file.
			err = errors.Wrap(
				win.EvtSeek(h, 1, bookmark, win.EvtSeekRelativeToBookmark),
				"failed to seek past bookmarked position")
		} else {
			l.log.Warnf("s Failed to seek to bookmarked location in %v (error: %v). "+
				"Recovering by reading the log from the beginning. (Did the file "+
				"change since it was last read?)", path, err)
			err = errors.Wrap(
				win.EvtSeek(h, 0, 0, win.EvtSeekRelativeToFirst),
				"failed to seek to beginning of log")
		}

		if err != nil {
			return err
		}
	}

	l.subscription = h
	return nil
}

func (l *winEventLogExp) Read() ([]Record, error) {
	batchSize := l.maxRead
	for {
		records, err := l.read(batchSize)
		if windows.RPC_S_INVALID_BOUND == err && batchSize/2 > 0 {
			batchSize /= 2
			incrementMetric(readErrors, err)
			if err := l.Close(); err != nil {
				return nil, errors.Wrap(err, "failed to recover from RPC_S_INVALID_BOUND")
			}
			if err := l.Open(l.lastRead); err != nil {
				return nil, errors.Wrap(err, "failed to recover from RPC_S_INVALID_BOUND")
			}
			continue
		} else if err == nil && Stop == l.config.NoMoreEvents {
			err = io.EOF
		}
		l.log.Debugf("Read() is returning %d records.", len(records))
		return records, err
	}
}

func (l *winEventLogExp) read(size int) ([]Record, error) {
	itr := win.NewEventIterator(l.subscription, size)
	defer itr.Close()

	var records []Record
	for itr.Next() {
		r, err := l.processHandle(itr.Handle())
		if err != nil {
			l.log.Warnw("Dropping event due to rendering error.", "error", err)
			incrementMetric(dropReasons, err)
			continue
		}
		records = append(records, *r)
	}
	return records, itr.Err()
}

func (l *winEventLogExp) processHandle(h win.EvtHandle) (*Record, error) {
	defer h.Close()

	evt, err := l.renderer.Render(h)
	if err != nil {
		return nil, err
	}

	r := &Record{
		API:   winEventLogExpAPIName,
		Event: *evt,
	}

	if l.file {
		r.File = l.channelName
	}

	r.Offset = checkpoint.EventLogState{
		Name:         l.channelName,
		RecordNumber: r.RecordID,
		Timestamp:    r.TimeCreated.SystemTime,
	}
	if r.Offset.Bookmark, err = l.createBookmarkFromEvent(h); err != nil {
		l.log.Warnw("Failed creating bookmark.", "error", err)
	}
	l.lastRead = r.Offset
	return r, nil
}

func (l *winEventLogExp) createBookmarkFromEvent(evtHandle win.EvtHandle) (string, error) {
	bookmark, err := win.NewBookmark(evtHandle)
	if err != nil {
		return "", errors.Wrap(err, "failed to create new bookmark from event handle")
	}
	defer bookmark.Close()

	return bookmark.XML()
}

func (l *winEventLogExp) Close() error {
	l.log.Debug("Closing subscription handle.")
	return l.subscription.Close()
}

// newWinEventLogExp creates and returns a new EventLog for reading event logs
// using the Windows Event Log.
func newWinEventLogExp(options *common.Config) (EventLog, error) {
	c := defaultWinEventLogExpConfig
	if err := readConfig(options, &c, winEventLogExpConfigKeys); err != nil {
		return nil, err
	}

	queryLog := c.Name
	isFile := false
	if info, err := os.Stat(c.Name); err != nil && info.Mode().IsRegular() {
		path, err := filepath.Abs(c.Name)
		if err != nil {
			return nil, err
		}
		isFile = true
		queryLog = "file://" + path
	}

	query, err := win.Query{
		Log:         queryLog,
		IgnoreOlder: c.SimpleQuery.IgnoreOlder,
		Level:       c.SimpleQuery.Level,
		EventID:     c.SimpleQuery.EventID,
		Provider:    c.SimpleQuery.Provider,
	}.Build()
	if err != nil {
		return nil, err
	}

	renderer, err := win.NewRenderer()
	if err != nil {
		return nil, err
	}

	l := &winEventLogExp{
		config:      c,
		query:       query,
		channelName: c.Name,
		file:        isFile,
		maxRead:     c.BatchReadSize,
		renderer:    renderer,
		log:         logp.NewLogger("wineventlog").With("channel", c.Name),
	}

	return l, nil
}

func init() {
	// Register wineventlog API if it is available.
	available, _ := win.IsAvailable()
	if available {
		Register(winEventLogExpAPIName, 0, newWinEventLogExp, win.Channels)
	}
}
