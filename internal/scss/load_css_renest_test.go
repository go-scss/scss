// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// renestImporter resolves a bare URL to a `.css` plain-CSS file when one is
// present in the file map, else to a `.scss` partial, returning the resolved key
// so the loader picks the right grammar (and the plain-CSS branch) by extension.
func renestImporter(files map[string]string) Importer {
	return func(url, _ string) (string, string, bool) {
		for _, cand := range []string{url, url + ".css", url + ".scss", "_" + url + ".scss"} {
			if src, ok := files[cand]; ok {
				return src, cand, true
			}
		}
		return "", "", false
	}
}

func renderRenest(t *testing.T, files map[string]string) string {
	t.Helper()
	res, err := Render("@use \"sass:meta\";\n"+files["input"], false, false, renestImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	return res.CSS
}

// TestLoadCSSNestedPlainOneLevel mirrors sass-spec
// css/plain/style_rule/nesting/through_load_css::one_level: a plain-CSS rule with
// no parent reference gains the enclosing selector as a descendant.
func TestLoadCSSNestedPlainOneLevel(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":     "a {@include meta.load-css(\"plain\")}\n",
		"plain.css": "b {c: d}\n",
	})
	want := "a b {\n  c: d;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestLoadCSSNestedPlainTwoLevels mirrors through_load_css::two_levels: the outer
// plain-CSS rule flattens under the enclosing selector while its native-CSS
// nested child is preserved verbatim.
func TestLoadCSSNestedPlainTwoLevels(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":     "a {@include meta.load-css(\"plain\")}\n",
		"plain.css": "b {c {d: e}}\n",
	})
	want := "a b {\n  c {\n    d: e;\n  }\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestLoadCSSNestedPlainParentRef mirrors through_load_css::top_level_parent: a
// plain-CSS rule that references the parent (`&`, native CSS nesting) is kept
// verbatim and wrapped in a copy of the enclosing selector.
func TestLoadCSSNestedPlainParentRef(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":     "a {@include meta.load-css(\"plain\")}\n",
		"plain.css": "& {b {c: d}}\n",
	})
	want := "a {\n  & {\n    b {\n      c: d;\n    }\n  }\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestLoadCSSNestedScssModule mirrors meta/load_css/plain_css::nested/parent_selector:
// an SCSS module's CSS is resolved in a clean context (its own `&` refers to its
// own parents) and then the enclosing selector is prepended as a descendant.
func TestLoadCSSNestedScssModule(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":       "a {@include meta.load-css(\"other\")}\n",
		"_other.scss": "b {\n  c & {x: y}\n}\n",
	})
	want := "a c b {\n  x: y;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestLoadCSSNestedComment covers the loud-comment branch of renestLoadedNode: a
// comment in loaded CSS is wrapped in a copy of the enclosing selector, matching
// dart-sass (`a { /* … */ }`).
func TestLoadCSSNestedComment(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":       "a {@include meta.load-css(\"other\")}\n",
		"_other.scss": "/* hi */\nb {e: f}\n",
	})
	want := "a {\n  /* hi */\n}\na b {\n  e: f;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestLoadCSSNestedAtRule covers the pass-through branch of renestLoadedNode: a
// declaration-only at-rule (e.g. @font-face) in loaded CSS hoists to the top
// level rather than being prefixed, matching dart-sass.
func TestLoadCSSNestedAtRule(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":       "a {@include meta.load-css(\"other\")}\n",
		"_other.scss": "@font-face {a: b}\n",
	})
	want := "@font-face {\n  a: b;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestLoadCSSPlainParseError covers the error branch of loadCSSInto's plain-CSS
// path: a malformed .css file loaded through meta.load-css surfaces a Sass error.
func TestLoadCSSPlainParseError(t *testing.T) {
	_, err := Render("@use \"sass:meta\";\n@include meta.load-css(\"bad\");\n", false, false,
		renestImporter(map[string]string{"bad.css": "}\n"}))
	if err == nil {
		t.Fatalf("expected a parse error for malformed plain CSS, got nil")
	}
}

// TestLoadCSSTopLevelPlain covers the top-level plain-CSS branch: a .css file
// loaded outside any rule is emitted verbatim (native nesting preserved).
func TestLoadCSSTopLevelPlain(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":     "@include meta.load-css(\"plain\");\n",
		"plain.css": "b {c {d: e}}\n",
	})
	want := "b {\n  c {\n    d: e;\n  }\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
