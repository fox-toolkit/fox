package fox

import (
	"fmt"
	"regexp"
	"strings"
)

type pattern struct {
	// Canonical cleaned pattern: hostname + CleanPath(path).
	str              string
	tokens           []token
	optionalCatchAll bool
	endHost          int
}

// parsePattern parses and validates a route pattern by splitting it into hostname and path,
// cleaning the path, and delegating to focused sub-parsers.
func (fox *Router) parsePattern(raw string) (*pattern, int, error) {
	endHost := strings.IndexByte(raw, '/')
	if endHost == -1 {
		return nil, 0, fmt.Errorf("%w: missing trailing '/' after hostname", ErrInvalidRoute)
	}

	var (
		paramCount int
		hostTokens []token
	)

	if endHost > 0 {
		var err error
		hostTokens, paramCount, err = fox.parseHostname(raw[:endHost])
		if err != nil {
			return nil, 0, err
		}
	}

	// Clean the path to normalize traversal patterns (e.g. /foo/../bar -> /bar).
	cleanedPath := CleanPath(raw[endHost:])

	pathTokens, optCatchAll, paramCount, err := fox.parsePath(cleanedPath, paramCount)
	if err != nil {
		return nil, 0, err
	}

	tokens := make([]token, 0, len(hostTokens)+len(pathTokens))
	tokens = append(tokens, hostTokens...)
	tokens = append(tokens, pathTokens...)

	return &pattern{
		str:              raw[:endHost] + cleanedPath,
		tokens:           tokens,
		endHost:          endHost,
		optionalCatchAll: optCatchAll,
	}, paramCount, nil
}

// hostnameValidator tracks RFC 5890 hostname label state during parsing.
// It validates static characters one at a time, while the caller handles parameter tokenization.
type hostnameValidator struct {
	partLen    int  // Current label length in bytes.
	totalLen   int  // Total hostname length in bytes.
	last       byte // Last static char for dot/dash adjacency rules.
	nonNumeric bool // True once we've seen a letter, hyphen, or parameter.
}

// checkByte validates a single static hostname character against RFC 5890 rules
// (dot/dash adjacency, label length, uppercase, illegal characters) and updates tracking state.
func (v *hostnameValidator) checkByte(c byte) error {
	switch {
	case 'a' <= c && c <= 'z' || c == '_':
		v.nonNumeric = true
		v.partLen++
	case '0' <= c && c <= '9':
		v.partLen++
	case c == '-':
		if v.last == '.' {
			return fmt.Errorf("%w: illegal '-' after '.' in hostname label", ErrInvalidRoute)
		}
		v.partLen++
		v.nonNumeric = true
	case c == '.':
		if v.last == '.' {
			return fmt.Errorf("%w: unexpected consecutive '.' in hostname", ErrInvalidRoute)
		}
		if v.last == '-' {
			return fmt.Errorf("%w: illegal '-' before '.' in hostname label", ErrInvalidRoute)
		}
		if v.partLen > 63 {
			return fmt.Errorf("%w: hostname label exceed 63 characters", ErrInvalidRoute)
		}
		v.totalLen += v.partLen + 1 // +1 counts the current dot.
		v.partLen = 0
	case 'A' <= c && c <= 'Z':
		return fmt.Errorf("%w: illegal uppercase character '%s' in hostname label", ErrInvalidRoute, string(c))
	default:
		return fmt.Errorf("%w: illegal character '%s' in hostname label", ErrInvalidRoute, string(c))
	}
	v.last = c
	return nil
}

// skipParam resets adjacency state after a {param} and marks the hostname as non-numeric.
func (v *hostnameValidator) skipParam() {
	v.last = 0
	v.nonNumeric = true
}

// postCheck runs final hostname validation: trailing dash/dot, all-numeric, label and total length.
func (v *hostnameValidator) postCheck() error {
	v.totalLen += v.partLen
	if v.last == '-' {
		return fmt.Errorf("%w: illegal trailing '-' in hostname label", ErrInvalidRoute)
	}
	if v.last == '.' {
		return fmt.Errorf("%w: illegal trailing '.' in hostname label", ErrInvalidRoute)
	}
	if !v.nonNumeric {
		return fmt.Errorf("%w: invalid all numeric hostname", ErrInvalidRoute)
	}
	if v.partLen > 63 {
		return fmt.Errorf("%w: hostname label exceed 63 characters", ErrInvalidRoute)
	}
	if v.totalLen > 253 {
		return fmt.Errorf("%w: hostname exceed 253 characters", ErrInvalidRoute)
	}
	return nil
}

// parseHostname validates and tokenizes the hostname portion of a route pattern.
// It enforces RFC 5890 rules for labels and returns the number of parameters found.
func (fox *Router) parseHostname(hostname string) ([]token, int, error) {
	var sb strings.Builder
	sb.Grow(len(hostname))
	tokens := make([]token, 0, 1) // At least one token.
	// Initialize last to dotDelim so that a leading '-' is caught by the "dash after dot" rule.
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
			if isWild {
				i++
				if i >= len(hostname) || hostname[i] != '{' {
					return nil, 0, fmt.Errorf("%w: missing '{param}' after '+' catch-all delimiter", ErrInvalidRoute)
				}
				if prevWild && staticSinceWild <= 1 {
					return nil, 0, fmt.Errorf("%w: consecutive wildcard not allowed", ErrInvalidRoute)
				}
			}
			name, re, n, err := fox.parseBrace(hostname[i:], dotDelim, false)
			if err != nil {
				return nil, 0, err
			}
			paramCount++
			if paramCount > fox.maxParams {
				return nil, 0, fmt.Errorf("%w: %w", ErrInvalidRoute, ErrTooManyParams)
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
			// After closing brace, next char must be '.' (hostname delimiter) or end.
			if i < len(hostname) && hostname[i] != '.' {
				return nil, 0, fmt.Errorf("%w: illegal character '%s' after '{param}'", ErrInvalidRoute, string(hostname[i]))
			}

		case '*':
			// Optional wildcard *{param} is suffix-only; hostname always has a path after it.
			i++
			if i < len(hostname) && hostname[i] == '{' {
				return nil, 0, fmt.Errorf("%w: '*{param}' allowed only as suffix", ErrInvalidRoute)
			}
			return nil, 0, fmt.Errorf("%w: missing '{param}' after '*' catch-all delimiter", ErrInvalidRoute)

		default:
			if err := validator.checkByte(c); err != nil {
				return nil, 0, err
			}
			sb.WriteByte(c)
			staticSinceWild++
			i++
		}
	}

	if err := validator.postCheck(); err != nil {
		return nil, 0, err
	}

	if sb.Len() > 0 {
		tokens = append(tokens, token{typ: nodeStatic, value: sb.String(), hsplit: true})
		sb.Reset()
	}

	return tokens, paramCount, nil
}

// parsePath validates and tokenizes the path portion of a route pattern.
// The path must already be cleaned via CleanPath. paramCount is the number of parameters
// already parsed (e.g. from the hostname). Returns tokens, whether the path ends with an
// optional catch-all *{param}, and the updated total parameter count.
func (fox *Router) parsePath(path string, paramCount int) ([]token, bool, int, error) {
	var sb strings.Builder
	sb.Grow(len(path))
	tokens := make([]token, 0, 1) // At least one token.
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
			if isWild {
				i++
				if i >= len(path) || path[i] != '{' {
					return nil, false, 0, fmt.Errorf(
						"%w: missing '{param}' after '%c' catch-all delimiter", ErrInvalidRoute, c,
					)
				}
				if prevWild && staticSinceWild <= 1 {
					return nil, false, 0, fmt.Errorf("%w: consecutive wildcard not allowed", ErrInvalidRoute)
				}
			}
			name, re, n, err := fox.parseBrace(path[i:], slashDelim, isOpt)
			if err != nil {
				return nil, false, 0, err
			}
			paramCount++
			if paramCount > fox.maxParams {
				return nil, false, 0, fmt.Errorf("%w: %w", ErrInvalidRoute, ErrTooManyParams)
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
			// Optional wildcard *{param} is only allowed as suffix (last thing in path).
			if isOpt {
				if i < len(path) {
					return nil, false, 0, fmt.Errorf("%w: '*{param}' allowed only as suffix", ErrInvalidRoute)
				}
				optCatchAll = true
			}
			// After closing brace, next char must be '/' or end of path.
			if i < len(path) && path[i] != '/' {
				return nil, false, 0, fmt.Errorf(
					"%w: illegal character '%s' after '{param}'", ErrInvalidRoute, string(path[i]),
				)
			}

		default:
			// Reject ASCII control characters.
			if c < ' ' || c == 0x7f {
				return nil, false, 0, fmt.Errorf("%w: illegal control character in path", ErrInvalidRoute)
			}
			sb.WriteByte(c)
			staticSinceWild++
			i++
		}
	}

	if sb.Len() > 0 {
		tokens = append(tokens, token{typ: nodeStatic, value: sb.String()})
		sb.Reset()
	}
	return tokens, optCatchAll, paramCount, nil
}

// parseBrace parses a parameter starting at '{' in s. It returns the parameter name,
// compiled regexp (nil if none), and total bytes consumed (including '{' and '}').
// delim is the segment delimiter ('/' for path, '.' for hostname).
// isOptional indicates *{} (optional catch-all, which disallows regexp constraints).
func (fox *Router) parseBrace(s string, delim byte, isOptional bool) (string, *regexp.Regexp, int, error) {
	// Skip s[0] (the opening '{') and start at nesting level 1 to account for it.
	idx := braceIndex(s[1:], 1)
	if idx == -1 {
		return "", nil, 0, fmt.Errorf("%w: unbalanced braces in parameter definition", ErrInvalidRoute)
	}

	content := s[1 : 1+idx] // Everything between { and }.
	consumed := 1 + idx + 1 // { + content + }

	// Split into name and optional regex on first ':'.
	name := content
	var rawRegex string
	hasRegex := false
	if colonIdx := strings.IndexByte(content, ':'); colonIdx >= 0 {
		name = content[:colonIdx]
		rawRegex = content[colonIdx+1:]
		hasRegex = true
	}

	// Validate name length before possibly expensive regexp compilation.
	if len(name) > fox.maxParamKeyBytes {
		return "", nil, 0, fmt.Errorf("%w: %w", ErrInvalidRoute, ErrParamKeyTooLarge)
	}

	if len(name) == 0 {
		if hasRegex {
			return "", nil, 0, fmt.Errorf("%w: missing parameter name", ErrInvalidRoute)
		}
		return "", nil, 0, fmt.Errorf("%w: missing parameter name between '{}'", ErrInvalidRoute)
	}

	// Validate name characters: no delimiters, no special chars.
	for j := 0; j < len(name); j++ {
		switch name[j] {
		// TODO: just put . and /, add also }
		case delim, '/', '*', '+', '{':
			return "", nil, 0, fmt.Errorf(
				"%w: illegal character '%s' in '{param}'", ErrInvalidRoute, string(name[j]),
			)
		}
	}

	if !hasRegex {
		return name, nil, consumed, nil
	}

	// Optional wildcards (*{param}) cannot have regexps because they match empty strings,
	// making it impossible to disambiguate routes with different regexps.
	if isOptional {
		return "", nil, 0, fmt.Errorf("%w: %w in optional wildcard", ErrInvalidRoute, ErrRegexpNotAllowed)
	}

	re, err := fox.compileParamRegexp(rawRegex)
	if err != nil {
		return "", nil, 0, err
	}
	return name, re, consumed, nil
}

// compileParamRegexp validates and compiles a regular expression constraint for a parameter.
func (fox *Router) compileParamRegexp(rawRegex string) (*regexp.Regexp, error) {
	if !fox.allowRegexp {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRoute, ErrRegexpNotAllowed)
	}
	if rawRegex == "" {
		return nil, fmt.Errorf("%w: missing regular expression", ErrInvalidRoute)
	}

	re, err := regexp.Compile("^" + rawRegex + "$")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRoute, err)
	}
	if re.NumSubexp() > 0 {
		return nil, fmt.Errorf(
			"%w: illegal capture group '%s': use (?:pattern) instead", ErrInvalidRoute, rawRegex,
		)
	}

	return re, nil
}

// braceIndex returns the index of the closing brace that balances an opening
// brace. It starts at startLevel opened brace.
//
// Example: For pattern "{id:[0-9]{1,3}}", the caller would pass "[0-9]{1,3}}" and 1
// (everything after the initial '{'), and this returns 10 (index of the final '}').
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
	p, _, err := fox.parsePattern(url)
	if err != nil {
		return parsedRoute{}, err
	}

	// Backward compatibility: callers store the original url as the route pattern,
	// so we must reject paths that CleanPath would normalize (e.g. //, ./, ../).
	// Once callers migrate to parsePattern (which returns the cleaned canonical form),
	// this check can be removed.
	if p.str != url {
		return parsedRoute{}, fmt.Errorf("%w: path is not clean, use CleanPath", ErrInvalidRoute)
	}

	paramCnt := 0
	for _, tk := range p.tokens {
		if tk.typ != nodeStatic {
			paramCnt++
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
