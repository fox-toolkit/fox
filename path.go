// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package fox

import (
	"bytes"
	"strings"
)

const stackBufSize = 128

// MergeSlashes merges consecutive slashes in path into a single slash. Only raw slashes
// are merged, an encoded %2F is left untouched.
func MergeSlashes(path string) string {
	i := -1
	for j := 1; j < len(path); j++ {
		if path[j] == '/' && path[j-1] == '/' {
			i = j - 1
			break
		}
	}
	if i < 0 {
		return path
	}

	buf := make([]byte, 0, stackBufSize)
	w := i + 1
	for j := i + 1; j < len(path); j++ {
		if path[j] == '/' && path[j-1] == '/' {
			continue
		}
		bufApp(&buf, path, w, path[j])
		w++
	}
	if len(buf) == 0 {
		return path[:w]
	}
	return string(buf[:w])
}

// CollapseDotSegments removes "." and ".." path segments as defined by RFC 3986 section 5.2.4.
// Unlike [path.Clean], consecutive slashes are preserved and delimit empty segments that a ".."
// may consume. It returns ok=false when a ".." segment would escape above the root.
func CollapseDotSegments(path string) (_ string, ok bool) {
	i := dotSegmentIndex(path)
	if i < 0 {
		return path, true
	}

	n := len(path)
	buf := make([]byte, 0, stackBufSize)
	r, w := i, i

	for r < n {
		switch {
		case path[r] == '.' && r+1 == n:
			// "." at end, only reachable for non-rooted input.
			r++
		case path[r] == '.' && path[r+1] == '/':
			// "./" prefix, only reachable for non-rooted input.
			r += 2
		case path[r] == '.' && path[r+1] == '.' && (r+2 == n || path[r+2] == '/'):
			// ".." or "../" prefix references above the root.
			return "", false
		case path[r] == '/' && r+1 < n && path[r+1] == '.' && (r+2 == n || path[r+2] == '/'):
			// "/." at end leaves a trailing slash, "/./" is dropped.
			if r+2 == n {
				bufApp(&buf, path, w, '/')
				w++
			}
			r += 2
		case path[r] == '/' && r+2 < n && path[r+1] == '.' && path[r+2] == '.' && (r+3 == n || path[r+3] == '/'):
			// "/.." at end or "/../": pop the last segment, empty segments included.
			if w == 0 {
				return "", false
			}
			var i int
			if len(buf) == 0 {
				i = strings.LastIndexByte(path[:w], '/')
			} else {
				i = bytes.LastIndexByte(buf[:w], '/')
			}
			if i < 0 {
				i = 0
			}
			w = i
			if r+3 == n {
				// A trailing dot segment leaves a trailing slash.
				bufApp(&buf, path, w, '/')
				w++
			}
			r += 3
		default:
			// Move the segment, including its leading slash, to the output.
			if path[r] == '/' {
				bufApp(&buf, path, w, '/')
				w++
				r++
			}
			for r < n && path[r] != '/' {
				bufApp(&buf, path, w, path[r])
				w++
				r++
			}
		}
	}

	if len(buf) == 0 {
		return path[:w], true
	}
	return string(buf[:w]), true
}

// Internal helper to lazily create a buffer if necessary.
// Calls to this function get inlined.
func bufApp(buf *[]byte, s string, w int, c byte) {
	b := *buf
	if len(b) == 0 {
		// No modification of the original string so far.
		// If the next character is the same as in the original string, we do
		// not yet have to allocate a buffer.
		if s[w] == c {
			return
		}

		// Otherwise use either the stack buffer, if it is large enough, or
		// allocate a new buffer on the heap, and copy all previous characters.
		if l := len(s); l > cap(b) {
			*buf = make([]byte, len(s))
		} else {
			*buf = (*buf)[:l]
		}
		b = *buf

		copy(b, s[:w])
	}
	b[w] = c
}

// escapeLeadingSlashes prevents open redirect vulnerabilities, in the context of trailing slash redirect, by URL-encoding
// problematic character sequences at the start of URLs.
func escapeLeadingSlashes(uri string) string {
	if len(uri) > 2 && (uri[0] == '\\' || uri[0] == '/') {
		if uri[1] == '/' {
			return uri[0:1] + "%2F" + uri[2:]
		}
		if uri[1] == '\\' {
			return uri[0:1] + "%5C" + uri[2:]
		}
	}
	return uri
}

// fixTrailingSlash ensures a consistent trailing slash handling for a given path.
// If the path has more than one character and ends with a slash, it removes the trailing slash.
// Otherwise, it adds a trailing slash to the path.
func fixTrailingSlash(path string) string {
	if len(path) > 1 && path[len(path)-1] == '/' {
		return path[:len(path)-1]
	}
	return path + "/"
}

// dotSegmentIndex returns the position where the first "." or ".." path element starts,
// including its leading slash if any, or -1 if the path contains none.
func dotSegmentIndex(path string) int {
	for i := 0; ; i++ {
		j := strings.IndexByte(path[i:], '.')
		if j < 0 {
			return -1
		}
		i += j
		if i == 0 || path[i-1] == '/' {
			// "." segment: "/." at end or "/./"
			if i+1 == len(path) || path[i+1] == '/' {
				return max(i-1, 0)
			}
			// ".." segment: "/.." at end or "/../"
			if path[i+1] == '.' && (i+2 == len(path) || path[i+2] == '/') {
				return max(i-1, 0)
			}
		}
	}
}

// hasDotSegment reports whether the path contains a "." or ".." path element.
func hasDotSegment(path string) bool {
	return dotSegmentIndex(path) >= 0
}
