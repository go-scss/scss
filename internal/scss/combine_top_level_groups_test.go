// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestCombineTopLevelGroupsSingleFile locks down dart-sass's deferred blank-line
// policy over a single file's combined top-level node list: a blank line precedes
// a top-level node iff the preceding visible node is a group end — the last node
// emitted for a top-level style-rule statement (a style rule, or the at-rule a
// nested rule bubbled into), and only that. Comments and declarations are never
// group ends. All expectations are byte-exact against dart-sass 1.102.
func TestCombineTopLevelGroupsSingleFile(t *testing.T) {
	cases := []struct{ in, out string }{
		// A nested rule bubbles a @media to the top level as the last node of its
		// origin style-rule statement: that @media becomes the group end, so the
		// following rule is blank-separated from it just as from a bare rule.
		{
			"x { a: 1 }\ny { @media m { z { b: 2 } } }\nw { c: 3 }\n",
			"x {\n  a: 1;\n}\n\n@media m {\n  y z {\n    b: 2;\n  }\n}\n\nw {\n  c: 3;\n}\n",
		},
		// Top-level loud comments are never group ends: no blank follows a comment,
		// but a style rule before a comment is a group end, so the comment is blank-
		// separated from the preceding rule.
		{
			"/* lead */\np { a: 1 }\n/* mid */\nq { b: 2 }\n",
			"/* lead */\np {\n  a: 1;\n}\n\n/* mid */\nq {\n  b: 2;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestCombineTopLevelGroupsModuleBoundary locks down the separation dart derives
// from its combined module tree — the boundary the eager per-load-site grouping
// could not see across. A rule ending one module (or an @import inlined into it)
// and a rule opening the next must be blank-separated exactly as sibling rules in
// one file, because every top-level style rule persists its group-end flag into
// the deferred combine. Byte-exact against dart-sass 1.102.
func TestCombineTopLevelGroupsModuleBoundary(t *testing.T) {
	files := map[string]string{
		"_imported.scss": "a { file: imported }\n",
		"_used.scss":     "@import \"imported\";\na { file: used }\n",
	}
	res, err := Render("@use \"used\";\nb { top: level }\n", false, false, sassAwareImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  file: imported;\n}\n\n" +
		"a {\n  file: used;\n}\n\n" +
		"b {\n  top: level;\n}\n"
	if res.CSS != want {
		t.Fatalf("module boundary blanks:\n want %q\n  got %q", want, res.CSS)
	}
}
