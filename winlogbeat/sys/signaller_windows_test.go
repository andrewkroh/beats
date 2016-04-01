package sys

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
	"time"
"fmt"
)

func TestSignaller(t *testing.T) {
	s, err := NewSignaller()
	if err != nil {
		t.Fatal(err)
	}
	defer assert.NoError(t, s.Close(), "error on signaller.Close()")

	select {
	default:
	case <-s.Channel():
		assert.Fail(t, "at init, not expecting the event to be signalled")
	}

	fmt.Println("Handle", s.event)
	err = windows.SetEvent(s.event)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-s.Channel():
	case <-time.Tick(100 * time.Millisecond):
		assert.Fail(t, "timeout waiting for event to be signalled")
	}
}
