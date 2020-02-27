// +build windows

package wineventlog

import (
	"go.uber.org/multierr"
	"golang.org/x/sys/windows"
)

const maxEvtNextHandles = 1024

type EventIterator struct {
	subscription EvtHandle                    // Handle from EvtQuery or EvtSubscription.
	batchSize    uint32                       // Number of handles to request by default.
	handles      [maxEvtNextHandles]EvtHandle // Handles returned by EvtNext.
	read         int                          // Current read index of handles.
	length       int                          // Length of the valid handles.
	lastErr      error                        // Last error returned by EvtNext.
}

// NewEventIterator creates a new iterator for the given EvtQuery or EvtSubscription
// handle. The batchSize is the number of handles the iterator will request
// when it calls EvtNext. batchSize must be less than 1024 (a reasonable value
// is 512, too big and you can hit windows.RPC_S_INVALID_BOUND errors depending
// on the size of the events).
func NewEventIterator(subscription EvtHandle, batchSize uint32) EventIterator {
	if batchSize > maxEvtNextHandles {
		batchSize = 1024
	}

	return EventIterator{
		subscription: subscription,
		batchSize:    batchSize,
	}
}

// Next advances the iterator to the next handle. After Next returns false, the
// Err method will return any error that occurred during iteration, except that
// if it was windows.ERROR_NO_MORE_ITEMS, Err will return nil.
func (itr *EventIterator) Next() bool {
	if itr.read < itr.length {
		itr.read++
		return true
	}

	var numReturned uint32
	if err := _EvtNext(itr.subscription, itr.batchSize, &itr.handles[0], 0, 0, &numReturned); err != nil {
		if windows.ERROR_NO_MORE_ITEMS != err {
			itr.lastErr = err
		}
		return false
	}

	itr.read = 0
	itr.length = int(numReturned)
	return true
}

// Handle returns the most recent handle read by Next(). You must Close() the
// returned Handle().
func (itr *EventIterator) Handle() EvtHandle {
	if itr.read < itr.length {
		return itr.handles[itr.read]
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
	var errs []error
	for i := itr.read; i < itr.length; i++ {
		if err := itr.handles[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return multierr.Combine(errs...)
}
