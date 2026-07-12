// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package netutil

import (
	"errors"
	"net"
	"net/netip"
	"strings"
)

// StripHostPort returns h without any trailing ":<port>". It also removes trailing period in the hostname.
// Per RFC 3696, The DNS specification permits a trailing period to be used to denote the root, e.g., "a.b.c" and "a.b.c."
// are equivalent, but the latter is more explicit and is required to be accepted by applications. Note that FQDN does
// not play well with TLS (see https://github.com/traefik/traefik/issues/9157#issuecomment-1180588735)
func StripHostPort(h string) string {
	if h == "" {
		return h
	}
	// If no port on host, return unchanged
	if !strings.Contains(h, ":") {
		return strings.TrimSuffix(h, ".")
	}

	host, _, err := net.SplitHostPort(h)
	if err != nil {
		return h // on error, return unchanged
	}
	return strings.TrimSuffix(host, ".")
}

// ParsePrefix parses s as a CIDR prefix or a single IP address treated as a full-length prefix.
// The result is in canonical form: host bits are masked and IPv4-mapped IPv6 values are normalized
// to their IPv4 form, so "::ffff:10.0.0.0/104" behaves as "10.0.0.0/8". Zoned addresses are rejected.
func ParsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		if prefix.Addr().Is4In6() && prefix.Bits() >= 96 {
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		return prefix.Masked(), nil
	}

	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	if addr.Zone() != "" {
		return netip.Prefix{}, errors.New("zones are not allowed")
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
