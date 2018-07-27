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
	"encoding/binary"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/allegro/bigcache"

	"github.com/elastic/beats/libbeat/logp"
)

var (
	startIP     = uint32(binary.BigEndian.Uint32(net.ParseIP("1.1.1.1").To4()))
	reverseName = []byte("pool-0-00-000-0.washdc.fios.verizon.net")
)

func ipKey(i int) string {
	ip := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(ip, startIP+uint32(i))
	return ip.String()
}

func timeGC() time.Duration {
	start := time.Now()
	runtime.GC()
	return time.Since(start)
}

func BenchmarkBigCacheSet(b *testing.B) {
	b.StopTimer()
	logp.TestingSetup()

	// Pre-allocate IP address keys to keep benchmem accurate.
	keys := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		keys[i] = ipKey(i)
	}

	// Allocate the cache. Reset stats and start the timer again.
	cache := newBigCache(b, b.N)
	b.ResetTimer()
	b.StartTimer()

	// Run the benchmark.
	for i := 0; i < b.N; i++ {
		cache.Set(keys[i], reverseName)
	}

	runtime.GC()
	b.Logf("Cache Len: %+v\n", cache.Len())
	b.Logf("With %T, GC took %s\n", cache, timeGC())
	_, _ = cache.Get("x") // Preserve cache until here, hopefully
}

func newBigCache(t testing.TB, items int) *bigcache.BigCache {
	c, err := newCache(logp.NewLogger(logName), successType, CacheSettings{time.Minute, items, 0})
	if err != nil {
		t.Fatal(err)
	}

	return c
}
