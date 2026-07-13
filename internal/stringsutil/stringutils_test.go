// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package stringsutil

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNormalizeRawPath(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		path       string
		want       string
		wellFormed bool
	}{
		{"plain ascii", "/foo/bar", "/foo/bar", "/foo/bar", true},
		{"canonical escaped slash", "/foo%2Fbar", "/foo/bar", "/foo%2Fbar", true},
		{"lowercase hex uppercased", "/foo%2fbar", "/foo/bar", "/foo%2Fbar", true},
		{"unreserved decoded", "/%61%42c", "/aBc", "/aBc", true},
		{"encoded percent kept", "/a%25b", "/a%b", "/a%25b", true},
		{"encoded utf8 kept uppercase", "/caf%c3%a9", "/café", "/caf%C3%A9", true},
		{"encoded space kept", "/hello%20world", "/hello world", "/hello%20world", true},
		{"double encoding preserved", "/foo%252fbar", "/foo%2fbar", "/foo%252fbar", true},
		{"four byte utf8 lowercase", "/%f0%90%8d%88", "/\xf0\x90\x8d\x88", "/%F0%90%8D%88", true},
		{"raw plus and star untouched", "/a+b/c*d", "/a+b/c*d", "/a+b/c*d", true},
		{"sub-delims kept raw", "/a(b)!'*,;=:@[]", "/a(b)!'*,;=:@[]", "/a(b)!'*,;=:@[]", true},
		{"raw utf8 encoded in place", "/foo%2Fcaf\xc3\xa9", "/foo/caf\xc3\xa9", "/foo%2Fcaf%C3%A9", false},
		{"raw utf8 alone", "/caf\xc3\xa9", "/caf\xc3\xa9", "/caf%C3%A9", false},
		{"raw brace encoded in place", "/foo%2Fb{r", "/foo/b{r", "/foo%2Fb%7Br", false},
		{"raw backslash encoded", "/a\\b", "/a\\b", "/a%5Cb", false},
		{"raw quote encoded", "/a\"b", "/a\"b", "/a%22b", false},
		{"encoded dot segments preserved with raw byte", "/%2E%2E/x\xc3\xa9", "/../x\xc3\xa9", "/../x%C3%A9", false},
		{"malformed escape", "/a%2%46b", "/a%2%46b", "", false},
		{"malformed escape after decoded escape", "/%61%2%46b", "/a%2%46b", "", false},
		{"trailing percent", "/100%", "/100%", "", false},
		{"invalid hex digits", "/%zz", "/%zz", "", false},
		{"decoded byte mismatch", "/a%2Fb", "/a/c", "", false},
		{"raw byte mismatch", "/abc", "/abd", "", false},
		{"path too long", "/ab", "/abc", "", false},
		{"path too short", "/abc", "/ab", "", false},
		{"empty path", "/a", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			norm, wellFormed := NormalizeRawPath(tc.raw, tc.path)
			assert.Equal(t, tc.want, norm)
			assert.Equal(t, tc.wellFormed, wellFormed)
		})
	}
}

func TestNormalizeRawPathNoAlloc(t *testing.T) {
	noAllocCases := []struct {
		name string
		raw  string
		path string
	}{
		{"plain path", "/foo/bar/baz", "/foo/bar/baz"},
		{"already canonical", "/caf%C3%A9", "/café"},
		{"canonical escaped slash", "/foo%2Fbar", "/foo/bar"},
		{"malformed", "/100%", "/100%"},
		{"inconsistent", "/abc", "/abd"},
	}

	for _, tc := range noAllocCases {
		t.Run(tc.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				_, _ = NormalizeRawPath(tc.raw, tc.path)
			})
			assert.Equal(t, float64(0), allocs)
		})
	}
}

func FuzzNormalizeRawPath_DifferentialNetURL(f *testing.F) {
	seeds := []string{
		"/foo%2Fbar", "/caf%c3%a9", "/%61%42c", "/a%25b", "/foo%2Fcaf\xc3\xa9",
		"/a(b)!'*,;=:@[]", "/a+b/c*d", "/%2E%2E/x", "/a{b}\\^`|", "/\xc3\xa9%2f%C3%A9",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, target string) {
		u, err := url.ParseRequestURI(target)
		if err != nil || u.RawPath == "" {
			t.Skip()
		}
		norm, wellFormed := NormalizeRawPath(u.RawPath, u.Path)
		require.NotEmpty(t, norm)
		if wellFormed {
			require.Equal(t, u.RawPath, u.EscapedPath())
		}

		for i := 0; i < len(norm); i++ {
			if norm[i] != '%' {
				require.True(t, IsRoutableRaw(norm[i]))
				continue
			}
			require.Less(t, i+2, len(norm))
			b, ok := DecodeHexPair(norm[i+1], norm[i+2])
			require.True(t, ok)
			require.False(t, IsUnreserved(b))
			require.Equal(t, UpperHex(norm[i+1]), norm[i+1])
			require.Equal(t, UpperHex(norm[i+2]), norm[i+2])
			i += 2
		}

		decoded, err := url.PathUnescape(norm)
		require.NoError(t, err)
		require.Equal(t, u.Path, decoded)

		again, wf := NormalizeRawPath(norm, u.Path)
		require.True(t, wf)
		require.Equal(t, norm, again)
	})
}

func TestEscapePath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"empty", "", ""},
		{"clean", "/foo/bar", "/foo/bar"},
		{"reserved kept", "/$&+,/:;=@", "/$&+,/:;=@"},
		{"unreserved kept", "/-._~Az09", "/-._~Az09"},
		{"star alone", "*", "*"},
		{"star in path", "/a*b", "/a%2Ab"},
		{"space", "/a b", "/a%20b"},
		{"percent", "/a%b", "/a%25b"},
		{"utf8", "/café", "/caf%C3%A9"},
		{"sub delims escaped", "/a(b)!'", "/a%28b%29%21%27"},
		{"high byte", "/\xff", "/%FF"},
		{"dirty first byte", " /a", "%20/a"},
		{"long heap buffer", "/segment/segment/segment/segment/segment/segment/segment/café", "/segment/segment/segment/segment/segment/segment/segment/caf%C3%A9"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, EscapePath(tc.path))
		})
	}
}

func TestEscapePath_DifferentialNetURL(t *testing.T) {
	for b := 0; b <= 0xFF; b++ {
		c := byte(b)
		s := "/a" + string([]byte{c}) + "b"
		u := &url.URL{Path: s}
		assert.Equal(t, u.EscapedPath(), EscapePath(s), "byte 0x%02X (%q)", b, string(c))
	}
}

func FuzzEscapePath_DifferentialNetURL(f *testing.F) {
	seeds := []string{
		"", "*", "/", "/users/42", "/café", "/a b/c~d", "/a%b", "/a(b)!'*,;=:@[]",
		"/api/v1/organizations/acme-corporation/projects/fox-router/download/café",
		string([]byte{0x00, 0xFF, '/'}),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, path string) {
		u := &url.URL{Path: path}
		require.Equal(t, u.EscapedPath(), EscapePath(path))
	})
}

func BenchmarkEscapePath(b *testing.B) {
	cases := []struct {
		name string
		path string
	}{
		{"clean", "/api/v1/organizations/acme-corporation/projects/fox-router"},
		{"escape stack", "/café/report"},
		{"escape heap", "/segment/segment/segment/segment/segment/segment/segment/café"},
		{"escape heap long", strings.Repeat("/segment/café", 16)},
	}
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				EscapePath(bc.path)
			}
		})
	}
}

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
