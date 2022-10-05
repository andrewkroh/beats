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

package cache

import (
	"fmt"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/elastic/elastic-agent-libs/config"
	"github.com/pkg/errors"
	"runtime"
)

type cache struct {
	Backend
	runtime *goja.Runtime
	this    *goja.Object
}

// newChainBuilder returns a javascript constructor that constructs a
// chainBuilder.
func newCacheConstructor(jsRuntime *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) != 1 {
			panic(jsRuntime.NewGoError(errors.New("New requires one argument")))
		}

		a0, ok := call.Argument(0).Export().(map[string]interface{})
		if !ok {
			panic(jsRuntime.NewGoError(errors.New("arg 0 must be an Object")))
		}

		var cacheConfig Config
		if ucfg, err := config.NewConfigFrom(a0); err != nil {
			panic(jsRuntime.NewGoError(err))
		} else if err := ucfg.Unpack(&cacheConfig); err != nil {
			panic(jsRuntime.NewGoError(err))
		}

		backend, err := New(cacheConfig)
		if err != nil {
			panic(jsRuntime.NewGoError(err))
		}

		c := &cache{Backend: backend, runtime: jsRuntime, this: call.This}
		c.this.Set("_private", c)
		c.this.Set("Put", c.Put)
		c.this.Set("Get", c.Get)
		c.this.Set("Delete", c.Delete)

		runtime.SetFinalizer(c, func(c *cache) {
			fmt.Println("CACHE FREE")
		})

		return nil
	}
}

// Require registers the module with the runtime.
func Require(runtime *goja.Runtime, module *goja.Object) {
	o := module.Get("exports").(*goja.Object)
	o.Set("New", newCacheConstructor(runtime))
}

// Enable adds cache to the given runtime.
func Enable(runtime *goja.Runtime) {
	runtime.Set("cache", require.Require(runtime, "cache"))
}

func init() {
	require.RegisterNativeModule("cache", Require)
}
