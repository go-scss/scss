// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestExtendSelectorPseudoSelfComposition pins the transitive `@extend`
// fixpoint for mutually self-composing selector pseudos (sass-spec
// libsass-closed-issues/issue_2055). Two rules extend `.thing`:
//
//	:not(.thing[disabled])       { @extend .thing; }
//	:has(:not(.thing[disabled])) { @extend .thing; }
//
// Because each extender itself contains `.thing`, adding the `:has(...)`
// extender must re-extend the extenders registered so far — including the
// `:has(...)` extender that was appended during the very same addExtension
// call. Dart Sass captures `_extensionsByExtender[target]` as a live List and
// mutates it in place, so its `.toList()` snapshot sees that just-added
// extender; the Go port must re-read the map's slice at call time to reproduce
// this, otherwise the pure `:has`-headed self-composition term is never
// generated. All expected values are dart-sass 1.102 output, byte-for-byte.
func TestExtendSelectorPseudoSelfComposition(t *testing.T) {
	src := ":not(.thing) {\n" +
		"    color: red;\n" +
		"}\n" +
		":not(.thing[disabled]) {\n" +
		"    @extend .thing;\n" +
		"    background: blue;\n" +
		"}\n" +
		":has(:not(.thing[disabled])) {\n" +
		"    @extend .thing;\n" +
		"    background: blue;\n" +
		"}\n"

	const want = ":not(.thing):not(:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled]))))) {\n" +
		"  color: red;\n" +
		"}\n" +
		"\n" +
		":not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled]))))):not([disabled]:has(:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled]))))))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled]))))):not([disabled]:has(:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))))))) {\n" +
		"  background: blue;\n" +
		"}\n" +
		"\n" +
		":has(:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled]))))):not([disabled]:has(:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled]))))))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled]))))):not([disabled]:has(:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled]))))))))) {\n" +
		"  background: blue;\n" +
		"}\n"

	if got := compile(t, src); got != want {
		t.Errorf("self-composition @extend mismatch\n got:\n%s\nwant:\n%s", got, want)
	}

	// The pure `:has`-headed self-composition term — the one Dart's live-list
	// aliasing produces and a stale slice-header capture drops — is
	// `:not([disabled]:has(<3-term expansion>))`. Guard it explicitly so a
	// regression in the fixpoint depth is caught even if the golden above is
	// ever regenerated.
	const hasHeadedTerm = ":not([disabled]:has(:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))):not([disabled]:not(.thing[disabled]):not([disabled]:has(:not(.thing[disabled]):not([disabled]:not(.thing[disabled])))))))"
	if !strings.Contains(want, hasHeadedTerm) {
		t.Fatalf("test golden lost the :has-headed self-composition term")
	}
	if got := compile(t, src); !strings.Contains(got, hasHeadedTerm) {
		t.Errorf("output missing the :has-headed self-composition term %q\n got:\n%s", hasHeadedTerm, got)
	}
}
