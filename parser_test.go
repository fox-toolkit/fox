package fox

import (
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
		wantErr              error
		name                 string
		path                 string
		wantStr              string // Expected parsed.str (defaults to path if empty).
		wantTokens           []token
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
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "empty parameter",
			path:    "/foo/bar{}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "missing arguments name after catch all",
			path:    "/foo/bar/*",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "missing arguments name after param",
			path:    "/foo/bar/{",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "catch all in the middle of the route",
			path:    "/foo/bar/*/baz",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "empty infix catch all",
			path:    "/foo/bar/+{}/baz",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "empty ending catch all",
			path:    "/foo/bar/baz/+{}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected character in param",
			path:    "/foo/{{bar}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected character in param",
			path:    "/foo/{*bar}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected character in catch-all",
			path:    "/foo/+{/bar}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "catch all not supported in hostname",
			path:    "a.b.c*/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal character in params hostname",
			path:    "a.b.c{/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal character in hostname label",
			path:    "a.b.c}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected character in param hostname",
			path:    "a.{.bar}.c/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected character in wildcard hostname",
			path:    "a.+{.bar}.c/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected character in param hostname",
			path:    "a.{/bar}.c/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected character in wildcard hostname",
			path:    "a.+{/bar}.c/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "in flight catch-all after param in one route segment",
			path:    "/foo/{bar}+{baz}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "multiple param in one route segment",
			path:    "/foo/{bar}{baz}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "in flight param after catch all",
			path:    "/foo/+{args}{param}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive catch all with no slash",
			path:    "/foo/+{args}+{param}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive catch all",
			path:    "/foo/+{args}/+{param}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive catch all with inflight",
			path:    "/foo/ab+{args}/+{param}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected char after inflight catch all",
			path:    "/foo/ab+{args}a",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unexpected char after catch all",
			path:    "/foo/+{args}a",
			wantErr: ErrInvalidRoute,
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
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal leading hyphen in hostname",
			path:    "-a.com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal leading dot in hostname",
			path:    ".a.com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal trailing hyphen in hostname",
			path:    "a.com-/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal trailing dot in hostname",
			path:    "a.com./",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal trailing dot in hostname after param",
			path:    "{tld}./foo/bar",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal single dot in hostname",
			path:    "./",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal hyphen before dot",
			path:    "a-.com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal hyphen after dot",
			path:    "a.-com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal double dot",
			path:    "a..com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal double dot with param state",
			path:    "{b}..com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal double dot with inflight param state",
			path:    "a{b}..com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "param not finishing with delimiter in hostname",
			path:    "{a}b{b}.com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive parameter in hostname",
			path:    "{a}{b}.com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "leading hostname label exceed 63 characters",
			path:    "uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu.b.com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "middle hostname label exceed 63 characters",
			path:    "a.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu.com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "trailing hostname label exceed 63 characters",
			path:    "a.b.uj01dowf1x5lk6lysurbr0lgbdd1wfyw8sm8q17mnt0i9igk774vcwr5rly5dguu/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "illegal character in domain",
			path:    "a.b!.com/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "invalid all-numeric label",
			path:    "123/",
			wantErr: ErrInvalidRoute,
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
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "hostname exceed 255 character",
			path:    "a.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjx.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr.78fayzyiqkt3hh2mquv9szfroeexx8qztscu3oudoyfarjl6jmdyxk2cefvzjxr/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "invalid all-numeric label",
			path:    "11.22.33/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "invalid uppercase label",
			path:    "ABC/",
			wantErr: ErrInvalidRoute,
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
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive wildcard with catch all empty not allowed",
			path:    "/+{foo}/*{baz}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive infix wildcard with catch all empty at start with regexp not allowed",
			path:    "/+{foo:[A-z]+}/*{baz:[0-9]+}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "hostname consecutive infix wildcard at start with regexp not allowed",
			path:    "/{foo:[A-z]+}.+{baz:[0-9]+}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive infix wildcard at start with and without regexp not allowed",
			path:    "/+{foo:[A-z]+}/+{baz}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "hostname consecutive infix wildcard at start with and without regexp not allowed",
			path:    "+{foo:[A-z]+}.+{baz}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive infix wildcard at start with regexp not allowed",
			path:    "/+{foo}/+{baz:[0-9]+}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "hostname consecutive infix wildcard at start with regexp not allowed",
			path:    "+{foo}.+{baz:[0-9]+}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive infix wildcard with regexp not allowed",
			path:    "/foo/+{bar:[A-z]+}/+{baz:[0-9]+}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "hostname consecutive infix wildcard with regexp not allowed",
			path:    "foo.+{bar:[A-z]+}.+{baz:[0-9]+}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive infix wildcard with first regexp not allowed",
			path:    "/foo/+{bar:[A-z]+}/+{baz}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "hostname consecutive infix wildcard with first regexp not allowed",
			path:    "foo.+{bar:[A-z]+}.+{baz}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "consecutive infix wildcard with second regexp not allowed",
			path:    "/foo/+{bar}/+{baz:[A-z]+}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "hostname consecutive infix wildcard with second regexp not allowed",
			path:    "foo.+{bar}.+{baz:[A-z]+}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "non slash char after regexp param not allowed",
			path:    "/foo/{bar:[A-z]+}a/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "non slash char after regexp wildcard not allowed",
			path:    "/foo/+{bar:[A-z]+}a/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "regexp wildcard not allowed in hostname",
			path:    "+{a.{b:[A-z]+}}.c/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "regexp wildcard not allowed in hostname",
			path:    "+{a.b.{c:[A-z]+}/",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "missing param name with regexp",
			path:    "/foo/{:[A-z]+}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "missing wildcard name with regexp",
			path:    "/foo/+{:[A-z]+}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "missing regular expression",
			path:    "/foo/{a:}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "missing regular expression with only ':'",
			path:    "/foo/{:}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unsupported regexp in optional wildcard",
			path:    "/foo/*{any:[A-z]+}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unbalanced braces in param regexp",
			path:    "/foo/{bar:[A-z]+",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unbalanced braces in wildcard regexp",
			path:    "/foo/+{bar:[A-z]+",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "balanced braces in param regexp with invalid char after",
			path:    "/foo/{bar:{}}a",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "balanced braces in wildcard regexp with invalid brace after",
			path:    "/foo/{bar:{}}}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "unbalanced braces in regexp complex",
			path:    "/foo/{bar:{{{{}}}}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "invalid regular expression",
			path:    "/foo/{bar:a{5,2}}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "invalid regular expression",
			path:    "/foo/{bar:\\k}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "capture group in regexp are not allowed",
			path:    "/foo/{bar:(foo|bar)}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "no opening brace after * wildcard",
			path:    "/foo/*:bar}",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "no infix catch all empty",
			path:    "/foo/*{any}/bar",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "no infix inflight catch all empty",
			path:    "/foo/uuid_*{any}/bar",
			wantErr: ErrInvalidRoute,
		},
		{
			name:    "no suffix catch all empty in hostname",
			path:    "a.b.*{any}/",
			wantErr: ErrInvalidRoute,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, _, err := f.parsePattern(tc.path)
			require.ErrorIs(t, err, tc.wantErr)
			if err != nil {
				return
			}
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
