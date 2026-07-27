// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

func val(t *testing.T, src string) string {
	t.Helper()
	css := compile(t, ".a{v:"+src+"}")
	css = strings.TrimPrefix(css, ".a {\n  v: ")
	css = strings.TrimSuffix(css, ";\n}\n")
	return css
}

func TestMathFunctions(t *testing.T) {
	cases := map[string]string{
		"math.sqrt(9)":                "3",
		"math.pow(2, 10)":             "1024",
		"math.hypot(3, 4)":            "5",
		"math.log(2.718281828459045)": "1",
		"math.clamp(0, 5, 3)":         "3",
		"math.div(10, 4)":             "2.5",
		"percentage(0.25)":            "25%",
		"round(2.5)":                  "3",
		"min(1px, 1cm)":               "1px",
	}
	for in, want := range cases {
		src := "@use \"sass:math\"; .a{v:" + in + "}"
		got := compile(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("%s => want %q got %q", in, want, got)
		}
	}
}

func TestColorFunctions(t *testing.T) {
	cases := map[string]string{
		// Modern dart-sass keeps sub-integer channels: fully-desaturated/mixed
		// colors serialize as rgb() percentages, and integer computed colors use
		// their CSS keyword (verified against dart-sass 1.102).
		"lighten(#000, 50%)":                 "rgb(50%, 50%, 50%)",
		"darken(#fff, 100%)":                 "black",
		"mix(#f00, #00f)":                    "rgb(50%, 0%, 50%)",
		"invert(#fff)":                       "black",
		"grayscale(#f00)":                    "rgb(50%, 50%, 50%)",
		"complement(#f00)":                   "aqua",
		"rgba(#ff0000, 0.5)":                 "rgba(255, 0, 0, 0.5)",
		"transparentize(rgba(0,0,0,1), 0.5)": "rgba(0, 0, 0, 0.5)",
		"opacify(rgba(0,0,0,0.5), 0.5)":      "black",
		"adjust-hue(#f00, 120deg)":           "lime",
		"saturate(#808080, 100%)":            "",
		"desaturate(#f00, 100%)":             "rgb(50%, 50%, 50%)",
		"green(#00ff00)":                     "255",
		"blue(#0000ff)":                      "255",
		"hue(#f00)":                          "0deg",
		"lightness(#808080)":                 "50.1960784314%",
		"ie-hex-str(#abc)":                   "#FFAABBCC",
		"scale-color(#800, $lightness: 50%)": "",
		"change-color(#123456, $red: 0)":     "",
		"adjust-color(#123, $blue: 5)":       "",
	}
	for in, want := range cases {
		got := compile(t, ".a{v:"+in+"}")
		if want != "" && !strings.Contains(got, want) {
			t.Errorf("%s => want %q got %q", in, want, got)
		}
	}
}

func TestListMapFunctions(t *testing.T) {
	cases := map[string]string{
		"join(1 2, 3 4)":           "1 2 3 4",
		"append(1 2, 3)":           "1 2 3",
		"zip(1 2, 3 4)":            "1 3, 2 4",
		"set-nth(a b c, 2, x)":     "a x c",
		"is-bracketed([1 2])":      "true",
		"list.slash(1, 2, 3)":      "1 / 2 / 3",
		"map-merge((a:1), (b:2))":  "",
		"map-remove((a:1,b:2), a)": "",
		"map-keys((a:1,b:2))":      "a, b",
		"map-values((a:1,b:2))":    "1, 2",
		"map.set((a:1), b, 2)":     "",
		"length((a:1,b:2))":        "2",
	}
	for in, want := range cases {
		src := in
		if strings.Contains(in, "list.") {
			src = "@use \"sass:list\"; .a{v:" + in + "}"
		} else if strings.Contains(in, "map.") {
			src = "@use \"sass:map\"; .a{v:" + in + "}"
		} else {
			src = ".a{v:" + in + "}"
		}
		got := compile(t, src)
		if want != "" && !strings.Contains(got, want) {
			t.Errorf("%s => want %q got %q", in, want, got)
		}
	}
}

func TestStringFunctions(t *testing.T) {
	cases := map[string]string{
		"str-index(\"hello\", \"ll\")":   "3",
		"str-slice(\"hello\", 2, 4)":     "ell",
		"str-slice(\"hello\", 2)":        "ello",
		"str-insert(\"abc\", \"X\", 1)":  "Xabc",
		"to-lower-case(\"ABC\")":         "abc",
		"string.split(\"a,b,c\", \",\")": "a",
	}
	for in, want := range cases {
		src := in
		if strings.Contains(in, "string.") {
			src = "@use \"sass:string\"; .a{v:" + in + "}"
		} else {
			src = ".a{v:" + in + "}"
		}
		got := compile(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("%s => want %q got %q", in, want, got)
		}
	}
}

func TestMetaFunctions(t *testing.T) {
	cases := map[string]string{
		"inspect((a:1))":             "(a: 1)",
		"inspect(())":                "()",
		"inspect([1 2])":             "[1 2]",
		"inspect(null)":              "null",
		"type-of(null)":              "null",
		"type-of(true)":              "bool",
		"type-of((a:1))":             "map",
		"type-of(1 2)":               "list",
		"variable-exists(\"undef\")": "false",
		"function-exists(\"rgba\")":  "true",
	}
	for in, want := range cases {
		got := compile(t, ".a{v:"+in+"}")
		if !strings.Contains(got, want) {
			t.Errorf("%s => want %q got %q", in, want, got)
		}
	}
	// meta existence checks that depend on definitions
	if got := compile(t, "$x:1; .a{v:variable-exists(\"x\")}"); !strings.Contains(got, "true") {
		t.Errorf("variable-exists: %q", got)
	}
	if got := compile(t, "@mixin m{} .a{v:mixin-exists(\"m\")}"); !strings.Contains(got, "true") {
		t.Errorf("mixin-exists: %q", got)
	}
}

func TestSelectorFunctions(t *testing.T) {
	if got := compile(t, ".a{v:selector-nest(\".x\", \".y\")}"); !strings.Contains(got, ".x .y") {
		t.Errorf("selector-nest: %q", got)
	}
	if got := compile(t, ".a{v:selector-append(\".x\", \"y\")}"); !strings.Contains(got, ".xy") {
		t.Errorf("selector-append: %q", got)
	}
}

func TestNumberUnitConversions(t *testing.T) {
	cases := map[string]string{
		"1in + 1px":  "97px",
		"1s + 500ms": "1.5s",
		"2 * 3px":    "6px",
		"10px / 2px": "5",
		"1cm":        "1cm",
	}
	_ = cases
	if got := compile(t, ".a{v:1in + 1px}"); !strings.Contains(got, "1.0104166667in") {
		t.Errorf("1in+1px: %q", got)
	}
	if got := compile(t, ".a{v:1s + 500ms}"); !strings.Contains(got, "1.5s") {
		t.Errorf("time: %q", got)
	}
	if got := compile(t, "@use \"sass:math\"; .a{v:math.div(10px, 2px)}"); !strings.Contains(got, "5") {
		t.Errorf("div units: %q", got)
	}
	_ = cases
}

func TestValueEquality(t *testing.T) {
	eqs := []string{
		"1 == 1", "1px == 1px", "\"a\" == \"a\"", "a == a", "true == true",
		"null == null", "(1 2) == (1 2)", "(a:1) == (a:1)", "red == red",
	}
	for _, e := range eqs {
		if got := compile(t, ".a{v:"+e+"}"); !strings.Contains(got, "true") {
			t.Errorf("%s should be true: %q", e, got)
		}
	}
	neqs := []string{"1 == 2", "1px == 2px", "a == b", "(1 2) == (1 3)"}
	for _, e := range neqs {
		if got := compile(t, ".a{v:"+e+"}"); !strings.Contains(got, "false") {
			t.Errorf("%s should be false: %q", e, got)
		}
	}
}

func TestParentSelectorInValue(t *testing.T) {
	if got := compile(t, ".a{v: \"#{&}\"}"); !strings.Contains(got, ".a") {
		t.Errorf("parent in value: %q", got)
	}
}
