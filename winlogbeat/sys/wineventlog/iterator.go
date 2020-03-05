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
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

const maxEvtNextHandles = 1024

type EventIterator struct {
	subscriptionFactory SubscriptionFactory          // Factory for producing a new subscription handle.
	subscription        EvtHandle                    // Handle from EvtQuery or EvtSubscription.
	batchSize           uint32                       // Number of handles to request by default.
	handles             [maxEvtNextHandles]EvtHandle // Handles returned by EvtNext.
	lastErr             error                        // Last error returned by EvtNext.
	active              []EvtHandle
}

type SubscriptionFactory func() (EvtHandle, error)

type EventIteratorOption func(*EventIterator)

func WithBatchSize(size int) EventIteratorOption {
	return func(itr *EventIterator) {
		if size > 0 {
			itr.batchSize = uint32(size)
		}
		if size > maxEvtNextHandles {
			itr.batchSize = 1024
		}
	}
}

// NewEventIterator creates a new iterator for the given EvtQuery or EvtSubscription
// handle. The batchSize is the number of handles the iterator will request
// when it calls EvtNext. batchSize must be less than 1024 (a reasonable value
// is 512, too big and you can hit windows.RPC_S_INVALID_BOUND errors depending
// on the size of the events).
func NewEventIterator(subscription SubscriptionFactory, opts ...EventIteratorOption) (*EventIterator, error) {
	handle, err := subscription()
	if err != nil {
		return nil, err
	}
	itr := &EventIterator{
		subscriptionFactory: subscription,
		subscription:        handle,
		batchSize:           512,
	}
	for _, opt := range opts {
		opt(itr)
	}

	return itr, nil
}

// Next advances the iterator to the next handle. After Next returns false, the
// Err method will return any error that occurred during iteration, except that
// if it was windows.ERROR_NO_MORE_ITEMS, Err will return nil.
func (itr *EventIterator) Next() bool {
	if itr.empty() {
		return itr.moreHandles()
	}

	itr.active = itr.active[1:]
	return !itr.empty()
}

func (itr *EventIterator) empty() bool {
	return len(itr.active) == 0
}

func (itr *EventIterator) moreHandles() bool {
	batchSize := itr.batchSize
	for batchSize > 0 {
		var numReturned uint32
		err := _EvtNext(itr.subscription, batchSize, &itr.handles[0], 0, 0, &numReturned)
		switch err {
		case windows.RPC_S_INVALID_BOUND:
			itr.lastErr = err
			itr.subscription.Close()
			itr.subscription, err = itr.subscriptionFactory()
			if err != nil {
				itr.lastErr = errors.Wrap(err, "failed to recover from RPC_S_INVALID_BOUND error")
				return false
			}

			// Reduce batch size and try again.
			batchSize /= 2
			continue
		case windows.ERROR_NO_MORE_ITEMS, windows.ERROR_INVALID_OPERATION:
		case nil:
			itr.lastErr = nil
			itr.active = itr.handles[:numReturned]
		default:
			itr.lastErr = err
		}
		break
	}
	return !itr.empty()
}

// Handle returns the most recent handle read by Next(). You must Close() the
// returned Handle().
func (itr *EventIterator) Handle() EvtHandle {
	if !itr.empty() {
		return itr.active[0]
	}
	return NilHandle
}

// Err returns the first non-ERROR_NO_MORE_ITEMS error encountered by the
// EventIterator.
//
// If windows.RPC_S_INVALID_BOUND is returned you should create a new
// EventIterator with a lower batchSize. After this error you must close the
// subscription handle used by EventIterator and re-open a subscription from the
// last read position.
func (itr *EventIterator) Err() error {
	return itr.lastErr
}

// Close closes any handles that were not iterated.
func (itr *EventIterator) Close() error {
	for _, h := range itr.active {
		h.Close()
	}
	return itr.subscription.Close()
}
