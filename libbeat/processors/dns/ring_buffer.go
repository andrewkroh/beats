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
	"encoding/binary"

	"github.com/pkg/errors"
)

type RingBuffer struct {
	buf  []byte // Raw data.
	head int    // Valid data begins here.
	tail int    // Valid data ends here.

	itemSize  int // Size of a single item in bytes.
	itemCap   int // Number of items the buffer holds.
	itemCount int // Number of items in buffer.
}

func NewRingBuffer(itemSize, itemCapacity int) *RingBuffer {
	return &RingBuffer{
		buf:      make([]byte, itemSize*itemCapacity),
		itemSize: itemSize,
		itemCap:  itemCapacity,
	}
}

func (rb *RingBuffer) Push(item []byte) (int, error) {
	if rb.itemCount == rb.itemCap {
		return 0, errors.New("buffer is full")
	}
	if len(item) < rb.itemSize {
		return 0, errors.New("item buffer is too small")
	}
	if n := copy(rb.buf[rb.head:], item[:rb.itemSize]); n != rb.itemSize {
		return 0, errors.Errorf("failed to write all %d item bytes to buffer", rb.itemSize)
	}
	index := rb.head / rb.itemSize
	rb.head += rb.itemSize
	if rb.head == len(rb.buf) {
		rb.head = 0
	}
	rb.itemCount++
	return index, nil
}

func (rb *RingBuffer) Pop(item []byte) error {
	if rb.itemCount == 0 {
		return errors.New("buffer is empty")
	}

	// item may nil if you want to pop the item without receiving its value.
	if item != nil {
		if err := rb.Peek(item); err != nil {
			return err
		}
	}

	rb.tail += rb.itemSize
	if rb.tail == len(rb.buf) {
		rb.tail = 0
	}
	rb.itemCount--

	return nil
}

func (rb *RingBuffer) Peek(item []byte) error {
	if rb.itemCount == 0 {
		return errors.New("buffer is empty")
	}
	if len(item) < rb.itemSize {
		return errors.New("item buffer is too small")
	}

	if n := copy(item, rb.buf[rb.tail:rb.tail+rb.itemSize]); n != rb.itemSize {
		return errors.Errorf("failed copying to item buffer, only wrote %d bytes", n)
	}

	return nil
}

func (rb *RingBuffer) Get(index int, item []byte) error {
	bufferIndex := index * rb.itemSize

	if bufferIndex+rb.itemSize > len(rb.buf) {
		return errors.Errorf("invalid item index %d", index)
	}
	if len(item) < rb.itemSize {
		return errors.New("item buffer is too small")
	}
	if n := copy(item, rb.buf[bufferIndex:bufferIndex+rb.itemSize]); n != rb.itemSize {
		return errors.Errorf("failed copying to item buffer, only wrote %d bytes", n)
	}

	return nil
}

func NewInt64Queue(itemCapacity int) *Int64Queue {
	return &Int64Queue{
		rb: NewRingBuffer(8, itemCapacity),
	}
}

type Int64Queue struct {
	rb      *RingBuffer
	itemBuf [8]byte
}

var endian binary.ByteOrder = binary.LittleEndian

func (q *Int64Queue) Push(item int64) (int, error) {
	endian.PutUint64(q.itemBuf[:], uint64(item))
	return q.rb.Push(q.itemBuf[:])
}

func (q *Int64Queue) Pop(read bool) (int64, error) {
	// Optimization to avoid reading if we don't care about the value.
	if !read {
		return 0, q.rb.Pop(nil)
	}

	if err := q.rb.Pop(q.itemBuf[:]); err != nil {
		return 0, err
	}
	return int64(endian.Uint64(q.itemBuf[:])), nil
}

func (q *Int64Queue) Peek() (int64, error) {
	if err := q.rb.Peek(q.itemBuf[:]); err != nil {
		return 0, err
	}
	return int64(endian.Uint64(q.itemBuf[:])), nil
}

func (q *Int64Queue) Get(index int) (int64, error) {
	if err := q.rb.Get(index, q.itemBuf[:]); err != nil {
		return 0, err
	}
	return int64(endian.Uint64(q.itemBuf[:])), nil
}
