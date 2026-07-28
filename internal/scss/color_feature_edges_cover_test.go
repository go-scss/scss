// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestColorNonFiniteChannels covers how non-finite (NaN / ±infinity) channel and
// alpha values are carried through colour construction and serialization, matching
// dart-sass 1.102 byte-for-byte. A non-alpha colour channel keeps its non-finite
// value as calc(NaN)/calc(infinity)/calc(-infinity); the alpha channel is clamped
// into [0, 1] with NaN collapsing to 0; a clamped legacy channel collapses a NaN
// to its bound.
func TestColorNonFiniteChannels(t *testing.T) {
	cases := map[string]string{
		// color() channels preserve non-finite values verbatim.
		"color(srgb calc(NaN) 0 0)":       "color(srgb calc(NaN) 0 0)",
		"color(srgb calc(infinity) 0 0)":  "color(srgb calc(infinity) 0 0)",
		"color(srgb 0 0 calc(NaN) / 0.5)": "color(srgb 0 0 calc(NaN) / 0.5)",
		// Alpha is clamped: NaN and -infinity -> 0, +infinity -> 1 (omitted).
		"color(srgb 0 0 0 / calc(NaN))":       "color(srgb 0 0 0 / 0)",
		"color(srgb 0 0 0 / calc(-infinity))": "color(srgb 0 0 0 / 0)",
		"color(srgb 0 0 0 / calc(infinity))":  "color(srgb 0 0 0)",
		// Legacy rgb channels are clamped 0-255: NaN -> 0, infinity -> 255.
		"rgb(0, 0, calc(NaN), 0.5)":       "rgba(0, 0, 0, 0.5)",
		"rgb(0, 0, calc(infinity), 0.5)":  "rgba(0, 0, 255, 0.5)",
		"rgb(0, 0, calc(-infinity), 0.5)": "rgba(0, 0, 0, 0.5)",
		// Other spaces: unclamped a/b channels keep NaN, clamped lightness -> 0.
		"oklab(0 0 calc(NaN))": "oklab(0% 0 calc(NaN))",
		"lab(calc(NaN) 0 0)":   "lab(0% 0 0)",
	}
	for in, want := range cases {
		if got := val(t, in); got != want {
			t.Errorf("val(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSlashListWithCalcOperand covers the slash-separated-list rule: a "/" whose
// operands are number literals or bare CSS calculations (calc/clamp/sqrt/…) forms
// a slash list rather than performing division, while variables, parenthesised
// expressions and the legacy-gated math globals (min/max/abs/round) still divide.
func TestSlashListWithCalcOperand(t *testing.T) {
	cases := map[string]string{
		"0 0 0 / calc(5)":    "0 0 0/5",
		"0 0 0 / calc(NaN)":  "0 0 0/calc(NaN)",
		"1 / calc(2)":        "1/2",
		"calc(2) / 3":        "2/3",
		"sqrt(4) / 3":        "2/3",
		"clamp(1, 2, 3) / 3": "2/3",
		"-2 / calc(3)":       "-2/3",
		// Non-numberish operands divide (matching dart's slash-div behaviour).
		"min(2, 4) / 3": "0.6666666667",
		"abs(2) / 3":    "0.6666666667",
		"(2) / 3":       "0.6666666667",
	}
	for in, want := range cases {
		if got := val(t, in); got != want {
			t.Errorf("val(%q) = %q, want %q", in, got, want)
		}
	}
	// A variable operand also divides rather than forming a slash list.
	css := compile(t, "$x: 1; .a{ v: $x / calc(2) }")
	if want := ".a {\n  v: 0.5;\n}\n"; css != want {
		t.Errorf("variable slash = %q, want %q", css, want)
	}
	// A namespaced calc function (math.sqrt) is not a bare calculation, so it
	// divides rather than forming a slash list.
	css = compile(t, `@use "sass:math"; .a{ v: math.sqrt(4) / 3 }`)
	if want := ".a {\n  v: 0.6666666667;\n}\n"; css != want {
		t.Errorf("namespaced calc slash = %q, want %q", css, want)
	}
}

// TestColorFilterOverloads covers the plain-CSS filter overloads of the
// grayscale/invert/opacity/saturate/alpha colour functions: a numeric or special
// (var()/unquoted-calc) argument is preserved as a plain CSS function call, and
// alpha() additionally round-trips the Microsoft "x=y" filter syntax.
func TestColorFilterOverloads(t *testing.T) {
	cases := map[string]string{
		// opacity(): number and special passthrough, else the colour's alpha.
		"opacity(10%)":         "opacity(10%)",
		"opacity(var(--c))":    "opacity(var(--c))",
		"opacity(calc(1 + 2))": "opacity(3)",
		"opacity(red)":         "1",
		// grayscale()/invert(): special passthrough.
		"grayscale(var(--c))": "grayscale(var(--c))",
		"invert(var(--c))":    "invert(var(--c))",
		"invert(0.5)":         "invert(0.5)",
		// saturate(): single-argument filter overload, positional and via $amount.
		"saturate(var(--c))": "saturate(var(--c))",
		"saturate(50%)":      "saturate(50%)",
		// alpha(): Microsoft filter round-trip.
		"alpha(c=d)":           "alpha(c=d)",
		"alpha(c=d, e=f, g=h)": "alpha(c=d, e=f, g=h)",
		"alpha(red)":           "1",
		// An unknown function keeps the singleEquals argument verbatim too.
		"unknownfilter(a=b)": "unknownfilter(a=b)",
	}
	for in, want := range cases {
		if got := val(t, in); got != want {
			t.Errorf("val(%q) = %q, want %q", in, got, want)
		}
	}
	// Named single-argument saturate overload.
	if got := val(t, "saturate($amount: 50%)"); got != "saturate(50%)" {
		t.Errorf("saturate($amount) = %q", got)
	}
	// color.opacity/color.alpha module overloads.
	for _, tc := range []struct{ src, want string }{
		{`@use "sass:color"; .a{ v: color.opacity(1) }`, ".a {\n  v: opacity(1);\n}\n"},
		{`@use "sass:color"; .a{ v: color.alpha(c=d) }`, ".a {\n  v: alpha(c=d);\n}\n"},
		{`@use "sass:color"; .a{ v: color.alpha(red) }`, ".a {\n  v: 1;\n}\n"},
	} {
		if got := compile(t, tc.src); got != tc.want {
			t.Errorf("compile(%q) = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestColorNamedArgForms covers named-argument dispatch of rgb()/hsl()/hwb() and
// the two-argument $color/$alpha overloads, including a special-value alpha that
// expands the colour into legacy rgb channels.
func TestColorNamedArgForms(t *testing.T) {
	cases := map[string]string{
		"rgb($channels: 0 255 127)":      "rgb(0, 255, 127)",
		"hsl($channels: 0 100% 50%)":     "hsl(0, 100%, 50%)",
		"rgb($color: #123, $alpha: 0.5)": "rgba(17, 34, 51, 0.5)",
		"rgb(blue, var(--foo))":          "rgb(0, 0, 255, var(--foo))",
		"hsl(blue, var(--foo))":          "hsl(blue, var(--foo))",
	}
	for in, want := range cases {
		if got := val(t, in); got != want {
			t.Errorf("val(%q) = %q, want %q", in, got, want)
		}
	}
	// Named hwb form via the sass:color module.
	src := `@use "sass:color"; .a{ v: color.hwb($hue: 0, $whiteness: 30%, $blackness: 40%) }`
	if want := ".a {\n  v: hsl(0, 33.3333333333%, 45%);\n}\n"; compile(t, src) != want {
		t.Errorf("named hwb = %q, want %q", compile(t, src), want)
	}
}
