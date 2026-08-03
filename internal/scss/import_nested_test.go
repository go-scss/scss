// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// A plain-CSS @import behaves exactly like a childless at-rule (`@b c;`):
// dart-sass's _visitStaticImport routes an import to the document's import
// region only when its parent is the root, and otherwise adds it as an ordinary
// child of the current parent. These cases pin that placement across the
// style-rule / @media / @at-root boundaries. Every `want` below is byte-exact
// dart-sass 1.102 output.
func TestPlainCSSImportPlacement(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			// Root-level: hoisted to the import region (dart _endOfImports).
			name: "top level",
			src:  `@import url("http://x"); foo { color: red; }`,
			want: "@import url(\"http://x\");\nfoo {\n  color: red;\n}\n",
		},
		{
			// Inside a style rule: stays nested in the enclosing selector.
			name: "inside rule",
			src:  `foo { @import url("http://x"); }`,
			want: "foo {\n  @import url(\"http://x\");\n}\n",
		},
		{
			// Reached through a mixin's @if branch inside a style rule — the two
			// libsass-todo-tests target cases (control-if / control-else inside).
			name: "mixin if inside rule",
			src:  "@mixin m() { @if (true) { @import url(\"http://x\"); } }\nfoo { @include m(); }",
			want: "foo {\n  @import url(\"http://x\");\n}\n",
		},
		{
			name: "mixin else inside rule",
			src:  "@mixin m() { @if (false) {} @else { @import url(\"http://x\"); } }\nfoo { @include m(); }",
			want: "foo {\n  @import url(\"http://x\");\n}\n",
		},
		{
			// Interleaves with declarations in source order within the block.
			name: "interleaved with decls",
			src:  `foo { color: blue; @import url("http://a"); width: 3px; }`,
			want: "foo {\n  color: blue;\n  @import url(\"http://a\");\n  width: 3px;\n}\n",
		},
		{
			// Nested style rule carries the resolved compound selector.
			name: "nested rule",
			src:  `foo { bar { @import url("http://x"); } }`,
			want: "foo bar {\n  @import url(\"http://x\");\n}\n",
		},
		{
			// Inside @media at the top level: nested in the media node (parent is
			// the media, not the root).
			name: "top-level media",
			src:  `@media screen { @import url("http://x"); }`,
			want: "@media screen {\n  @import url(\"http://x\");\n}\n",
		},
		{
			// Inside a rule inside @media: nested in the rule inside the media.
			name: "media then rule",
			src:  `foo { @media screen { @import url("http://x"); } }`,
			want: "@media screen {\n  foo {\n    @import url(\"http://x\");\n  }\n}\n",
		},
		{
			// Default @at-root escapes the style rule: the import returns to the
			// root import region, bare (no enclosing selector).
			name: "at-root escapes rule",
			src:  `foo { @at-root { @import url("http://x"); } }`,
			want: "@import url(\"http://x\");\n",
		},
		{
			// @at-root (with: rule) keeps the rule but escapes an enclosing media,
			// so the import stays nested in the rule at the root.
			name: "at-root with rule keeps selector",
			src:  `@media screen { foo { @at-root (with: rule) { @import url("http://x"); } } }`,
			want: "foo {\n  @import url(\"http://x\");\n}\n",
		},
		{
			// @at-root default inside a rule inside @media keeps the media frame but
			// escapes the rule: the import nests bare in the media node.
			name: "at-root default keeps media escapes rule",
			src:  `@media screen { foo { @at-root { @import url("http://x"); } } }`,
			want: "@media screen {\n  @import url(\"http://x\");\n}\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := Render(c.src, false, false, nil)
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			if res.CSS != c.want {
				t.Fatalf("got:\n%q\nwant:\n%q", res.CSS, c.want)
			}
		})
	}
}
