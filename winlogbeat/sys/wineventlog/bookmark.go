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
	"syscall"

	"github.com/pkg/errors"
)

type Bookmark EvtHandle

func (b Bookmark) Close() error {
	return EvtHandle(b).Close()
}

func (b Bookmark) XML() (string, error) {
	var bufferUsed uint32
	err := _EvtRender(NilHandle, EvtHandle(b), EvtRenderBookmark, 0, nil, &bufferUsed, nil)
	if err != nil && err != ERROR_INSUFFICIENT_BUFFER {
		return "", errors.Errorf("expected ERROR_INSUFFICIENT_BUFFER but got %v", err)
	}

	bb := newByteBuffer()
	bb.SetLength(int(bufferUsed * 2))
	defer bb.free()

	err = _EvtRender(NilHandle, EvtHandle(b), EvtRenderBookmark, uint32(len(bb.buf)), &bb.buf[0], &bufferUsed, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to render bookmark XML")
	}

	return UTF16BytesToString(bb.buf)
}

func NewBookmark(eventHandle EvtHandle) (Bookmark, error) {
	h, err := _EvtCreateBookmark(nil)
	if err != nil {
		return 0, err
	}
	if err = _EvtUpdateBookmark(h, eventHandle); err != nil {
		h.Close()
		return 0, err
	}
	return Bookmark(h), nil
}

func NewBookmarkFromXML(xml string) (Bookmark, error) {
	utf16, err := syscall.UTF16PtrFromString(xml)
	if err != nil {
		return 0, err
	}
	h, err := _EvtCreateBookmark(utf16)
	return Bookmark(h), err
}
