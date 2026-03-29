// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package fox

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrRouteNotFound           = errors.New("route not found")
	ErrRouteConflict           = errors.New("route conflict")
	ErrRouteNameExist          = errors.New("route name already registered")
	ErrInvalidRoute            = errors.New("invalid route")
	ErrDiscardedResponseWriter = errors.New("discarded response writer")
	ErrNoClientIPResolver      = errors.New("no client ip resolver")
	ErrReadOnlyTxn             = errors.New("write on read-only transaction")
	ErrSettledTxn              = errors.New("transaction settled")
	ErrParamKeyTooLarge        = errors.New("parameter key too large")
	ErrTooManyParams           = errors.New("too many params")
	ErrTooManyMatchers         = errors.New("too many matchers")
	ErrRegexpNotAllowed        = errors.New("regexp not allowed")
	ErrInvalidConfig           = errors.New("invalid config")
	ErrInvalidMatcher          = errors.New("invalid matcher")
)

// RouteConflictError represents a conflict that occurred during route registration.
// It contains the route being registered, and the existing routes that caused the conflict.
type RouteConflictError struct {
	// New is the route that was being registered when the conflict was detected.
	New *Route
	// Conflicts contains the previously registered routes that conflict with New.
	Conflicts []*Route
	// isShadowed indicate that the New route shadow other routes.
	isShadowed bool
}

func (e *RouteConflictError) Error() string {
	sb := new(strings.Builder)
	sb.WriteString("route conflict: new route\n")
	routef(sb, e.New, 4, true)

	if e.isShadowed {
		if e.New.pattern.optionalCatchAll {
			sb.WriteString("\nis shadowed by")
		} else {
			sb.WriteString("\nwould shadow")
		}
	} else {
		sb.WriteString("\nconflicts with")
	}

	for _, conflict := range e.Conflicts {
		sb.WriteByte('\n')
		routef(sb, conflict, 4, true)
	}

	return sb.String()
}

// Unwrap returns the sentinel value [ErrRouteConflict].
func (e *RouteConflictError) Unwrap() error {
	return ErrRouteConflict
}

// RouteNameConflictError represents a conflict that occurred during route name registration.
// It contains the route being registered, and the existing route that caused the conflict.
type RouteNameConflictError struct {
	// New is the route that was being registered when the conflict was detected.
	New *Route
	// Conflict is the previously registered route that conflict with New.
	Conflict *Route
}

func (e *RouteNameConflictError) Error() string {
	sb := new(strings.Builder)
	sb.WriteString("route name already registered: new route\n")
	routef(sb, e.New, 4, true)
	sb.WriteString("\nconflicts with\n")
	routef(sb, e.Conflict, 4, true)
	return sb.String()
}

// Unwrap returns the sentinel value [ErrRouteNameExist].
func (e *RouteNameConflictError) Unwrap() error {
	return ErrRouteNameExist
}

func newRouteNotFoundError(route *Route) error {
	sb := new(strings.Builder)
	sb.WriteString("route\n")
	routef(sb, route, 4, false)
	sb.WriteString("\nis not registered")
	return fmt.Errorf("%w: %s", ErrRouteNotFound, sb.String())
}

type PatternError struct {
	Pattern string // provided pattern
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
