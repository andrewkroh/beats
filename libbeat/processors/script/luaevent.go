package script

import (
	"github.com/pkg/errors"

	"github.com/layeh/gopher-luar"
	"github.com/yuin/gopher-lua"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
)

// LuaEvent wraps beat.Event to provide a simple interface that's easy to use
// from lua and to hide direct access to the fields.
type LuaEvent struct {
	event *beat.Event
}

func (e *LuaEvent) Get(key string) interface{} {
	v, _ := e.event.GetValue(key)
	return v
}

func (e *LuaEvent) Put(key string, value interface{}) error {
	_, err := e.event.PutValue(key, value)
	return err
}

func (e *LuaEvent) Delete(key string) error {
	return e.event.Delete(key)
}

func (e *LuaEvent) Rename(from, to string) error {
	// Fields cannot be overwritten. Either the target field has to be dropped
	// first or renamed first.
	v, _ := e.event.GetValue(to)
	if v != nil {
		return errors.Errorf("to field <%s> already exists", to)
	}

	v, err := e.event.GetValue(from)
	if err != nil {
		// Ignore ErrKeyNotFound errors
		if errors.Cause(err) == common.ErrKeyNotFound {
			return nil
		}
		return errors.Wrapf(err, "failed to get 'from' key <%s>", from)
	}

	// Deletion must happen first to support cases where a becomes a.b.
	err = e.event.Delete(from)
	if err != nil {
		return errors.Wrapf(err, "failed to delete 'from' key")
	}

	_, err = e.event.PutValue(to, v)
	if err != nil {
		return errors.Wrapf(err, "failed to write value to <%s>", to)
	}

	return nil
}

func (e *LuaEvent) Drop() {
	e.event = nil
}

func (e *LuaEvent) getLValue(L *lua.LState) lua.LValue {
	return luar.New(L, e)
}
