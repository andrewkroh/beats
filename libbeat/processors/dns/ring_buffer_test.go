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

package dns

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRingBuffer(t *testing.T) {
	rb := NewRingBuffer(1, 3)

	var err error
	var itemIndex int
	if itemIndex, err = rb.Push([]byte{1}); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 0, itemIndex)
	t.Logf("push: %+v", rb)

	buf := make([]byte, 1)
	if err = rb.Peek(buf); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, []byte{1}, buf)
	t.Logf("peek: %+v", rb)

	if err = rb.Pop(buf); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, []byte{1}, buf)
	t.Logf("pop: %+v", rb)

	if itemIndex, err = rb.Push([]byte{2}); err != nil {
		t.Fatal(err)
	}
	t.Logf("push: %+v", rb)
	assert.Equal(t, 1, itemIndex)

	if _, err = rb.Push([]byte{3}); err != nil {
		t.Fatal(err)
	}
	t.Logf("push: %+v", rb)

	if _, err = rb.Push([]byte{4}); err != nil {
		t.Fatal(err)
	}
	t.Logf("push: %+v", rb)

	_, err = rb.Push([]byte{5})
	assert.Error(t, err, "buffer should be full")
	t.Logf("push: %+v", rb)

	// POP
	if err = rb.Pop(buf); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, []byte{2}, buf)
	t.Logf("pop: %+v", rb)

	if err = rb.Pop(buf); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, []byte{3}, buf)
	t.Logf("pop: %+v", rb)

	if err = rb.Pop(buf); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, []byte{4}, buf)
	t.Logf("pop: %+v", rb)

	assert.Error(t, rb.Pop(buf), "buffer should be empty")
	t.Logf("pop: %+v", rb)
}

func TestInt64Queue(t *testing.T) {
	q := NewInt64Queue(2)
	idx, err := q.Push(math.MinInt64)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, idx)
	idx, err = q.Push(math.MaxInt64)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, idx)
	_, err = q.Push(0)
	assert.Error(t, err)

	v, err := q.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, math.MaxInt64, v)

	v, err = q.Pop(true)
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, math.MinInt64, v)

	v, err = q.Pop(true)
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, math.MaxInt64, v)
}
