// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "math"

// mathModuleVars holds the constant variables exported by the sass:math module.
// These mirror dart-sass's math module ($pi, $e, $epsilon, the safe-integer
// bounds and the double magnitude bounds).
var mathModuleVars = map[string]Value{
	"pi":               &Number{Val: math.Pi},
	"e":                &Number{Val: math.E},
	"epsilon":          &Number{Val: 2.220446049250313e-16},
	"max-safe-integer": &Number{Val: 9007199254740991},
	"min-safe-integer": &Number{Val: -9007199254740991},
	"max-number":       &Number{Val: math.MaxFloat64},
	"min-number":       &Number{Val: math.SmallestNonzeroFloat64},
}

// builtinModuleVar resolves a variable exported by a built-in module (only
// sass:math currently exports any). The name is matched dash/underscore
// insensitively, as Sass identifiers are.
func builtinModuleVar(mod, name string) (Value, bool) {
	if mod != "math" {
		return nil, false
	}
	v, ok := mathModuleVars[normIdent(name)]
	return v, ok
}
