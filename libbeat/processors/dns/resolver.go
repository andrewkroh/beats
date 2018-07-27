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
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"

	"github.com/elastic/beats/libbeat/monitoring"
	"github.com/elastic/beats/libbeat/monitoring/adapter"
)

var ptrResponseTimeMetric = metrics.NewUniformSample(1028)

func init() {
	reg := adapter.GetGoMetrics(monitoring.Default, "processor.dns", adapter.Accept)
	reg.Register("ptr_lookup", metrics.NewHistogram(ptrResponseTimeMetric))
}

type PTR struct {
	Host string // Hostname.
	TTL  uint32 // TTL in seconds.
}

type PTRResolver interface {
	LookupPTR(ip string) (*PTR, error)
}

type MiekgResolver struct {
	client  *dns.Client
	servers []string
}

const etcResolvConf = "/etc/resolv.conf"

func NewMiekgResolver(timeout time.Duration, servers []string) (*MiekgResolver, error) {
	if len(servers) == 0 {
		config, err := dns.ClientConfigFromFile(etcResolvConf)
		if err != nil || len(config.Servers) == 0 {
			return nil, errors.New("no dns servers configured")
		}
		servers = config.Servers
	}

	// Add port if one was not specified.
	for i, s := range servers {
		if _, _, err := net.SplitHostPort(s); err != nil {
			withPort := s + ":53"
			if _, _, retryErr := net.SplitHostPort(withPort); retryErr == nil {
				servers[i] = withPort
				continue
			}
			return nil, err
		}
	}

	if timeout == 0 {
		timeout = 300 * time.Millisecond
	}

	return &MiekgResolver{
		client: &dns.Client{
			Net:     "udp",
			Timeout: timeout,
		},
		servers: servers,
	}, nil
}

func (res *MiekgResolver) LookupPTR(ip string) (*PTR, error) {
	if len(res.servers) == 0 {
		return nil, errors.New("no dns servers configured")
	}

	m := new(dns.Msg)

	arpa, err := dns.ReverseAddr(ip)
	if err != nil {
		return nil, err
	}
	m.SetQuestion(arpa, dns.TypePTR)
	m.RecursionDesired = true

	var rtnErr error
	for _, server := range res.servers {
		r, rtt, err := res.client.Exchange(m, server)
		if err != nil {
			// Try next server if any. Otherwise return retErr.
			rtnErr = err
			continue
		}
		ptrResponseTimeMetric.Update(int64(rtt))
		if r.Rcode != dns.RcodeSuccess {
			name, found := dns.RcodeToString[r.Rcode]
			if !found {
				name = "response code " + strconv.Itoa(r.Rcode)
			}
			return nil, errors.Errorf("server responded with %v", name)
		}

		for _, a := range r.Answer {
			if ptr, ok := a.(*dns.PTR); ok {
				return &PTR{
					Host: strings.TrimSuffix(ptr.Ptr, "."),
					TTL:  ptr.Hdr.Ttl,
				}, nil
			}
		}

		return nil, errors.New("no PTR record was found the response")
	}

	if rtnErr != nil {
		return nil, rtnErr
	}

	// This should never get here.
	panic("LookupPTR should have returned a response.")
}
