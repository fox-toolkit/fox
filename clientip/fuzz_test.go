// The code in this package is derivative of https://github.com/realclientip/realclientip-go (all credit to Adam Pritchard).
// Mount of this source code is governed by a BSD Zero Clause License that can be found
// at https://github.com/realclientip/realclientip-go/blob/main/LICENSE.

package clientip

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/fox-toolkit/fox"
)

// Fuzz_parseForwardedListItem exercises the Forwarded-header "for=" parser --
// the package's most involved hand-written parsing -- against arbitrary input.
// Callers feed this untrusted header data, so the contract under test is
// robustness: it must never panic and never take pathological time, whatever
// the input.
//
// There is deliberately no result oracle: re-deriving the "correct" parse would
// just reimplement the function. The one cheap invariant we do assert is the
// return contract -- a valid address is canonical and round-trips through
// [ParseAddr] unchanged.
func Fuzz_parseForwardedListItem(f *testing.F) {
	seeds := []string{
		`For="[2607:f8b0:4004:83f::200e]:4711"`, `fOR="[2607:f8b0:4004:83f::200e]"`,
		`for="2607:f8b0:4004:83f::200e"`, `FOR=[2607:f8b0:4004:83f::200e]`,
		`For=[2607:f8b0:4004:83f::200e]:4711`, `For="[fe80::abcd%zone]:4711"`,
		`For="fe80::abcd%zone"`, `FoR=192.0.2.60:4711`, `for=192.0.2.60`,
		`for="192.0.2.60"`, `for="192.0.2.60:4823"`, `for=192.0.2.999`,
		`for="2607:f8b0:4004:83f::999999"`, `for="_test"`, `for=`,
		`by=1.1.1.1; for=2.2.2.2;host=myhost; proto=https`,
		`by=1::1;host=myhost;for=2::2;proto=https`,
		`by=1::1;host=myhost;proto=https;for=2.2.2.2`,
		`for="[::ffff:188.0.2.128]"`, `for="[::ffff:188.0.2.128]:49428"`,
		`for="[0:0:0:0:0:ffff:bc15:0006]"`, `for="[64:ff9b::188.0.2.128]"`,
		`for=127.0.0.1`, `for="[::1]"`, `for="1.1.1.1`, `for="::1]"`,
		`for="[0:0:0:0:0:ffff:bc15:0006"]`, `for=1.1.1.\1`, `for= 1.1.1.1`,
		"ads\x00jkl&#*(383fdljk",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, fwd string) {
		addr := parseForwardedListItem(fwd)
		if !addr.IsValid() {
			return
		}
		back, err := ParseAddr(addr.String())
		if err != nil || back != addr {
			t.Fatalf("parseForwardedListItem(%q) = %q which does not round-trip (%q, %v)", fwd, addr, back, err)
		}
	})
}

// Fuzz_ClientIP_XFF feeds arbitrary X-Forwarded-For header values and arbitrary
// RemoteAddr values through the XFF-based resolvers and checks invariants that
// are independent of how the resolvers compute their answer -- genuine oracles
// rather than a reimplementation of the walk:
//
//   - Any non-error result is a valid IP in canonical form: re-parsing its
//     string yields the identical address.
//   - Feeding a non-error result back in as the whole header, with the same
//     RemoteAddr, yields the same result (idempotence of the full resolver).
//   - LeftmostNonPrivate / RightmostNonPrivate never return a private/local IP.
//   - RightmostTrustedRange never returns an IP inside a trusted range -- which
//     must hold whether the result came from the header or from the verified
//     peer (RemoteAddr).
//   - A TrustedPeer-wrapped resolver never returns a (forged) header value when
//     the peer is a valid IP outside the trusted ranges: a non-error result
//     implies the peer was either trusted or not a usable IP.
func Fuzz_ClientIP_XFF(f *testing.F) {
	seeds := []struct{ xff, remoteAddr string }{
		{"1.1.1.1", ""},
		{"1.1.1.1, 2.2.2.2", "10.0.0.1:80"},                // trusted peer
		{"10.0.0.1, 192.168.1.1, 3.3.3.3", "9.9.9.9:1234"}, // untrusted peer
		{"::1, 2607:f8b0:4004:83f::200e", "@"},             // Unix-socket peer
		{"  fe80::1%eth0 , 4.4.4.4 ", "[fe80::1%eth0]:9"},
		{"not-an-ip, 5.5.5.5", "ohno"}, // unparseable peer
		{"1.1.1.1,,2.2.2.2", "[2607:f8b0::1]:443"},
		{"::ffff:188.0.2.128", "192.168.1.1:5"},
		{"", "9.9.9.9"},
	}
	for _, s := range seeds {
		f.Add(s.xff, s.remoteAddr)
	}

	trustedRanges, err := ParsePrefixes("10.0.0.0/8", "192.168.0.0/16", "fc00::/7")
	if err != nil {
		f.Fatalf("ParsePrefixes failed: %v", err)
	}
	trustedRangeResolver := TrustedIPRangeFunc(func() ([]netip.Prefix, error) {
		return trustedRanges, nil
	})

	leftmost := must(NewLeftmostNonPrivate(XForwardedForKey, 64))
	rightmost := must(NewRightmostNonPrivate(XForwardedForKey))
	trusted := must(NewRightmostTrustedRange(XForwardedForKey, trustedRangeResolver))
	// A header-only resolver (which cannot self-verify the peer) wrapped in the peer gate.
	gated := must(NewTrustedPeer(trustedRangeResolver, must(NewRightmostTrustedCount(XForwardedForKey, 2))))

	// Each resolver paired with the post-condition its result must satisfy.
	cases := []struct {
		resolver fox.ClientIPResolver
		postcond func(netip.Addr) bool
		condName string
	}{
		{leftmost, func(a netip.Addr) bool { return !isContainedInRanges(a, privateAndLocalRanges) }, "non-private"},
		{rightmost, func(a netip.Addr) bool { return !isContainedInRanges(a, privateAndLocalRanges) }, "non-private"},
		{trusted, func(a netip.Addr) bool { return !isContainedInRanges(a, trustedRanges) }, "outside-trusted-ranges"},
	}

	f.Fuzz(func(t *testing.T, xff, remoteAddr string) {
		req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
		req.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		c := fox.NewTestContextOnly(w, req)

		for _, tc := range cases {
			c.Request().Header = http.Header{xForwardedForHdr: []string{xff}}
			addr, err := tc.resolver.ClientIP(c)
			if err != nil {
				continue
			}

			// Must be a valid, canonical IP.
			back, perr := ParseAddr(addr.String())
			if perr != nil || back != addr {
				t.Fatalf("%T result %q is not canonical (re-parses to %q, %v) (XFF %q remoteAddr %q)",
					tc.resolver, addr, back, perr, xff, remoteAddr)
			}

			// Post-condition for this resolver. For RightmostTrustedRange this also
			// covers the peer path: whether the result comes from RemoteAddr or from
			// the header, it must still be outside the trusted ranges.
			if !tc.postcond(addr) {
				t.Fatalf("%T result %q violates %s (XFF %q remoteAddr %q)", tc.resolver, addr, tc.condName, xff, remoteAddr)
			}

			// Idempotence: feeding the result back as the whole header, with the same
			// RemoteAddr, yields it.
			c.Request().Header = http.Header{xForwardedForHdr: []string{addr.String()}}
			again, err := tc.resolver.ClientIP(c)
			if err != nil || again != addr {
				t.Fatalf("%T not idempotent: %q -> (%q, %v) (remoteAddr %q)", tc.resolver, addr, again, err, remoteAddr)
			}
		}

		// The peer gate must never let a header value through when the peer is a valid IP
		// outside the trusted ranges. A non-error result therefore implies the peer was
		// trusted or not a usable IP. (We don't assert the value itself -- when the peer is
		// trusted the gate is transparent to the inner resolver, whose output the other
		// invariants don't constrain here.)
		c.Request().Header = http.Header{xForwardedForHdr: []string{xff}}
		if addr, err := gated.ClientIP(c); err == nil {
			if back, perr := ParseAddr(addr.String()); perr != nil || back != addr {
				t.Fatalf("TrustedPeer result %q is not canonical (XFF %q remoteAddr %q)", addr, xff, remoteAddr)
			}
			if peer, perr := ParseAddr(remoteAddr); perr == nil && !isContainedInRanges(peer, trustedRanges) {
				t.Fatalf("TrustedPeer leaked header for untrusted peer: result %q (XFF %q remoteAddr %q)", addr, xff, remoteAddr)
			}
		}
	})
}
