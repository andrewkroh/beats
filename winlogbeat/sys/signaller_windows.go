package sys

import (
	"errors"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
"fmt"
	"time"
	"runtime"
)

const maximumWaitObjects = 64

// Add -trace to enable debug prints around syscalls.
//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsignaller_windows.go signaller_windows.go

// Windows API calls
//sys   _ResetEvent(event syscall.Handle) (err error) = kernel32.ResetEvent
//sys   _WaitForMultipleObjects(count uint32, handles *syscall.Handle, waitAll uint32, milliseconds uint32) (index uint32, err error) = kernel32.WaitForMultipleObjects

// event represents auto-reset, initially non-signaled Windows event.
// It is used to communicate between go and asm parts of this package.
type Signaller struct {
	event  windows.Handle
	signal chan struct{}
}

func (e *Signaller) Handle() windows.Handle {
	return e.event
}

func (e *Signaller) Channel() <-chan struct{} {
	return e.signal
}

type watcher struct {
	signals     []chan struct{}
	waitHandles []windows.Handle
	useCount    int
	guard       sync.Mutex

	wg          sync.WaitGroup
	once        sync.Once
	err         chan error
}

func newWatcher() (*watcher, error) {
	var err error
	signals := make([]chan struct{}, 0, maximumWaitObjects)
	waitHandles := make([]windows.Handle, 0, maximumWaitObjects)
	for i := 0; i < maximumWaitObjects; i++ {
		signals[0] = make(chan error, 1)
		waitHandles[i], err = windows.CreateEvent(nil, 1, 1, nil)
		if err != nil {
			return nil, err
		}
	}

	w := &watcher{
		signals: signals,
		waitHandles: waitHandles,
		useCount: 1,
		err: make(chan error, 1),
	}

	w.wg.Add(1)
	go w.watchWaitHandles()
	return w, nil
}

func (w *watcher) stop() {
	windows.SetEvent(w.waitHandles[0])
	w.wg.Wait()
}

func (w *watcher) watchWaitHandles() {
	defer w.wg.Done()
	runtime.LockOSThread()

	nCount := uint32(len(w.waitHandles))
	for {
		wait, err := _WaitForMultipleObjects(nCount, &w.waitHandles[0], 0, windows.INFINITE)
		switch {
		case wait >= windows.WAIT_OBJECT_0 && wait <= windows.WAIT_OBJECT_0 + nCount - 1:
			i := wait - windows.WAIT_OBJECT_0
			if i == 0 {
				return
			}
			w.signals[i] <- struct{}{}
		case wait >= windows.WAIT_ABANDONED && wait <= windows.WAIT_ABANDONED + nCount - 1:
			i := wait - windows.WAIT_ABANDONED
			w.err <- fmt.Errorf("WaitForMultipleObjects abandoned wait handle index=%d", i)
		case wait == windows.WAIT_FAILED:
			w.err <- fmt.Errorf("WaitForMultipleObjects error: %v", err)
		default:
			w.err <- fmt.Errorf("WaitForMultipleObjects unexpected return value %d", wait)
		}
	}
}

func (w *watcher) NewSignaller() (*Signaller, error) {
	w.guard.Lock()
	defer w.guard.Unlock()

	if w.useCount >= len(w.waitHandles) {
		return nil, errors.New("no signallers available")
	}

	s := &Signaller{
		event: w.waitHandles[w.useCount],
		signal: w.signals[w.useCount],
	}
	w.useCount++
	return s, nil
}
