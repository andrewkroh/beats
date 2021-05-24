package awss3

import (
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

func TestSemaphore(t *testing.T) {
	s := newSem(5)

	n := s.Acquire(5)
	assert.Equal(t, n, 5)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		n := s.Acquire(2)
		assert.Equal(t, n, 1)
	}()

	s.Release(1)
	wg.Wait()
}
