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
	"math"

	"github.com/allegro/bigcache"
	"github.com/pkg/errors"

	"github.com/elastic/beats/libbeat/logp"
)

var errLookupFailed = errors.New("dns lookup failed (cached failure)")

type CachingResolver struct {
	resolver     PTRResolver
	successCache *bigcache.BigCache
	failureCache *bigcache.BigCache
	log          *logp.Logger
}

func NewCachingResolver(c CacheConfig, resolver PTRResolver) (*CachingResolver, error) {
	log := logp.NewLogger(logName)

	successCache, err := newCache(log, successType, c.SuccessCache)
	if err != nil {
		return nil, err
	}

	failureCache, err := newCache(log, failedType, c.FailureCache)
	if err != nil {
		return nil, err
	}

	return &CachingResolver{
		resolver:     resolver,
		successCache: successCache,
		failureCache: failureCache,
		log:          log,
	}, nil
}

func (r *CachingResolver) LookupPTR(ip string) (*PTR, error) {
	data, err := r.successCache.Get(ip)
	if err == nil {
		return &PTR{Host: string(data)}, nil
	}

	_, err = r.failureCache.Get(ip)
	if err == nil {
		return nil, errLookupFailed
	}

	ptr, err := r.resolver.LookupPTR(ip)
	if err != nil {
		r.log.Debugw("Reverse DNS lookup failed.",
			"error", err, "ip", ip)

		if cacheErr := r.failureCache.Set(ip, nil); cacheErr != nil {
			r.log.Warnw("Failed adding IP to reverse DNS failure cache",
				"error", err, "ip", ip)
		}
		return nil, err
	}

	if err := r.successCache.Set(ip, []byte(ptr.Host)); err != nil {
		r.log.Warnw("Failed adding IP to reverse DNS success cache",
			"error", err, "ip", ip)
	}
	return ptr, nil
}

//
// bigcache constructor helpers
//

type bigCacheLogger struct {
	log *logp.Logger
}

func (bcl *bigCacheLogger) Printf(format string, args ...interface{}) {
	bcl.log.Infof(format, args...)
}

type cacheType uint8

const (
	successType cacheType = iota + 1
	failedType
)

func newCache(log *logp.Logger, typ cacheType, settings CacheSettings) (*bigcache.BigCache, error) {
	conf := bigcache.DefaultConfig(settings.TTL)

	// Not all of bigcache honors the provided logger so disable logging by
	// setting verbose=false so that messages do not bypass logp.
	conf.Verbose = false
	conf.Logger = &bigCacheLogger{log.Named("cache")}

	// Max DNS name with dots is 253 bytes.
	conf.MaxEntrySize = 253
	if typ == failedType {
		conf.MaxEntrySize = 1
	}

	// Initial capacity given in number of entries. (It can grow beyond this but
	// with an additional allocation cost.)
	conf.MaxEntriesInWindow = settings.InitialCapacity

	// Upper bound on number of items based on total size. It only accepts the value
	// in MB so we lose a lot of precision.
	if settings.MaxCapacity == 0 {
		conf.HardMaxCacheSize = 0
	} else {
		conf.HardMaxCacheSize = max(toMegabytes(settings.MaxCapacity*conf.MaxEntrySize),
			max(toMegabytes(settings.InitialCapacity*conf.MaxEntrySize), 1))
	}
	log.Debugf("cache config (type=%v): %+v", typ, conf)

	cache, err := bigcache.NewBigCache(conf)
	if err != nil {
		return nil, err
	}

	return cache, nil
}

func toMegabytes(bytes int) int {
	const MB = 1024 * 1024
	return int(math.Round(float64(bytes) / float64(MB)))
}

func max(a, b int) int {
	if a >= b {
		return a
	}
	return b
}
