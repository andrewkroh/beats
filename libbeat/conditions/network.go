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

package conditions

import (
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"

	"github.com/elastic/beats/libbeat/logp"
)

var (
	// RFC 1918
	privateIPv4 = []net.IPNet{
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.IPv4Mask(255, 0, 0, 0)},
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.IPv4Mask(255, 240, 0, 0)},
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.IPv4Mask(255, 255, 0, 0)},
	}

	// RFC 4193
	privateIPv6 = net.IPNet{
		IP:   net.IP{0xfd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		Mask: net.IPMask{0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
)

// Network is a condition that tests if an IP address is in a network range.
type Network struct {
	fields map[string]networkMatcher
	log    *logp.Logger
}

type networkMatcher struct {
	name     string
	contains func(ip net.IP) bool
}

func (n networkMatcher) Contains(ip net.IP) bool {
	return n.contains(ip)
}

func (n networkMatcher) String() string {
	return n.name
}

// NewNetworkCondition builds a new Network using the given configuration.
func NewNetworkCondition(fields map[string]interface{}) (*Network, error) {
	cond := &Network{
		fields: map[string]networkMatcher{},
		log:    logp.NewLogger(logName),
	}

	for field, value := range fields {
		sValue, err := ExtractString(value)
		if err != nil {
			return nil, fmt.Errorf("condition attempted to set '%v' -> '%v' "+
				"and encountered unexpected type '%T', only strings are "+
				"allowed", field, value, value)
		}

		// Parse keywords.
		nv := networkMatcher{name: sValue}
		switch sValue {
		case "loopback":
			nv.contains = func(ip net.IP) bool { return ip.IsLoopback() }
		case "global_unicast", "unicast":
			nv.contains = func(ip net.IP) bool { return ip.IsGlobalUnicast() }
		case "link_local_unicast":
			nv.contains = func(ip net.IP) bool { return ip.IsLinkLocalUnicast() }
		case "interface_local_multicast":
			nv.contains = func(ip net.IP) bool { return ip.IsInterfaceLocalMulticast() }
		case "link_local_multicast":
			nv.contains = func(ip net.IP) bool { return ip.IsLinkLocalMulticast() }
		case "multicast":
			nv.contains = func(ip net.IP) bool { return ip.IsMulticast() }
		case "unspecified":
			nv.contains = func(ip net.IP) bool { return ip.IsUnspecified() }
		case "private":
			nv.contains = isPrivateNetwork
		case "public":
			nv.contains = func(ip net.IP) bool { return !isLocalOrPrivate(ip) }

		// Parse Network.
		default:
			subnet, err := extractCIDR(sValue)
			if err != nil {
				return nil, err
			}
			nv.name = subnet.String()
			nv.contains = subnet.Contains
		}

		cond.fields[field] = nv
	}

	return cond, nil
}

// extractCIDR extracts a Network from an unknown type.
func extractCIDR(value string) (*net.IPNet, error) {
	_, mask, err := net.ParseCIDR(value)
	return mask, errors.Wrap(err, "failed to parse CIDR, values must be "+
		"an IP address and prefix length, like '192.0.2.0/24' or "+
		"'2001:db8::/32', as defined in RFC 4632 and RFC 4291.")
}

// extractIP return an IP address if unk is an IP address string or a net.IP.
// Otherwise it returns nil.
func extractIP(unk interface{}) net.IP {
	switch v := unk.(type) {
	case string:
		return net.ParseIP(v)
	case net.IP:
		return v
	default:
		return nil
	}
}

// Check determines whether the given event matches this condition.
func (c *Network) Check(event ValuesMap) bool {
	for field, network := range c.fields {
		value, err := event.GetValue(field)
		if err != nil {
			return false
		}

		ip := extractIP(value)
		if ip == nil {
			c.log.Debugf("Invalid IP address in field=%v for network condition", field)
			return false
		}

		if !network.Contains(ip) {
			return false
		}
	}

	return true
}

// String returns a string representation of the Network condition.
func (c *Network) String() string {
	var sb strings.Builder
	sb.WriteString("network:(")
	var i int
	for field, network := range c.fields {
		sb.WriteString(field)
		sb.WriteString(":")
		sb.WriteString(network.String())
		if i < len(c.fields)-1 {
			sb.WriteString(" AND ")
		}
		i++
	}
	sb.WriteString(")")
	return sb.String()
}

func isPrivateNetwork(ip net.IP) bool {
	for _, net := range privateIPv4 {
		if net.Contains(ip) {
			return true
		}
	}

	return privateIPv6.Contains(ip)
}

func isLocalOrPrivate(ip net.IP) bool {
	return isPrivateNetwork(ip) ||
		ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.Equal(net.IPv4bcast) ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast()
}
