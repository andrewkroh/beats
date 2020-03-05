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

package wineventlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventIterator(t *testing.T) {
	itr, err := NewEventIterator(func() (EvtHandle, error) {
		return openLog(t, security4752File), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { assert.NoError(t, itr.Close()) }()

	for itr.Next() {
		h := itr.Handle()
		assert.NotZero(t, h)
		h.Close()
	}
	if err := itr.Err(); err != nil {
		t.Fatal(err)
	}
}
