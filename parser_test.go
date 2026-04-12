package fox

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"testing"

	"github.com/fox-toolkit/fox/internal/iterutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePattern(t *testing.T) {
	f := MustRouter(AllowRegexpParam(true))

	staticToken := func(v string, hsplit bool) token {
		return token{
			value:  v,
			typ:    nodeStatic,
			hsplit: hsplit,
		}
	}

	paramToken := func(v, reg string) token {
		tk := token{
			value: v,
			typ:   nodeParam,
		}
		if reg != "" {
			tk.regexp = regexp.MustCompile("^" + reg + "$")
		}
		return tk
	}

	wildcardToken := func(v, reg string) token {
		tk := token{
			value: v,
			typ:   nodeWildcard,
		}
		if reg != "" {
			tk.regexp = regexp.MustCompile("^" + reg + "$")
		}
		return tk
	}

	cases := []struct {
		name             string
		path             string
		wantN            int
		wantTokens       []token
		optionalCatchAll bool
	}{
		{
			name:       "valid static route",
			path:       "/foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/foo/bar", false))),
		},
		{
			name:  "top level domain param",
			path:  "{tld}/foo/bar",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("tld", ""),
				staticToken("/foo/bar", false),
			)),
		},
		{
			name:  "top level domain wildcard",
			path:  "+{tld}/foo/bar",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("tld", ""),
				staticToken("/foo/bar", false),
			)),
		},
		{
			name:  "valid catch all route",
			path:  "/foo/bar/+{arg}",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				wildcardToken("arg", ""),
			)),
		},
		{
			name:  "valid param route",
			path:  "/foo/bar/{baz}",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				paramToken("baz", ""),
			)),
		},
		{
			name:  "valid multi params route",
			path:  "/foo/{bar}/{baz}",
			wantN: 2,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", ""),
				staticToken("/", false),
				paramToken("baz", ""),
			)),
		},
		{
			name:  "valid same params route",
			path:  "/foo/{bar}/{bar}",
			wantN: 2,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", ""),
				staticToken("/", false),
				paramToken("bar", ""),
			)),
		},
		{
			name:  "valid multi params and catch all route",
			path:  "/foo/{bar}/{baz}/+{arg}",
			wantN: 3,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", ""),
				staticToken("/", false),
				paramToken("baz", ""),
				staticToken("/", false),
				wildcardToken("arg", ""),
			)),
		},
		{
			name:  "valid inflight param",
			path:  "/foo/xyz:{bar}",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/xyz:", false),
				paramToken("bar", ""),
			)),
		},
		{
			name:  "valid inflight catchall",
			path:  "/foo/xyz:+{bar}",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/xyz:", false),
				wildcardToken("bar", ""),
			)),
		},
		{
			name:  "valid multi inflight param and catch all",
			path:  "/foo/xyz:{bar}/abc:{bar}/+{arg}",
			wantN: 3,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/xyz:", false),
				paramToken("bar", ""),
				staticToken("/abc:", false),
				paramToken("bar", ""),
				staticToken("/", false),
				wildcardToken("arg", ""),
			)),
		},
		{
			name:  "catch all with arg in the middle of the route",
			path:  "/foo/bar/+{bar}/baz",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/baz", false),
			)),
		},
		{
			name:  "multiple catch all suffix and inflight with arg in the middle of the route",
			path:  "/foo/bar/+{bar}/x+{args}/y/+{z}/{b}",
			wantN: 4,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/x", false),
				wildcardToken("args", ""),
				staticToken("/y/", false),
				wildcardToken("z", ""),
				staticToken("/", false),
				paramToken("b", ""),
			)),
		},
		{
			name:  "inflight catch all with arg in the middle of the route",
			path:  "/foo/bar/damn+{bar}/baz",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/damn", false),
				wildcardToken("bar", ""),
				staticToken("/baz", false),
			)),
		},
		{
			name:  "catch all with arg in the middle of the route and param after",
			path:  "/foo/bar/+{bar}/{baz}",
			wantN: 2,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/", false),
				paramToken("baz", ""),
			)),
		},
		{
			name:  "simple domain and path",
			path:  "foo/bar",
			wantN: 0,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("foo", true),
				staticToken("/bar", false),
			)),
		},
		{
			name:  "simple domain with trailing slash",
			path:  "foo/",
			wantN: 0,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("foo", true),
				staticToken("/", false),
			)),
		},
		{
			name:  "period in param path allowed",
			path:  "foo/{.bar}",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("foo", true),
				staticToken("/", false),
				paramToken(".bar", ""),
			)),
		},
		{
			name: "missing trailing slash after hostname",
			path: "foo.com",
		},
		{
			name:  "empty parameter",
			path:  "/foo/bar{}",
			wantN: 0,
		},
		{
			name:  "missing arguments name after catch all",
			path:  "/foo/bar/*",
			wantN: 0,
		},
		{
			name:  "missing arguments name after param",
			path:  "/foo/bar/{",
			wantN: 0,
		},
		{
			name:  "catch all in the middle of the route",
			path:  "/foo/bar/*/baz",
			wantN: 0,
		},
		{
			name:  "empty infix catch all",
			path:  "/foo/bar/+{}/baz",
			wantN: 0,
		},
		{
			name:  "empty ending catch all",
			path:  "/foo/bar/baz/+{}",
			wantN: 0,
		},
		{
			name:  "unexpected character in param",
			path:  "/foo/{{bar}",
			wantN: 0,
		},
		{
			name:  "unexpected character in param",
			path:  "/foo/{*bar}",
			wantN: 0,
		},
		{
			name:  "unexpected character in catch-all",
			path:  "/foo/+{/bar}",
			wantN: 0,
		},
		{
			name:  "catch all not supported in hostname",
			path:  "a.b.c*/",
			wantN: 0,
		},
		{
			name:  "illegal character in params hostname",
			path:  "a.b.c{/",
			wantN: 0,
		},
		{
			name:  "illegal character in hostname label",
			path:  "a.b.c}/",
			wantN: 0,
		},
		{
			name:  "unexpected character in param hostname",
			path:  "a.{.bar}.c/",
			wantN: 0,
		},
		{
			name:  "unexpected character in wildcard hostname",
			path:  "a.+{.bar}.c/",
			wantN: 0,
		},
		{
			name:  "unexpected character in param hostname",
			path:  "a.{/bar}.c/",
			wantN: 0,
		},
		{
			name:  "unexpected character in wildcard hostname",
			path:  "a.+{/bar}.c/",
			wantN: 0,
		},
		{
			name:  "in flight catch-all after param in one route segment",
			path:  "/foo/{bar}+{baz}",
			wantN: 0,
		},
		{
			name:  "multiple param in one route segment",
			path:  "/foo/{bar}{baz}",
			wantN: 0,
		},
		{
			name:  "in flight param after catch all",
			path:  "/foo/+{args}{param}",
			wantN: 0,
		},
		{
			name:  "consecutive catch all with no slash",
			path:  "/foo/+{args}+{param}",
			wantN: 0,
		},
		{
			name:  "consecutive catch all",
			path:  "/foo/+{args}/+{param}",
			wantN: 0,
		},
		{
			name:  "consecutive catch all with inflight",
			path:  "/foo/ab+{args}/+{param}",
			wantN: 0,
		},
		{
			name:  "unexpected char after inflight catch all",
			path:  "/foo/ab+{args}a",
			wantN: 0,
		},
		{
			name:  "unexpected char after catch all",
			path:  "/foo/+{args}a",
			wantN: 0,
		},
		{
			name:  "prefix catch-all in hostname",
			path:  "+{any}.com/foo",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("any", ""),
				staticToken(".com", true),
				staticToken("/foo", false),
			)),
		},
		{
			name:  "infix catch-all in hostname",
			path:  "a.+{any}.com/foo",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.", true),
				wildcardToken("any", ""),
				staticToken(".com", true),
				staticToken("/foo", false),
			)),
		},
		{
			name:  "illegal catch-all in hostname",
			path:  "a.b.+{any}/foo",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.b.", true),
				wildcardToken("any", ""),
				staticToken("/foo", false),
			)),
		},
		{
			name:  "static hostname with catch-all path",
			path:  "a.b.com/+{any}",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.b.com", true),
				staticToken("/", false),
				wildcardToken("any", ""),
			)),
		},
		{
			name:  "illegal control character in path",
			path:  "example.com/foo\x00",
			wantN: 0,
		},
		{
			name:  "illegal leading hyphen in hostname",
			path:  "-a.com/",
			wantN: 0,
		},
		{
			name:  "illegal leading dot in hostname",
			path:  ".a.com/",
			wantN: 0,
		},
		{
			name:  "illegal trailing hyphen in hostname",
			path:  "a.com-/",
			wantN: 0,
		},
		{
			name:  "illegal trailing dot in hostname",
			path:  "a.com./",
			wantN: 0,
		},
		{
			name:  "illegal trailing dot in hostname after param",
			path:  "{tld}./foo/bar",
			wantN: 0,
		},
		{
			name:  "illegal single dot in hostname",
			path:  "./",
			wantN: 0,
		},
		{
			name:  "illegal hyphen before dot",
			path:  "a-.com/",
			wantN: 0,
		},
		{
			name:  "illegal hyphen after dot",
			path:  "a.-com/",
			wantN: 0,
		},
		{
			name:  "illegal double dot",
			path:  "a..com/",
			wantN: 0,
		},
		{
			name:  "illegal double dot with param state",
			path:  "{b}..com/",
			wantN: 0,
		},
		{
			name:  "illegal double dot with inflight param state",
			path:  "a{b}..com/",
			wantN: 0,
		},
		{
			name:  "param not finishing with delimiter in hostname",
			path:  "{a}b{b}.com/",
			wantN: 0,
		},
		{
			name:  "consecutive parameter in hostname",
			path:  "{a}{b}.com/",
			wantN: 0,
		},
		{
			name:  "leading hostname label exceed 63 characters",
			path:  "uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu.b.com/",
			wantN: 0,
		},
		{
			name:  "middle hostname label exceed 63 characters",
			path:  "a.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu.com/",
			wantN: 0,
		},
		{
			name:  "trailing hostname label exceed 63 characters",
			path:  "a.b.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu/",
			wantN: 0,
		},
		{
			name:  "illegal character in domain",
			path:  "a.b!.com/",
			wantN: 0,
		},
		{
			name:  "invalid all-numeric label",
			path:  "123/",
			wantN: 0,
		},
		{
			name:  "all-numeric label with param",
			path:  "123.{a}.456/",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("123.", true),
				paramToken("a", ""),
				staticToken(".456", true),
				staticToken("/", false),
			)),
		},
		{
			name:  "all-numeric label with wildcard",
			path:  "123.+{a}.456/",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("123.", true),
				wildcardToken("a", ""),
				staticToken(".456", true),
				staticToken("/", false),
			)),
		},
		{
			name:  "all-numeric label with path wildcard",
			path:  "123.456/{abc}",
			wantN: 0,
		},
		{
			name:  "hostname exceed 255 character",
			path:  "a.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr/",
			wantN: 0,
		},
		{
			name:  "invalid all-numeric label",
			path:  "11.22.33/",
			wantN: 0,
		},
		{
			name:  "invalid uppercase label",
			path:  "ABC/",
			wantN: 0,
		},
		{
			name:  "2 regular params in domain",
			path:  "{a}.{b}.com/",
			wantN: 2,
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("a", ""),
				staticToken(".", true),
				paramToken("b", ""),
				staticToken(".com", true),
				staticToken("/", false),
			)),
		},
		{
			name:  "253 character with .",
			path:  "78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzj/",
			wantN: 0,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzj", true),
				staticToken("/", false),
			)),
		},
		{
			name:  "param does not count at character",
			path:  "{a}.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzj/",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("a", ""),
				staticToken(".78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzj", true),
				staticToken("/", false),
			)),
		},
		{
			name:  "hostname variant with multiple catch all suffix and inflight with arg in the middle of the route",
			path:  "example.com/foo/bar/+{bar}/x+{args}/y/+{z}/{b}",
			wantN: 4,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("example.com", true),
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/x", false),
				wildcardToken("args", ""),
				staticToken("/y/", false),
				wildcardToken("z", ""),
				staticToken("/", false),
				paramToken("b", ""),
			)),
		},
		{
			name:  "hostname variant with inflight catch all with arg in the middle of the route",
			path:  "example.com/foo/bar/damn+{bar}/baz",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("example.com", true),
				staticToken("/foo/bar/damn", false),
				wildcardToken("bar", ""),
				staticToken("/baz", false),
			)),
		},
		{
			name:  "hostname variant catch all with arg in the middle of the route and param after",
			path:  "example.com/foo/bar/+{bar}/{baz}",
			wantN: 2,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("example.com", true),
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/", false),
				paramToken("baz", ""),
			)),
		},
		{
			name:  "complex domain and path",
			path:  "{ab}.{c}.de{f}.com/foo/bar/+{bar}/x+{args}/y/+{z}/{b}",
			wantN: 7,
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("ab", ""),
				staticToken(".", true),
				paramToken("c", ""),
				staticToken(".de", true),
				paramToken("f", ""),
				staticToken(".com", true),
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/x", false),
				wildcardToken("args", ""),
				staticToken("/y/", false),
				wildcardToken("z", ""),
				staticToken("/", false),
				paramToken("b", ""),
			)),
		},
		{
			name: "path with double slash",
			path: "/foo//bar",
		},
		{
			name: "path with > double slash",
			path: "/foo///bar",
		},
		{
			name: "path with slash dot slash",
			path: "/foo/./bar",
		},
		{
			name: "path with slash dot slash",
			path: "/foo/././bar",
		},
		{
			name: "path with double dot parent reference",
			path: "/foo/../bar",
		},
		{
			name: "path with double dot parent reference",
			path: "/foo/../../bar",
		},
		{
			name: "path ending with slash dot",
			path: "/foo/.",
		},
		{
			name: "path ending with slash double dot",
			path: "/foo/..",
		},
		{
			name: "path ending with slash dot",
			path: "/.",
		},
		{
			name: "path ending with slash double dot",
			path: "/..",
		},
		// Allowed dot and slash combinaison
		{
			name: "last path segment starting with slash dot and text",
			path: "/foo/.bar",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/.bar", false),
			)),
		},
		{
			name: "last path segment starting with slash dot and text",
			path: "/foo/..bar",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/..bar", false),
			)),
		},
		{
			name: "path segment starting with slash dot and text",
			path: "/foo/.bar/baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/.bar/baz", false),
			)),
		},
		{
			name:  "path segment starting with slash dot and param",
			path:  "/foo/.{foo}/baz",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/.", false),
				paramToken("foo", ""),
				staticToken("/baz", false),
			)),
		},
		{
			name: "path segment starting with slash dot and text",
			path: "/foo/..bar/baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/..bar/baz", false),
			)),
		},
		{
			name:  "path segment starting with slash dot and param",
			path:  "/foo/..{foo}/baz",
			wantN: 1,
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/..", false),
				paramToken("foo", ""),
				staticToken("/baz", false),
			)),
		},
		{
			name: "path segment ending with dot slash",
			path: "/foo/bar./baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar./baz", false),
			)),
		},
		{
			name: "path segment ending with double dot slash",
			path: "/foo/bar../baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar../baz", false),
			)),
		},
		{
			name: "path segment with > double dot",
			path: "/foo/.../baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/.../baz", false),
			)),
		},
		{
			name: "path segment ending with slash and > double dot",
			path: "/foo/...",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/...", false),
			)),
		},
		{
			name: "last path segment ending with dot",
			path: "/foo/bar.",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar.", false),
			)),
		},
		{
			name: "last path segment ending with double dot",
			path: "/foo/bar..",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar..", false),
			)),
		},
		{
			name: "path segment with dot",
			path: "/foo/a.b.c",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/a.b.c", false),
			)),
		},
		{
			name: "path segment with double dot",
			path: "/foo/a..b..c",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/a..b..c", false),
			)),
		},
		// Regexp
		{
			name: "simple ending param with regexp",
			path: "/foo/{bar:[A-z]+}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", "[A-z]+"),
			)),
			wantN: 1,
		},
		{
			name: "simple ending param with regexp",
			path: "/foo/+{bar:[A-z]+}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				wildcardToken("bar", "[A-z]+"),
			)),
			wantN: 1,
		},
		{
			name: "simple infix param with regexp",
			path: "/foo/{bar:[A-z]+}/baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", "[A-z]+"),
				staticToken("/baz", false),
			)),
			wantN: 1,
		},
		{
			name: "multi infix and ending param with regexp",
			path: "/foo/{bar:[A-z]+}/{baz:[0-9]+}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", "[A-z]+"),
				staticToken("/", false),
				paramToken("baz", "[0-9]+"),
			)),
			wantN: 2,
		},
		{
			name: "multi infix and ending wildcard with regexp",
			path: "/foo/+{bar:[A-z]+}/a+{baz:[0-9]+}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				wildcardToken("bar", "[A-z]+"),
				staticToken("/a", false),
				wildcardToken("baz", "[0-9]+"),
			)),
			wantN: 2,
		},
		{
			name: "consecutive infix regexp wildcard and regexp param allowed",
			path: "/foo/+{bar:[A-z]+}/{baz:[0-9]+}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				wildcardToken("bar", "[A-z]+"),
				staticToken("/", false),
				paramToken("baz", "[0-9]+"),
			)),
			wantN: 2,
		},
		{
			name: "hostname starting with regexp",
			path: "{a:[A-z]+}.b.c/foo",
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("a", "[A-z]+"),
				staticToken(".b.c", true),
				staticToken("/foo", false),
			)),
			wantN: 1,
		},
		{
			name: "hostname with middle param regexp",
			path: "a.{b:[A-z]+}.c/foo",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.", true),
				paramToken("b", "[A-z]+"),
				staticToken(".c", true),
				staticToken("/foo", false),
			)),
			wantN: 1,
		},
		{
			name: "hostname ending with param regexp",
			path: "a.b.{c:[A-z]+}/foo",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.b.", true),
				paramToken("c", "[A-z]+"),
				staticToken("/foo", false),
			)),
			wantN: 1,
		},
		{
			name: "non capturing group allowed in regexp",
			path: "/foo/{bar:(?:foo|bar)}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", "(?:foo|bar)"),
			)),
			wantN: 1,
		},
		{
			name: "regexp wildcard at the beginning of the path",
			path: "/+{foo:[A-z]+}/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/", false),
				wildcardToken("foo", "[A-z]+"),
				staticToken("/bar", false),
			)),
			wantN: 1,
		},
		{
			name: "regexp wildcard at the beginning of the host",
			path: "+{a:[A-z]+}.b.c/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("a", "[A-z]+"),
				staticToken(".b.c", true),
				staticToken("/", false),
			)),
			wantN: 1,
		},
		{
			name: "consecutive wildcard from hostname to path",
			path: "+{foo}/+{bar}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("foo", ""),
				staticToken("/", false),
				wildcardToken("bar", ""),
			)),
			wantN: 2,
		},
		{
			name: "consecutive wildcard with empty catch all from hostname to path",
			path: "+{foo}/*{bar}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("foo", ""),
				staticToken("/", false),
				wildcardToken("bar", ""),
			)),
			wantN:            2,
			optionalCatchAll: true,
		},
		{
			name: "param then wildcard regexp",
			path: "{a}.+{b:b}/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("a", ""),
				staticToken(".", true),
				wildcardToken("b", "b"),
				staticToken("/", false),
			)),
			wantN: 2,
		},
		{
			name: "param regexp then wildcard regexp",
			path: "{a:a}.+{b:b}/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("a", "a"),
				staticToken(".", true),
				wildcardToken("b", "b"),
				staticToken("/", false),
			)),
			wantN: 2,
		},
		{
			name: "catch all empty as suffix",
			path: "/foo/*{any}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				wildcardToken("any", ""),
			)),
			wantN:            1,
			optionalCatchAll: true,
		},
		{
			name: "consecutive infix wildcard at start with regexp not allowed",
			path: "/+{foo:[A-z]+}/+{baz:[0-9]+}",
		},
		{
			name: "consecutive wildcard with catch all empty not allowed",
			path: "/+{foo}/*{baz}",
		},
		{
			name: "consecutive infix wildcard with catch all empty at start with regexp not allowed",
			path: "/+{foo:[A-z]+}/*{baz:[0-9]+}",
		},
		{
			name: "hostname consecutive infix wildcard at start with regexp not allowed",
			path: "/{foo:[A-z]+}.+{baz:[0-9]+}/",
		},
		{
			name: "consecutive infix wildcard at start with and without regexp not allowed",
			path: "/+{foo:[A-z]+}/+{baz}",
		},
		{
			name: "hostname consecutive infix wildcard at start with and without regexp not allowed",
			path: "+{foo:[A-z]+}.+{baz}/",
		},
		{
			name: "consecutive infix wildcard at start with regexp not allowed",
			path: "/+{foo}/+{baz:[0-9]+}/",
		},
		{
			name: "hostname consecutive infix wildcard at start with regexp not allowed",
			path: "+{foo}.+{baz:[0-9]+}/",
		},
		{
			name: "consecutive infix wildcard with regexp not allowed",
			path: "/foo/+{bar:[A-z]+}/+{baz:[0-9]+}",
		},
		{
			name: "hostname consecutive infix wildcard with regexp not allowed",
			path: "foo.+{bar:[A-z]+}.+{baz:[0-9]+}/",
		},
		{
			name: "consecutive infix wildcard with first regexp not allowed",
			path: "/foo/+{bar:[A-z]+}/+{baz}",
		},
		{
			name: "hostname consecutive infix wildcard with first regexp not allowed",
			path: "foo.+{bar:[A-z]+}.+{baz}/",
		},
		{
			name: "consecutive infix wildcard with second regexp not allowed",
			path: "/foo/+{bar}/+{baz:[A-z]+}/",
		},
		{
			name: "hostname consecutive infix wildcard with second regexp not allowed",
			path: "foo.+{bar}.+{baz:[A-z]+}/",
		},
		{
			name: "non slash char after regexp param not allowed",
			path: "/foo/{bar:[A-z]+}a/",
		},
		{
			name: "non slash char after regexp wildcard not allowed",
			path: "/foo/+{bar:[A-z]+}a/",
		},
		{
			name: "regexp wildcard not allowed in hostname",
			path: "+{a.{b:[A-z]+}}.c/",
		},
		{
			name: "regexp wildcard not allowed in hostname",
			path: "+{a.b.{c:[A-z]+}/",
		},
		{
			name: "missing param name with regexp",
			path: "/foo/{:[A-z]+}",
		},
		{
			name: "missing wildcard name with regexp",
			path: "/foo/+{:[A-z]+}",
		},
		{
			name: "missing regular expression",
			path: "/foo/{a:}",
		},
		{
			name: "missing regular expression with only ':'",
			path: "/foo/{:}",
		},
		{
			name: "unsupported regexp in optional wildcard",
			path: "/foo/*{any:[A-z]+}",
		},
		{
			name: "unbalanced braces in param regexp",
			path: "/foo/{bar:[A-z]+",
		},
		{
			name: "unbalanced braces in wildcard regexp",
			path: "/foo/+{bar:[A-z]+",
		},
		{
			name: "balanced braces in param regexp with invalid char after",
			path: "/foo/{bar:{}}a",
		},
		{
			name: "balanced braces in wildcard regexp with invalid brace after",
			path: "/foo/{bar:{}}}",
		},
		{
			name: "unbalanced braces in regexp complex",
			path: "/foo/{bar:{{{{}}}}",
		},
		{
			name: "invalid regular expression",
			path: "/foo/{bar:a{5,2}}",
		},
		{
			name: "invalid regular expression",
			path: "/foo/{bar:\\k}",
		},
		{
			name: "capture group in regexp are not allowed",
			path: "/foo/{bar:(foo|bar)}",
		},
		{
			name: "no opening brace after * wildcard",
			path: "/foo/*:bar}",
		},
		{
			name: "no infix catch all empty",
			path: "/foo/*{any}/bar",
		},
		{
			name: "no infix inflight catch all empty",
			path: "/foo/uuid_*{any}/bar",
		},
		{
			name: "no suffix catch all empty in hostname",
			path: "a.b.*{any}/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pat, paramCnt, err := f.parsePattern(tc.path)
			if err != nil {
				var patErr *PatternError
				require.ErrorAs(t, err, &patErr)
				return
			}
			assert.Equal(t, tc.wantN, paramCnt)
			assert.Equal(t, tc.wantTokens, pat.tokens)
			assert.Equal(t, tc.optionalCatchAll, pat.optionalCatchAll)
			if err == nil {
				assert.Equal(t, strings.IndexByte(tc.path, '/'), pat.endHost)
			}
		})
	}
}

func TestParsePatternParamsConstraint(t *testing.T) {
	t.Run("param limit", func(t *testing.T) {
		f, _ := NewRouter(WithMaxRouteParams(3))
		_, _, err := f.parsePattern("/{1}/{2}/{3}")
		assert.NoError(t, err)
		_, _, err = f.parsePattern("/{1}/{2}/{3}/{4}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/ab{1}/{2}/cd/{3}/{4}/ef")
		assert.Error(t, err)
	})
	t.Run("param key limit", func(t *testing.T) {
		f, _ := NewRouter(WithMaxRouteParamKeyBytes(3))
		_, _, err := f.parsePattern("/{abc}/{abc}/{abc}")
		assert.NoError(t, err)
		_, _, err = f.parsePattern("/{abcd}/{abc}/{abc}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{abc}/{abcd}/{abc}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{abc}/{abc}/{abcd}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{abc}/+{abcd}/{abc}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{abc}/{abc}/+{abcdef}")
		assert.Error(t, err)
	})
	t.Run("param key limit with regexp", func(t *testing.T) {
		f, _ := NewRouter(WithMaxRouteParamKeyBytes(3), AllowRegexpParam(true))
		_, _, err := f.parsePattern("/{abc:a}/{abc:a}/{abc:a}")
		assert.NoError(t, err)
		_, _, err = f.parsePattern("/{abcd:a}/{abc:a}/{abc:a}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{abc:a}/{abcd:a}/{abc:a}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{abc:a}/{abc:a}/{abcd:a}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{abc:a}/+{abcd:a}/{abc:a}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{abc:a}/{abc:a}/+{abcdef:a}")
		assert.Error(t, err)
	})
	t.Run("disabled regexp support for param", func(t *testing.T) {
		f, _ := NewRouter()
		_, _, err := f.parsePattern("/{a}/{b}/{c}")
		assert.NoError(t, err)
		// path params
		_, _, err = f.parsePattern("/{a:a}/{b}/{c}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{a}/{b:b}/{c}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{a}/{b}/{c:c}")
		assert.Error(t, err)
		// hostname params
		_, _, err = f.parsePattern("{a:a}.{b}.{c}/")
		assert.Error(t, err)
		_, _, err = f.parsePattern("{a}.{b:b}.{c}/")
		assert.Error(t, err)
		_, _, err = f.parsePattern("{a}.{b}.{c:c}/")
		assert.Error(t, err)
	})
	t.Run("disabled regexp support for wildcard", func(t *testing.T) {
		f, _ := NewRouter()
		_, _, err := f.parsePattern("/{a}/{b}/{c}")
		assert.NoError(t, err)
		// wildcard
		_, _, err = f.parsePattern("/+{a:a}/{b}/{c}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{a}/+{b:b}/{c}")
		assert.Error(t, err)
		_, _, err = f.parsePattern("/{a}/{b}/+{c:c}")
		assert.Error(t, err)
	})
}

func TestPatternErrorPositionDump(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		options []GlobalOption
	}{
		{"hostname missing trailing slash", "foo.com", nil},
		{"hostname label starts with dash", "-a.com/", nil},
		{"hostname label starts with dot", ".a.com/", nil},
		{"hostname dash after dot", "a.-b.com/", nil},
		{"hostname consecutive dots", "a..com/", nil},
		{"hostname label ends with dash", "a-.com/", nil},
		{"hostname label exceeds 63 chars at dot", "uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu.com/", nil},
		{"hostname uppercase character", "A.com/", nil},
		{"hostname illegal character in label", "a!.com/", nil},
		{"hostname trailing dash", "a.com-/", nil},
		{"hostname trailing dot", "a.com./", nil},
		{"hostname all numeric", "123/", nil},
		{"hostname trailing label exceeds 63 chars", "a.b.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu/", nil},
		{"hostname exceeds 253 characters", "a.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr/", nil},
		{"hostname missing parameter after + delimiter", "+a.com/", nil},
		{"hostname consecutive wildcard", "+{a}.+{b}.com/", nil},
		{"hostname too many parameters", "{a}.{b}.com/", []GlobalOption{WithMaxRouteParams(1)}},
		{"hostname illegal character after parameter", "{a}b.com/", nil},
		{"hostname optional wildcard not allowed", "a.*{any}/", nil},
		{"hostname bare star missing parameter", "a.b*/", nil},
		{"path missing parameter after + delimiter", "/foo/+bar", nil},
		{"path consecutive wildcard", "/+{a}/+{b}", nil},
		{"path too many parameters", "/foo/{a}/{b}", []GlobalOption{WithMaxRouteParams(1)}},
		{"path optional wildcard not as suffix", "/foo/*{any}/bar", nil},
		{"path illegal character after parameter", "/foo/{a}b", nil},
		{"path illegal control character", "/foo\x01bar", nil},
		{"path consecutive slashes", "/foo//bar", nil},
		{"path consecutive slashes with hostname", "example.com/foo//bar", nil},
		{"path dot segment single dot mid", "/foo/./bar", nil},
		{"path dot segment single dot end", "/foo/.", nil},
		{"path dot segment double dot mid", "/foo/../bar", nil},
		{"path dot segment double dot end", "/foo/..", nil},
		{"path root single dot", "/.", nil},
		{"path root double dot", "/..", nil},
		{"unbalanced braces", "/foo/{bar", nil},
		{"parameter key too large", "/foo/{abcd}", []GlobalOption{WithMaxRouteParamKeyBytes(3)}},
		{"missing parameter name", "/foo/{}", nil},
		{"illegal character in parameter name", "/foo/{*bar}", nil},
		{"regexp not allowed in optional wildcard", "/foo/*{any:[A-z]+}", nil},
		{"regexp feature not enabled", "/foo/{a:[A-z]+}", nil},
		{"regexp missing expression", "/foo/{a:}", []GlobalOption{AllowRegexpParam(true)}},
		{"regexp compile error", "/foo/{a:a{5,2}}", []GlobalOption{AllowRegexpParam(true)}},
		{"regexp capture group not allowed", "/foo/{a:(foo|bar)}", []GlobalOption{AllowRegexpParam(true)}},
	}

	for _, tc := range cases {
		f := MustRouter(tc.options...)
		_, _, err := f.parsePattern(tc.pattern)
		var pe *PatternError
		if errors.As(err, &pe) {
			fmt.Printf("%-50s start=%-3d end=%-3d\n%s\n\n", tc.name, pe.Start, pe.End, pe.Error())
		} else {
			fmt.Printf("%-50s %v\n\n", tc.name, err)
		}
	}
}

func TestPatternErrorPosition(t *testing.T) {
	cases := []struct {
		name       string
		pattern    string
		options    []GlobalOption
		wantType   string
		wantReason string
		wantStart  int
		wantEnd    int
		wantMsg    string
	}{
		{
			name:       "hostname missing trailing slash",
			pattern:    "foo.com",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  7,
			wantEnd:    7,
			wantMsg:    "missing trailing '/' after hostname",
		},
		{
			name:       "hostname label starts with dash",
			pattern:    "-a.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    1,
			wantMsg:    "label starts with '-'",
		},
		{
			name:       "hostname label starts with dot",
			pattern:    ".a.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    1,
			wantMsg:    "label starts with '.'",
		},
		{
			name:       "hostname dash after dot",
			pattern:    "a.-b.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  2,
			wantEnd:    3,
			wantMsg:    "illegal character after '.'",
		},
		{
			name:       "hostname consecutive dots",
			pattern:    "a..com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  1,
			wantEnd:    3,
			wantMsg:    "illegal consecutive '.'",
		},
		{
			name:       "hostname label ends with dash",
			pattern:    "a-.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  1,
			wantEnd:    2,
			wantMsg:    "label ends with '-'",
		},
		{
			name:       "hostname label exceeds 63 chars at dot",
			pattern:    "uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu.com/",
			wantType:   "hostname",
			wantReason: "constraint",
			wantStart:  0,
			wantEnd:    64,
			wantMsg:    "label exceeds 63 characters",
		},
		{
			name:       "hostname uppercase character",
			pattern:    "A.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    1,
			wantMsg:    "uppercase character in label",
		},
		{
			name:       "hostname illegal character in label",
			pattern:    "a!.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  1,
			wantEnd:    2,
			wantMsg:    "illegal character in label",
		},
		{
			name:       "hostname trailing dash",
			pattern:    "a.com-/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    6,
			wantMsg:    "illegal trailing '-'",
		},
		{
			name:       "hostname trailing dot",
			pattern:    "a.com./",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    6,
			wantMsg:    "illegal trailing '.'",
		},
		{
			name:       "hostname all numeric",
			pattern:    "123/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    3,
			wantMsg:    "all numeric",
		},
		{
			name:       "hostname trailing label exceeds 63 chars",
			pattern:    "a.b.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu/",
			wantType:   "hostname",
			wantReason: "constraint",
			wantStart:  4,
			wantEnd:    68,
			wantMsg:    "label exceeds 63 characters",
		},
		{
			name:       "hostname exceeds 253 characters",
			pattern:    "a.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr/",
			wantType:   "hostname",
			wantReason: "constraint",
			wantStart:  0,
			wantEnd:    256,
			wantMsg:    "exceeds 253 characters",
		},
		{
			name:       "hostname missing parameter after + delimiter",
			pattern:    "+a.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    1,
			wantMsg:    "missing parameter after delimiter",
		},
		{
			name:       "hostname consecutive wildcard",
			pattern:    "+{a}.+{b}.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    9,
			wantMsg:    "consecutive wildcard",
		},
		{
			name:       "hostname too many parameters",
			pattern:    "{a}.{b}.com/",
			options:    []GlobalOption{WithMaxRouteParams(1)},
			wantType:   "hostname",
			wantReason: "constraint",
			wantStart:  4,
			wantEnd:    7,
			wantMsg:    "too many parameters",
		},
		{
			name:       "hostname illegal character after parameter",
			pattern:    "{a}b.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  3,
			wantEnd:    4,
			wantMsg:    "illegal character after parameter",
		},
		{
			name:       "hostname optional wildcard not allowed",
			pattern:    "a.*{any}/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  2,
			wantEnd:    8,
			wantMsg:    "optional wildcard allowed only as suffix",
		},
		{
			name:       "hostname bare star missing parameter",
			pattern:    "a.b*/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  3,
			wantEnd:    4,
			wantMsg:    "missing parameter after delimiter",
		},
		{
			name:       "path missing parameter after + delimiter",
			pattern:    "/foo/+bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    6,
			wantMsg:    "missing parameter after delimiter",
		},
		{
			name:       "path consecutive wildcard",
			pattern:    "/+{a}/+{b}",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  6,
			wantEnd:    10,
			wantMsg:    "consecutive wildcard",
		},
		{
			name:       "path too many parameters",
			pattern:    "/foo/{a}/{b}",
			options:    []GlobalOption{WithMaxRouteParams(1)},
			wantType:   "path",
			wantReason: "constraint",
			wantStart:  9,
			wantEnd:    12,
			wantMsg:    "too many parameters",
		},
		{
			name:       "path optional wildcard not as suffix",
			pattern:    "/foo/*{any}/bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    11,
			wantMsg:    "optional wildcard allowed only as suffix",
		},
		{
			name:       "path illegal character after parameter",
			pattern:    "/foo/{a}b",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  8,
			wantEnd:    9,
			wantMsg:    "illegal character after parameter",
		},
		{
			name:       "path illegal control character",
			pattern:    "/foo\x01bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  4,
			wantEnd:    5,
			wantMsg:    "illegal control character",
		},
		{
			name:       "path consecutive slashes",
			pattern:    "/foo//bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  4,
			wantEnd:    6,
			wantMsg:    "consecutive '/'",
		},
		{
			name:       "path consecutive slashes with hostname",
			pattern:    "example.com/foo//bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  15,
			wantEnd:    17,
			wantMsg:    "consecutive '/'",
		},
		{
			name:       "path dot segment single dot mid",
			pattern:    "/foo/./bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  4,
			wantEnd:    7,
			wantMsg:    "dot segment",
		},
		{
			name:       "path dot segment single dot end",
			pattern:    "/foo/.",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  4,
			wantEnd:    6,
			wantMsg:    "dot segment",
		},
		{
			name:       "path dot segment double dot mid",
			pattern:    "/foo/../bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  4,
			wantEnd:    8,
			wantMsg:    "dot segment",
		},
		{
			name:       "path dot segment double dot end",
			pattern:    "/foo/..",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  4,
			wantEnd:    7,
			wantMsg:    "dot segment",
		},
		{
			name:       "path root single dot",
			pattern:    "/.",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    2,
			wantMsg:    "dot segment",
		},
		{
			name:       "path root double dot",
			pattern:    "/..",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    3,
			wantMsg:    "dot segment",
		},
		{
			name:       "unbalanced braces",
			pattern:    "/foo/{bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    9,
			wantMsg:    "unbalanced braces",
		},
		{
			name:       "parameter key too large",
			pattern:    "/foo/{abcd}",
			options:    []GlobalOption{WithMaxRouteParamKeyBytes(3)},
			wantType:   "path",
			wantReason: "constraint",
			wantStart:  6,
			wantEnd:    10,
			wantMsg:    "key too large",
		},
		{
			name:       "missing parameter name",
			pattern:    "/foo/{}",
			wantType:   "path",
			wantReason: "parameter",
			wantStart:  5,
			wantEnd:    7,
			wantMsg:    "missing name",
		},
		{
			name:       "illegal character in parameter name",
			pattern:    "/foo/{*bar}",
			wantType:   "path",
			wantReason: "parameter",
			wantStart:  6,
			wantEnd:    7,
			wantMsg:    "illegal character in name",
		},
		{
			name:       "regexp not allowed in optional wildcard",
			pattern:    "/foo/*{any:[A-z]+}",
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  5,
			wantEnd:    18,
			wantMsg:    "not allowed in optional wildcard",
		},
		{
			name:       "regexp feature not enabled",
			pattern:    "/foo/{a:[A-z]+}",
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  8,
			wantEnd:    14,
			wantMsg:    "feature not enabled",
		},
		{
			name:       "regexp missing expression",
			pattern:    "/foo/{a:}",
			options:    []GlobalOption{AllowRegexpParam(true)},
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  8,
			wantEnd:    8,
			wantMsg:    "missing expression",
		},
		{
			name:       "regexp compile error",
			pattern:    "/foo/{a:a{5,2}}",
			options:    []GlobalOption{AllowRegexpParam(true)},
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  8,
			wantEnd:    14,
			wantMsg:    "compile error",
		},
		{
			name:       "regexp capture group not allowed",
			pattern:    "/foo/{a:(foo|bar)}",
			options:    []GlobalOption{AllowRegexpParam(true)},
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  8,
			wantEnd:    17,
			wantMsg:    "capture group",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := MustRouter(tc.options...)
			_, _, err := f.parsePattern(tc.pattern)
			require.Error(t, err)
			var pe *PatternError
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, tc.wantType, pe.Type)
			assert.Equal(t, tc.wantReason, pe.Reason)
			assert.Equal(t, tc.wantStart, pe.Start)
			assert.Equal(t, tc.wantEnd, pe.End)
			assert.Contains(t, pe.Error(), tc.wantMsg)
			fmt.Println(err)
		})
	}
}

func TestPatternErrorUnwrap(t *testing.T) {
	t.Run("regexp compile error wraps underlying error", func(t *testing.T) {
		f := MustRouter(AllowRegexpParam(true))
		_, _, err := f.parsePattern("/foo/{a:a{5,2}}")
		require.Error(t, err)
		var pe *PatternError
		require.ErrorAs(t, err, &pe)
		var syntaxErr *syntax.Error
		assert.ErrorAs(t, pe, &syntaxErr)
	})
	t.Run("non-regexp error returns nil on unwrap", func(t *testing.T) {
		f := MustRouter()
		_, _, err := f.parsePattern("/foo//bar")
		require.Error(t, err)
		var pe *PatternError
		require.ErrorAs(t, err, &pe)
		assert.Nil(t, pe.Unwrap())
	})
	t.Run("empty pattern returns ErrInvalidRoute", func(t *testing.T) {
		f := MustRouter()
		_, _, err := f.parsePattern("")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRoute)
		var pe *PatternError
		assert.False(t, errors.As(err, &pe))
	})
}
