// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package fox

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var mergeSlashesTests = []struct {
	path, want string
}{
	// Already merged
	{"", ""},
	{"/", "/"},
	{"/abc", "/abc"},
	{"/a/b/c", "/a/b/c"},
	{"/a/b/c/", "/a/b/c/"},
	{"*", "*"},

	// Consecutive slashes
	{"//", "/"},
	{"///", "/"},
	{"//abc", "/abc"},
	{"///abc", "/abc"},
	{"/abc//", "/abc/"},
	{"/abc///def//ghi", "/abc/def/ghi"},
	{"//abc//", "/abc/"},
	{"/a//b", "/a/b"},

	// Encoded slashes are not merged
	{"/a/%2F/b", "/a/%2F/b"},
	{"/a%2F%2Fb", "/a%2F%2Fb"},
	{"/a/%2F//b", "/a/%2F/b"},

	// Dot segments are preserved
	{"/a//../b", "/a/../b"},
	{"/a//./b", "/a/./b"},
}

func TestMergeSlashes(t *testing.T) {
	for _, tc := range mergeSlashesTests {
		assert.Equalf(t, tc.want, MergeSlashes(tc.path), "MergeSlashes(%q)", tc.path)
		assert.Equalf(t, tc.want, MergeSlashes(tc.want), "MergeSlashes(%q)", tc.want)
	}
}

var collapseDotSegmentsTests = []struct {
	path, want string
	reject     bool
}{
	// No dot segments
	{path: "", want: ""},
	{path: "/", want: "/"},
	{path: "/abc", want: "/abc"},
	{path: "/a/b/c/", want: "/a/b/c/"},
	{path: "/...", want: "/..."},
	{path: "/a/..b/c", want: "/a/..b/c"},
	{path: "/a/b../c", want: "/a/b../c"},
	{path: "/.well-known/a", want: "/.well-known/a"},
	{path: "*", want: "*"},

	// "." segments
	{path: "/./abc", want: "/abc"},
	{path: "/abc/./def", want: "/abc/def"},
	{path: "/abc/.", want: "/abc/"},
	{path: "/abc/./", want: "/abc/"},
	{path: "/./././", want: "/"},
	{path: ".", want: ""},
	{path: "./", want: ""},

	// ".." segments
	{path: "/abc/def/../jkl", want: "/abc/jkl"},
	{path: "/abc/def/..", want: "/abc/"},
	{path: "/abc/def/../", want: "/abc/"},
	{path: "/abc/def/../ghi/../jkl", want: "/abc/jkl"},
	{path: "/abc/def/../..", want: "/"},
	{path: "/abc/def/../../", want: "/"},
	{path: "a/../b", want: "/b"},
	{path: "a/..", want: "/"},

	// Empty segments are real segments (RFC 3986), consecutive slashes preserved
	{path: "/a//b", want: "/a//b"},
	{path: "/a//../b", want: "/a/b"},
	{path: "//../x", want: "/x"},
	{path: "//..", want: "/"},
	{path: "/a/.//b", want: "/a//b"},
	{path: "//a/../b", want: "//b"},

	// Escaping above the root is rejected
	{path: "/..", reject: true},
	{path: "/../", reject: true},
	{path: "/../abc", reject: true},
	{path: "/abc/../..", reject: true},
	{path: "/abc/../../def", reject: true},
	{path: "/abc/def/../../..", reject: true},
	{path: "..", reject: true},
	{path: "../", reject: true},
	{path: "../abc", reject: true},
	{path: "./..", reject: true},
}

func TestCollapseDotSegments(t *testing.T) {
	for _, tc := range collapseDotSegmentsTests {
		got, ok := CollapseDotSegments(tc.path)
		if tc.reject {
			assert.Falsef(t, ok, "CollapseDotSegments(%q)", tc.path)
			continue
		}
		assert.Truef(t, ok, "CollapseDotSegments(%q)", tc.path)
		assert.Equalf(t, tc.want, got, "CollapseDotSegments(%q)", tc.path)
		got, ok = CollapseDotSegments(got)
		assert.Truef(t, ok, "CollapseDotSegments(%q)", tc.want)
		assert.Equalf(t, tc.want, got, "CollapseDotSegments(%q)", tc.want)
	}
}

func TestNormalizeHelpers_Mallocs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping malloc count in short mode")
	}

	for _, tc := range mergeSlashesTests {
		allocs := testing.AllocsPerRun(100, func() { MergeSlashes(tc.want) })
		assert.Zerof(t, allocs, "MergeSlashes(%q)", tc.want)
	}
	for _, tc := range collapseDotSegmentsTests {
		if tc.reject {
			continue
		}
		allocs := testing.AllocsPerRun(100, func() { CollapseDotSegments(tc.want) })
		assert.Zerof(t, allocs, "CollapseDotSegments(%q)", tc.want)
	}
}

func BenchmarkMergeSlashes(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, tc := range mergeSlashesTests {
			MergeSlashes(tc.path)
		}
	}
}

func BenchmarkCollapseDotSegments(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, tc := range collapseDotSegmentsTests {
			CollapseDotSegments(tc.path)
		}
	}
}

func Test_fixTrailingSlash(t *testing.T) {
	assert.Equal(t, "/foo/", fixTrailingSlash("/foo"))
	assert.Equal(t, "/foo", fixTrailingSlash("/foo/"))
	assert.Equal(t, "/", fixTrailingSlash(""))
}

func Test_hasDotSegment(t *testing.T) {
	for _, path := range []string{"/.", "/..", "/./", "/../", "/a/.", "/a/..", "/a/./b", "/a/../b", "/a//../b", ".", ".."} {
		assert.Truef(t, hasDotSegment(path), "path %s", path)
	}
	for _, path := range []string{"", "/", "/foo", "/a.b", "/a..b", "/...", "/a/...", "/.well-known/a", "/foo/.bar", "/foo/..bar", "/foo/bar.", "/foo/bar.."} {
		assert.Falsef(t, hasDotSegment(path), "path %s", path)
	}
}

func Test_escapeLeadingSlashes(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "protocol relative URL double forward slash",
			uri:  "//evil.com",
			want: "/%2Fevil.com",
		},
		{
			name: "forward slash backslash",
			uri:  "/\\evil.com",
			want: "/%5Cevil.com",
		},
		{
			name: "backslash forward slash",
			uri:  "\\/evil.com",
			want: "\\%2Fevil.com",
		},
		{
			name: "double backslash",
			uri:  "\\\\evil.com",
			want: "\\%5Cevil.com",
		},
		{
			name: "only double forward slash",
			uri:  "//",
			want: "//",
		},
		{
			name: "safe forward slash",
			uri:  "/example.com",
			want: "/example.com",
		},
		{
			name: "safe backslash",
			uri:  "\\example.com",
			want: "\\example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeLeadingSlashes(tc.uri)
			assert.Equal(t, tc.want, got)
		})
	}
}

func FuzzCollapseDotSegments(f *testing.F) {
	for _, tc := range collapseDotSegmentsTests {
		f.Add(tc.path)
	}
	f.Add("/a/b/../../..")
	f.Add("/a//b/./../c/%2E%2E/d/..")

	f.Fuzz(func(t *testing.T, path string) {
		got, ok := CollapseDotSegments(path)
		if !ok {
			return
		}
		assert.False(t, hasDotSegment(got))
		again, ok := CollapseDotSegments(got)
		assert.True(t, ok)
		assert.Equal(t, got, again)
	})
}

func FuzzMergeSlashes(f *testing.F) {
	for _, tc := range mergeSlashesTests {
		f.Add(tc.path)
	}

	f.Fuzz(func(t *testing.T, path string) {
		got := MergeSlashes(path)
		assert.NotContains(t, got, "//")
		assert.Equal(t, strings.ReplaceAll(path, "/", ""), strings.ReplaceAll(got, "/", ""))
		assert.Equal(t, got, MergeSlashes(got))
		if !strings.Contains(path, "//") {
			assert.Equal(t, path, got)
		}
	})
}

func FuzzHasEmptyOrDotSegment(f *testing.F) {
	for _, tc := range collapseDotSegmentsTests {
		f.Add(tc.path)
	}
	for _, tc := range mergeSlashesTests {
		f.Add(tc.path)
	}

	f.Fuzz(func(t *testing.T, path string) {
		if !strings.HasPrefix(path, "/") {
			t.Skip()
		}
		composed, ok := CollapseDotSegments(MergeSlashes(path))
		changed := !ok || composed != path
		assert.Equal(t, changed, hasEmptyOrDotSegment(path))
	})
}
