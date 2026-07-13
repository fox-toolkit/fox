// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package clientip

import (
	"net/netip"
)

type config struct {
	ipRanges []netip.Prefix
}

type TrustedRangeOption interface {
	applyRight(*config)
}

type ExcludedRangeOption interface {
	applyLeft(*config)
}

type rightmostNonPrivateOptionFunc func(*config)

func (o rightmostNonPrivateOptionFunc) applyRight(c *config) {
	o(c)
}

type leftmostNonPrivateOptionFunc func(*config)

func (o leftmostNonPrivateOptionFunc) applyLeft(c *config) {
	o(c)
}

// TrustLoopback enables or disables the inclusion of loopback ip ranges in the trusted ip ranges.
func TrustLoopback(enable bool) TrustedRangeOption {
	return rightmostNonPrivateOptionFunc(func(c *config) {
		if enable {
			c.ipRanges = append(c.ipRanges, loopbackRanges...)
		}
	})
}

// TrustLinkLocal enables or disables the inclusion of link local ip ranges in the trusted ip ranges.
func TrustLinkLocal(enable bool) TrustedRangeOption {
	return rightmostNonPrivateOptionFunc(func(c *config) {
		if enable {
			c.ipRanges = append(c.ipRanges, linkLocalRanges...)
		}
	})
}

// TrustPrivateNet enables or disables the inclusion of private-space ip ranges in the trusted ip ranges.
func TrustPrivateNet(enable bool) TrustedRangeOption {
	return rightmostNonPrivateOptionFunc(func(c *config) {
		if enable {
			c.ipRanges = append(c.ipRanges, privateRange...)
		}
	})
}

// ExcludeLoopback enables or disables the inclusion of loopback ip ranges in the excluded ip ranges.
func ExcludeLoopback(enable bool) ExcludedRangeOption {
	return leftmostNonPrivateOptionFunc(func(c *config) {
		if enable {
			c.ipRanges = append(c.ipRanges, loopbackRanges...)
		}
	})
}

// ExcludeLinkLocal enables or disables the inclusion of link local ip ranges in the excluded ip ranges.
func ExcludeLinkLocal(enable bool) ExcludedRangeOption {
	return leftmostNonPrivateOptionFunc(func(c *config) {
		if enable {
			c.ipRanges = append(c.ipRanges, linkLocalRanges...)
		}
	})
}

// ExcludePrivateNet enables or disables the inclusion of private-space ip ranges in the excluded ip ranges.
func ExcludePrivateNet(enable bool) ExcludedRangeOption {
	return leftmostNonPrivateOptionFunc(func(c *config) {
		if enable {
			c.ipRanges = append(c.ipRanges, privateRange...)
		}
	})
}
