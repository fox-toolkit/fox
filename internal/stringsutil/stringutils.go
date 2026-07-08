// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package stringsutil

import "strings"

// EqualStringsASCIIIgnoreCase performs case-insensitive comparison of two strings
// containing ASCII characters. Only supports ASCII letters (A-Z, a-z), digits (0-9), hyphen (-) and underscore (_).
// Used for hostname matching where registered routes follow LDH standard.
func EqualStringsASCIIIgnoreCase(s1, s2 string) bool {
	// Easy case.
	if len(s1) != len(s2) {
		return false
	}
	for i := 0; i < len(s1); i++ {
		if !EqualASCIIIgnoreCase(s1[i], s2[i]) {
			return false
		}
	}
	return true
}

// EqualASCIIIgnoreCase performs case-insensitive comparison of two ASCII bytes.
// Only supports ASCII letters (A-Z, a-z), digits (0-9), hyphen (-) and underscore (_).
// Used for hostname matching where registered routes follow LDH standard.
func EqualASCIIIgnoreCase(s, t uint8) bool {
	// Easy case.
	if t == s {
		return true
	}

	// Make s < t to simplify what follows.
	if t < s {
		t, s = s, t
	}

	// ASCII only, s/t must be upper/lower case
	if 'A' <= s && s <= 'Z' && t == s+'a'-'A' {
		return true
	}

	return false
}

// ToLowerASCII converts an ASCII uppercase letter (A-Z) to lowercase (a-z).
// All other bytes are returned unchanged. Does not validate ASCII range;
func ToLowerASCII(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// IsUnreserved reports whether b is an RFC 3986 unreserved character:
// ALPHA / DIGIT / "-" / "." / "_" / "~". Unreserved characters are the only
// ones that can be safely percent-decoded without changing the meaning of a URI.
func IsUnreserved(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9' ||
		b == '-' || b == '.' || b == '_' || b == '~'
}

// IsRoutableRaw reports whether b can appear raw (outside an escape sequence) in a
// routing path, i.e. whether [NormalizeRawPath] can emit it raw. Derived from net/url
// path escaping and pinned by a differential test.
func IsRoutableRaw(b byte) bool {
	switch b {
	case '$', '&', '+', ',', '/', ':', ';', '=', '@', // never escaped by net/url in a path
		'!', '\'', '(', ')', '*', '[', ']': // kept raw when present on the wire
		return true
	}
	return IsUnreserved(b)
}

// DecodeHexPair decodes two ASCII hexadecimal digits into a byte. It returns
// false if either character is not a hexadecimal digit.
func DecodeHexPair(hi, lo byte) (byte, bool) {
	h, ok := hexDigit(hi)
	if !ok {
		return 0, false
	}
	l, ok := hexDigit(lo)
	if !ok {
		return 0, false
	}
	return h<<4 | l, true
}

func hexDigit(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// UpperHex converts an ASCII lowercase hexadecimal digit (a-f) to uppercase (A-F).
// All other bytes are returned unchanged.
func UpperHex(c byte) byte {
	if 'a' <= c && c <= 'f' {
		return c - ('a' - 'A')
	}
	return c
}

const upperhex = "0123456789ABCDEF"

// NormalizeRawPath returns the canonical routing form of the escaped path raw, verifying that
// raw is an encoding of the decoded path (see [url.URL.RawPath]): percent-encoded unreserved
// characters (see [IsUnreserved]) are decoded, the remaining hex sequences are normalized to
// uppercase and bytes that cannot appear raw in a routing path (see [IsRoutableRaw]) are
// percent-encoded in place. The path is kept as-is from the first malformed escape and the
// input is returned unchanged, without allocation, when already canonical.
// It reports whether raw is well-formed (no malformed escape, no non-routable raw byte) and
// whether raw is an encoding of path; when consistent is false, norm is empty and the routing
// path must be derived from path instead.
func NormalizeRawPath(raw, path string) (norm string, wellFormed, consistent bool) {
	var buf strings.Builder
	wellFormed = true
	frozen := false
	j := 0     // cursor into path, raw must decode to path byte for byte
	start := 0 // start of the pending run copied as-is from raw
	i := 0
	for i < len(raw) {
		c := raw[i]
		if c == '%' {
			var b byte
			var ok bool
			if i+2 < len(raw) {
				b, ok = DecodeHexPair(raw[i+1], raw[i+2])
			}
			if !ok {
				// Malformed or truncated escape, the remainder is kept as-is by the final
				// flush. A dangling '%' must not recombine with a following escape into a
				// valid sequence, and no semantics is invented for the tail.
				wellFormed, frozen = false, true
				break
			}
			if j >= len(path) || path[j] != b {
				return "", false, false
			}
			j++
			hi, lo := UpperHex(raw[i+1]), UpperHex(raw[i+2])
			switch {
			case IsUnreserved(b):
				if buf.Len() == 0 {
					buf.Grow(len(raw))
				}
				buf.WriteString(raw[start:i])
				buf.WriteByte(b)
				start = i + 3
			case raw[i+1] != hi || raw[i+2] != lo:
				if buf.Len() == 0 {
					buf.Grow(len(raw))
				}
				buf.WriteString(raw[start:i])
				buf.WriteByte('%')
				buf.WriteByte(hi)
				buf.WriteByte(lo)
				start = i + 3
			}
			i += 3
			continue
		}
		if j >= len(path) || path[j] != c {
			return "", false, false
		}
		j++
		if !IsRoutableRaw(c) {
			wellFormed = false
			if buf.Len() == 0 {
				buf.Grow(len(raw) + 8)
			}
			buf.WriteString(raw[start:i])
			buf.WriteByte('%')
			buf.WriteByte(upperhex[c>>4])
			buf.WriteByte(upperhex[c&0x0F])
			start = i + 1
		}
		i++
	}
	// A frozen tail commits to raw, the remainder is not checked against path.
	if !frozen && j != len(path) {
		return "", false, false
	}
	if buf.Len() == 0 {
		return raw, wellFormed, true
	}
	buf.WriteString(raw[start:])
	return buf.String(), wellFormed, true
}
