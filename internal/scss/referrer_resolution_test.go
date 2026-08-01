// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"path"
	"strings"
	"testing"
)

// referrerImporter is a referrer-aware virtual-filesystem importer: it resolves a
// bare url relative to the DIRECTORY of the referrer (the canonical key of the
// file issuing the load) first, then relative to the root, trying the ordinary
// partial/extension candidates. It mirrors how Dart Sass's filesystem importer
// resolves url against baseUrl before the load path, which is what lets a load
// issued from a mixin/@content/@import body resolve relative to that body's own
// file. It returns the resolved key so the loader threads it onward as the next
// referrer.
func referrerImporter(files map[string]string) Importer {
	try := func(base, url string) (string, string, bool) {
		joined := path.Clean(path.Join(base, url))
		name := path.Base(joined)
		parent := path.Dir(joined)
		cands := []string{joined + ".scss", path.Join(parent, "_"+name+".scss")}
		if strings.HasSuffix(url, ".scss") {
			cands = []string{joined, path.Join(parent, "_"+name)}
		}
		for _, c := range cands {
			c = strings.TrimPrefix(c, "./")
			if src, ok := files[c]; ok {
				return src, c, true
			}
		}
		return "", "", false
	}
	return func(url, referrer string) (string, string, bool) {
		if strings.HasPrefix(url, "sass:") {
			return "", "", false
		}
		if referrer != "" {
			if src, key, ok := try(path.Dir(referrer), url); ok {
				return src, key, true
			}
		}
		return try(".", url)
	}
}

// TestReferrerThroughOtherMixin reproduces the sass-spec
// core_functions/meta/load_css/plain_css::through_other_mixin case at the engine
// level: a meta.load-css issued inside a mixin defined in subdir/_midstream.scss
// must resolve "upstream" relative to subdir/, not the entry file, so it picks
// subdir/_upstream.scss. Dart Sass 1.102 emits `a { b: in subdir; }`.
func TestReferrerThroughOtherMixin(t *testing.T) {
	files := map[string]string{
		"_upstream.scss":         "a {b: in main dir}\n",
		"subdir/_midstream.scss": "@use \"sass:meta\";\n@mixin load-css($module) {\n  @include meta.load-css($module);\n}\n",
		"subdir/_upstream.scss":  "a {b: in subdir}\n",
	}
	src := "@use \"subdir/midstream\";\n@include midstream.load-css(\"upstream\");\n"
	res, err := Render(src, false, false, referrerImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  b: in subdir;\n}\n"
	if res.CSS != want {
		t.Fatalf("referrer-relative load-css:\n got %q\nwant %q", res.CSS, want)
	}
}

// TestReferrerContentBlockResolvesToCaller guards the @content referrer edge: a
// meta.load-css written inside a @content block belongs to the include site's
// file, so it must resolve relative to the CALLER (the entry file here), not the
// mixin's defining file — even though the mixin lives in a subdir. The entry-dir
// _target.scss must win over subdir/_target.scss.
func TestReferrerContentBlockResolvesToCaller(t *testing.T) {
	files := map[string]string{
		"_target.scss":        "a {from: main}\n",
		"subdir/_wrap.scss":   "@mixin wrap {\n  @content;\n}\n",
		"subdir/_target.scss": "a {from: subdir}\n",
	}
	src := "@use \"sass:meta\";\n@use \"subdir/wrap\";\n@include wrap.wrap {\n  @include meta.load-css(\"target\");\n}\n"
	res, err := Render(src, false, false, referrerImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  from: main;\n}\n"
	if res.CSS != want {
		t.Fatalf("@content load-css should resolve to caller:\n got %q\nwant %q", res.CSS, want)
	}
}

// TestReferrerNestedImportResolvesRelative guards the legacy-@import referrer
// edge: a file inlined by @import resolves a further relative @import against its
// OWN directory. subdir/_a.scss imports "b", which must pick subdir/_b.scss, not
// an entry-dir _b.scss.
func TestReferrerNestedImportResolvesRelative(t *testing.T) {
	files := map[string]string{
		"_b.scss":        "a {from: main}\n",
		"subdir/_a.scss": "@import \"b\";\n",
		"subdir/_b.scss": "a {from: subdir}\n",
	}
	res, err := Render("@import \"subdir/a\";\n", false, false, referrerImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  from: subdir;\n}\n"
	if res.CSS != want {
		t.Fatalf("nested @import should resolve relative to its own file:\n got %q\nwant %q", res.CSS, want)
	}
}

// TestReferrerTopLevelUseResolvesRelative guards the @use referrer edge inside a
// module: subdir/_a.scss @uses "b", which must resolve to subdir/_b.scss.
func TestReferrerTopLevelUseResolvesRelative(t *testing.T) {
	files := map[string]string{
		"_b.scss":        "$x: main;\n",
		"subdir/_a.scss": "@use \"b\";\na {x: b.$x}\n",
		"subdir/_b.scss": "$x: subdir;\n",
	}
	res, err := Render("@use \"subdir/a\";\n", false, false, referrerImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  x: subdir;\n}\n"
	if res.CSS != want {
		t.Fatalf("module @use should resolve relative to its own file:\n got %q\nwant %q", res.CSS, want)
	}
}
