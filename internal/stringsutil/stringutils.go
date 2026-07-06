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

// NormalizeRoutingPath returns the canonical routing form of an escaped path:
// percent-encoded unreserved characters (see [IsUnreserved]) are decoded and
// the remaining hex sequences are normalized to uppercase.
// The path is kept as-is from the first malformed escape sequence. The input
// is returned unchanged, without allocation, when already canonical.
func NormalizeRoutingPath(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '%' || i+2 >= len(s) {
			if buf.Len() > 0 {
				buf.WriteByte(c)
			}
			continue
		}
		b, ok := DecodeHexPair(s[i+1], s[i+2])
		if !ok {
			// Malformed escape, copy the remainder as-is and stop. A dangling
			// '%' must not recombine with a following escape into a valid sequence.
			if buf.Len() > 0 {
				buf.WriteString(s[i:])
			}
			break
		}
		hiUpper := UpperHex(s[i+1])
		loUpper := UpperHex(s[i+2])
		switch {
		case IsUnreserved(b):
			if buf.Len() == 0 {
				buf.Grow(len(s))
				buf.WriteString(s[:i])
			}
			buf.WriteByte(b)
		case s[i+1] != hiUpper || s[i+2] != loUpper:
			if buf.Len() == 0 {
				buf.Grow(len(s))
				buf.WriteString(s[:i])
			}
			buf.WriteByte('%')
			buf.WriteByte(hiUpper)
			buf.WriteByte(loUpper)
		default:
			if buf.Len() > 0 {
				buf.WriteByte('%')
				buf.WriteByte(hiUpper)
				buf.WriteByte(loUpper)
			}
		}
		i += 2
	}
	if buf.Len() == 0 {
		return s
	}
	return buf.String()
}
