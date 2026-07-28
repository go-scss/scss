// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
	"testing"
)

// calcVal compiles `a{b: <expr>}` and returns the serialized value of b.
func calcVal(t *testing.T, expr string) string {
	t.Helper()
	css := compile(t, "a{b: "+expr+"}")
	p := strings.SplitN(css, "b: ", 2)
	if len(p) < 2 {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(p[1], ";", 2)[0])
}

func calcValC(t *testing.T, expr string) string {
	t.Helper()
	css := compileC(t, "a{b:"+expr+"}")
	// compressed: value between ':' and '}' of the last rule.
	i := strings.Index(css, "{b:")
	if i < 0 {
		return ""
	}
	rest := css[i+3:]
	return strings.TrimSuffix(strings.SplitN(rest, "}", 2)[0], "")
}

func TestCalcResolvedForms(t *testing.T) {
	cases := map[string]string{
		// operator simplification
		"calc(1px + 2px)":          "3px",
		"calc(1px - 2px)":          "-1px",
		"calc(1px * 2)":            "2px",
		"calc(1px / 2)":            "0.5px",
		"calc(2 * 3 / 5 * 7 / 11)": "0.7636363636",
		"calc(1px + 20px - 300px)": "-279px",
		"calc(pi)":                 "3.1415926536",
		"calc(e)":                  "2.7182818285",
		"calc((1 + 2) * 3)":        "9",
		"calc(1)":                  "1",
		"calc(1px)":                "1px",
		"calc(+1px)":               "1px",
		"calc(-1px)":               "-1px",
		"calc(calc(1px + 2px))":    "3px",
		// min/max/clamp resolved
		"min(1px, 2px)":        "1px",
		"max(1px, 2px)":        "2px",
		"min(1, 2, 3)":         "1",
		"max(3cm, 1in)":        "3cm",
		"clamp(1px, 2px, 3px)": "2px",
		"clamp(2px, 1px, 3px)": "2px",
		"clamp(1px, 4px, 3px)": "3px",
		// math functions resolved
		"hypot(3px, 4px)": "5px",
		"sqrt(4)":         "2",
		"calc(cos(0))":    "1",
		"calc(sin(0))":    "0",
		"calc(tan(0))":    "0",
		"asin(0.5)":       "30deg",
		"acos(1)":         "0deg",
		"atan(1)":         "45deg",
		"abs(-5)":         "5",
		"abs(5)":          "5",
		"exp(0)":          "1",
		"sign(3)":         "1",
		"sign(-5.6)":      "-1",
		"sign(0)":         "0",
		"pow(2, 3)":       "8",
		"log(100, 10)":    "2",
		"calc(log(e))":    "1",
		"atan2(1, 1)":     "45deg",
		"rem(5, 3)":       "2",
		"rem(-5, 3)":      "-2",
		"mod(5, 3)":       "2",
		"mod(-5, 3)":      "1",
		// round strategies
		"round(1.5)":                 "2",
		"round(117, 25)":             "125",
		"round(117cm, 25mm)":         "117.5cm",
		"round(nearest, 17px, 5px)":  "15px",
		"round(up, 17px, 5px)":       "20px",
		"round(down, 17px, 5px)":     "15px",
		"round(to-zero, -17px, 5px)": "-15px",
		"round(23px, -10px)":         "20px",
		"round(down, 23px, -10px)":   "20px",
		"round(up, 23px, -10px)":     "30px",
		"round(1px)":                 "1px",
		"round(to-zero, 17px, 5px)":  "15px",
		"min(1px + 2, 3px)":          "3px",
		// calc-size
		"calc-size(auto, 100px)": "calc-size(auto, 100px)",
	}
	for in, want := range cases {
		if got := calcVal(t, in); got != want {
			t.Errorf("%s => got %q want %q", in, got, want)
		}
	}
}

func TestCalcSymbolicForms(t *testing.T) {
	cases := map[string]string{
		"calc(1px + 2%)":                     "calc(1px + 2%)",
		"calc(1px + 2% - var(--c))":          "calc(1px + 2% - var(--c))",
		"calc(1px - (2% - var(--c)))":        "calc(1px - (2% - var(--c)))",
		"calc(1px - (var(--a) + var(--b)))":  "calc(1px - (var(--a) + var(--b)))",
		"calc(1px + (var(--a) + var(--b)))":  "calc(1px + var(--a) + var(--b))",
		"calc(1px * var(--c))":               "calc(1px * var(--c))",
		"calc(1px / (2 * var(--c)))":         "calc(1px / (2 * var(--c)))",
		"calc((var(--a) + var(--b)) * 1px)":  "calc((var(--a) + var(--b)) * 1px)",
		"calc(1px + -2%)":                    "calc(1px - 2%)",
		"calc(1px * (2 / var(--c)))":         "calc(1px * 2 / var(--c))",
		"min(1px, 2%)":                       "min(1px, 2%)",
		"min(var(--a), 1px)":                 "min(var(--a), 1px)",
		"max(var(--a), var(--b))":            "max(var(--a), var(--b))",
		"clamp(1px, var(--x), 3px)":          "clamp(1px, var(--x), 3px)",
		"hypot(var(--a), 1px)":               "hypot(var(--a), 1px)",
		"hypot(1%, 2%)":                      "hypot(1%, 2%)",
		"sqrt(var(--x))":                     "sqrt(var(--x))",
		"sin(var(--x))":                      "sin(var(--x))",
		"atan(var(--x))":                     "atan(var(--x))",
		"abs(var(--x))":                      "abs(var(--x))",
		"exp(var(--x))":                      "exp(var(--x))",
		"sign(var(--x))":                     "sign(var(--x))",
		"sign(5%)":                           "sign(5%)",
		"pow(var(--x), 2)":                   "pow(var(--x), 2)",
		"log(var(--x))":                      "log(var(--x))",
		"log(var(--x), 2)":                   "log(var(--x), 2)",
		"log(2, var(--x))":                   "log(2, var(--x))",
		"atan2(var(--a), 1)":                 "atan2(var(--a), 1)",
		"atan2(1%, 2)":                       "atan2(1%, 2)",
		"atan2(1px, 2s)":                     "atan2(1px, 2s)",
		"rem(var(--a), 2)":                   "rem(var(--a), 2)",
		"rem(1px, 2s)":                       "rem(1px, 2s)",
		"mod(var(--a), 2)":                   "mod(var(--a), 2)",
		"mod(1px, 2s)":                       "mod(1px, 2s)",
		"round(1px, 10%)":                    "round(1px, 10%)",
		"round(up, var(--c))":                "round(up, var(--c))",
		"round(var(--x), 5px)":               "round(var(--x), 5px)",
		"round(nearest, 1px, 10%)":           "round(nearest, 1px, 10%)",
		"hypot(3px, var(--x))":               "hypot(3px, var(--x))",
		"hypot(3px, 4s)":                     "hypot(3px, 4s)",
		"calc(1px - -2%)":                    "calc(1px + 2%)",
		"calc(var(--a) * var(--b))":          "calc(var(--a) * var(--b))",
		"round(var(--x))":                    "round(var(--x))",
		"round(nearest, var(--x), var(--y))": "round(nearest, var(--x), var(--y))",
		"calc-size(auto)":                    "calc-size(auto)",
		"calc(var(--c))":                     "calc(var(--c))",
		"calc((var(--c)))":                   "calc((var(--c)))",
	}
	for in, want := range cases {
		if got := calcVal(t, in); got != want {
			t.Errorf("%s => got %q want %q", in, got, want)
		}
	}
}

func TestCalcNestedAndSimplify(t *testing.T) {
	cases := map[string]string{
		// calcCalc returns a nested calculation unchanged.
		"calc(min(var(--a), var(--b)))": "min(var(--a), var(--b))",
		// nested calc whose arg simplifies to a paren-needing string -> wrapped.
		"calc(1px + calc(var(--c) var(--d)))": "calc(1px + (var(--c) var(--d)))",
		// nested calc wrapping a CalcOp is unwrapped (return v.Args[0]).
		"calc(1px + calc(var(--a) + var(--b)))": "calc(1px + var(--a) + var(--b))",
		// nested calc of a plain number unwraps to that number.
		"calc(1% + calc(1px))": "calc(1% + 1px)",
		// min with non-comparable numbers stays symbolic.
		"min(1px, 1s)": "min(1px, 1s)",
		// space list with a parenthesized operation element.
		"calc((var(--a) + var(--b)) var(--c))": "calc((var(--a) + var(--b)) var(--c))",
		// nested calculation term serialized inside a bigger calc.
		"calc(1px + min(var(--a), var(--b)))": "calc(1px + min(var(--a), var(--b)))",
	}
	for in, want := range cases {
		if got := calcVal(t, in); got != want {
			t.Errorf("%s => got %q want %q", in, got, want)
		}
	}
	// valueToCalcTerm: an unquoted string from a variable is accepted.
	if got := compile(t, "@use 'sass:string'; $x: string.unquote(\"var(--y)\"); a{b: calc($x)}"); !strings.Contains(got, "calc(var(--y))") {
		t.Errorf("calc(var from unquote): %q", got)
	}
	// rem negative-zero and sign-difference branches.
	if got := calcVal(t, "rem(6, -3)"); got != "0" {
		t.Errorf("rem(6,-3): %q", got)
	}
	if got := calcVal(t, "rem(-5, 3)"); got != "-2" {
		t.Errorf("rem(-5,3): %q", got)
	}
	if got := calcVal(t, "rem(-5, infinity)"); got != "-5" {
		t.Errorf("rem(-5,infinity): %q", got)
	}
}

func TestCalcInfinityNaNSerialization(t *testing.T) {
	cases := map[string]string{
		"sign(nan)":                         "calc(NaN)",
		"calc(infinity * 1px + var(--c))":   "calc(infinity * 1px + var(--c))",
		"calc(-infinity + var(--c))":        "calc(-infinity + var(--c))",
		"calc(nan + var(--c))":              "calc(NaN + var(--c))",
		"calc(infinity / 1px + var(--c))":   "calc(infinity / 1px + var(--c))",
		"calc(1px * 1s + var(--c))":         "calc(1px * 1s + var(--c))",
		"calc(var(--c) / (1px * 1s))":       "calc(var(--c) / (1px * 1s))",
		"calc(var(--c) / (infinity * 1px))": "calc(var(--c) / (infinity * 1px))",
	}
	for in, want := range cases {
		if got := calcVal(t, in); got != want {
			t.Errorf("%s => got %q want %q", in, got, want)
		}
	}
}

func TestCalcRoundInfiniteAndNaN(t *testing.T) {
	// These exercise the infinite/NaN branches of roundWithStep via math.div so
	// the sign of the (possibly signed-zero) result is observable.
	cases := map[string]string{
		"@use 'sass:math'; a{b: math.div(1, round(5, infinity))}":          "infinity",
		"@use 'sass:math'; a{b: math.div(1, round(-5, infinity))}":         "-infinity",
		"@use 'sass:math'; a{b: math.div(1, round(up, 5, infinity))}":      "0",
		"@use 'sass:math'; a{b: math.div(1, round(up, -5, infinity))}":     "-infinity",
		"@use 'sass:math'; a{b: math.div(1, round(down, 5, infinity))}":    "infinity",
		"@use 'sass:math'; a{b: math.div(1, round(down, -5, infinity))}":   "0",
		"@use 'sass:math'; a{b: math.div(1, round(to-zero, 5, infinity))}": "infinity",
		"@use 'sass:math'; a{b: math.div(1, round(nearest, 0, infinity))}": "infinity",
	}
	for in, want := range cases {
		if got := compile(t, in); !strings.Contains(got, want) {
			t.Errorf("%s => %q, want contains %q", in, got, want)
		}
	}
	// number/step NaN and infinite branches (resolve to a value we can print).
	direct := map[string]string{
		"round(infinity, infinity)": "NaN",
		"round(5, 0)":               "NaN",
		"round(nan, 5)":             "NaN",
		"round(infinity, 5)":        "infinity",
	}
	for in, want := range direct {
		if got := calcVal(t, in); !strings.Contains(got, want) {
			t.Errorf("%s => %q want contains %q", in, got, want)
		}
	}
}

func TestCalcCompressedSerialization(t *testing.T) {
	// Compressed output drops spaces around * and / but keeps them around + and -.
	cases := map[string]string{
		"calc(1px + var(--c))":          "calc(1px + var(--c))",
		"calc(1px*var(--c))":            "calc(1px*var(--c))",
		"calc(1px/var(--c))":            "calc(1px/var(--c))",
		"calc(infinity*1px + var(--c))": "calc(infinity*1px + var(--c))",
	}
	for in, want := range cases {
		if got := calcValC(t, in); got != want {
			t.Errorf("compressed %s => got %q want %q", in, got, want)
		}
	}
}

func TestCalcEquality(t *testing.T) {
	if got := calcVal(t, "calc(1px + var(--x)) == calc(1px + var(--x))"); got != "true" {
		t.Errorf("calc equality: %q", got)
	}
	if got := calcVal(t, "calc(1px + var(--x)) == calc(2px + var(--x))"); got != "false" {
		t.Errorf("calc inequality (arg): %q", got)
	}
	if got := calcVal(t, "min(var(--a), 1px) == max(var(--a), 1px)"); got != "false" {
		t.Errorf("calc inequality (name): %q", got)
	}
	if got := calcVal(t, "min(var(--a), sqrt(var(--x))) == min(var(--a), sqrt(var(--x)))"); got != "true" {
		t.Errorf("nested calc equality: %q", got)
	}
	if got := calcVal(t, "calc(1px + var(--x)) == 3px"); got != "false" {
		t.Errorf("calc vs number: %q", got)
	}
}

func TestCalcMetaFunctions(t *testing.T) {
	cases := map[string]string{
		"@use 'sass:meta'; a{b: meta.type-of(calc(var(--c)))}":                                                "calculation",
		"@use 'sass:meta'; a{b: meta.type-of(clamp(1px, var(--x), 3px))}":                                     "calculation",
		"@use 'sass:list';@use 'sass:meta'; a{b: list.length(meta.calc-args(calc(var(--c))))}":                "1",
		"@use 'sass:list';@use 'sass:meta'; a{b: list.nth(meta.calc-args(calc(var(--c))), 1)}":                "var(--c)",
		"@use 'sass:list';@use 'sass:meta'; a{b: list.length(meta.calc-args(clamp(1%, 2px, 3px)))}":           "3",
		"@use 'sass:list';@use 'sass:meta'; a{b: list.nth(meta.calc-args(calc(1px + var(--c))), 1)}":          "1px + var(--c)",
		"@use 'sass:list';@use 'sass:meta'; a{b: list.nth(meta.calc-args(min(var(--a), sqrt(var(--x)))), 2)}": "sqrt(var(--x))",
	}
	for in, want := range cases {
		if got := compile(t, in); !strings.Contains(got, want) {
			t.Errorf("%s => %q, want contains %q", in, got, want)
		}
	}
	mustErr(t, "@use 'sass:meta'; a{b: meta.calc-args(1px)}")
}

func TestEnvFunction(t *testing.T) {
	cases := map[string]string{
		"a{b: env(safe-area-inset-top)}": "env(safe-area-inset-top)",
		"$x: 10px; a{b: env(#{$x})}":     "env(10px)",
		"a{b: env((a) b)}":               "env((a) b)",
	}
	for in, want := range cases {
		if got := compile(t, in); !strings.Contains(got, want) {
			t.Errorf("%s => %q want contains %q", in, got, want)
		}
	}
	mustErr(t, "a{b: env(#{1px)}") // unterminated interpolation
}

func TestMathModuleFunctions(t *testing.T) {
	cases := map[string]string{
		"@use 'sass:math'; a{b: math.asin(-0.5)}":        "-30deg",
		"@use 'sass:math'; a{b: math.acos(0.5)}":         "60deg",
		"@use 'sass:math'; a{b: math.atan(1)}":           "45deg",
		"@use 'sass:math'; a{b: math.atan2(1, -1)}":      "135deg",
		"@use 'sass:math'; a{b: math.atan2(1cm, -10mm)}": "135deg",
		"@use 'sass:math'; a{b: math.exp(0)}":            "1",
		"@use 'sass:math'; a{b: math.exp(1)}":            "2.7182818285",
	}
	for in, want := range cases {
		if got := compile(t, in); !strings.Contains(got, want) {
			t.Errorf("%s => %q want contains %q", in, got, want)
		}
	}
}

func TestColorSpecialFunctions(t *testing.T) {
	cases := map[string]string{
		"a{b: rgb(calc(1px + 1%) 2 3 / 0.4)}":                                  "rgb(calc(1px + 1%), 2, 3, 0.4)",
		"a{b: rgb(1 2 3 / var(--c))}":                                          "rgb(1, 2, 3, var(--c))",
		"a{b: rgb(1, 2, calc(1px + 1%))}":                                      "rgb(1, 2, calc(1px + 1%))",
		"a{b: rgb(1, 2, 3, calc(1px + 1%))}":                                   "rgb(1, 2, 3, calc(1px + 1%))",
		"a{b: rgb(var(--foo) 2 / 0.4)}":                                        "rgb(var(--foo) 2/0.4)",
		"a{b: hsl(calc(1deg + var(--x)) 2% 3%)}":                               "hsl(calc(1deg + var(--x)), 2%, 3%)",
		"@use 'sass:string'; a{b: rgb(string.unquote(\"calc(1)\") 2 3 / 0.4)}": "rgb(calc(1), 2, 3, 0.4)",
	}
	for in, want := range cases {
		if got := compile(t, in); !strings.Contains(got, want) {
			t.Errorf("%s => %q want contains %q", in, got, want)
		}
	}
}

func TestCalcErrors(t *testing.T) {
	errs := []string{
		"a{b: min()}",                                  // missing argument
		"a{b: calc(1px, 2px)}",                         // too many args
		"a{b: calc($x: 1)}",                            // keyword arg
		"a{b: calc((1, 2, 3)...)}",                     // rest arg
		"a{b: sqrt(1px)}",                              // forbid units
		"a{b: exp(1px)}",                               // exp units
		"a{b: pow(1px, 2)}",                            // pow units
		"a{b: log(1px)}",                               // log units
		"a{b: round(up, 5px)}",                         // strategy needs step
		"a{b: round((var(--a) + var(--b)), 1px, 2px)}", // invalid strategy
		"a{b: calc(\"foo\")}",                          // quoted string in calc
		"a{b: calc(red and blue)}",                     // non-calc operation
		"a{b: calc(not true)}",                         // unary op in calc
		"a{b: calc((a: b))}",                           // map in calc
		"a{b: calc((1, 2))}",                           // comma list in calc
		"$x: (1 2 3); a{b: calc($x)}",                  // list value in calc
		"a{b: min(\"a\", \"b\")}",                      // legacy fallback with unsafe args
	}
	for _, s := range errs {
		mustErr(t, s)
	}
}

func TestCalcHelpersDirect(t *testing.T) {
	// isSpecialNumberString / isSpecialVariableString.
	if !isSpecialNumberString("calc(1)") || !isSpecialNumberString("VAR(--x)") || isSpecialNumberString("red") || isSpecialNumberString("ca") {
		t.Error("isSpecialNumberString")
	}
	if !isSpecialVariableString(&SassString{Text: "var(--x)"}) {
		t.Error("isSpecialVariableString")
	}
	// legacyColorFunction.
	if !legacyColorFunction("rgba") || legacyColorFunction("oklch") {
		t.Error("legacyColorFunction")
	}
	// calcPrecedence.
	if calcPrecedence("*") != 2 || calcPrecedence("+") != 1 {
		t.Error("calcPrecedence")
	}
	// needsParentheses branches.
	np := []struct {
		s    string
		want bool
	}{
		{"", false}, {" ", true}, {"a", false}, {"a ", true},
		{"var(", true}, {"vax(", false}, {"ab", false}, {"a/b", true},
		{"abc", false}, {"a*c", true}, {"abcd", false}, {"ab cd", true},
		{"v", false}, {"va", false}, {"var", false},
	}
	for _, c := range np {
		if got := needsParentheses(c.s); got != c.want {
			t.Errorf("needsParentheses(%q)=%v want %v", c.s, got, c.want)
		}
	}
	// signIncludingZero and sign2.
	if signIncludingZero(-0.0) == 1 {
		// -0.0 == 0 in Go comparisons; ensure Signbit path used elsewhere.
	}
	if sign2(2) != 1 || sign2(-2) != -1 || sign2(0) != 0 {
		t.Error("sign2")
	}
	// serializeCalcTerm on each term type.
	if serializeCalcTerm(&Number{Val: 2, Numer: []string{"px"}}, false) != "2px" {
		t.Error("serializeCalcTerm number")
	}
	if serializeCalcTerm(&SassString{Text: "var(--x)"}, false) != "var(--x)" {
		t.Error("serializeCalcTerm string")
	}
	op := &CalcOp{Op: "+", Left: &Number{Val: 1}, Right: &SassString{Text: "var(--x)"}}
	if serializeCalcTerm(op, false) != "1 + var(--x)" {
		t.Error("serializeCalcTerm op")
	}
	// valueOfTerm on a Value and a non-Value (CalcOp).
	if _, ok := valueOfTerm(&Number{Val: 1}).(*Number); !ok {
		t.Error("valueOfTerm value")
	}
	if _, ok := valueOfTerm(op).(*SassString); !ok {
		t.Error("valueOfTerm op")
	}
	// calc value method surface.
	c := &SassCalculation{Name: "calc", Args: []calcTerm{&SassString{Text: "var(--x)"}}}
	if !c.isTruthy() || c.sep() != SepUndecided || len(c.asList()) != 1 {
		t.Error("SassCalculation value methods")
	}
	if !c.equals(&SassCalculation{Name: "calc", Args: []calcTerm{&SassString{Text: "var(--x)"}}}) {
		t.Error("SassCalculation equals")
	}
	if c.equals(&Number{Val: 1}) {
		t.Error("SassCalculation equals non-calc")
	}
	// calcTermEquals across mismatched types.
	if calcTermEquals(&Number{Val: 1}, &SassString{Text: "x"}) {
		t.Error("calcTermEquals mismatch")
	}
	if calcTermEquals(&CalcOp{Op: "+"}, &Number{Val: 1}) {
		t.Error("calcTermEquals op-vs-number")
	}
}

func TestExprCalcSafeDirect(t *testing.T) {
	e := &evaluator{}
	num := &NumberLit{Val: 1}
	safe := []Expr{
		&Binary{Op: "+", Left: num, Right: num},
		&FuncCall{Name: "var"},
		&ListExpr{Sep: SepSpace, Elements: []Expr{num, num}},
		num,
		&Paren{Expr: num},
		&InterpExpr{Expr: num},
		&VarRef{Name: "x"},
		&Unary{Op: "-", Expr: num},
		&Unary{Op: "+", Expr: num},
		&Ident{Name: "foo"},
		&StringLit{Quoted: false, Parts: literalInterp("foo")},
		&StringLit{Quoted: false, Parts: &Interp{Parts: []any{num}}}, // non-plain -> "" -> safe
	}
	for i, ex := range safe {
		if !e.exprCalcSafe(ex) {
			t.Errorf("expected calc-safe [%d] %T", i, ex)
		}
	}
	unsafe := []Expr{
		&Binary{Op: "and", Left: num, Right: num},
		&BoolLit{V: true},
		&ColorLit{},
		&ListExpr{Sep: SepComma, Elements: []Expr{num, num}},
		&ListExpr{Sep: SepSpace, Bracketed: true, Elements: []Expr{num, num}},
		&ListExpr{Sep: SepSpace, Elements: []Expr{num}},
		&ListExpr{Sep: SepSpace, Elements: []Expr{num, &BoolLit{}}},
		&MapExpr{},
		&NullLit{},
		&Parent{},
		&Unary{Op: "not", Expr: num},
		&Unary{Op: "-", Expr: &VarRef{Name: "x"}},
		&Ident{Name: "!important"},
		&StringLit{Quoted: true, Parts: literalInterp("x")},
	}
	for i, ex := range unsafe {
		if e.exprCalcSafe(ex) {
			t.Errorf("expected NOT calc-safe [%d] %T", i, ex)
		}
	}
	// plainCalcSafe edge branches.
	for _, s := range []string{"!x", "#x", "u+0", "url(x"} {
		if plainCalcSafe(s) {
			t.Errorf("plainCalcSafe(%q) should be false", s)
		}
	}
	if !plainCalcSafe("abc") {
		t.Error("plainCalcSafe(abc)")
	}
	// argsCalcSafe direct branches: nil, named, and unsafe element.
	if e.argsCalcSafe(nil) {
		t.Error("argsCalcSafe(nil)")
	}
	if e.argsCalcSafe(&ArgList{Args: []Arg{{Name: "x", Value: num}}}) {
		t.Error("argsCalcSafe(named)")
	}
	if e.argsCalcSafe(&ArgList{Args: []Arg{{Value: &BoolLit{}}}}) {
		t.Error("argsCalcSafe(unsafe)")
	}
	if !e.argsCalcSafe(&ArgList{Args: []Arg{{Value: num}}}) {
		t.Error("argsCalcSafe(safe)")
	}
}

func TestCalcDefensiveDirect(t *testing.T) {
	e := &evaluator{}
	// calcTermEquals with an out-of-domain term hits the trailing false.
	if calcTermEquals(sassTrue, sassTrue) {
		t.Error("calcTermEquals non-term")
	}
	// writeCalcTerm default case (a Value that isn't a calc term type).
	if serializeCalcTerm(sassTrue, false) != "true" {
		t.Errorf("serializeCalcTerm bool: %q", serializeCalcTerm(sassTrue, false))
	}
	// calcSimplify rejects an out-of-domain value.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("calcSimplify(bool) should panic")
			}
		}()
		e.calcSimplify(sassTrue)
	}()
	// needsParentheses extra branches: uppercase var, char-4 special, tail loop.
	nps := map[string]bool{"VAR(": true, "abc/": true, "abcd/": true, "abcde": false, "var/": true}
	for s, want := range nps {
		if got := needsParentheses(s); got != want {
			t.Errorf("needsParentheses(%q)=%v want %v", s, got, want)
		}
	}
}

func TestCalcNumberHelpersDirect(t *testing.T) {
	px := func(v float64) *Number { return &Number{Val: v, Numer: []string{"px"}} }
	// convertValueToMatch fallback on incompatible units.
	if got := (px(1)).convertValueToMatch(&Number{Numer: []string{"s"}}); got != 1 {
		t.Errorf("convertValueToMatch incompatible: %v", got)
	}
	// isComparableTo: unitless is comparable to anything; incompatible units not.
	if !(&Number{Val: 1}).isComparableTo(px(2)) {
		t.Error("unitless comparable")
	}
	if px(1).isComparableTo(&Number{Val: 1, Numer: []string{"s"}}) {
		t.Error("px not comparable to s")
	}
	// coerceToMatch with units (converts) and unitless (adopts).
	if got := (&Number{Val: 2, Numer: []string{"cm"}}).coerceToMatch(px(0)); got.Numer[0] != "px" {
		t.Errorf("coerceToMatch units: %+v", got)
	}
	if got := (&Number{Val: 2}).coerceToMatch(px(0)); got.Numer[0] != "px" || got.Val != 2 {
		t.Errorf("coerceToMatch unitless: %+v", got)
	}
	// moduloLikeSass across all branches.
	nan := math.IsNaN
	if !nan(moduloLikeSass(math.Inf(1), 3)) {
		t.Error("mod inf dividend")
	}
	if moduloLikeSass(5, math.Inf(1)) != 5 {
		t.Error("mod inf modulus same sign")
	}
	if !nan(moduloLikeSass(-5, math.Inf(1))) {
		t.Error("mod inf modulus diff sign")
	}
	if moduloLikeSass(5, 3) != 2 {
		t.Error("mod pos")
	}
	if !nan(moduloLikeSass(5, 0)) {
		t.Error("mod zero")
	}
	if moduloLikeSass(-6, -3) != 0 {
		t.Error("mod neg exact")
	}
	if moduloLikeSass(1, -3) != -2 {
		t.Error("mod neg wrap")
	}
	if moduloLikeSass(-5, -3) != -2 {
		t.Error("mod neg")
	}
	// signIncludingZero.
	if signIncludingZero(negZero()) != -1 || signIncludingZero(0) != 1 || signIncludingZero(3) != 1 || signIncludingZero(-3) != -1 {
		t.Error("signIncludingZero")
	}
}

func negZero() float64 {
	z := 0.0
	return -z
}
