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

package goja

import (
	"github.com/dop251/goja"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/logp"
)

type jsEvent struct {
	obj *goja.Object
	log *logp.Logger
	vm  *goja.Runtime

	event   *beat.Event
	dropped bool
}

func newJSEvent(log *logp.Logger, vm *goja.Runtime) (*jsEvent, error) {
	e := &jsEvent{
		log: log,
		vm: vm,
	}

	obj, err := makeObject(e, vm)
	if err != err {
		return nil, err
	}
	e.obj = obj

	return e, nil
}

func (e *jsEvent) Wrap(b *beat.Event) *goja.Object {
	e.event = b
	e.dropped = false
	return e.obj
}

func makeObject(e *jsEvent, vm *goja.Runtime) (*goja.Object, error) {
	obj := vm.NewObject()

	var err error
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

func (e *jsEvent) get(call goja.FunctionCall) goja.Value {
	key := call.Argument(0).String()
	if key == "" {
		return goja.Null()
	}

	v, err := e.event.GetValue(key)
	if err != nil {
		return goja.Null()
	}

	jsVal := e.vm.ToValue(v)
	if jsVal == nil {
		return goja.Null()
	}

	return jsVal
}

func (e *jsEvent) put(call goja.FunctionCall) goja.Value {
	key := call.Argument(0).String()
	if key == "" {
		return e.vm.ToValue(false)
	}

	val := call.Argument(1).Export()
	if val == nil {
		return e.vm.ToValue(false)
	}

	if _, err := e.event.PutValue(key, val); err != nil {
		e.log.Warnf("event put failed for key=%v value=%v", key, val)
		return e.vm.ToValue(false)
	}

	return e.vm.ToValue(true)
}

func (e *jsEvent) rename(call goja.FunctionCall) goja.Value {
	var (
		from = call.Argument(0).String()
		to   = call.Argument(1).String()
	)
	if from == "" || to == "" {
		return e.vm.ToValue(false)
	}

	// Fields cannot be overwritten. Either the target field has to be deleted
	// or renamed.
	if v, _ := e.event.GetValue(to); v != nil {
		e.log.Debugf("rename failed: to field <%s> already exists", to)
		return e.vm.ToValue(false)
	}

	v, err := e.event.GetValue(from)
	if err != nil {
		e.log.Debugf("rename failed: from field <%s> does not exist", from)
		return e.vm.ToValue(false)
	}

	// Deletion must happen first to support cases where a becomes a.b.
	if err = e.event.Delete(from); err != nil {
		e.log.Debugf("rename failed: from field <%s> could not be deleted: %v", from, err)
		return e.vm.ToValue(false)
	}

	_, err = e.event.PutValue(to, v)
	if err != nil {
		e.log.Warnf("rename failed: put failed for to=%v value=%v", to, v)
		return e.vm.ToValue(false)
	}

	return e.vm.ToValue(true)
}

func (e *jsEvent) delete(call goja.FunctionCall) goja.Value {
	key := call.Argument(0).String()
	if key == "" {
		return e.vm.ToValue(false)
	}

	if err := e.event.Delete(key); err != nil {
		return e.vm.ToValue(false)
	}

	return e.vm.ToValue(true)
}

func (e *jsEvent) drop(call goja.FunctionCall) goja.Value {
	e.dropped = true
	return goja.Undefined()
}
