// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestMediaScriptQueries exercises the parse-into-interpolation media-query
// model: SassScript in feature values and range bounds is evaluated at compile
// time, range syntax and boolean logic are supported, and interpolated query
// text is treated as opaque (its keyword casing preserved) while raw keywords
// are normalized. Every expected output is byte-exact against dart-sass 1.102.
func TestMediaScriptQueries(t *testing.T) {
	cases := []struct{ in, out string }{
		{"$w: 300px;\n@media (min-width: $w) { a { b: c; } }", "@media (min-width: 300px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (width: 500px + 100px) { a { b: c; } }", "@media (width: 600px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (max-width: min(3px, 4px)) { a { b: c; } }", "@media (max-width: 3px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (height < 600px) { a { b: c; } }", "@media (height < 600px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (height <= 600px) { a { b: c; } }", "@media (height <= 600px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (height = 600px) { a { b: c; } }", "@media (height = 600px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (height >= 600px) { a { b: c; } }", "@media (height >= 600px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (height > 600px) { a { b: c; } }", "@media (height > 600px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"$m: 600px;\n@media (50px + 50px < width < $m) { a { b: c; } }", "@media (100px < width < 600px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (10px >= width > 15px) { a { b: c; } }", "@media (10px >= width > 15px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (10px > width >= 15px) { a { b: c; } }", "@media (10px > width >= 15px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (width < (1 < 2)) { a { b: c; } }", "@media (width < true) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (width < [1 < 2]) { a { b: c; } }", "@media (width < [true]) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (width < if(1 < 2, 5px, 6px)) { a { b: c; } }", "@media (width < 5px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (color) { a { b: c; } }", "@media (color) {\n  a {\n    b: c;\n  }\n}\n"},
		{"$t: screen;\n@media #{$t} and (min-width: 5px + 5px) { a { b: c; } }", "@media screen and (min-width: 10px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media ((a) and (b)) { a { b: c; } }", "@media ((a) and (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media ((a) or (b)) { a { b: c; } }", "@media ((a) or (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (not (a)) { a { b: c; } }", "@media not (a) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media ((a) and (b) and (c)) { a { b: c; } }", "@media ((a) and (b) and (c)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (a) and ((b) or (c)) { a { b: c; } }", "@media (a) and ((b) or (c)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (NoT (a)) { a { b: c; } }", "@media not (a) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media ((a) AnD (b)) { a { b: c; } }", "@media ((a) and (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media ((a) Or (b)) { a { b: c; } }", "@media ((a) or (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (#{\"NoT (a)\"}) { a { b: c; } }", "@media (NoT (a)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (#{\"(a) AnD (b)\"}) { a { b: c; } }", "@media ((a) AnD (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (#{\"(a) oR (b)\"}) { a { b: c; } }", "@media ((a) oR (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (#{\"not (a)\"}) { a { b: c; } }", "@media not (a) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (#{\"(a) and (b)\"}) { a { b: c; } }", "@media ((a) and (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (#{\"(a) foo (b)\"}) { a { b: c; } }", "@media ((a) foo (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (#{\"(a) or (b) and (c)\"}) { a { b: c; } }", "@media ((a) or (b) and (c)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media ((a) and #{\"(b)\"}) { a { b: c; } }", "@media ((a) and (b)) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media screen /* loud */ and (color) { a { b: c; } }", "@media screen and (color) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media screen // silent\n   and (color) { a { b: c; } }", "@media screen and (color) {\n  a {\n    b: c;\n  }\n}\n"},
		{"$x: 5px;\n@media (width < #{$x}) { a { b: c; } }", "@media (width < 5px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (width < #{\"3px\"}) { a { b: c; } }", "@media (width < 3px) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@media (max-width: #{1 + 2}px) { a { b: c; } }", "@media (max-width: 3px) {\n  a {\n    b: c;\n  }\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestMediaScriptErrors covers the media-query parser's error branches: a
// malformed interpolation and an empty prelude both surface a compile error.
func TestMediaScriptErrors(t *testing.T) {
	for _, src := range []string{
		"@media (#{1) { a { b: c; } }",           // feature interpolation missing "}"
		"@media #{1 { a { b: c; } }",             // prelude interpolation missing "}"
		"@media (min-width: 5px { a { b: c; } }", // feature missing ")"
		"@media{ a { b: c; } }",                  // empty media prelude
	} {
		if _, err := Render(src, false, false, nil); err == nil {
			t.Errorf("expected error compiling %q, got none", src)
		}
	}
}
