// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss_test

import (
	"strings"
	"testing"

	scss "github.com/go-scss/scss"
)

// mapImporter builds an Importer backed by an in-memory file map, for exercising
// the module resolution/export separation (@use / @forward / @import).
func mapImporter(files map[string]string) scss.Importer {
	return func(url string) (string, string, bool) {
		if s, ok := files[url]; ok {
			return s, url, true
		}
		return "", "", false
	}
}

// compileModEnv compiles src against an in-memory file set and returns the CSS,
// failing the test on a compile error.
func compileModEnv(t *testing.T, files map[string]string, src string) string {
	t.Helper()
	res, err := scss.CompileString(src, &scss.Options{Importer: mapImporter(files)})
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	return res.CSS
}

// assertContains fails unless every want substring is present in got.
func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("output missing %q\n--- got ---\n%s", w, got)
		}
	}
}

// TestUseAsStarResolution covers `@use as *` bare-name resolution of variables,
// functions and mixins through the environment's global-module list (they are
// resolvable locally but not re-exported).
func TestUseAsStarResolution(t *testing.T) {
	files := map[string]string{
		"other": "$member: value;\n@function member() {@return value}\n@mixin mx() {a {b: c}}",
	}
	assertContains(t, compileModEnv(t, files, `@use "other" as *; a {v: $member}`), "v: value")
	assertContains(t, compileModEnv(t, files, `@use "other" as *; a {v: member()}`), "v: value")
	assertContains(t, compileModEnv(t, files, `@use "other" as *; @include mx`), "b: c")
}

// TestUseAsStarGlobalWriteThrough covers a global (top-level and !global)
// assignment writing through to the source module's slot so the module's own
// function observes the new value (dart use/member/global).
func TestUseAsStarGlobalWriteThrough(t *testing.T) {
	files := map[string]string{
		"other": "$member: value;\n@function get-member() {@return $member}",
	}
	assertContains(t, compileModEnv(t, files,
		`@use "other" as *; $member: new value; a {b: get-member()}`), "b: new value")
	assertContains(t, compileModEnv(t, files,
		`@use "other" as *; a { $member: new value !global; b: get-member() }`), "b: new value")
	// A nested, non-!global assignment creates a local and does NOT write through.
	assertContains(t, compileModEnv(t, files,
		`@use "other" as *; a { $member: new value; b: get-member() }`), "b: value")
}

// TestUseAsStarNotReExported covers that a `@use as *` member is NOT part of the
// importing module's exported API, so it is invisible to a downstream module that
// @uses it (dart use/error/member transitive: the call falls back to plain CSS).
func TestUseAsStarNotReExported(t *testing.T) {
	files := map[string]string{
		"midstream": `@use "upstream" as *;`,
		"upstream":  "@function upstream() {@return value}",
	}
	assertContains(t, compileModEnv(t, files,
		`@use "midstream" as *; a {b: upstream()}`), "b: upstream()")
}

// TestForwardNotLocallyCallable covers that a @forward re-exports members without
// making them callable in the forwarding stylesheet itself.
func TestForwardNotLocallyCallable(t *testing.T) {
	files := map[string]string{"other": "@function c() {@return d}"}
	// Locally, c() is not defined and falls back to plain CSS.
	assertContains(t, compileModEnv(t, files, `@forward "other"; a {b: c()}`), "b: c()")
	// But a module that @uses the forwarder sees c() as a re-exported member.
	files["fwd"] = `@forward "other";`
	assertContains(t, compileModEnv(t, files, `@use "fwd"; a {b: fwd.c()}`), "b: d")
}

// TestForwardShadowedWritePrecedence covers that a namespaced assignment to a
// module that both defines and @forwards a variable of the same name writes the
// FORWARDED slot, leaving the module's own definition untouched.
func TestForwardShadowedWritePrecedence(t *testing.T) {
	files := map[string]string{
		"midstream": "@forward \"upstream\";\n$a: midstream value;\n@function get-midstream-a() {@return $a}",
		"upstream":  "$a: upstream value;\n@function get-upstream-a() {@return $a}",
	}
	got := compileModEnv(t, files,
		`@use "midstream"; midstream.$a: new value; b { m: midstream.get-midstream-a(); u: midstream.get-upstream-a() }`)
	assertContains(t, got, "m: midstream value", "u: new value")
}

// TestForwardPrefixAndShow covers `@forward as p-*` prefixing and show/hide
// filtering composed through the module accessors (allVars/allFuncs/allMixins
// and filterForwarded).
func TestForwardPrefixAndShow(t *testing.T) {
	files := map[string]string{
		"upstream": "$c: e;\n@function c() {@return cval}\n@mixin mx() {a {p: q}}",
		"pre":      `@forward "upstream" as d_*;`,
		"shown":    `@forward "upstream" show $c, c;`,
		"hidden":   `@forward "upstream" hide mx;`,
	}
	assertContains(t, compileModEnv(t, files, `@use "pre"; a {b: pre.$d-c}`), "b: e")
	assertContains(t, compileModEnv(t, files, `@use "pre"; a {b: pre.d-c()}`), "b: cval")
	assertContains(t, compileModEnv(t, files, `@use "shown"; a {b: shown.$c; c: shown.c()}`), "b: e", "c: cval")
	assertContains(t, compileModEnv(t, files, `@use "hidden"; a {b: hidden.$c}`), "b: e")
}

// TestModuleMetaEnumeration covers meta.module-variables/-functions/-mixins and
// the exists/get reflection over a namespaced module including forwarded members.
func TestModuleMetaEnumeration(t *testing.T) {
	files := map[string]string{
		"upstream": "$uv: 1;\n@function uf() {@return 2}\n@mixin um() {a {x: y}}",
		"mid":      "@forward \"upstream\";\n$mv: 3;",
	}
	got := compileModEnv(t, files, `@use "sass:meta";
@use "mid";
a {
  vars: meta.inspect(map-keys(meta.module-variables("mid")));
  funcs: meta.inspect(map-keys(meta.module-functions("mid")));
  mixins: meta.inspect(map-keys(meta.module-mixins("mid")));
}`)
	assertContains(t, got, "uv", "mv", "uf", "um")
}

// TestUseAsStarMetaExists covers meta.variable-exists / function-exists /
// mixin-exists / global-variable-exists resolving `@use as *` members, plus
// get-function / get-mixin over a global module.
func TestUseAsStarMetaExists(t *testing.T) {
	files := map[string]string{
		"other": "$member: value;\n@function member() {@return value}\n@mixin mx() {a {b: c}}",
	}
	got := compileModEnv(t, files, `@use "sass:meta";
@use "other" as *;
a {
  ve: variable-exists(member);
  gve: global-variable-exists(member);
  fe: function-exists(member);
  me: mixin-exists(mx);
  gf: meta.inspect(meta.get-function("member"));
}
@include meta.apply(meta.get-mixin("mx"));`)
	assertContains(t, got, "ve: true", "gve: true", "fe: true", "me: true", "b: c")
}

// TestNestedGlobalVarHoist covers that a `!global` assignment in a never-executed
// branch still creates a null slot in the module (dart nested_global_variable),
// walking into the various nested statement bodies.
func TestNestedGlobalVarHoist(t *testing.T) {
	files := map[string]string{
		"direct":   "x {\n  @if false { $member: value !global; }\n}",
		"loop":     "@each $i in 1 2 { @if false { $lv: v !global; } }\n@for $j from 1 through 1 { }\n@while false { $wv: v !global; }",
		"rule":     "a { @if false { $rv: v !global; } }",
		"imported": "x {\n  @if false { $tm: value !global; }\n}",
		"used":     `@import "imported";`,
	}
	assertContains(t, compileModEnv(t, files,
		`@use "sass:meta"; @use "direct"; a {b: meta.inspect(direct.$member)}`), "b: null")
	assertContains(t, compileModEnv(t, files,
		`@use "sass:meta"; @use "loop"; a {b: meta.inspect(loop.$wv)}`), "b: null")
	assertContains(t, compileModEnv(t, files,
		`@use "sass:meta"; @use "rule"; a {b: meta.inspect(rule.$rv)}`), "b: null")
	// Through a legacy @import, the imported source's slot belongs to the module.
	assertContains(t, compileModEnv(t, files,
		`@use "sass:meta"; @use "used"; a {b: meta.inspect(used.$tm)}`), "b: null")
}

// TestImportForwardWriteThrough covers a legacy @import of a @forwarding file:
// re-exported members are locally usable, a later assignment writes through to
// the source module, and a later import shadows an earlier one.
func TestImportForwardWriteThrough(t *testing.T) {
	files := map[string]string{
		"midstream":  `@forward "upstream";`,
		"upstream":   "$a: old value;\n@function get-a() {@return $a}",
		"midstream1": `@forward "upstream1";`,
		"upstream1":  "$b: 1;",
		"midstream2": `@forward "upstream2";`,
		"upstream2":  "$b: 2;",
	}
	// variable_use + write-through
	assertContains(t, compileModEnv(t, files,
		`@import "midstream"; $a: new value; b {c: get-a()}`), "c: new value")
	// nested write-through
	assertContains(t, compileModEnv(t, files,
		`b { @import "midstream"; $a: new value; c: get-a() }`), "c: new value")
	// override: later import wins
	assertContains(t, compileModEnv(t, files,
		`@import "midstream1"; f {a: $b} @import "midstream2"; s {a: $b}`), "a: 1", "a: 2")
}

// TestImportForwardConfigThroughForward covers the implicit-configuration
// snapshot picking up an @import-re-exported variable so it flows into a later
// import's @forward !default (dart-sass#2641 through_forward).
func TestImportForwardConfigThroughForward(t *testing.T) {
	files := map[string]string{
		"config_wrapper": `@forward "config";`,
		"config":         "$a: configured;",
		"midstream":      `@forward "upstream";`,
		"upstream":       "$a: original !default;\nb {c: $a}",
	}
	assertContains(t, compileModEnv(t, files,
		`@import "config_wrapper"; @import "midstream";`), "c: configured")
}

// TestNamespacedVarWrites covers assignNamespacedVar reaching a module's own
// variable slot and writing through a filtered @forward view to the underlying
// module (module.setVar own-write and writeThrough branches).
func TestNamespacedVarWrites(t *testing.T) {
	files := map[string]string{
		"own":   "$v: 1;\n@function get() {@return $v}",
		"shown": `@forward "up" show $c, get;`,
		"up":    "$c: e;\n@function get() {@return $c}",
	}
	// Write to a module's own variable.
	assertContains(t, compileModEnv(t, files,
		`@use "own"; own.$v: 9; a {b: own.get()}`), "b: 9")
	// Write through a filtered (show) forward view to the underlying module.
	assertContains(t, compileModEnv(t, files,
		`@use "shown"; shown.$c: 9; a {b: shown.get()}`), "b: 9")
}

// TestHoistContainerStatements exercises the global-slot hoist walking into every
// nested-statement container type (declaration body, mixin/function bodies,
// @include, @at-root, @media, @supports and generic at-rules).
func TestHoistContainerStatements(t *testing.T) {
	files := map[string]string{
		"containers": `@mixin cm() { @if false { $mg: 1 !global; } }
@function cf() { @if false { $fg: 1 !global; } @return 1 }
@at-root a { @if false { $ag: 1 !global; } }
@media screen { c { @if false { $dg: 1 !global; } } }
@supports (a: b) { f { @if false { $sg: 1 !global; } } }
@font-face { @if false { $kg: 1 !global; } }
p { font: { size: 1px } }
@include cm;`,
	}
	got := compileModEnv(t, files, `@use "sass:meta"; @use "containers" as c;
z { a: meta.inspect(c.$mg); b: meta.inspect(c.$fg); d: meta.inspect(c.$ag); e: meta.inspect(c.$sg) }`)
	assertContains(t, got, "a: null", "b: null", "d: null", "e: null")
}

// TestHoistThroughImportEdges exercises the @import edge branches of the imported
// slot hoist: a plain (media-qualified) import, a .css import, an unresolvable
// import, and a diamond that revisits an already-seen file (cycle guard).
func TestHoistThroughImportEdges(t *testing.T) {
	files := map[string]string{
		"withimports":  "@import \"plaincss.css\";\n@import \"screenonly\" screen;\n@import \"missing-xyz\";\n@import \"diamond_b\";\n@import \"diamond_c\";",
		"plaincss.css": "a {b: c}",
		"screenonly":   "d {e: f}",
		"diamond_b":    `@import "diamond_d";`,
		"diamond_c":    `@import "diamond_d";`,
		"diamond_d":    "g {\n  @if false { $dd: 1 !global; }\n}",
	}
	got := compileModEnv(t, files,
		`@use "sass:meta"; @use "withimports"; z {a: meta.inspect(withimports.$dd)}`)
	assertContains(t, got, "a: null")
}

// TestImportForwardMixinThrough covers a mixin re-exported through a legacy
// @import (importForwardedModule's mixin seeding).
func TestImportForwardMixinThrough(t *testing.T) {
	files := map[string]string{
		"midstream": `@forward "upstream";`,
		"upstream":  "@mixin a() {b {c: d}}",
	}
	assertContains(t, compileModEnv(t, files,
		`@import "midstream"; @include a`), "c: d")
}

// TestGlobalWriteThroughSkipsNonOwner covers a global write-through probing
// several `@use as *` modules: the first that does not expose the name reports
// no match (module.setVar's not-found return) before the owning one is reached.
func TestGlobalWriteThroughSkipsNonOwner(t *testing.T) {
	files := map[string]string{
		"ma": "$other: 1;",
		"mb": "$x: 1;\n@function bget() {@return $x}",
	}
	assertContains(t, compileModEnv(t, files,
		`@use "ma" as *; @use "mb" as *; $x: 9 !global; c {v: bget()}`), "v: 9")
}

// TestHoistImportParseErrorIgnored covers the global-slot hoist tolerating an
// unparseable @import reached inside a never-called mixin: the hoist skips it and
// compilation still succeeds because the mixin is never invoked.
func TestHoistImportParseErrorIgnored(t *testing.T) {
	files := map[string]string{
		"mixmod":  "@mixin unused() { @import \"badfile\"; }\n$ok: 1;",
		"badfile": "a { b",
	}
	assertContains(t, compileModEnv(t, files,
		`@use "mixmod"; c {v: mixmod.$ok}`), "v: 1")
}

// TestImportTransitiveAsStarNotLeaked covers that a `@use as *` inside an
// @import'd file does not leak to the importing stylesheet.
func TestImportTransitiveAsStarNotLeaked(t *testing.T) {
	files := map[string]string{
		"midstream": `@use "upstream" as *;`,
		"upstream":  "@function upstream() {@return value}",
	}
	assertContains(t, compileModEnv(t, files,
		`@import "midstream"; a {b: upstream()}`), "b: upstream()")
}
