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

package javascript

import (
	"github.com/robertkrimen/otto"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/logp"
)

type jsEvent struct {
	obj *otto.Object
	log *logp.Logger

	event   *beat.Event
	dropped bool
}

func newJSEvent(log *logp.Logger, vm *otto.Otto) (*jsEvent, error) {
	e := &jsEvent{}

	obj, err := makeObject(e, vm)
	if err != err {
		return nil, err
	}
	e.obj = obj

	return e, nil
}

func (e *jsEvent) Wrap(b *beat.Event) *otto.Object {
	e.event = b
	e.dropped = false
	return e.obj
}

func makeObject(e *jsEvent, vm *otto.Otto) (*otto.Object, error) {
	obj, err := vm.Object("({})")
	if err != nil {
		return nil, err
	}

	if err = obj.Set("get", e.get); err != nil {
		return nil, err
	}
	if err = obj.Set("put", e.put); err != nil {
		return nil, err
	}
	if err = obj.Set("rename", e.rename); err != nil {
		return nil, err
	}
	if err = obj.Set("delete", e.delete); err != nil {
		return nil, err
	}
	if err = obj.Set("drop", e.drop); err != nil {
		return nil, err
	}

	return obj, nil
}

func (e *jsEvent) get(call otto.FunctionCall) otto.Value {
	key := call.Argument(0).String()
	if key == "" {
		return otto.NullValue()
	}

	v, err := e.event.GetValue(key)
	if err != nil {
		return otto.NullValue()
	}

	jsVal, err := call.Otto.ToValue(v)
	if err != nil {
		return otto.NullValue()
	}

	return jsVal
}

func (e *jsEvent) put(call otto.FunctionCall) otto.Value {
	key := call.Argument(0).String()
	if key == "" {
		return otto.FalseValue()
	}

	val, _ := call.Argument(1).Export()
	if val == nil {
		return otto.FalseValue()
	}

	if _, err := e.event.PutValue(key, val); err != nil {
		e.log.Warnf("event put failed for key=%v value=%v", key, val)
		return otto.FalseValue()
	}

	return otto.TrueValue()
}

func (e *jsEvent) rename(call otto.FunctionCall) otto.Value {
	var (
		from = call.Argument(0).String()
		to   = call.Argument(1).String()
	)
	if from == "" || to == "" {
		return otto.FalseValue()
	}

	// Fields cannot be overwritten. Either the target field has to be deleted
	// or renamed.
	if v, _ := e.event.GetValue(to); v != nil {
		e.log.Debugf("rename failed: to field <%s> already exists", to)
		return otto.FalseValue()
	}

	v, err := e.event.GetValue(from)
	if err != nil {
		e.log.Debugf("rename failed: from field <%s> does not exist", from)
		return otto.FalseValue()
	}

	// Deletion must happen first to support cases where a becomes a.b.
	if err = e.event.Delete(from); err != nil {
		e.log.Debugf("rename failed: from field <%s> could not be deleted: %v", from, err)
		return otto.FalseValue()
	}

	_, err = e.event.PutValue(to, v)
	if err != nil {
		e.log.Warnf("rename failed: put failed for to=%v value=%v", to, v)
		return otto.FalseValue()
	}

	return otto.TrueValue()
}

func (e *jsEvent) delete(call otto.FunctionCall) otto.Value {
	key := call.Argument(0).String()
	if key == "" {
		return otto.FalseValue()
	}

	if err := e.event.Delete(key); err != nil {
		return otto.FalseValue()
	}

	return otto.TrueValue()
}

func (e *jsEvent) drop(call otto.FunctionCall) otto.Value {
	e.dropped = true
	return otto.UndefinedValue()
}
