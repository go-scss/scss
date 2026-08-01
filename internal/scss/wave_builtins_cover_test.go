// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestWaveMapBuiltins exercises the map.deep-merge/deep-remove and the nested
// key-path forms of map.get/set/merge/has-key/remove.
func TestWaveMapBuiltins(t *testing.T) {
	pre := "@use \"sass:map\"; @use \"sass:meta\"; "
	cases := []struct{ src, want string }{
		// deep-merge: recursive merge, empty-list-as-map, scalar overwrite.
		{"a{b: meta.inspect(map.deep-merge((c: (d: e)), (c: (f: g))))}", "(c: (d: e, f: g))"},
		{"a{b: meta.inspect(map.deep-merge((c: (d: e)), (c: ())))}", "(c: (d: e))"},
		{"a{b: meta.inspect(map.deep-merge((c: e), (c: (f: g))))}", "(c: (f: g))"},
		// deep-remove: top-level, nested, and not-found (unchanged) paths.
		{"a{b: meta.inspect(map.deep-remove((c: d, e: f), c))}", "(e: f)"},
		{"a{b: meta.inspect(map.deep-remove((c: (d: e, f: g)), c, d))}", "(c: (f: g))"},
		{"a{b: meta.inspect(map.deep-remove((c: d), e))}", "(c: d)"},
		{"a{b: meta.inspect(map.deep-remove((c: 1), c, d))}", "(c: 1)"},
		// nested set, including intermediate-not-a-map replacement and named form.
		{"a{b: meta.inspect(map.set((c: (d: e)), c, f, g))}", "(c: (d: e, f: g))"},
		{"a{b: meta.inspect(map.set((c: 1), c, d, f))}", "(c: (d: f))"},
		{"a{b: meta.inspect(map.set($map: (c: d), $key: c, $value: e))}", "(c: e)"},
		// nested merge, simple + named + deep path.
		{"a{b: meta.inspect(map.merge((c: d), (e: f)))}", "(c: d, e: f)"},
		{"a{b: meta.inspect(map.merge($map1: (c: d), $map2: (e: f)))}", "(c: d, e: f)"},
		{"a{b: meta.inspect(map.merge((c: ()), c, (d: e)))}", "(c: (d: e))"},
		{"a{b: meta.inspect(map.merge((c: 1), c, d, (e: f)))}", "(c: (d: (e: f)))"},
		// nested has-key (positional + named + rest keys), true and false.
		{"a{b: map.has-key((c: (d: (e: f))), c, d, e)}", "true"},
		{"a{b: map.has-key((c: (d: (e: f))), c, x)}", "false"},
		{"a{b: map.has-key((c: (d: e)), c, d, x)}", "false"},
		{"a{b: map.has-key($map: (c: d), $key: c)}", "true"},
		// remove named + nested get edge cases.
		{"a{b: meta.inspect(map.remove($map: (c: d), $key: c))}", "()"},
		{"a{b: meta.inspect(map.get((c: (d: e)), c))}", "(d: e)"},
		// missing intermediate key mid-path (has-key !ok branch).
		{"a{b: map.has-key((c: (d: e)), z, d)}", "false"},
	}
	for _, c := range cases {
		got := compile(t, pre+c.src)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q => want substr %q, got %q", c.src, c.want, got)
		}
	}
	// Error branches: missing required arguments and non-map operands.
	for _, src := range []string{
		pre + "a{b: map.has-key((c: d))}",       // missing $key
		pre + "a{b: map.deep-remove((c: d))}",   // missing $key
		pre + "a{b: map.merge((c: d))}",         // missing $map2
		pre + "a{b: map.set((c: d))}",           // missing $value
		pre + "a{b: map.deep-merge((c: d), 1)}", // map2 not a map
		pre + "a{b: map.deep-remove(1, c)}",     // map not a map
	} {
		mustErr(t, src)
	}
}

// TestWaveMathBuiltins exercises math module variables and the clamp/div/log
// branches added in this wave.
func TestWaveMathBuiltins(t *testing.T) {
	m := "@use \"sass:math\"; "
	cases := []struct{ src, want string }{
		{m + "a{b: math.$pi * 1e15}", "3141592653589793"},
		{m + "a{b: math.$e * 1e15}", "2718281828459045"},
		{m + "a{b: math.$max-safe-integer}", "9007199254740991"},
		{m + "a{b: math.$min-safe-integer}", "-9007199254740991"},
		{m + "a{b: math.$epsilon * 1e31}", "2220446049250313"},
		{m + "a{b: math.$max-number}", "17976931348623157"},
		{m + "a{b: math.$min-number * 1e300 * 1e39}", "4940656458412465"},
		// clamp: inverted range, below-min, above-max, in-range, units preserved.
		{m + "a{b: math.clamp(1, 2, 0)}", "1"},
		{m + "a{b: math.clamp(1, 0, 10)}", "1"},
		{m + "a{b: math.clamp(1, 20, 10)}", "10"},
		{m + "a{b: math.clamp(1, 5, 10)}", "5"},
		{m + "a{b: math.clamp(180deg, 0.75turn, 360deg)}", "0.75turn"},
		{m + "a{b: math.clamp(180deg, 1turn, 360deg)}", "360deg"},
		// div fallback to a slash string for non-numeric operands.
		{m + "a{b: math.div(x, 3)}", "x/3"},
		{m + "a{b: math.div(6, y)}", "6/y"},
		{m + "a{b: math.div(6px, 3px)}", "2"},
		// log with an explicit base and a null base (natural log).
		{m + "a{b: math.log(8, 2)}", "3"},
		{m + "a{b: math.log(2, null)}", "0.6931471806"},
	}
	for _, c := range cases {
		got := compile(t, c.src)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q => want substr %q, got %q", c.src, c.want, got)
		}
	}
	// clamp with incompatible units errors (numGE conversion failure).
	mustErr(t, m+"a{b: math.clamp(1px, 5em, 10px)}")
	// A bare built-in module variable via `as *`, plus meta.module-variables.
	if got := compile(t, "@use \"sass:math\" as *; a{b: $e * 0 + 1}"); !strings.Contains(got, "1") {
		t.Errorf("as-* math var: got %q", got)
	}
	if got := compile(t, "@use \"sass:meta\"; @use \"sass:math\"; a{b: meta.inspect(meta.module-variables(\"math\"))}"); !strings.Contains(got, "\"pi\"") {
		t.Errorf("module-variables(math): got %q", got)
	}
	// An unknown built-in module variable is an error.
	mustErr(t, m+"a{b: math.$nonexistent}")
}

// TestWaveStringBuiltins exercises the reworked slice/split, ASCII-only case
// conversion, and quoted-string escape decoding + re-serialization.
func TestWaveStringBuiltins(t *testing.T) {
	s := "@use \"sass:string\"; "
	cases := []struct{ src, want string }{
		// slice: end 0 -> empty, inclusive ranges, negative end, inverted range.
		{s + "a{b: string.slice(\"cde\", 1, 0)}", "b: \"\""},
		{s + "a{b: string.slice(\"cde\", 1, 2)}", "b: \"cd\""},
		{s + "a{b: string.slice(\"cde\", 1, -2)}", "b: \"cd\""},
		{s + "a{b: string.slice(\"cdef\", 3, 2)}", "b: \"\""},
		{s + "a{b: string.slice(\"cdef\", 2, 100)}", "b: \"def\""},
		{s + "a{b: string.slice(\"cdef\", -100)}", "b: \"cdef\""},
		{s + "a{b: string.slice(\"cde\", 0)}", "b: \"cde\""},
		{s + "a{b: string.slice(\"cde\", 100)}", "b: \"\""},
		// split: empty separator, empty string, limit, unquoted, named limit.
		{s + "a{b: string.split(\"abc\", \"\")}", "\"a\", \"b\", \"c\""},
		{s + "a{b: string.split(\"\", \"/\")}", "b: []"},
		{s + "a{b: string.split(\"a, b, c, d\", \", \", 2)}", "\"a\", \"b\", \"c, d\""},
		{s + "a{b: string.split(abc, \"\")}", "a, b, c"},
		{s + "a{b: string.split($string: \"a/b/c\", $separator: \"/\", $limit: 1)}", "\"a\", \"b/c\""},
		// ASCII-only case conversion leaves other bytes untouched.
		{s + "a{b: string.to-upper-case(\"a1z\")}", "A1Z"},
		{s + "a{b: string.to-lower-case(\"A1Z\")}", "a1z"},
		// escape decoding: hex, hex+trailing space, literal, invalid -> U+FFFD.
		{s + "a{b: string.length(\"c\\0308\")}", "b: 2"},
		{s + "a{b: string.length(\"\\E000\")}", "b: 1"},
		{s + "a{b: string.index(\"c\\0308 a\", \"a\")}", "b: 3"},
		{s + "a{b: string.length(\"\\41 bc\")}", "b: 3"},
		{s + "a{b: string.length(\"\\g\")}", "b: 1"},
		{s + "a{b: string.length(\"\\0\")}", "b: 1"},
		{s + "a{b: string.length(\"\\110000\")}", "b: 1"},
	}
	for _, c := range cases {
		got := compile(t, c.src)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q => want substr %q, got %q", c.src, c.want, got)
		}
	}
	// split limit must be >= 1.
	mustErr(t, s+"a{b: string.split(\"a\", \"x\", 0)}")
	// A private-use character re-serialises as a lowercase hex escape.
	if got := compile(t, s+"a{b: string.quote(\"\\E000\")}"); !strings.Contains(got, `\e000`) {
		t.Errorf("private-use escape: got %q", got)
	}
	// A line continuation (backslash-newline) is removed.
	if got := compile(t, s+"a{b: string.length(\"x\\\ny\")}"); !strings.Contains(got, "b: 2") {
		t.Errorf("line continuation: got %q", got)
	}
	// A backslash before EOF inside an unterminated string decodes to U+FFFD.
	if _, err := Render(s+"a{b: \"x\\", false, false, nil); err != nil {
		_ = err // tolerated: exercises the EOF branch of consumeStringEscape
	}
}

// TestWaveMetaBuiltins exercises the argument-list keyword capture, inspect
// fidelity, module-member existence with $module, and nested-property prefixes.
func TestWaveMetaBuiltins(t *testing.T) {
	meta := "@use \"sass:meta\"; "
	cases := []struct{ src, want string }{
		// keywords over a real argument list, and re-spreading it.
		{meta + "@function kw($a...){@return meta.inspect(meta.keywords($a))} a{b: kw($c: d, $e: f)}", "(c: d, e: f)"},
		{meta + "@function inner($x){@return $x} @function outer($a...){@return inner($a...)} a{b: outer($x: 5)}", "b: 5"},
		// type-of an argument list.
		{meta + "@function t($a...){@return meta.type-of($a)} a{b: t(1)}", "arglist"},
		// inspect fidelity: empty bracketed, singleton comma/slash, nested parens.
		{meta + "a{b: meta.inspect([])}", "b: []"},
		{meta + "a{b: meta.inspect((1,))}", "(1,)"},
		{meta + "@use \"sass:list\"; a{b: meta.inspect(list.slash(1, 2, 3))}", "1 / 2 / 3"},
		{meta + "a{b: meta.inspect((1 2, 3 4))}", "1 2, 3 4"},
		{meta + "a{b: meta.inspect(((1, 2) (3, 4)))}", "(1, 2) (3, 4)"},
		{meta + "@use \"sass:list\"; a{b: meta.inspect(list.slash((1, 2), (3, 4)))}", "(1, 2) / (3, 4)"},
		{meta + "@use \"sass:list\"; a{b: meta.inspect(list.slash(1))}", "(1/)"},
		{meta + "a{b: meta.inspect(((1, 2): 3))}", "((1, 2): 3)"},
		{meta + "a{b: meta.inspect((1 2: 3))}", "(1 2: 3)"},
		// content-exists is true for an empty content block.
		{meta + "@mixin m {b {c: meta.content-exists()} @content} @include m {}", "c: true"},
		// feature-exists for a supported feature.
		{meta + "a{b: meta.feature-exists(global-variable-shadowing)}", "b: true"},
	}
	for _, c := range cases {
		got := compile(t, c.src)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q => want substr %q, got %q", c.src, c.want, got)
		}
	}
	// meta.keywords requires an argument list.
	mustErr(t, meta+"a{b: meta.keywords(1)}")

	// $module forms of the *-exists reflection functions, over a user module and
	// a built-in module, resolve module membership.
	files := map[string]string{
		"other": "@mixin mx() {} @function fn() {@return 1} $v: 1;",
	}
	base := "@use \"sass:meta\"; @use \"other\"; @use \"sass:color\"; @use \"sass:math\"; "
	modCases := []struct{ src, want string }{
		{base + "a{b: meta.mixin-exists(\"mx\", \"other\")}", "b: true"},
		{base + "a{b: meta.mixin-exists(\"nope\", \"other\")}", "b: false"},
		{base + "a{b: meta.mixin-exists(\"x\", \"color\")}", "b: false"},
		{base + "a{b: meta.mixin-exists(\"apply\", \"meta\")}", "b: true"},
		{base + "a{b: meta.function-exists(\"fn\", \"other\")}", "b: true"},
		{base + "a{b: meta.function-exists(\"red\", \"color\")}", "b: true"},
		{base + "a{b: meta.function-exists(\"nope\", \"color\")}", "b: false"},
		{base + "a{b: meta.function-exists(\"nope\", \"other\")}", "b: false"},
		{base + "a{b: meta.global-variable-exists(\"v\", \"other\")}", "b: true"},
		{base + "a{b: meta.global-variable-exists(\"nope\", \"other\")}", "b: false"},
		{base + "a{b: meta.global-variable-exists(\"pi\", \"math\")}", "b: true"},
		{base + "a{b: meta.global-variable-exists(\"v\", \"color\")}", "b: false"},
		// An unknown module namespace resolves to "not present" for each kind.
		{base + "a{b: meta.mixin-exists(\"x\", \"bogus\")}", "b: false"},
		{base + "a{b: meta.function-exists(\"x\", \"bogus\")}", "b: false"},
		{base + "a{b: meta.global-variable-exists(\"x\", \"bogus\")}", "b: false"},
	}
	for _, c := range modCases {
		res, err := renderImp(t, c.src, files)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if !strings.Contains(res.CSS, c.want) {
			t.Errorf("%q => want substr %q, got %q", c.src, c.want, res.CSS)
		}
	}

	// A nested property block applies its prefix to declarations produced by an
	// @include inside it.
	np := "@mixin m() { b: value } $n: p; a { #{$n}: {@include m()} }"
	if got := compile(t, meta+np); !strings.Contains(got, "p-b: value") {
		t.Errorf("nested-property @include prefix: got %q", got)
	}
}

// TestWaveSerializerCharset exercises the @charset / BOM prefixing for non-ASCII
// output. A source-level @charset is always dropped (dart consumes it purely for
// encoding detection), so the serializer's @charset is the only one emitted.
func TestWaveSerializerCharset(t *testing.T) {
	// Non-ASCII output gains an @charset rule (expanded).
	if got := compile(t, "a{b: \"föö\"}"); !strings.HasPrefix(got, "@charset \"UTF-8\";\n") {
		t.Errorf("expected @charset prefix, got %q", got)
	}
	// An author-written @charset is dropped, so exactly one @charset remains and
	// its label is the serializer's UTF-8 (never the author's), even for a
	// non-UTF-8 source declaration. Byte-exact against dart-sass 1.102.
	got := compile(t, "@charset \"iso-8859-1\"; a{b: \"föö\"}")
	if strings.Count(got, "@charset") != 1 || !strings.HasPrefix(got, "@charset \"UTF-8\";\n") {
		t.Errorf("expected a single UTF-8 @charset, got %q", got)
	}
	// A source @charset over ASCII output is dropped entirely (both syntaxes),
	// and the name match is case-sensitive so @CHARSET survives as an at-rule.
	if got := compile(t, "@charset \"a\";\nx{y:z}"); got != "x {\n  y: z;\n}\n" {
		t.Errorf("expected dropped @charset, got %q", got)
	}
	if got := compile(t, "@CHARSET \"a\";\nx{y:z}"); got != "@CHARSET \"a\";\nx {\n  y: z;\n}\n" {
		t.Errorf("expected kept @CHARSET, got %q", got)
	}
	// Compressed non-ASCII output is prefixed with a UTF-8 BOM.
	if c := compileC(t, "a{b: \"föö\"}"); !strings.HasPrefix(c, "\uFEFF") {
		t.Errorf("expected BOM prefix in compressed output, got %q", c)
	}
	// Pure-ASCII output has neither prefix.
	if got := compile(t, "a{b: c}"); strings.Contains(got, "@charset") {
		t.Errorf("unexpected @charset for ASCII output: %q", got)
	}
}
