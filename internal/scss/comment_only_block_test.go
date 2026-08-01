// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestCommentOnlyBlock covers dart-sass's one-line rendering of a block whose
// sole visible child is a loud comment on the opening-brace source line:
// `sel { /**/ }` for style rules and `@rule { /**/ }` for at-rules. A block with
// any other child, or a comment on a later line, keeps its `}` on its own line.
// Every expected output is byte-verified against dart-sass 1.102.
func TestCommentOnlyBlock(t *testing.T) {
	cases := []struct{ in, want string }{
		// Style rule: sole brace-line comment collapses to one line.
		{"a {/**/}\n", "a { /**/ }\n"},
		// At-rules preserve the same one-line form.
		{"@font-face {/**/}\n", "@font-face { /**/ }\n"},
		{"@keyframes x {/**/}\n", "@keyframes x { /**/ }\n"},
		{"@foo {/**/}\n", "@foo { /**/ }\n"},
		// An empty nested rule alongside the comment is dropped, leaving the
		// comment the sole visible child (still one line).
		{"a {/**/ b {}}\n", "a { /**/ }\n"},
		// Two visible children (comment + declaration) keep the closing brace on
		// its own line; the first-child comment still trails the opening brace.
		{"a {/**/ b: c}\n", "a { /**/\n  b: c;\n}\n"},
		// A comment on the line after the brace is not a brace-line trailing
		// comment, so the block stays multi-line.
		{"a {\n  /**/\n}\n", "a {\n  /**/\n}\n"},
		// A sole sourceMappingURL comment is dropped, so the block is empty (not a
		// one-liner): the comment-only fast path rejects sourceURL comments.
		{"a {/*# sourceMappingURL=x */}\n", "a {\n}\n"},
	}
	for _, c := range cases {
		res, err := Render(c.in, false, false, nil)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.in, err)
			continue
		}
		if res.CSS != c.want {
			t.Errorf("comment-only block %q:\n got %q\nwant %q", c.in, res.CSS, c.want)
		}
	}
}
