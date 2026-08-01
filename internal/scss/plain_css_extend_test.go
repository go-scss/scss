// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// cssMapImporter resolves @use/@import URLs against an in-memory file map,
// trying a plain-CSS candidate as well so a plain `.css` module can be loaded.
func cssMapImporter(files map[string]string) Importer {
	return func(url, _ string, _ bool) (string, string, bool) {
		for _, cand := range []string{
			url, url + ".scss", "_" + url + ".scss",
			url + ".css", "_" + url + ".css",
		} {
			if src, ok := files[cand]; ok {
				return src, cand, true
			}
		}
		return "", "", false
	}
}

// TestPlainCSSExtendScope verifies that a downstream @extend reaches the rules
// of a @used plain-CSS module (dart treats plain CSS as a real module with its
// own extension store), while every untargeted plain-CSS rule stays verbatim.
// Oracle values are dart-sass 1.102.
func TestPlainCSSExtendScope(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		files map[string]string
		want  string
	}{
		{
			// css/plain/extend: the used plain rule `b` is extended to `b, a`.
			name: "extend into used plain css",
			src:  "@use \"plain\";\na {@extend b}\n",
			files: map[string]string{
				"_plain.css": "b {c: d}\n",
			},
			want: "b, a {\n  c: d;\n}\n",
		},
		{
			// The extend targets only p1's rule; a different plain-CSS module p2
			// stays verbatim (per-module extend scopes never leak into siblings).
			name: "other plain module stays verbatim",
			src:  "@use \"p1\";\n@use \"p2\";\na {@extend b}\n",
			files: map[string]string{
				"_p1.css": "b {c: d}\n",
				"_p2.css": ".keep {x: y}\n",
			},
			want: "b, a {\n  c: d;\n}\n\n.keep {\n  x: y;\n}\n",
		},
		{
			// No downstream @extend targets the module: output is unchanged.
			name: "no extend leaves plain css untouched",
			src:  "@use \"plain\";\n",
			files: map[string]string{
				"_plain.css": "b {c: d}\n",
			},
			want: "b {\n  c: d;\n}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Render(tc.src, false, false, cssMapImporter(tc.files))
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			if res.CSS != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", res.CSS, tc.want)
			}
		})
	}
}

// TestRegisterPlainRulesForExtend white-boxes the round-trip guard: a rule whose
// verbatim selector fails to parse, or does not round-trip through the Sass
// selector serialiser, is left as pure raw (never registered), so exotic
// plain-CSS output can never be altered by a downstream @extend.
func TestRegisterPlainRulesForExtend(t *testing.T) {
	nodes := []cssNode{
		&cssStyleRule{raw: true, rawSel: "b"},                     // parses + round-trips -> registered
		&cssStyleRule{raw: true, rawSel: "a,b"},                   // parses but "a, b" != "a,b" -> skipped
		&cssStyleRule{raw: true, rawSel: "@@@ bad"},               // parse error -> skipped
		&cssDeclaration{name: "x", value: &SassString{Text: "y"}}, // not a rule -> skipped
	}
	sub := newEvaluator(nil)
	registerPlainRulesForExtend(sub, nodes)
	if len(sub.extendEvents) != 1 {
		t.Fatalf("registered %d rules, want 1", len(sub.extendEvents))
	}
	if got := sub.extendEvents[0].rule.rawSel; got != "b" {
		t.Fatalf("registered rule %q, want %q", got, "b")
	}
}

// TestRegisterPlainCSSExtendScopeNoRules verifies that a plain-CSS module with
// no extendable rules adopts no scope (nothing to extend, so no scope overhead).
func TestRegisterPlainCSSExtendScopeNoRules(t *testing.T) {
	e := newEvaluator(nil)
	before := len(*e.allScopes)
	e.registerPlainCSSExtendScope([]cssNode{
		&cssStyleRule{raw: true, rawSel: "a,b"}, // does not round-trip -> not registered
	})
	if len(*e.allScopes) != before {
		t.Fatalf("adopted a scope for a module with no extendable rules")
	}
}
