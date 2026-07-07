// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package stringsutil

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEqualASCIIIgnoreCase(t *testing.T) {
	cases := []struct {
		name string
		s    uint8
		t    uint8
		want bool
	}{
		{"same lowercase letter", 'a', 'a', true},
		{"same uppercase letter", 'A', 'A', true},
		{"same digit", '5', '5', true},
		{"same hyphen", '-', '-', true},
		{"A and a", 'A', 'a', true},
		{"a and A", 'a', 'A', true},
		{"Z and z", 'Z', 'z', true},
		{"z and Z", 'z', 'Z', true},
		{"M and m", 'M', 'm', true},
		{"m and M", 'm', 'M', true},
		{"A and B", 'A', 'B', false},
		{"a and b", 'a', 'b', false},
		{"A and b", 'A', 'b', false},
		{"a and B", 'a', 'B', false},
		{"0 and 0", '0', '0', true},
		{"9 and 9", '9', '9', true},
		{"0 and 1", '0', '1', false},
		{"5 and 6", '5', '6', false},
		{"hyphen and hyphen", '-', '-', true},
		{"hyphen and A", '-', 'A', false},
		{"hyphen and a", '-', 'a', false},
		{"hyphen and 0", '-', '0', false},
		{"@ and A", '@', 'A', false},
		{"Z and [", 'Z', '[', false},
		{"` and a", '`', 'a', false},
		{"z and {", 'z', '{', false},
		{"null and A", 0, 'A', false},
		{"A and null", 'A', 0, false},
		{"space and A", ' ', 'A', false},
		{"A and space", 'A', ' ', false},
		{"! and A", '!', 'A', false},
		{"A and !", 'A', '!', false},
		{"/ and A", '/', 'A', false},
		{"A and /", 'A', '/', false},
		{"high byte and A", 0xFF, 'A', false},
		{"A and high byte", 'A', 0xFF, false},
		{"high byte and a", 0xFF, 'a', false},
		{"a and high byte", 'a', 0xFF, false},
		{"@ and `", '@', '`', false},
		{"0 and P", '0', 'P', false},
		{"A-1 and a", 'A' - 1, 'a', false},
		{"Z+1 and z", 'Z' + 1, 'z', false},
		{"a-1 and A", 'a' - 1, 'A', false},
		{"z+1 and Z", 'z' + 1, 'Z', false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, EqualASCIIIgnoreCase(tc.s, tc.t))
		})
	}
}

func TestEqualStringsASCIIIgnoreCase(t *testing.T) {
	cases := []struct {
		name string
		s1   string
		s2   string
		want bool
	}{
		{"empty strings", "", "", true},
		{"empty and non-empty", "", "a", false},
		{"same lowercase", "hello", "hello", true},
		{"same uppercase", "HELLO", "HELLO", true},
		{"same mixed", "HeLLo", "HeLLo", true},
		{"different case simple", "hello", "HELLO", true},
		{"different case mixed", "HeLLo", "hEllO", true},
		{"different length 1", "hello", "helloworld", false},
		{"different length 2", "helloworld", "hello", false},
		{"different content", "hello", "world", false},
		{"different content case", "HELLO", "world", false},
		{"with digits same", "test123", "TEST123", true},
		{"with digits different", "test123", "test456", false},
		{"with hyphens", "hello-world", "HELLO-WORLD", true},
		{"with underscore", "hello_world", "HELLO_WORLD", true},
		{"hostname like", "example.com", "EXAMPLE.COM", true},
		{"subdomain", "api-v2.example.com", "API-V2.EXAMPLE.COM", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, EqualStringsASCIIIgnoreCase(tc.s1, tc.s2))
		})
	}
}

func TestToLowerASCII(t *testing.T) {
	cases := []struct {
		name string
		b    byte
		want byte
	}{
		{"uppercase A", 'A', 'a'},
		{"uppercase Z", 'Z', 'z'},
		{"uppercase M", 'M', 'm'},
		{"lowercase a", 'a', 'a'},
		{"lowercase z", 'z', 'z'},
		{"lowercase m", 'm', 'm'},
		{"digit 0", '0', '0'},
		{"digit 9", '9', '9'},
		{"digit 5", '5', '5'},
		{"hyphen", '-', '-'},
		{"underscore", '_', '_'},
		{"dot", '.', '.'},
		{"space", ' ', ' '},
		{"before A", 'A' - 1, 'A' - 1},
		{"after Z", 'Z' + 1, 'Z' + 1},
		{"before a", 'a' - 1, 'a' - 1},
		{"after z", 'z' + 1, 'z' + 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ToLowerASCII(tc.b))
		})
	}
}

func TestNormalizeRoutingPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty string", "", ""},
		{"plain ascii", "/foo/bar", "/foo/bar"},
		{"no encoding", "/users/hello/world", "/users/hello/world"},
		{"uppercase hex pair", "/caf%C3%A9", "/caf%C3%A9"},
		{"uppercase encoded slash", "/foo%2Fbar", "/foo%2Fbar"},
		{"multiple uppercase", "/%C3%A9/%E2%82%AC", "/%C3%A9/%E2%82%AC"},
		{"lowercase hex pair", "/caf%c3%a9", "/caf%C3%A9"},
		{"lowercase encoded slash", "/foo%2fbar", "/foo%2Fbar"},
		{"mixed case hi lowercase", "/caf%c3%A9", "/caf%C3%A9"},
		{"mixed case lo lowercase", "/caf%C3%a9", "/caf%C3%A9"},
		{"multiple lowercase", "/%c3%a9/%e2%82%ac", "/%C3%A9/%E2%82%AC"},
		{"mixed sequences", "/foo%C3%a9/bar%2f", "/foo%C3%A9/bar%2F"},
		{"lowercase at start", "%c3%a9/foo", "%C3%A9/foo"},
		{"lowercase at end", "/foo/%c3%a9", "/foo/%C3%A9"},
		{"uppercase then lowercase", "/%C3%A9/%c3%a9", "/%C3%A9/%C3%A9"},
		{"three byte utf8 lowercase", "/%e2%82%ac", "/%E2%82%AC"},
		{"four byte utf8 lowercase", "/%f0%90%8d%88", "/%F0%90%8D%88"},
		{"encoded space", "/hello%20world", "/hello%20world"},
		{"encoded path segment", "/users/caf%c3%a9/profile", "/users/caf%C3%A9/profile"},
		{"encoded slash in path", "/api/v1/foo%2fbar/baz", "/api/v1/foo%2Fbar/baz"},
		{"double encoding preserved", "/foo%252fbar", "/foo%252fbar"},
		{"encoded digits decoded", "%20%30%09", "%200%09"},
		{"encoded percent kept encoded digits decoded", "/%25%30%39", "/%2509"},
		{"encoded lowercase letter decoded", "/a%61b", "/aab"},
		{"encoded uppercase letters decoded", "/%41%42%43", "/ABC"},
		{"encoded tilde decoded", "/%7E", "/~"},
		{"encoded tilde lowercase hex decoded", "/%7e", "/~"},
		{"encoded marks decoded", "/%2D%5F%2E", "/-_."},
		{"decode and uppercase mixed", "/caf%c3%a9/%61", "/caf%C3%A9/a"},
		{"encoded dot segment decoded", "/%2E%2E/", "/../"},
		{"trailing percent", "/100%", "/100%"},
		{"truncated escape", "/%2", "/%2"},
		{"invalid hex digits", "/%zz", "/%zz"},
		{"invalid escape then text", "/%g1abc", "/%g1abc"},
		{"invalid escape then valid escape", "/%zz%61", "/%zz%61"},
		{"invalid second hex digit", "/%4z", "/%4z"},
		{"valid escape then invalid escape", "/a%61%zz", "/aa%zz"},
		{"valid escape then truncated hex", "/%61%4z", "/a%4z"},
		{"normalized escape then trailing percent", "/caf%c3%a9/100%", "/caf%C3%A9/100%"},
		{"percent before valid escape", "/%%41", "/%%41"},
		{"malformed escape does not recombine", "/a%2%46b", "/a%2%46b"},
		{"malformed escape after decoded escape stops normalization", "/%61%2%46b", "/a%2%46b"},
		{"raw plus and star untouched", "/a+b/c*d", "/a+b/c*d"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeRoutingPath(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestIsRoutableRaw_DifferentialNetURL pins the routable-raw byte set to net/url:
// a byte is live raw exactly when a wire request-target can carry it unencoded,
// i.e. when EscapedPath returns the raw path unchanged.
func TestIsRoutableRaw_DifferentialNetURL(t *testing.T) {
	for b := 0; b <= 0xFF; b++ {
		c := byte(b)
		raw := "/a" + string([]byte{c}) + "b"
		wire := false
		if u, err := url.ParseRequestURI(raw); err == nil && u.EscapedPath() == raw {
			wire = true
		}
		assert.Equal(t, wire, IsRoutableRaw(c), "byte 0x%02X (%q)", b, string(c))
	}
}

func TestNormalizeRoutingPathNoAlloc(t *testing.T) {
	noAllocCases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"plain path", "/foo/bar/baz"},
		{"already uppercase", "/caf%C3%A9"},
		{"multiple uppercase", "/%C3%A9/%2F/%20"},
		{"no encoding", "/users/hello"},
		{"raw plus and star", "/a+b/c*d"},
		{"malformed escape", "/100%"},
	}

	for _, tc := range noAllocCases {
		t.Run(tc.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				_ = NormalizeRoutingPath(tc.in)
			})
			assert.Equal(t, float64(0), allocs)
		})
	}
}
