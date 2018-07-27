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
	"time"

	"github.com/pkg/errors"

	"github.com/elastic/beats/libbeat/common"
)

type Config struct {
	CacheConfig
	Reverse     common.MapStr `config:"reverse"`
	reverseFlat map[string]string
}

type CacheConfig struct {
	SuccessCache CacheSettings `config:"success_cache"`
	FailureCache CacheSettings `config:"failure_cache"`
}

type CacheSettings struct {
	TTL             time.Duration `config:"ttl"`
	InitialCapacity int           `config:"initial_capacity" validate:"min=0"`
	MaxCapacity     int           `config:"max_capacity"     validate:"min=0"`
}

func (c *Config) Validate() error {
	c.reverseFlat = map[string]string{}
	for k, v := range c.Reverse.Flatten() {
		target, ok := v.(string)
		if !ok {
			return errors.Errorf("target field for dns reverse lookup of %v "+
				"must be a string but got %T", k, v)
		}
		c.reverseFlat[k] = target
	}

	if c.SuccessCache.MaxCapacity != 0 && c.SuccessCache.MaxCapacity < c.SuccessCache.InitialCapacity {
		return errors.Errorf("success_cache.max_capacity must be >= success_cache.initial_capacity")
	}
	if c.FailureCache.MaxCapacity != 0 && c.FailureCache.MaxCapacity < c.FailureCache.InitialCapacity {
		return errors.Errorf("failure_cache.max_capacity must be >= failure_cache.initial_capacity")
	}
	return nil
}

var defaultConfig = Config{
	CacheConfig: CacheConfig{
		SuccessCache: CacheSettings{
			TTL:             5 * time.Minute,
			InitialCapacity: 1000,
			MaxCapacity:     10000,
		},
		FailureCache: CacheSettings{
			TTL:             time.Minute,
			InitialCapacity: 1000,
			MaxCapacity:     10000,
		},
	},
}
