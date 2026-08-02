// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestImportForwardRenest mirrors sass-spec
// directives/import/configuration/nested: a legacy @import nested inside a style
// rule pulls in a file that @forwards another module, and that forwarded
// module's CSS must be re-nested under the enclosing selector rather than lifted
// to the stylesheet root. The importing rule also snapshots its local `$a` as an
// implicit configuration for the forwarded `!default`. Verified against
// dart-sass 1.102: output is `a b { c: configured; }`.
func TestImportForwardRenest(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":           "a {\n  $a: configured;\n  @import \"midstream\";\n}\n",
		"_midstream.scss": "@forward \"upstream\";\n",
		"_upstream.scss":  "$a: original !default;\nb {c: $a}\n",
	})
	want := "a b {\n  c: configured;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestClearGroupEnd covers clearGroupEnd for both node kinds it must handle: a
// style rule and an at-rule (both can carry a top-level group-end flag), plus the
// no-op default for a node that never does (a comment). A module's re-nested CSS
// may lead with either kind, so both branches are exercised.
func TestClearGroupEnd(t *testing.T) {
	sr := &cssStyleRule{isGroupEnd: true}
	ar := &cssAtRule{isGroupEnd: true}
	cm := &cssComment{}
	clearGroupEnd(sr)
	clearGroupEnd(ar)
	clearGroupEnd(cm) // no-op: comments are never group ends
	if sr.isGroupEnd || ar.isGroupEnd {
		t.Fatalf("clearGroupEnd left a flag set: rule=%v atrule=%v", sr.isGroupEnd, ar.isGroupEnd)
	}
}

// TestImportForwardRenestBubbleSplit covers the block-reset half of the re-nested
// module-CSS path: a declaration written after the nested @import must open a
// fresh copy of the enclosing selector positioned after the re-nested content,
// exactly as a literally-nested rule splits its parent. Verified against
// dart-sass 1.102.
func TestImportForwardRenestBubbleSplit(t *testing.T) {
	got := renderRenest(t, map[string]string{
		"input":           "a {\n  @import \"midstream\";\n  x: y;\n}\n",
		"_midstream.scss": "@forward \"upstream\";\n",
		"_upstream.scss":  "b {c: d}\n",
	})
	want := "a b {\n  c: d;\n}\na {\n  x: y;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
