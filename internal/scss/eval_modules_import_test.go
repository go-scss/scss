// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// mapImporter resolves @use/@forward/@import URLs against an in-memory file map,
// applying the usual partial/extension candidate rules (_name.scss, name.scss).
func mapImporter(files map[string]string) Importer {
	return func(url string) (string, string, bool) {
		if strings.HasPrefix(url, "sass:") {
			return "", "", false
		}
		for _, cand := range []string{
			url, url + ".scss", "_" + url + ".scss",
			url + ".import.scss", "_" + url + ".import.scss",
		} {
			if src, ok := files[cand]; ok {
				return src, cand, true
			}
		}
		return "", "", false
	}
}

func renderWith(t *testing.T, src string, files map[string]string) string {
	t.Helper()
	res, err := Render(src, false, false, mapImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	return res.CSS
}

// TestImportConfigFlow exercises the dart-sass import configuration flow: a
// legacy @import of a file that @forwards a module lets variables set in the
// importing scope configure that module's !default variables.
func TestImportConfigFlow(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		files map[string]string
		want  string
	}{
		{
			name: "same_file_forward",
			src:  "$a: configured;\n@import \"midstream\";\n",
			files: map[string]string{
				"_midstream.scss": "@forward \"upstream\";\n",
				"_upstream.scss":  "$a: original !default;\nb {c: $a}\n",
			},
			want: "b {\n  c: configured;\n}\n",
		},
		{
			name: "unrelated_var_ignored",
			src:  "$a: configured;\n$d: other;\n@import \"midstream\";\n",
			files: map[string]string{
				"_midstream.scss": "@forward \"upstream\";\n",
				"_upstream.scss":  "$a: original !default;\nb {c: $a}\n",
			},
			want: "b {\n  c: configured;\n}\n",
		},
		{
			name: "midstream_local_ignored",
			src:  "$a: configured;\n@import \"midstream\";\n",
			files: map[string]string{
				"_midstream.scss": "$a: midstream;\n@forward \"upstream\";\n",
				"_upstream.scss":  "$a: original !default;\nb {c: $a}\n",
			},
			want: "b {\n  c: configured;\n}\n",
		},
		{
			name: "no_config_keeps_default",
			src:  "@import \"midstream\";\n",
			files: map[string]string{
				"_midstream.scss": "$a: midstream;\n@forward \"upstream\";\n",
				"_upstream.scss":  "$a: original !default;\nb {c: $a}\n",
			},
			want: "b {\n  c: original;\n}\n",
		},
		{
			name: "prefixed_forward_unprefixes_config",
			src:  "$d-a: configured;\n@import \"midstream\";\n",
			files: map[string]string{
				"_midstream.scss": "@forward \"upstream\" as d-*;\n",
				"_upstream.scss":  "$a: original !default;\nb {c: $a}\n",
			},
			want: "b {\n  c: configured;\n}\n",
		},
		{
			name: "import_without_forward_no_config",
			src:  "$a: configured;\n@import \"plainfile\";\n",
			files: map[string]string{
				"_plainfile.scss": "b {c: $a}\n",
			},
			want: "b {\n  c: configured;\n}\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderWith(t, c.src, c.files)
			if got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

// TestThroughForwardConfig drives throughForwardConfig directly to cover the
// prefix / show / hide / empty branches, including a prefix that fails to match.
func TestThroughForwardConfig(t *testing.T) {
	v := newNumber(1)
	base := map[string]Value{"a": v, "b": v, "d-c": v}

	if got := throughForwardConfig(nil, &Forward{}); got != nil {
		t.Errorf("empty base should return nil, got %v", got)
	}

	// Prefix keeps only prefixed names, stripped of the prefix.
	pfx := throughForwardConfig(base, &Forward{Prefix: "d-"})
	if len(pfx) != 1 {
		t.Fatalf("prefix: got %d entries, want 1: %v", len(pfx), pfx)
	}
	if _, ok := pfx["c"]; !ok {
		t.Errorf("prefix: expected unprefixed key c, got %v", pfx)
	}

	// Show limits to listed variables (bare names lacking $ are ignored).
	show := throughForwardConfig(base, &Forward{HasShow: true, Show: []string{"$a", "mixinName"}})
	if len(show) != 1 || show["a"] == nil {
		t.Errorf("show: got %v, want only a", show)
	}

	// Hide drops listed variables.
	hide := throughForwardConfig(base, &Forward{HasHide: true, Hide: []string{"$a"}})
	if hide["a"] != nil || hide["b"] == nil {
		t.Errorf("hide: got %v, want a removed b kept", hide)
	}
}

// TestImplicitConfigSnapshot covers the empty (nil) and multi-scope cases.
func TestImplicitConfigSnapshot(t *testing.T) {
	e := newEvaluator(nil)
	if snap := e.implicitConfigSnapshot(); snap != nil {
		t.Errorf("empty env snapshot should be nil, got %v", snap)
	}
	e.env.scopes[0]["a"] = newNumber(1)
	e.env.pushScope()
	e.env.scopes[1]["a"] = newNumber(2) // inner scope wins
	e.env.scopes[1]["b"] = newNumber(3)
	snap := e.implicitConfigSnapshot()
	if n, ok := snap["a"].(*Number); !ok || n.Val != 2 {
		t.Errorf("inner scope should win for a, got %v", snap["a"])
	}
	if _, ok := snap["b"]; !ok {
		t.Errorf("b missing from snapshot: %v", snap)
	}
}

func TestStmtsHaveForward(t *testing.T) {
	with, err := parseStylesheet("@forward \"x\";\n")
	if err != nil {
		t.Fatal(err)
	}
	if !stmtsHaveForward(with) {
		t.Error("expected @forward to be detected")
	}
	without, err := parseStylesheet("@use \"x\";\na {b: c}\n")
	if err != nil {
		t.Fatal(err)
	}
	if stmtsHaveForward(without) {
		t.Error("did not expect a @forward")
	}
}

// TestImportURLInterpolation covers interpolation inside a plain-CSS `url(...)`
// @import prelude: dart-sass evaluates the interpolation at compile time rather
// than emitting the literal `#{...}`. A url() without interpolation still
// round-trips verbatim. Byte-exact against dart-sass 1.102.
func TestImportURLInterpolation(t *testing.T) {
	cases := []struct{ in, out string }{
		{`@import url("a#{1 + 1}b.css");`, "@import url(\"a2b.css\");\n"},
		{"$p: \"https\";\n@import url(\"#{$p}://ex.com/x\");", "@import url(\"https://ex.com/x\");\n"},
		{"@use \"sass:string\";\n$f: string.unquote(\"Droid+Sans\");\n@import url(\"http://x/css?family=#{$f}\");",
			"@import url(\"http://x/css?family=Droid+Sans\");\n"},
		// No interpolation: verbatim round-trip is preserved.
		{`@import url(foo.css);`, "@import url(foo.css);\n"},
		{`@import url("x.css");`, "@import url(\"x.css\");\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
