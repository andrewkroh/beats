// +build windows

package wineventlog

import (
	"github.com/pkg/errors"
	"go.uber.org/multierr"
	"golang.org/x/sys/windows"
)

const maxEvtNextHandles = 1024

type EventIterator struct {
	subscriptionFactory SubscriptionFactory          // Factory for producing a new subscription handle.
	subscription EvtHandle // Handle from EvtQuery or EvtSubscription.
	batchSize    uint32                       // Number of handles to request by default.
	handles      [maxEvtNextHandles]EvtHandle // Handles returned by EvtNext.
	read         int                          // Current read index of handles.
	length       int                          // Length of the valid handles.
	lastErr      error                        // Last error returned by EvtNext.
}

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

type SubscriptionFactory func() (EvtHandle, error)

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
		subscription: handle,
		batchSize: 512,
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
	if itr.read < itr.length {
		itr.read++
		return true
	}

	return itr.moreHandles()
}

func (itr *EventIterator) moreHandles() bool {
	itr.read, itr.length = 0, 0

	var numReturned, batchSize uint32 = 0, itr.batchSize
	for batchSize > 0 {
		err := _EvtNext(itr.subscription, batchSize, &itr.handles[0], 0, 0, &numReturned)
		switch err {
		case nil:
			itr.length = int(numReturned)
			return true
		case windows.RPC_S_INVALID_BOUND:
			batchSize /= 2
			itr.lastErr = err
			itr.subscription.Close()
			itr.subscription = NilHandle
			itr.subscription, err = itr.subscriptionFactory()
			if err != nil {
				itr.lastErr = errors.Wrap(err, "failed to recover from RPC_S_INVALID_BOUND error")
				return false
			}
			continue
		case windows.ERROR_NO_MORE_ITEMS:
			return false
		default:
			itr.lastErr = err
			return false
		}
	}
	return false
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
	if err := itr.subscription.Close(); err != nil {
		itr.subscription = NilHandle
		errs = append(errs, err)
	}
	for i := itr.read; i < itr.length; i++ {
		if err := itr.handles[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return multierr.Combine(errs...)
}
