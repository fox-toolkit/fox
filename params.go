// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package fox

import "context"

type ctxKey struct{}

var paramsKey = ctxKey{}

// Param is a single URL parameter captured during routing. Key is the parameter name
// as declared in the route pattern and Value is the segment captured from the request.
type Param struct {
	Key   string
	Value string
}

// Params is the list of URL parameters captured by a route. The order matches the
// declaration order of parameters in the route pattern.
type Params []Param

// Get returns the value of the parameter with the given name, or an empty string
// if no such parameter is present.
func (p Params) Get(name string) string {
	for i := range p {
		if p[i].Key == name {
			return p[i].Value
		}
	}
	return ""
}

// Has reports whether a parameter with the given name is present.
func (p Params) Has(name string) bool {
	for i := range p {
		if p[i].Key == name {
			return true
		}
	}

	return false
}

func (p Params) clone() Params {
	cloned := make(Params, len(p))
	copy(cloned, p)
	return cloned
}

// ParamsFromContext retrieves the [Params] captured by the router from a [context.Context].
// It returns nil if no parameters are stored in ctx.
func ParamsFromContext(ctx context.Context) Params {
	p, _ := ctx.Value(paramsKey).(Params)
	return p
}
