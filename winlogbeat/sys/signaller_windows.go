package sys

import (
	"errors"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
"fmt"
)

// Add -trace to enable debug prints around syscalls.
//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsignaller_windows.go signaller_windows.go

// Windows API calls
//sys   _ResetEvent(event syscall.Handle) (err error) = kernel32.ResetEvent

// event represents auto-reset, initially non-signaled Windows event.
// It is used to communicate between go and asm parts of this package.
type Signaller struct {
	event  windows.Handle
	signal chan struct{}

	stopOnce sync.Once
	done     chan struct{}
}

func NewSignaller() (*Signaller, error) {
	var manualReset uint32 = 1
	var initialState uint32 = 1
	h, err := windows.CreateEvent(nil, manualReset, initialState, nil)
	if err != nil {
		return nil, err
	}

	s := &Signaller{
		event:  h,
		signal: make(chan struct{}),
		done:   make(chan struct{}, 1),
	}

	go func() {
		defer close(s.signal)
		for {
			fmt.Println("waiting")
			err := s.wait()
			if err != nil {
				fmt.Println("wait err", err)
				return
			}

			fmt.Println("Wait signalled")
			select {
			case <-s.done:
				return
			case s.signal <- struct{}{}:
				_ResetEvent(syscall.Handle(s.event))
			}
		}
	}()

	return s, nil
}

func (e *Signaller) Handle() windows.Handle {
	return e.event
}

func (e *Signaller) Close() error {
	e.stopOnce.Do(func() {
		close(e.done)
	})
	return windows.CloseHandle(e.event)
}

func (e *Signaller) Channel() <-chan struct{} {
	return e.signal
}

func (e *Signaller) wait() error {
	s, err := windows.WaitForSingleObject(e.event, windows.INFINITE)
	switch s {
	case windows.WAIT_OBJECT_0:
		break
	case windows.WAIT_FAILED:
		return err
	default:
		return errors.New("unexpected result from WaitForSingleObject")
	}
	return nil
}
