package fox

import (
	"fmt"
	"regexp"
	"strings"
)

// PatternError is a structured error for invalid route patterns. It carries the reason,
// the offending position, and the pattern itself, enabling programmatic diagnostics.
type PatternError struct {
	Pattern string // canonical form of the route pattern
	Type    string // hostname | path
	Reason  string // syntax | parameter | regexp | constraint
	Hint    string // hint
	Start   int    // start offset of the offending segment
	End     int    // end offset of the offending segment
}

// Error returns a human-readable error message with a visual pointer to the offending segment.
func (e *PatternError) Error() string {
	var sb strings.Builder
	sb.WriteString("pattern: ")
	if e.Type != "" {
		sb.WriteString(e.Type)
		sb.WriteString(": ")
	}
	sb.WriteString(e.Reason)
	sb.WriteString(": ")
	sb.WriteString(e.Hint)
	if e.Pattern != "" {
		sb.WriteByte('\n')
		sb.WriteString("      ")
		sb.WriteString(e.Pattern)
		sb.WriteByte('\n')
		sb.WriteString("      ")
		for i := 0; i < e.Start; i++ {
			sb.WriteByte(' ')
		}
		n := e.End - e.Start
		if n <= 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			sb.WriteByte('^')
		}
	}
	return sb.String()
}

func newPatternError(reason string, start, end int, msg string) *PatternError {
	return &PatternError{
		Reason: reason,
		Start:  start,
		End:    end,
		Hint:   msg,
	}
}

type pattern struct {
	str              string // canonical cleaned pattern
	tokens           []token
	optionalCatchAll bool
	endHost          int
}

func (fox *Router) parsePattern(raw string) (*pattern, int, error) {
	endHost := strings.IndexByte(raw, '/')
	if endHost == -1 {
		if len(raw) == 0 {
			return nil, 0, &PatternError{
				Pattern: raw,
				Reason:  "syntax",
				Hint:    "empty pattern",
			}
		}
		raw += "/"
		endHost = len(raw) - 1
	}

	cleanedPath := CleanPath(raw[endHost:])
	canonicalPattern := raw[:endHost] + cleanedPath

	var (
		paramCount int
		hostTokens []token
	)

	if endHost > 0 {
		var pe *PatternError
		hostTokens, paramCount, pe = fox.parseHostname(raw[:endHost])
		if pe != nil {
			pe.Pattern = canonicalPattern
			pe.Type = "hostname"
			return nil, 0, pe
		}
	}

	pathTokens, optCatchAll, paramCount, pe := fox.parsePath(cleanedPath, paramCount)
	if pe != nil {
		pe.Pattern = canonicalPattern
		pe.Type = "path"
		pe.Start += endHost
		pe.End += endHost
		return nil, 0, pe
	}

	tokens := make([]token, 0, len(hostTokens)+len(pathTokens))
	tokens = append(tokens, hostTokens...)
	tokens = append(tokens, pathTokens...)

	return &pattern{
		str:              canonicalPattern,
		tokens:           tokens,
		endHost:          endHost,
		optionalCatchAll: optCatchAll,
	}, paramCount, nil
}

type hostnameValidator struct {
	partLen    int  // Current label length in bytes.
	totalLen   int  // Total hostname length in bytes.
	last       byte // Last static char for dot/dash adjacency rules.
	nonNumeric bool // True once we've seen a letter, hyphen, or parameter.
}

func (v *hostnameValidator) checkByte(c byte, pos int) *PatternError {
	switch {
	case 'a' <= c && c <= 'z' || c == '_':
		v.nonNumeric = true
		v.partLen++
	case '0' <= c && c <= '9':
		v.partLen++
	case c == '-':
		if v.last == '.' {
			return newPatternError("syntax", pos, pos+1, "illegal character after '.'")
		}
		v.partLen++
		v.nonNumeric = true
	case c == '.':
		if v.last == '.' {
			return newPatternError("syntax", pos, pos+1, "illegal consecutive '.'")
		}
		if v.last == '-' {
			return newPatternError("syntax", pos-1, pos, "label ends with '-'")
		}
		if v.partLen > 63 {
			return newPatternError("constraint", pos-v.partLen, pos, "label exceeds 63 characters")
		}
		v.totalLen += v.partLen + 1 // +1 counts the current dot.
		v.partLen = 0
	case 'A' <= c && c <= 'Z':
		return newPatternError("syntax", pos, pos+1, "uppercase character in label")
	default:
		return newPatternError("syntax", pos, pos+1, "illegal character in label")
	}
	v.last = c
	return nil
}

func (v *hostnameValidator) skipParam() {
	v.last = 0
	v.nonNumeric = true
}

func (v *hostnameValidator) postCheck(hostnameLen int) *PatternError {
	v.totalLen += v.partLen
	if v.last == '-' {
		return newPatternError("syntax", hostnameLen-1, hostnameLen, "illegal trailing '-'")
	}
	if v.last == '.' {
		return newPatternError("syntax", hostnameLen-1, hostnameLen, "illegal trailing '.'")
	}
	if !v.nonNumeric {
		return newPatternError("syntax", 0, hostnameLen, "all numeric")
	}
	if v.partLen > 63 {
		return newPatternError("constraint", hostnameLen-v.partLen, hostnameLen, "label exceeds 63 characters")
	}
	if v.totalLen > 253 {
		return newPatternError("constraint", 0, hostnameLen, "exceeds 253 characters")
	}
	return nil
}

func (fox *Router) parseHostname(hostname string) ([]token, int, *PatternError) {
	var sb strings.Builder
	sb.Grow(len(hostname))
	tokens := make([]token, 0, 5)
	validator := hostnameValidator{last: dotDelim}
	var (
		paramCount      int
		prevWild        bool
		staticSinceWild int
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
					return nil, 0, newPatternError("syntax", i-1, i, "consecutive wildcard")
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
			validator.skipParam()
			if i < len(hostname) && hostname[i] != '.' {
				return nil, 0, newPatternError("syntax", i, i+1, "illegal character after parameter")
			}

		case '*':
			i++
			if i < len(hostname) && hostname[i] == '{' {
				return nil, 0, newPatternError("syntax", i-1, i+1, "optional wildcard allowed only as suffix")
			}
			return nil, 0, newPatternError("syntax", i-1, i, "missing parameter after delimiter")

		default:
			if pe := validator.checkByte(c, i); pe != nil {
				return nil, 0, pe
			}
			sb.WriteByte(c)
			staticSinceWild++
			i++
		}
	}

	if pe := validator.postCheck(len(hostname)); pe != nil {
		return nil, 0, pe
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
			if sb.Len() > 0 {
				tokens = append(tokens, token{typ: nodeStatic, value: sb.String()})
				sb.Reset()
			}
			isOpt := c == '*'
			isWild := c == '+' || isOpt
			paramStart := i
			if isWild {
				i++
				if i >= len(path) || path[i] != '{' {
					return nil, false, 0, newPatternError("syntax", i-1, i, "missing parameter after delimiter")
				}
				if prevWild && staticSinceWild <= 1 {
					return nil, false, 0, newPatternError("syntax", i-1, i, "consecutive wildcard")
				}
			}
			name, re, n, pe := fox.parseBrace(path[i:], slashDelim, isOpt)
			if pe != nil {
				pe.Start += i
				pe.End += i
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
			if c < ' ' || c == 0x7f {
				return nil, false, 0, newPatternError("syntax", i, i+1, "illegal control character")
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
		switch name[j] {
		// TODO: just put . and /, add also }
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

	re, err := regexp.Compile("^" + rawRegex + "$")
	if err != nil {
		return nil, newPatternError("regexp", 0, len(rawRegex), fmt.Sprintf("compile error: %s", err))
	}
	if re.NumSubexp() > 0 {
		return nil, newPatternError("regexp", 0, len(rawRegex), "capture group, use (?:...) instead")
	}

	return re, nil
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

// parsedRoute is a compatibility bridge for callers that have not yet migrated to parsePattern.
// It translates the new pattern type back into the old field layout.
type parsedRoute struct {
	token         []token
	paramCnt      int
	endHost       int
	startCatchAll int
}

// parseRoute wraps parsePattern to provide the old parsedRoute return type.
// Callers should migrate to parsePattern directly.
func (fox *Router) parseRoute(url string) (parsedRoute, error) {
	p, paramCnt, err := fox.parsePattern(url)
	if err != nil {
		return parsedRoute{}, err
	}

	// Backward compatibility: callers store the original url as the route pattern,
	// so we must reject paths that CleanPath would normalize (e.g. //, ./, ../).
	// Once callers migrate to parsePattern (which returns the cleaned canonical form),
	// this check can be removed.
	if p.str != url {
		endHost := strings.IndexByte(url, '/')
		if endHost == -1 {
			endHost = 0
		}
		return parsedRoute{}, &PatternError{
			Pattern: url,
			Type:    "path",
			Reason:  "syntax",
			Start:   endHost,
			End:     len(url),
			Hint:    "not clean, use CleanPath",
		}
	}

	startCatchAll := 0
	if p.optionalCatchAll {
		// Reconstruct the startCatchAll index for backwards compatibility.
		// Callers use: startCatchAll > 0 && pattern[startCatchAll] == '*'
		// So we need the index of '*' in the original pattern string.
		startCatchAll = strings.LastIndexByte(url, '*')
	}

	return parsedRoute{
		token:         p.tokens,
		paramCnt:      paramCnt,
		endHost:       p.endHost,
		startCatchAll: startCatchAll,
	}, nil
}

func cleanPattern(pattern string) string {
	idx := strings.IndexByte(pattern, '/')
	if idx == -1 {
		if len(pattern) == 0 {
			return pattern
		}
		return pattern + "/"
	}
	return pattern[:idx] + CleanPath(pattern[idx+1:])
}
