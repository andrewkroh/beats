// +build windows

package wineventlog

import "github.com/pkg/errors"

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
