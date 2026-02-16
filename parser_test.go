package fox

import (
	"errors"
	"fmt"
	"regexp"
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
		name                 string
		path                 string
		wantStr              string // Expected parsed.str (defaults to path if empty).
		wantTokens           []token
		wantErr              bool
		wantOptionalCatchAll bool
	}{
		{
			name:       "valid static route",
			path:       "/foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/foo/bar", false))),
		},
		{
			name: "top level domain param",
			path: "{tld}/foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("tld", ""),
				staticToken("/foo/bar", false),
			)),
		},
		{
			name: "top level domain wildcard",
			path: "+{tld}/foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("tld", ""),
				staticToken("/foo/bar", false),
			)),
		},
		{
			name: "valid catch all route",
			path: "/foo/bar/+{arg}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				wildcardToken("arg", ""),
			)),
		},
		{
			name: "valid param route",
			path: "/foo/bar/{baz}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				paramToken("baz", ""),
			)),
		},
		{
			name: "valid multi params route",
			path: "/foo/{bar}/{baz}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", ""),
				staticToken("/", false),
				paramToken("baz", ""),
			)),
		},
		{
			name: "valid same params route",
			path: "/foo/{bar}/{bar}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", ""),
				staticToken("/", false),
				paramToken("bar", ""),
			)),
		},
		{
			name: "valid multi params and catch all route",
			path: "/foo/{bar}/{baz}/+{arg}",
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
			name: "valid inflight param",
			path: "/foo/xyz:{bar}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/xyz:", false),
				paramToken("bar", ""),
			)),
		},
		{
			name: "valid inflight catchall",
			path: "/foo/xyz:+{bar}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/xyz:", false),
				wildcardToken("bar", ""),
			)),
		},
		{
			name: "valid multi inflight param and catch all",
			path: "/foo/xyz:{bar}/abc:{bar}/+{arg}",
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
			name: "catch all with arg in the middle of the route",
			path: "/foo/bar/+{bar}/baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/baz", false),
			)),
		},
		{
			name: "multiple catch all suffix and inflight with arg in the middle of the route",
			path: "/foo/bar/+{bar}/x+{args}/y/+{z}/{b}",
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
			name: "inflight catch all with arg in the middle of the route",
			path: "/foo/bar/damn+{bar}/baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/damn", false),
				wildcardToken("bar", ""),
				staticToken("/baz", false),
			)),
		},
		{
			name: "catch all with arg in the middle of the route and param after",
			path: "/foo/bar/+{bar}/{baz}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/", false),
				paramToken("baz", ""),
			)),
		},
		{
			name: "simple domain and path",
			path: "foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("foo", true),
				staticToken("/bar", false),
			)),
		},
		{
			name: "simple domain with trailing slash",
			path: "foo/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("foo", true),
				staticToken("/", false),
			)),
		},
		{
			name: "period in param path allowed",
			path: "foo/{.bar}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("foo", true),
				staticToken("/", false),
				paramToken(".bar", ""),
			)),
		},
		{
			name:    "missing a least one slash",
			path:    "foo.com",
			wantErr: true,
		},
		{
			name:    "empty parameter",
			path:    "/foo/bar{}",
			wantErr: true,
		},
		{
			name:    "missing arguments name after catch all",
			path:    "/foo/bar/*",
			wantErr: true,
		},
		{
			name:    "missing arguments name after param",
			path:    "/foo/bar/{",
			wantErr: true,
		},
		{
			name:    "catch all in the middle of the route",
			path:    "/foo/bar/*/baz",
			wantErr: true,
		},
		{
			name:    "empty infix catch all",
			path:    "/foo/bar/+{}/baz",
			wantErr: true,
		},
		{
			name:    "empty ending catch all",
			path:    "/foo/bar/baz/+{}",
			wantErr: true,
		},
		{
			name:    "unexpected character in param",
			path:    "/foo/{{bar}",
			wantErr: true,
		},
		{
			name:    "unexpected character in param",
			path:    "/foo/{*bar}",
			wantErr: true,
		},
		{
			name:    "unexpected character in catch-all",
			path:    "/foo/+{/bar}",
			wantErr: true,
		},
		{
			name:    "catch all not supported in hostname",
			path:    "a.b.c*/",
			wantErr: true,
		},
		{
			name:    "illegal character in params hostname",
			path:    "a.b.c{/",
			wantErr: true,
		},
		{
			name:    "illegal character in hostname label",
			path:    "a.b.c}/",
			wantErr: true,
		},
		{
			name:    "unexpected character in param hostname",
			path:    "a.{.bar}.c/",
			wantErr: true,
		},
		{
			name:    "unexpected character in wildcard hostname",
			path:    "a.+{.bar}.c/",
			wantErr: true,
		},
		{
			name:    "unexpected character in param hostname",
			path:    "a.{/bar}.c/",
			wantErr: true,
		},
		{
			name:    "unexpected character in wildcard hostname",
			path:    "a.+{/bar}.c/",
			wantErr: true,
		},
		{
			name:    "in flight catch-all after param in one route segment",
			path:    "/foo/{bar}+{baz}",
			wantErr: true,
		},
		{
			name:    "multiple param in one route segment",
			path:    "/foo/{bar}{baz}",
			wantErr: true,
		},
		{
			name:    "in flight param after catch all",
			path:    "/foo/+{args}{param}",
			wantErr: true,
		},
		{
			name:    "consecutive catch all with no slash",
			path:    "/foo/+{args}+{param}",
			wantErr: true,
		},
		{
			name:    "consecutive catch all",
			path:    "/foo/+{args}/+{param}",
			wantErr: true,
		},
		{
			name:    "consecutive catch all with inflight",
			path:    "/foo/ab+{args}/+{param}",
			wantErr: true,
		},
		{
			name:    "unexpected char after inflight catch all",
			path:    "/foo/ab+{args}a",
			wantErr: true,
		},
		{
			name:    "unexpected char after catch all",
			path:    "/foo/+{args}a",
			wantErr: true,
		},
		{
			name: "prefix catch-all in hostname",
			path: "+{any}.com/foo",
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("any", ""),
				staticToken(".com", true),
				staticToken("/foo", false),
			)),
		},
		{
			name: "infix catch-all in hostname",
			path: "a.+{any}.com/foo",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.", true),
				wildcardToken("any", ""),
				staticToken(".com", true),
				staticToken("/foo", false),
			)),
		},
		{
			name: "illegal catch-all in hostname",
			path: "a.b.+{any}/foo",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.b.", true),
				wildcardToken("any", ""),
				staticToken("/foo", false),
			)),
		},
		{
			name: "static hostname with catch-all path",
			path: "a.b.com/+{any}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.b.com", true),
				staticToken("/", false),
				wildcardToken("any", ""),
			)),
		},
		{
			name:    "illegal control character in path",
			path:    "example.com/foo\x00",
			wantErr: true,
		},
		{
			name:    "illegal leading hyphen in hostname",
			path:    "-a.com/",
			wantErr: true,
		},
		{
			name:    "illegal leading dot in hostname",
			path:    ".a.com/",
			wantErr: true,
		},
		{
			name:    "illegal trailing hyphen in hostname",
			path:    "a.com-/",
			wantErr: true,
		},
		{
			name:    "illegal trailing dot in hostname",
			path:    "a.com./",
			wantErr: true,
		},
		{
			name:    "illegal trailing dot in hostname after param",
			path:    "{tld}./foo/bar",
			wantErr: true,
		},
		{
			name:    "illegal single dot in hostname",
			path:    "./",
			wantErr: true,
		},
		{
			name:    "illegal hyphen before dot",
			path:    "a-.com/",
			wantErr: true,
		},
		{
			name:    "illegal hyphen after dot",
			path:    "a.-com/",
			wantErr: true,
		},
		{
			name:    "illegal double dot",
			path:    "a..com/",
			wantErr: true,
		},
		{
			name:    "illegal double dot with param state",
			path:    "{b}..com/",
			wantErr: true,
		},
		{
			name:    "illegal double dot with inflight param state",
			path:    "a{b}..com/",
			wantErr: true,
		},
		{
			name:    "param not finishing with delimiter in hostname",
			path:    "{a}b{b}.com/",
			wantErr: true,
		},
		{
			name:    "consecutive parameter in hostname",
			path:    "{a}{b}.com/",
			wantErr: true,
		},
		{
			name:    "leading hostname label exceed 63 characters",
			path:    "uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu.b.com/",
			wantErr: true,
		},
		{
			name:    "middle hostname label exceed 63 characters",
			path:    "a.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu.com/",
			wantErr: true,
		},
		{
			name:    "trailing hostname label exceed 63 characters",
			path:    "a.b.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu/",
			wantErr: true,
		},
		{
			name:    "illegal character in domain",
			path:    "a.b!.com/",
			wantErr: true,
		},
		{
			name:    "invalid all-numeric label",
			path:    "123/",
			wantErr: true,
		},
		{
			name: "all-numeric label with param",
			path: "123.{a}.456/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("123.", true),
				paramToken("a", ""),
				staticToken(".456", true),
				staticToken("/", false),
			)),
		},
		{
			name: "all-numeric label with wildcard",
			path: "123.+{a}.456/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("123.", true),
				wildcardToken("a", ""),
				staticToken(".456", true),
				staticToken("/", false),
			)),
		},
		{
			name:    "all-numeric label with path wildcard",
			path:    "123.456/{abc}",
			wantErr: true,
		},
		{
			name:    "hostname exceed 255 character",
			path:    "a.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr/",
			wantErr: true,
		},
		{
			name:    "invalid all-numeric label",
			path:    "11.22.33/",
			wantErr: true,
		},
		{
			name:    "invalid uppercase label",
			path:    "ABC/",
			wantErr: true,
		},
		{
			name: "2 regular params in domain",
			path: "{a}.{b}.com/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("a", ""),
				staticToken(".", true),
				paramToken("b", ""),
				staticToken(".com", true),
				staticToken("/", false),
			)),
		},
		{
			name: "253 character with .",
			path: "78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzj/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzj", true),
				staticToken("/", false),
			)),
		},
		{
			name: "param does not count at character",
			path: "{a}.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzj/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("a", ""),
				staticToken(".78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzj", true),
				staticToken("/", false),
			)),
		},
		{
			name: "hostname variant with multiple catch all suffix and inflight with arg in the middle of the route",
			path: "example.com/foo/bar/+{bar}/x+{args}/y/+{z}/{b}",
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
			name: "hostname variant with inflight catch all with arg in the middle of the route",
			path: "example.com/foo/bar/damn+{bar}/baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("example.com", true),
				staticToken("/foo/bar/damn", false),
				wildcardToken("bar", ""),
				staticToken("/baz", false),
			)),
		},
		{
			name: "hostname variant catch all with arg in the middle of the route and param after",
			path: "example.com/foo/bar/+{bar}/{baz}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("example.com", true),
				staticToken("/foo/bar/", false),
				wildcardToken("bar", ""),
				staticToken("/", false),
				paramToken("baz", ""),
			)),
		},
		{
			name: "complex domain and path",
			path: "{ab}.{c}.de{f}.com/foo/bar/+{bar}/x+{args}/y/+{z}/{b}",
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
		// CleanPath normalizes traversal patterns instead of rejecting them.
		{
			name:       "double slash cleaned",
			path:       "/foo//bar",
			wantStr:    "/foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/foo/bar", false))),
		},
		{
			name:       "triple slash cleaned",
			path:       "/foo///bar",
			wantStr:    "/foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/foo/bar", false))),
		},
		{
			name:       "slash dot slash cleaned",
			path:       "/foo/./bar",
			wantStr:    "/foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/foo/bar", false))),
		},
		{
			name:       "slash dot slash dot slash cleaned",
			path:       "/foo/././bar",
			wantStr:    "/foo/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/foo/bar", false))),
		},
		{
			name:       "double dot parent reference cleaned",
			path:       "/foo/../bar",
			wantStr:    "/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/bar", false))),
		},
		{
			name:       "double parent reference cleaned",
			path:       "/foo/../../bar",
			wantStr:    "/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/bar", false))),
		},
		{
			name:       "trailing slash dot cleaned",
			path:       "/foo/.",
			wantStr:    "/foo/",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/foo/", false))),
		},
		{
			name:       "trailing slash double dot cleaned",
			path:       "/foo/..",
			wantStr:    "/",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/", false))),
		},
		{
			name:       "root slash dot cleaned",
			path:       "/.",
			wantStr:    "/",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/", false))),
		},
		{
			name:       "root slash double dot cleaned",
			path:       "/..",
			wantStr:    "/",
			wantTokens: slices.Collect(iterutil.SeqOf(staticToken("/", false))),
		},
		// Allowed dot and slash combination
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
			name: "path segment starting with slash dot and param",
			path: "/foo/.{foo}/baz",
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
			name: "path segment starting with slash dot and param",
			path: "/foo/..{foo}/baz",
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
		},
		{
			name: "simple ending param with regexp",
			path: "/foo/+{bar:[A-z]+}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				wildcardToken("bar", "[A-z]+"),
			)),
		},
		{
			name: "simple infix param with regexp",
			path: "/foo/{bar:[A-z]+}/baz",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", "[A-z]+"),
				staticToken("/baz", false),
			)),
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
		},
		{
			name: "hostname starting with regexp",
			path: "{a:[A-z]+}.b.c/foo",
			wantTokens: slices.Collect(iterutil.SeqOf(
				paramToken("a", "[A-z]+"),
				staticToken(".b.c", true),
				staticToken("/foo", false),
			)),
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
		},
		{
			name: "hostname ending with param regexp",
			path: "a.b.{c:[A-z]+}/foo",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("a.b.", true),
				paramToken("c", "[A-z]+"),
				staticToken("/foo", false),
			)),
		},
		{
			name: "non capturing group allowed in regexp",
			path: "/foo/{bar:(?:foo|bar)}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				paramToken("bar", "(?:foo|bar)"),
			)),
		},
		{
			name: "regexp wildcard at the beginning of the path",
			path: "/+{foo:[A-z]+}/bar",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/", false),
				wildcardToken("foo", "[A-z]+"),
				staticToken("/bar", false),
			)),
		},
		{
			name: "regexp wildcard at the beginning of the host",
			path: "+{a:[A-z]+}.b.c/",
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("a", "[A-z]+"),
				staticToken(".b.c", true),
				staticToken("/", false),
			)),
		},
		{
			name: "consecutive wildcard from hostname to path",
			path: "+{foo}/+{bar}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("foo", ""),
				staticToken("/", false),
				wildcardToken("bar", ""),
			)),
		},
		{
			name: "consecutive wildcard with empty catch all from hostname to path",
			path: "+{foo}/*{bar}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				wildcardToken("foo", ""),
				staticToken("/", false),
				wildcardToken("bar", ""),
			)),
			wantOptionalCatchAll: true,
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
		},
		{
			name: "catch all empty as suffix",
			path: "/foo/*{any}",
			wantTokens: slices.Collect(iterutil.SeqOf(
				staticToken("/foo/", false),
				wildcardToken("any", ""),
			)),
			wantOptionalCatchAll: true,
		},
		{
			name:    "consecutive infix wildcard at start with regexp not allowed",
			path:    "/+{foo:[A-z]+}/+{baz:[0-9]+}",
			wantErr: true,
		},
		{
			name:    "consecutive wildcard with catch all empty not allowed",
			path:    "/+{foo}/*{baz}",
			wantErr: true,
		},
		{
			name:    "consecutive infix wildcard with catch all empty at start with regexp not allowed",
			path:    "/+{foo:[A-z]+}/*{baz:[0-9]+}",
			wantErr: true,
		},
		{
			name:    "hostname consecutive infix wildcard at start with regexp not allowed",
			path:    "/{foo:[A-z]+}.+{baz:[0-9]+}/",
			wantErr: true,
		},
		{
			name:    "consecutive infix wildcard at start with and without regexp not allowed",
			path:    "/+{foo:[A-z]+}/+{baz}",
			wantErr: true,
		},
		{
			name:    "hostname consecutive infix wildcard at start with and without regexp not allowed",
			path:    "+{foo:[A-z]+}.+{baz}/",
			wantErr: true,
		},
		{
			name:    "consecutive infix wildcard at start with regexp not allowed",
			path:    "/+{foo}/+{baz:[0-9]+}/",
			wantErr: true,
		},
		{
			name:    "hostname consecutive infix wildcard at start with regexp not allowed",
			path:    "+{foo}.+{baz:[0-9]+}/",
			wantErr: true,
		},
		{
			name:    "consecutive infix wildcard with regexp not allowed",
			path:    "/foo/+{bar:[A-z]+}/+{baz:[0-9]+}",
			wantErr: true,
		},
		{
			name:    "hostname consecutive infix wildcard with regexp not allowed",
			path:    "foo.+{bar:[A-z]+}.+{baz:[0-9]+}/",
			wantErr: true,
		},
		{
			name:    "consecutive infix wildcard with first regexp not allowed",
			path:    "/foo/+{bar:[A-z]+}/+{baz}",
			wantErr: true,
		},
		{
			name:    "hostname consecutive infix wildcard with first regexp not allowed",
			path:    "foo.+{bar:[A-z]+}.+{baz}/",
			wantErr: true,
		},
		{
			name:    "consecutive infix wildcard with second regexp not allowed",
			path:    "/foo/+{bar}/+{baz:[A-z]+}/",
			wantErr: true,
		},
		{
			name:    "hostname consecutive infix wildcard with second regexp not allowed",
			path:    "foo.+{bar}.+{baz:[A-z]+}/",
			wantErr: true,
		},
		{
			name:    "non slash char after regexp param not allowed",
			path:    "/foo/{bar:[A-z]+}a/",
			wantErr: true,
		},
		{
			name:    "non slash char after regexp wildcard not allowed",
			path:    "/foo/+{bar:[A-z]+}a/",
			wantErr: true,
		},
		{
			name:    "regexp wildcard not allowed in hostname",
			path:    "+{a.{b:[A-z]+}}.c/",
			wantErr: true,
		},
		{
			name:    "regexp wildcard not allowed in hostname",
			path:    "+{a.b.{c:[A-z]+}/",
			wantErr: true,
		},
		{
			name:    "missing param name with regexp",
			path:    "/foo/{:[A-z]+}",
			wantErr: true,
		},
		{
			name:    "missing wildcard name with regexp",
			path:    "/foo/+{:[A-z]+}",
			wantErr: true,
		},
		{
			name:    "missing regular expression",
			path:    "/foo/{a:}",
			wantErr: true,
		},
		{
			name:    "missing regular expression with only ':'",
			path:    "/foo/{:}",
			wantErr: true,
		},
		{
			name:    "unsupported regexp in optional wildcard",
			path:    "/foo/*{any:[A-z]+}",
			wantErr: true,
		},
		{
			name:    "unbalanced braces in param regexp",
			path:    "/foo/{bar:[A-z]+",
			wantErr: true,
		},
		{
			name:    "unbalanced braces in wildcard regexp",
			path:    "/foo/+{bar:[A-z]+",
			wantErr: true,
		},
		{
			name:    "balanced braces in param regexp with invalid char after",
			path:    "/foo/{bar:{}}a",
			wantErr: true,
		},
		{
			name:    "balanced braces in wildcard regexp with invalid brace after",
			path:    "/foo/{bar:{}}}",
			wantErr: true,
		},
		{
			name:    "unbalanced braces in regexp complex",
			path:    "/foo/{bar:{{{{}}}}",
			wantErr: true,
		},
		{
			name:    "invalid regular expression",
			path:    "/foo/{bar:a{5,2}}",
			wantErr: true,
		},
		{
			name:    "invalid regular expression",
			path:    "/foo/{bar:\\k}",
			wantErr: true,
		},
		{
			name:    "capture group in regexp are not allowed",
			path:    "/foo/{bar:(foo|bar)}",
			wantErr: true,
		},
		{
			name:    "no opening brace after * wildcard",
			path:    "/foo/*:bar}",
			wantErr: true,
		},
		{
			name:    "no infix catch all empty",
			path:    "/foo/*{any}/bar",
			wantErr: true,
		},
		{
			name:    "no infix inflight catch all empty",
			path:    "/foo/uuid_*{any}/bar",
			wantErr: true,
		},
		{
			name:    "no suffix catch all empty in hostname",
			path:    "a.b.*{any}/",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, _, err := f.parsePattern(tc.path)
			if tc.wantErr {
				require.Error(t, err)
				var pe *PatternError
				require.True(t, errors.As(err, &pe), "expected *PatternError, got %T", err)
				assert.True(t, errors.Is(err, ErrInvalidRoute), "PatternError should unwrap to ErrInvalidRoute")
				assert.NotEmpty(t, pe.Pattern, "Pattern must be set")
				assert.NotEmpty(t, pe.Reason, "Reason must be set")
				assert.True(t, pe.Start >= 0, "Start must be >= 0")
				assert.True(t, pe.End >= pe.Start, "End must be >= Start")
				assert.True(t, pe.End <= len(pe.Pattern), "End must be <= len(Pattern)")
				return
			}
			require.NoError(t, err)
			wantStr := tc.wantStr
			if wantStr == "" {
				wantStr = tc.path
			}
			assert.Equal(t, wantStr, parsed.str)
			assert.Equal(t, tc.wantTokens, parsed.tokens)
			assert.Equal(t, tc.wantOptionalCatchAll, parsed.optionalCatchAll)
			assert.Equal(t, strings.IndexByte(tc.path, '/'), parsed.endHost)
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

func TestPatternErrorPosition(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		wantType   string
		wantReason string
		wantStart  int
		wantEnd    int
		wantMsg    string
	}{
		{
			name:       "uppercase character in hostname",
			path:       "example.Com/path",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  8,
			wantEnd:    9,
			wantMsg:    "uppercase character in label",
		},
		{
			name:       "all numeric hostname",
			path:       "1234567/path",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    7,
			wantMsg:    "all numeric",
		},
		{
			name:       "missing trailing slash",
			path:       "foo.com",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    7,
			wantMsg:    "missing trailing '/'",
		},
		{
			name:       "empty parameter in path",
			path:       "/foo/bar{}",
			wantType:   "path",
			wantReason: "parameter",
			wantStart:  8,
			wantEnd:    10,
			wantMsg:    "missing name",
		},
		{
			name:       "illegal char in param name",
			path:       "/foo/{*bar}",
			wantType:   "path",
			wantReason: "parameter",
			wantStart:  6,
			wantEnd:    7,
			wantMsg:    "illegal character in name",
		},
		{
			name:       "unbalanced braces in path",
			path:       "/foo/{bar:[A-z]+",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    16,
			wantMsg:    "unbalanced braces",
		},
		{
			name:       "missing param after + in path",
			path:       "/foo/bar/+baz",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  9,
			wantEnd:    10,
			wantMsg:    "missing parameter after delimiter",
		},
		{
			name:       "consecutive wildcard in path",
			path:       "/foo/+{args}/+{param}",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  13,
			wantEnd:    14,
			wantMsg:    "consecutive wildcard",
		},
		{
			name:       "illegal character after param in path",
			path:       "/foo/{bar}+{baz}",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  10,
			wantEnd:    11,
			wantMsg:    "character after parameter",
		},
		{
			name:       "illegal control character in path",
			path:       "example.com/foo\x00",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  15,
			wantEnd:    16,
			wantMsg:    "control character",
		},
		{
			name:       "capture group in regexp",
			path:       "/foo/{bar:(foo|bar)}",
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  10,
			wantEnd:    19,
			wantMsg:    "capture group, use (?:...) instead",
		},
		{
			name:       "missing regular expression",
			path:       "/foo/{a:}",
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  8,
			wantEnd:    8,
			wantMsg:    "missing expression",
		},
		{
			name:       "regexp not allowed in optional wildcard",
			path:       "/foo/*{any:[A-z]+}",
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  6,
			wantEnd:    18,
			wantMsg:    "not allowed in optional wildcard",
		},
		{
			name:       "optional wildcard only as suffix",
			path:       "/foo/*{any}/bar",
			wantType:   "path",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    11,
			wantMsg:    "optional wildcard allowed only as suffix",
		},
		{
			name:       "trailing dot in hostname",
			path:       "a.com./",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    6,
			wantMsg:    "trailing '.'",
		},
		{
			name:       "trailing hyphen in hostname",
			path:       "a.com-/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  5,
			wantEnd:    6,
			wantMsg:    "illegal trailing '-'",
		},
		{
			name:       "hostname label exceed 63 characters",
			path:       "a.b.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu/",
			wantType:   "hostname",
			wantReason: "constraint",
			wantStart:  4,
			wantEnd:    68,
			wantMsg:    "label exceeds 63 characters",
		},
		{
			name:       "too many params",
			path:       "/{1}/{2}/{3}/{4}",
			wantType:   "path",
			wantReason: "constraint",
			wantStart:  13,
			wantEnd:    16,
			wantMsg:    "too many parameters",
		},
		{
			name:       "param key too large",
			path:       "/{abcd}",
			wantType:   "path",
			wantReason: "constraint",
			wantStart:  2,
			wantEnd:    6,
			wantMsg:    "key too large",
		},
		{
			name:       "hostname illegal character after param",
			path:       "{a}b{b}.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  3,
			wantEnd:    4,
			wantMsg:    "character after parameter",
		},
		{
			name:       "hostname consecutive dot",
			path:       "a..com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  2,
			wantEnd:    3,
			wantMsg:    "consecutive '.'",
		},
		{
			name:       "regexp not allowed with disabled regexp",
			path:       "/{a:a}",
			wantType:   "path",
			wantReason: "regexp",
			wantStart:  4,
			wantEnd:    5,
			wantMsg:    "not enabled",
		},
		{
			name:       "hostname missing param after + delimiter",
			path:       "+baz.com/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  0,
			wantEnd:    1,
			wantMsg:    "missing parameter after delimiter",
		},
		{
			name:       "hostname optional wildcard only as suffix",
			path:       "a.b.*{any}/",
			wantType:   "hostname",
			wantReason: "syntax",
			wantStart:  4,
			wantEnd:    6,
			wantMsg:    "optional wildcard allowed only as suffix",
		},
		{
			name:       "missing param name with regexp",
			path:       "/foo/{:[A-z]+}",
			wantType:   "path",
			wantReason: "parameter",
			wantStart:  5,
			wantEnd:    14,
			wantMsg:    "missing name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var f *Router
			if tc.name == "regexp not allowed with disabled regexp" {
				f = MustRouter()
			} else if tc.name == "too many params" {
				var err error
				f, err = NewRouter(WithMaxRouteParams(3), AllowRegexpParam(true))
				require.NoError(t, err)
			} else if tc.name == "param key too large" {
				var err error
				f, err = NewRouter(WithMaxRouteParamKeyBytes(3), AllowRegexpParam(true))
				require.NoError(t, err)
			} else {
				f = MustRouter(AllowRegexpParam(true))
			}

			_, _, err := f.parsePattern(tc.path)
			require.Error(t, err)
			var pe *PatternError
			require.True(t, errors.As(err, &pe), "expected *PatternError, got %T: %v", err, err)
			assert.Equal(t, tc.wantType, pe.Type, "type mismatch")
			assert.Equal(t, tc.wantReason, pe.Reason, "reason mismatch")
			assert.Equal(t, tc.wantStart, pe.Start, "start mismatch")
			assert.Equal(t, tc.wantEnd, pe.End, "end mismatch")
			assert.Contains(t, pe.Error(), tc.wantMsg, "message mismatch")
			fmt.Println(pe)
		})
	}
}

func TestX(t *testing.T) {
	f := MustRouter()
	f.MustAdd(MethodGet, "/foo/{asfsadf*}/baz", emptyHandler)
}
