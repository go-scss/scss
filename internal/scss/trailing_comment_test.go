// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestTrailingLoudComment covers dart-sass's _isTrailingComment placement for
// the cases go-scss models: a loud comment on the same source line as the
// declaration it follows, or as the block's opening brace when it is the first
// child, is written on that line; otherwise it stays on its own line. Each
// expected output is byte-verified against dart-sass 1.102.
func TestTrailingLoudComment(t *testing.T) {
	cases := []struct{ in, want string }{
		// Trailing after a declaration on the same line.
		{"a {\n  x: y; /* c */\n  z: w;\n}\n",
			"a {\n  x: y; /* c */\n  z: w;\n}\n"},
		// First child on the opening-brace line attaches to the brace.
		{"a { /* c */\n  x: y;\n}\n",
			"a { /* c */\n  x: y;\n}\n"},
		// A comment on the line AFTER the declaration stays on its own line.
		{"a {\n  x: y;\n  /* c */\n  z: w;\n}\n",
			"a {\n  x: y;\n  /* c */\n  z: w;\n}\n"},
		// A first child on the line after the brace stays on its own line.
		{"a {\n  /* c */\n  x: y;\n}\n",
			"a {\n  /* c */\n  x: y;\n}\n"},
		// A comment following a bubbled/nested construct is not attached to a
		// declaration (prev is not a declaration) and stays on its own line.
		{"a {\n  @media screen { b: c }\n  /* c */\n  d: e;\n}\n",
			"@media screen {\n  a {\n    b: c;\n  }\n}\na {\n  /* c */\n  d: e;\n}\n"},
		// A custom-property declaration is also a trailing anchor.
		{"a {\n  --x: y; /* c */\n}\n",
			"a {\n  --x: y; /* c */\n}\n"},
		// A comment whose previous sibling is not a declaration (here a childless
		// at-rule) and sits on a later line stays on its own line.
		{"a {\n  @foo b;\n  /* c */\n  x: y;\n}\n",
			"a {\n  @foo b;\n  /* c */\n  x: y;\n}\n"},
	}
	for _, c := range cases {
		res, err := Render(c.in, false, false, nil)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.in, err)
			continue
		}
		if res.CSS != c.want {
			t.Errorf("trailing comment %q:\n got %q\nwant %q", c.in, res.CSS, c.want)
		}
	}
}

// TestTrailingLoudCommentIndented confirms the trailing-comment model does not
// apply to the indented syntax (positions are pre-converted, so line info is
// absent): the comment keeps its own line, matching dart-sass 1.102.
func TestTrailingLoudCommentIndented(t *testing.T) {
	res, err := Render("a\n  x: y\n  /* c */\n  z: w\n", true, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "a {\n  x: y;\n  /* c */\n  z: w;\n}\n"
	if res.CSS != want {
		t.Errorf("indented trailing comment:\n got %q\nwant %q", res.CSS, want)
	}
}
