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
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/logp"
)

func TestCachingResolver(t *testing.T) {
	logp.TestingSetup()

	c := CacheConfig{
		SuccessCache: CacheSettings{time.Minute, 100, 100},
		FailureCache: CacheSettings{time.Minute, 100, 100},
	}
	r, err := NewCachingResolver(c, &stubResolver{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("success", func(t *testing.T) {
		// Test success lookups.
		for i := 0; i < 5; i++ {
			name, err := r.LookupPTR(gatewayIP)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, gatewayName, name.Host)
		}
		assert.EqualValues(t, 1, r.successCache.Stats().Misses)
		assert.EqualValues(t, 1, r.failureCache.Stats().Misses)
		assert.EqualValues(t, 4, r.successCache.Stats().Hits)
	})

	t.Run("failed", func(t *testing.T) {
		name, err := r.LookupPTR(gatewayIP + "9")
		if assert.Error(t, err) {
			t.Log(err)
			assert.Contains(t, err.Error(), "fake")
		}
		assert.Zero(t, name)

		name, err = r.LookupPTR(gatewayIP + "9")
		if assert.Error(t, err) {
			t.Log(err)
			assert.Contains(t, err.Error(), "cached failure")
		}
		assert.Zero(t, name)

		assert.EqualValues(t, 1+2, r.successCache.Stats().Misses)
		assert.EqualValues(t, 1+1, r.failureCache.Stats().Misses)
		assert.EqualValues(t, 4, r.successCache.Stats().Hits)
		assert.EqualValues(t, 1, r.failureCache.Stats().Hits)
	})
}

const (
	gatewayIP   = "192.168.0.1"
	gatewayName = "default.gateway.example"
)

type stubResolver struct{}

func (r *stubResolver) LookupPTR(ip string) (*PTR, error) {
	if ip == gatewayIP {
		return &PTR{Host: gatewayName}, nil
	}

	return nil, errors.New("lookup timeout (fake)")
}
