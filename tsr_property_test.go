package fox

// Property tests for trailing slash recommendation (tsr) in lookupByPath. Random route sets
// are checked against per-route oracle routers for the following invariants:
//   - a recommended tsr always direct matches the corrected path (no redirect loop or 404)
//   - StrictSlash routes are never tsr candidates
//   - tsr never crosses a consecutive slash boundary
//   - params recorded on a tsr match equal those of the direct match on the corrected path
//   - lazy and non-lazy lookups agree
//   - no direct match is missed and no tsr opportunity is missed (suffix regex
//     catch-alls excluded: their capture includes the trailing slash, so a rejected
//     capture is a plain no match, not a tsr opportunity)
// The ServeHTTP test additionally checks redirect loops, open redirect and dot segment guards.

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	tsrPropertySeeds = 400
	tsrServeSeeds    = 200
)

type tsrGen struct{ r *rand.Rand }

func (g *tsrGen) pick(ss []string) string { return ss[g.r.Intn(len(ss))] }

var tsrStatics = []string{"foo", "bar", "b", "foobar", "x", "a.b", "z-z"}

func (g *tsrGen) segment(last bool) string {
	n := g.r.Intn(100)
	switch {
	case n < 40:
		return g.pick(tsrStatics)
	case n < 58:
		return g.pick([]string{"{p}", "{p:[a-z]+}", "{p:[0-9]+}"})
	case n < 72:
		if last {
			return g.pick([]string{"*{w}", "+{w}", "+{w:[a-z]+}", "*{w:[0-9]+}"})
		}
		return g.pick([]string{"+{w}", "+{w:[a-z]+}"})
	case n < 82:
		return "" // produces consecutive slashes
	default:
		return g.pick(tsrStatics)
	}
}

func (g *tsrGen) pattern() string {
	depth := 1 + g.r.Intn(3)
	var sb strings.Builder
	for i := 0; i < depth; i++ {
		sb.WriteByte('/')
		sb.WriteString(g.segment(i == depth-1))
	}
	s := sb.String()
	if g.r.Intn(3) == 0 && !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

// instantiate replaces params and wildcards with concrete values (sometimes violating regexes).
func (g *tsrGen) instantiate(pattern string) string {
	var sb strings.Builder
	i := 0
	for i < len(pattern) {
		c := pattern[i]
		isWild := (c == '*' || c == '+') && i+1 < len(pattern) && pattern[i+1] == '{'
		if c == '{' || isWild {
			end := strings.IndexByte(pattern[i:], '}') + i
			sb.WriteString(g.valueFor(pattern[i : end+1]))
			i = end + 1
			continue
		}
		sb.WriteByte(c)
		i++
	}
	return sb.String()
}

func (g *tsrGen) valueFor(seg string) string {
	switch {
	case seg == "{p}":
		return g.pick([]string{"foo", "1", "zz"})
	case strings.HasPrefix(seg, "{p:[a-z]"):
		return g.pick([]string{"abc", "zz", "ABC", "42"})
	case strings.HasPrefix(seg, "{p:[0-9]"):
		return g.pick([]string{"123", "7", "abc"})
	case seg == "*{w}":
		return g.pick([]string{"", "x", "x/y"})
	case seg == "+{w}":
		return g.pick([]string{"x", "x/y"})
	case strings.HasPrefix(seg, "+{w:"):
		return g.pick([]string{"abc", "ab/cd", "AB"})
	case strings.HasPrefix(seg, "*{w:"):
		return g.pick([]string{"123", "", "abc"})
	default:
		return "v"
	}
}

func (g *tsrGen) probeVariants(p string) []string {
	out := []string{p}
	if strings.HasSuffix(p, "/") {
		out = append(out, strings.TrimSuffix(p, "/"))
	} else {
		out = append(out, p+"/")
	}
	out = append(out, p+"//")
	if len(p) > 1 {
		out = append(out, p[:len(p)-1]) // chop last char: mid-edge probes
	}
	// duplicate a random slash: probe double slash mid path
	idxs := []int{}
	for i, c := range p {
		if c == '/' {
			idxs = append(idxs, i)
		}
	}
	if len(idxs) > 0 {
		at := idxs[g.r.Intn(len(idxs))]
		out = append(out, p[:at]+"/"+p[at:])
	}
	return out
}

// endsWithRegexCatchAll reports whether the pattern's last segment is a regex wildcard.
func endsWithRegexCatchAll(pattern string) bool {
	last := pattern[strings.LastIndexByte(pattern, '/')+1:]
	return (strings.HasPrefix(last, "+{") || strings.HasPrefix(last, "*{")) && strings.Contains(last, ":")
}

func tsrModeName(m TrailingSlashOption) string {
	switch m {
	case StrictSlash:
		return "strict"
	case RelaxedSlash:
		return "relaxed"
	case RedirectSlash:
		return "redirect"
	}
	return "?"
}

// lookupPathOnly runs lookupByPath against the router's pattern tree.
func lookupPathOnly(f *Router, method, path string, lazy bool) (*Route, []string, bool) {
	tree := f.getTree()
	c := newTestContext(f)
	idx, n, tsr := lookupByPath(tree.patterns, method, path, c, lazy, 0)
	if n == nil {
		return nil, nil, false
	}
	var params []string
	if !lazy {
		params = append(params, *c.params...)
	}
	return n.routes[idx], params, tsr
}

type tsrRegRoute struct {
	pattern string
	method  string
	mode    TrailingSlashOption
	iso     *Router
}

func TestTsrPhase2Property(t *testing.T) {
	fails := 0
	report := func(format string, args ...any) {
		fails++
		if fails <= 25 {
			t.Errorf(format, args...)
		}
	}

	for seed := int64(0); seed < tsrPropertySeeds; seed++ {
		g := &tsrGen{r: rand.New(rand.NewSource(seed))}
		modes := []TrailingSlashOption{StrictSlash, RelaxedSlash, RedirectSlash}
		methods := []string{http.MethodGet, http.MethodGet, http.MethodGet, http.MethodPost}

		f, err := NewRouter(AllowRegexpParam(true))
		require.NoError(t, err)

		var routes []tsrRegRoute
		nPat := 2 + g.r.Intn(9)
		for range nPat {
			pat := g.pattern()
			mode := modes[g.r.Intn(len(modes))]
			method := methods[g.r.Intn(len(methods))]
			if _, err := f.Add([]string{method}, pat, emptyHandler, WithHandleTrailingSlash(mode)); err != nil {
				continue
			}
			iso, err := NewRouter(AllowRegexpParam(true))
			require.NoError(t, err)
			_, err = iso.Add([]string{method}, pat, emptyHandler, WithHandleTrailingSlash(mode))
			require.NoError(t, err)
			routes = append(routes, tsrRegRoute{pattern: pat, method: method, mode: mode, iso: iso})
		}
		if len(routes) == 0 {
			continue
		}

		routesDesc := func() []string {
			out := make([]string, 0, len(routes))
			for _, rr := range routes {
				out = append(out, rr.method+" "+rr.pattern+" ("+tsrModeName(rr.mode)+")")
			}
			return out
		}

		probes := map[string]struct{}{
			"": {}, "/": {}, "//": {}, "///": {}, "/x": {}, "/x/": {}, "/x//": {},
		}
		for _, rr := range routes {
			for range 2 {
				inst := g.instantiate(rr.pattern)
				for _, v := range g.probeVariants(inst) {
					probes[v] = struct{}{}
				}
			}
		}

		for p := range probes {
			rt, params, tsr := lookupPathOnly(f, http.MethodGet, p, false)
			rtL, _, tsrL := lookupPathOnly(f, http.MethodGet, p, true)

			// I7: lazy and non-lazy agree
			if rt != rtL || tsr != tsrL {
				report("lazy mismatch: seed=%d routes=%v probe=%q lazy=(%v,%v) nonlazy=(%v,%v)",
					seed, routesDesc(), p, rtL, tsrL, rt, tsr)
			}

			if tsr {
				// I4: StrictSlash routes are never tsr candidates
				if rt.handleSlash == StrictSlash {
					report("strict tsr candidate: seed=%d routes=%v probe=%q candidate=%s",
						seed, routesDesc(), p, rt.pattern.str)
				}
				// I3: tsr never crosses a // boundary
				if strings.HasSuffix(p, "//") {
					report("tsr crosses // boundary: seed=%d routes=%v probe=%q candidate=%s",
						seed, routesDesc(), p, rt.pattern.str)
				}
				alt := fixTrailingSlash(p)
				// I1: the corrected path must direct match (no redirect loop / no 404 after redirect)
				altRt, altParams, altTsr := lookupPathOnly(f, http.MethodGet, alt, false)
				if altRt == nil || altTsr {
					report("tsr target does not direct match: seed=%d routes=%v probe=%q alt=%q candidate=%s altRt=%v altTsr=%v",
						seed, routesDesc(), p, alt, rt.pattern.str, altRt, altTsr)
				} else if altRt == rt && !slices.Equal(altParams, params) {
					// I5: params recorded by tsr equal params of the direct match on the corrected path
					report("tsr params mismatch: seed=%d routes=%v probe=%q alt=%q candidate=%s tsrParams=%v altParams=%v",
						seed, routesDesc(), p, alt, rt.pattern.str, params, altParams)
				}
				// I2b: candidate route itself must direct match alt in isolation, with same params
				for _, rr := range routes {
					if rr.pattern != rt.pattern.str || !slices.Contains(rt.methods, rr.method) {
						continue
					}
					isoRt, isoParams, isoTsr := lookupPathOnly(rr.iso, http.MethodGet, alt, false)
					if isoRt == nil || isoTsr {
						report("tsr candidate does not match corrected path: seed=%d routes=%v probe=%q alt=%q candidate=%s",
							seed, routesDesc(), p, alt, rt.pattern.str)
					} else if !slices.Equal(isoParams, params) {
						report("tsr candidate params mismatch: seed=%d routes=%v probe=%q alt=%q candidate=%s tsrParams=%v isoParams=%v",
							seed, routesDesc(), p, alt, rt.pattern.str, params, isoParams)
					}
					break
				}
			}

			// I8: if any isolated route direct matches p, the combined tree must not return a miss
			directIso := false
			for _, rr := range routes {
				if r2, _, t2 := lookupPathOnly(rr.iso, http.MethodGet, p, true); r2 != nil && !t2 {
					directIso = true
					break
				}
			}
			if directIso && rt == nil {
				report("direct match miss: seed=%d routes=%v probe=%q", seed, routesDesc(), p)
			}

			// I6: completeness. No direct match anywhere, no // crossing, but a non strict route
			// direct matches the corrected path => tsr must be recommended.
			// Suffix regex catch-alls are excluded by design: they capture the full tail
			// including any trailing slash, so a rejected capture is a plain no match.
			if rt == nil && !directIso && p != "" && p != "/" && !strings.HasSuffix(p, "//") {
				alt := fixTrailingSlash(p)
				for _, rr := range routes {
					if rr.mode == StrictSlash || endsWithRegexCatchAll(rr.pattern) {
						continue
					}
					if r2, _, t2 := lookupPathOnly(rr.iso, http.MethodGet, alt, true); r2 != nil && !t2 {
						report("missed tsr: seed=%d routes=%v probe=%q alt=%q would match %s (%s)",
							seed, routesDesc(), p, alt, rr.pattern, tsrModeName(rr.mode))
						break
					}
				}
			}
		}
	}

	if fails > 25 {
		t.Errorf("... and %d more failures", fails-25)
	}
}

func TestTsrPhase2ServeHTTPRedirect(t *testing.T) {
	fails := 0
	report := func(format string, args ...any) {
		fails++
		if fails <= 25 {
			t.Errorf(format, args...)
		}
	}

	for seed := int64(1000); seed < 1000+tsrServeSeeds; seed++ {
		g := &tsrGen{r: rand.New(rand.NewSource(seed))}
		f, err := NewRouter(WithHandleTrailingSlash(RedirectSlash), AllowRegexpParam(true))
		require.NoError(t, err)

		var patterns []string
		nPat := 2 + g.r.Intn(8)
		for range nPat {
			pat := g.pattern()
			if _, err := f.Add(MethodGet, pat, emptyHandler); err != nil {
				continue
			}
			patterns = append(patterns, pat)
		}
		if len(patterns) == 0 {
			continue
		}

		probes := map[string]struct{}{"/x": {}, "/x/": {}, "/x//": {}, "/x/..": {}, "/../": {}, "/x/./": {}}
		for _, pat := range patterns {
			for range 2 {
				inst := g.instantiate(pat)
				for _, v := range g.probeVariants(inst) {
					probes[v] = struct{}{}
				}
			}
		}

		for p := range probes {
			if !strings.HasPrefix(p, "/") {
				continue
			}
			cur := p
			visited := map[string]bool{}
			for hop := 0; ; hop++ {
				if visited[cur] {
					report("redirect loop: seed=%d routes=%v probe=%q revisits %q", seed, patterns, p, cur)
					break
				}
				visited[cur] = true
				req := httptest.NewRequest(http.MethodGet, cur, nil)
				w := httptest.NewRecorder()
				f.ServeHTTP(w, req)
				if w.Code != http.StatusMovedPermanently && w.Code != http.StatusPermanentRedirect {
					if hop > 0 && w.Code != http.StatusOK {
						// A redirect must land on a real route, unless the location was
						// escaped for open redirect protection.
						if !strings.Contains(cur, "%") {
							report("redirect landed on %d: seed=%d routes=%v probe=%q cur=%q", w.Code, seed, patterns, p, cur)
						}
					}
					break
				}
				if hop >= 3 {
					report("too many redirects: seed=%d routes=%v probe=%q", seed, patterns, p)
					break
				}
				loc := w.Header().Get(HeaderLocation)
				if loc == "" {
					report("redirect without location: seed=%d routes=%v probe=%q cur=%q", seed, patterns, p, cur)
					break
				}
				if hasDotSegment(loc) {
					report("redirect location with dot segment: seed=%d routes=%v probe=%q loc=%q", seed, patterns, p, loc)
					break
				}
				if strings.HasPrefix(loc, "//") || strings.HasPrefix(loc, "/\\") {
					report("open redirect location: seed=%d routes=%v probe=%q loc=%q", seed, patterns, p, loc)
					break
				}
				cur = loc
			}
		}
	}

	if fails > 25 {
		t.Errorf("... and %d more failures", fails-25)
	}
}
