package fox

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fox-toolkit/fox/internal/stringsutil"
)

type pattern struct {
	str              string // canonical pattern
	tokens           []token
	optionalCatchAll bool
	endHost          int
}

func (fox *Router) parsePattern(raw string) (pattern, int, error) {
	endHost := strings.IndexByte(raw, '/')
	if endHost == -1 {
		if len(raw) == 0 {
			return pattern{}, 0, fmt.Errorf("%w: empty pattern", ErrInvalidRoute)
		}
		return pattern{}, 0, &PatternError{
			Pattern: raw,
			Type:    "hostname",
			Reason:  "syntax",
			Hint:    "missing trailing '/' after hostname",
			Start:   len(raw),
			End:     len(raw),
		}
	}

	path := raw[endHost:]

	var (
		paramCount int
		hostTokens []token
	)

	if endHost > 0 {
		var pe *PatternError
		hostTokens, paramCount, pe = fox.parseHostname(raw[:endHost])
		if pe != nil {
			pe.Pattern = raw
			pe.Type = "hostname"
			return pattern{}, 0, pe
		}
	}

	// Canonicalize the path before parsing so the tree is keyed on the decoded
	// form and encoded dot-segments (e.g. %2E%2E) are caught by validation.
	// Normalization errors point into the raw pattern, while parsePath errors
	// below point into the canonical form.
	normPath, pe := normalizePatternPath(path, true)
	if pe != nil {
		pe.Pattern = raw
		pe.Type = "path"
		pe.Start += endHost
		pe.End += endHost
		return pattern{}, 0, pe
	}
	canonical := raw
	if normPath != path {
		canonical = raw[:endHost] + normPath
	}

	pathTokens, optCatchAll, paramCount, pe := fox.parsePath(normPath, paramCount)
	if pe != nil {
		pe.Pattern = canonical
		pe.Type = "path"
		pe.Start += endHost
		pe.End += endHost
		return pattern{}, 0, pe
	}

	tokens := make([]token, 0, len(hostTokens)+len(pathTokens))
	tokens = append(tokens, hostTokens...)
	tokens = append(tokens, pathTokens...)

	return pattern{
		str:              canonical,
		tokens:           tokens,
		endHost:          endHost,
		optionalCatchAll: optCatchAll,
	}, paramCount, nil
}

// normalizeSearchPattern returns the canonical form of a pattern used as a
// search argument (e.g. [Router.Route], [Iter.Routes], [Iter.PatternPrefix]).
// Unlike registration, malformed escape sequences are not rejected but kept
// as-is. They cannot match any route anyway.
func normalizeSearchPattern(pattern string) string {
	endHost := strings.IndexByte(pattern, '/')
	if endHost == -1 {
		return pattern
	}
	normPath, _ := normalizePatternPath(pattern[endHost:], false)
	if normPath == pattern[endHost:] {
		return pattern
	}
	return pattern[:endHost] + normPath
}

// normalizePatternPath returns the canonical form of the path part of a route
// pattern. Percent-encoded unreserved characters are decoded and the remaining
// hex sequences are normalized to uppercase, like [stringsutil.NormalizeRoutingPath].
// Brace segments, including regex constraints, are never decoded. In strict mode,
// malformed escape sequences are rejected with a [PatternError] positioned
// relative to path. Otherwise the path is kept as-is from the first malformed
// escape. The input is returned unchanged, without allocation, when already canonical.
func normalizePatternPath(path string, strict bool) (string, *PatternError) {
	var buf strings.Builder
	i := 0
	for i < len(path) {
		c := path[i]

		if c == '{' {
			end := braceIndex(path[i+1:], 1)
			if end == -1 {
				// Unbalanced braces. Copy the remainder as-is and let parsePath
				// report the error with consistent positions.
				if buf.Len() > 0 {
					buf.WriteString(path[i:])
				}
				break
			}
			if buf.Len() > 0 {
				buf.WriteString(path[i : i+1+end+1])
			}
			i += 1 + end + 1
			continue
		}

		if c != '%' {
			if buf.Len() > 0 {
				buf.WriteByte(c)
			}
			i++
			continue
		}

		if i+2 >= len(path) {
			if strict {
				return "", newPatternError("syntax", i, len(path), "invalid percent-encoding")
			}
			// Malformed escape, copy the remainder as-is and stop. A dangling
			// '%' must not recombine with a following escape into a valid sequence.
			if buf.Len() > 0 {
				buf.WriteString(path[i:])
			}
			break
		}

		b, ok := stringsutil.DecodeHexPair(path[i+1], path[i+2])
		if !ok {
			if strict {
				return "", newPatternError("syntax", i, i+3, "invalid percent-encoding")
			}
			if buf.Len() > 0 {
				buf.WriteString(path[i:])
			}
			break
		}

		hiUpper := stringsutil.UpperHex(path[i+1])
		loUpper := stringsutil.UpperHex(path[i+2])
		switch {
		case stringsutil.IsUnreserved(b):
			if buf.Len() == 0 {
				buf.Grow(len(path))
				buf.WriteString(path[:i])
			}
			buf.WriteByte(b)
		case path[i+1] != hiUpper || path[i+2] != loUpper:
			if buf.Len() == 0 {
				buf.Grow(len(path))
				buf.WriteString(path[:i])
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
		i += 3
	}

	if buf.Len() == 0 {
		return path, nil
	}
	return buf.String(), nil
}

func (fox *Router) parseHostname(hostname string) ([]token, int, *PatternError) {
	var sb strings.Builder
	sb.Grow(len(hostname))
	tokens := make([]token, 0, 5)
	var (
		paramCount      int
		prevWild        bool
		staticSinceWild int
		partLen         int
		totalLen        int
		labelStart      int
		last            = dotDelim
		nonNumeric      bool
	)

	i := 0
	for i < len(hostname) {
		c := hostname[i]

		switch c {
		case '{', '+':
			if sb.Len() > 0 {
				tokens = append(tokens, token{typ: nodeStatic, value: sb.String(), hsplit: true})
				sb.Reset()
			}
			isWild := c == '+'
			paramStart := i
			if isWild {
				i++
				if i >= len(hostname) || hostname[i] != '{' {
					return nil, 0, newPatternError("syntax", i-1, i, "missing parameter after delimiter")
				}
				if prevWild && staticSinceWild <= 1 {
					paramEnd := len(hostname)
					if idx := braceIndex(hostname[i+1:], 1); idx >= 0 {
						paramEnd = i + 1 + idx + 1
					}
					return nil, 0, newPatternError("syntax", i-1, paramEnd, "consecutive wildcard")
				}
			}
			name, re, n, pe := fox.parseBrace(hostname[i:], dotDelim, false)
			if pe != nil {
				pe.Start += i
				pe.End += i
				return nil, 0, pe
			}
			paramCount++
			if paramCount > fox.maxParams {
				return nil, 0, newPatternError("constraint", paramStart, i+n, "too many parameters")
			}

			kind := nodeParam
			if isWild {
				kind = nodeWildcard
				prevWild = true
				staticSinceWild = 0
			} else {
				prevWild = false
			}
			tokens = append(tokens, token{typ: kind, value: name, regexp: re})
			i += n
			last = 0
			nonNumeric = true
			if i < len(hostname) && hostname[i] != '.' {
				return nil, 0, newPatternError("syntax", i, i+1, "illegal character after parameter")
			}

		case '*':
			i++
			if i < len(hostname) && hostname[i] == '{' {
				paramEnd := len(hostname)
				if idx := braceIndex(hostname[i+1:], 1); idx >= 0 {
					paramEnd = i + 1 + idx + 1
				}
				return nil, 0, newPatternError("syntax", i-1, paramEnd, "optional wildcard allowed only as suffix")
			}
			return nil, 0, newPatternError("syntax", i-1, i, "missing parameter after delimiter")

		default:
			switch {
			case 'a' <= c && c <= 'z' || c == '_':
				nonNumeric = true
				partLen++
			case '0' <= c && c <= '9':
				partLen++
			case c == '-':
				if last == '.' {
					if i == 0 {
						return nil, 0, newPatternError("syntax", i, i+1, "label starts with '-'")
					}
					return nil, 0, newPatternError("syntax", i, i+1, "illegal character after '.'")
				}
				partLen++
				nonNumeric = true
			case c == '.':
				if last == '.' {
					if i == 0 {
						return nil, 0, newPatternError("syntax", i, i+1, "label starts with '.'")
					}
					return nil, 0, newPatternError("syntax", i-1, i+1, "illegal consecutive '.'")
				}
				if last == '-' {
					return nil, 0, newPatternError("syntax", i-1, i, "label ends with '-'")
				}
				if partLen > 63 {
					return nil, 0, newPatternError("constraint", labelStart, i, "label exceeds 63 characters")
				}
				totalLen += partLen + 1 // +1 counts the current dot.
				partLen = 0
				labelStart = i + 1
			case 'A' <= c && c <= 'Z':
				return nil, 0, newPatternError("syntax", i, i+1, "uppercase character in label")
			default:
				return nil, 0, newPatternError("syntax", i, i+1, "illegal character in label")
			}
			last = c
			sb.WriteByte(c)
			staticSinceWild++
			i++
		}
	}

	totalLen += partLen
	if last == '-' {
		return nil, 0, newPatternError("syntax", len(hostname)-1, len(hostname), "illegal trailing '-'")
	}
	if last == '.' {
		return nil, 0, newPatternError("syntax", len(hostname)-1, len(hostname), "illegal trailing '.'")
	}
	if !nonNumeric {
		return nil, 0, newPatternError("syntax", 0, len(hostname), "all numeric")
	}
	if partLen > 63 {
		return nil, 0, newPatternError("constraint", labelStart, len(hostname), "label exceeds 63 characters")
	}
	if totalLen > 253 {
		return nil, 0, newPatternError("constraint", 0, len(hostname), "exceeds 253 characters")
	}

	if sb.Len() > 0 {
		tokens = append(tokens, token{typ: nodeStatic, value: sb.String(), hsplit: true})
	}
	return tokens, paramCount, nil
}

func (fox *Router) parsePath(path string, paramCount int) ([]token, bool, int, *PatternError) {
	var sb strings.Builder
	sb.Grow(len(path))
	tokens := make([]token, 0, 5)
	var (
		prevWild        bool
		staticSinceWild int
		optCatchAll     bool
	)

	i := 0
	for i < len(path) {
		c := path[i]

		switch c {
		case '{', '+', '*':
			isOpt := c == '*'
			isWild := c == '+' || isOpt
			// The wildcard syntax requires an immediate '{'. A bare '+' or '*'
			// is a literal character (both may appear unescaped in a request path).
			if isWild && (i+1 >= len(path) || path[i+1] != '{') {
				sb.WriteByte(c)
				staticSinceWild++
				i++
				continue
			}
			if sb.Len() > 0 {
				tokens = append(tokens, token{typ: nodeStatic, value: sb.String()})
				sb.Reset()
			}
			paramStart := i
			if isWild {
				i++
				if prevWild && staticSinceWild <= 1 {
					paramEnd := len(path)
					if idx := braceIndex(path[i+1:], 1); idx >= 0 {
						paramEnd = i + 1 + idx + 1
					}
					return nil, false, 0, newPatternError("syntax", i-1, paramEnd, "consecutive wildcard")
				}
			}
			name, re, n, pe := fox.parseBrace(path[i:], slashDelim, isOpt)
			if pe != nil {
				pe.Start += i
				pe.End += i
				if isOpt && pe.Reason == "regexp" {
					pe.Start = paramStart
				}
				return nil, false, 0, pe
			}
			paramCount++
			if paramCount > fox.maxParams {
				return nil, false, 0, newPatternError("constraint", paramStart, i+n, "too many parameters")
			}

			kind := nodeParam
			if isWild {
				kind = nodeWildcard
				prevWild = true
				staticSinceWild = 0
			} else {
				prevWild = false
			}
			tokens = append(tokens, token{typ: kind, value: name, regexp: re})
			i += n
			if isOpt {
				if i < len(path) {
					return nil, false, 0, newPatternError("syntax", paramStart, i, "optional wildcard allowed only as suffix")
				}
				optCatchAll = true
			}
			if i < len(path) && path[i] != '/' {
				return nil, false, 0, newPatternError("syntax", i, i+1, "illegal character after parameter")
			}

		default:
			if isASCIIControl(c) {
				return nil, false, 0, newPatternError("syntax", i, i+1, "illegal control character")
			}
			if c == '/' && i > 0 && path[i-1] == '/' {
				return nil, false, 0, newPatternError("syntax", i-1, i+1, "consecutive '/'")
			}
			if c == '.' && i > 0 && path[i-1] == '/' {
				next := i + 1
				if next >= len(path) || path[next] == '/' {
					// "/." at end or "/./"
					end := next
					if next < len(path) {
						end = next + 1
					}
					return nil, false, 0, newPatternError("syntax", i-1, end, "dot segment")
				}
				if path[next] == '.' {
					afterDots := next + 1
					if afterDots >= len(path) || path[afterDots] == '/' {
						// "/.." at end or "/../"
						end := afterDots
						if afterDots < len(path) {
							end = afterDots + 1
						}
						return nil, false, 0, newPatternError("syntax", i-1, end, "dot segment")
					}
				}
			}
			sb.WriteByte(c)
			staticSinceWild++
			i++
		}
	}

	if sb.Len() > 0 {
		tokens = append(tokens, token{typ: nodeStatic, value: sb.String()})
	}
	return tokens, optCatchAll, paramCount, nil
}

func (fox *Router) parseBrace(s string, delim byte, isOptional bool) (string, *regexp.Regexp, int, *PatternError) {
	// Skip s[0] (the opening '{') and start at nesting level 1 to account for it.
	idx := braceIndex(s[1:], 1)
	if idx == -1 {
		return "", nil, 0, newPatternError("syntax", 0, len(s), "unbalanced braces")
	}

	content := s[1 : 1+idx] // Everything between { and }.
	consumed := 1 + idx + 1 // { + content + }

	name := content
	var rawRegex string
	hasRegex := false
	colonIdx := -1
	if ci := strings.IndexByte(content, ':'); ci >= 0 {
		colonIdx = ci
		name = content[:colonIdx]
		rawRegex = content[colonIdx+1:]
		hasRegex = true
	}

	if len(name) > fox.maxParamKeyBytes {
		return "", nil, 0, newPatternError("constraint", 1, 1+len(name), "key too large")
	}

	if len(name) == 0 {
		return "", nil, 0, newPatternError("parameter", 0, consumed, "missing name")
	}

	for j := 0; j < len(name); j++ {
		c := name[j]
		if isASCIIControl(c) {
			return "", nil, 0, newPatternError("parameter", 1+j, 1+j+1, "illegal control character in name")
		}
		switch c {
		case delim, '/', '*', '+', '{':
			return "", nil, 0, newPatternError("parameter", 1+j, 1+j+1, "illegal character in name")
		}
	}

	if !hasRegex {
		return name, nil, consumed, nil
	}

	if isOptional {
		return "", nil, 0, newPatternError("regexp", 0, consumed, "not allowed in optional wildcard")
	}

	re, pe := fox.compileParamRegexp(rawRegex)
	if pe != nil {
		regexOffset := 1 + colonIdx + 1
		pe.Start += regexOffset
		pe.End += regexOffset
		return "", nil, 0, pe
	}
	return name, re, consumed, nil
}

// compileParamRegexp validates and compiles a regular expression constraint for a parameter.
// Positions in the returned PatternError are relative to rawRegex.
func (fox *Router) compileParamRegexp(rawRegex string) (*regexp.Regexp, *PatternError) {
	if !fox.allowRegexp {
		return nil, newPatternError("regexp", 0, len(rawRegex), "feature not enabled")
	}
	if rawRegex == "" {
		return nil, newPatternError("regexp", 0, 0, "missing expression")
	}

	re, err := regexp.Compile("^(?:" + rawRegex + ")$")
	if err != nil {
		return nil, &PatternError{
			Reason: "regexp",
			Start:  0,
			End:    len(rawRegex),
			Hint:   err.Error(),
			err:    err,
		}
	}
	if re.NumSubexp() > 0 {
		return nil, newPatternError("regexp", 0, len(rawRegex), "capture group, use (?:...) instead")
	}

	return re, nil
}

// isASCIIControl reports whether c is an ASCII control character: a C0 byte
// (0x00 to 0x1F) or DEL (0x7F).
func isASCIIControl(c byte) bool {
	return c < 0x20 || c == 0x7f
}

// braceIndex returns the index of the closing brace that balances an opening
// brace. It starts at startLevel opened brace.
func braceIndex(s string, startLevel int) int {
	level := startLevel

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			level++
		case '}':
			if level--; level == 0 {
				return i
			}
		}
	}
	return -1
}
