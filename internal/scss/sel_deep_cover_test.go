// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestSelectorDeepArms drives the selector module and the @extend engine through
// the algorithm arms that a flat case list misses: namespaced type/universal
// selectors, :host/:host-context/::slotted/:has/:not unification and
// superselector logic, sibling-combinator weaving, vendor-prefixed pseudos,
// :nth-child arguments and leading-combinator handling.
func TestSelectorDeepArms(t *testing.T) {
	sel := func(body string) string {
		return `@use "sass:selector";@use "sass:meta";.a{out: ` + body + `}`
	}
	cases := []string{
		// namespaced type / universal parse + serialize (write with ns).
		sel(`meta.inspect(selector.parse("svg|*"))`),
		sel(`meta.inspect(selector.parse("svg|a"))`),
		sel(`meta.inspect(selector.parse("|a"))`),
		sel(`meta.inspect(selector.parse("|*"))`),
		sel(`meta.inspect(selector.parse("*|*"))`),
		// pseudo-element with an argument that is not ::slotted.
		sel(`meta.inspect(selector.parse("::part(name)"))`),
		// multi-combinator complex (writeCombinators i>0).
		sel(`meta.inspect(selector.parse("c > > d"))`),
		// vendor-prefixed pseudo (unvendor loop past a second dash).
		sel(`selector.is-superselector("::-webkit-scrollbar", "::-webkit-scrollbar")`),
		sel(`meta.inspect(selector.parse(":-moz-any(a, b)"))`),
		// :nth-child arguments, including "of <selector>".
		sel(`selector.is-superselector(":nth-child(2n)", ":nth-child(2n)")`),
		sel(`meta.inspect(selector.parse(":nth-child(2n of .x)"))`),
		sel(`selector.replace(".a:nth-child(2)", ".a:nth-child(3)", ".b")`),
		// :host / :host-context / ::slotted unification and superselector arms.
		sel(`meta.inspect(selector.unify("*", ":host"))`),
		sel(`meta.inspect(selector.unify(":hover", ":host"))`),
		sel(`meta.inspect(selector.unify(".a", ":host"))`),
		sel(`meta.inspect(selector.unify(":host(x)", "a"))`),
		sel(`meta.inspect(selector.unify(":host", ":host(.x)"))`),
		sel(`selector.is-superselector(":host(a)", ":host(a)")`),
		sel(`selector.is-superselector("::slotted(a)", "::slotted(a)")`),
		sel(`selector.is-superselector("::slotted(a)", "::slotted(b)")`),
		sel(`selector.is-superselector(":host-context(a)", ":host-context(a)")`),
		// :has superselector arms.
		sel(`selector.is-superselector(":has(a)", ":has(a)")`),
		sel(`selector.is-superselector(":has(a)", ":has(b)")`),
		// :not superselector arms across type/id/pseudo.
		sel(`selector.is-superselector(":not(a)", "b")`),
		sel(`selector.is-superselector(":not(#a)", "#b")`),
		sel(`selector.is-superselector(":not(a, #b)", "c#d")`),
		sel(`selector.is-superselector("a:not(b)", "a:not(b)")`),
		// sibling-combinator weaving and unification.
		sel(`meta.inspect(selector.unify("a ~ b", "c ~ d"))`),
		sel(`meta.inspect(selector.unify("a + b", "c ~ d"))`),
		sel(`meta.inspect(selector.unify("a ~ b", "c + d"))`),
		sel(`meta.inspect(selector.unify("a > b", "c ~ d"))`),
		sel(`selector.is-superselector("a ~ c", "a ~ b ~ c")`),
		sel(`selector.is-superselector("a + c", "a + c")`),
		sel(`selector.is-superselector("a ~ c", "a + c")`),
		// leading combinators in nest / parse.
		sel(`meta.inspect(selector.parse("> a"))`),
		sel(`selector.nest("a", "> b", "~ c")`),
		// empty comma arms in the parser.
		sel(`meta.inspect(selector.parse(".a, , .b"))`),
	}
	for _, src := range cases {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("deep selector case failed: %q: %v", src, err)
		}
	}
}

// TestExtendDeepArms exercises @extend paths involving sibling combinators,
// leading combinators, rootish weaving, pseudo extenders and long transitive
// chains, reaching the extension-store arms the simple cases skip.
func TestExtendDeepArms(t *testing.T) {
	cases := []string{
		// specificity of a pseudo-class extender (default arm).
		`.a{x:y} .b:hover{@extend .a}`,
		// sibling-combinator contexts on both the target and the extender.
		`.x ~ .a{p:q} .b{@extend .a}`,
		`.x + .a{p:q} .b{@extend .a}`,
		`.a{p:q} .x ~ .b{@extend .a}`,
		`.a{p:q} .x + .b{@extend .a}`,
		// rootish weaving: extend inside a :root-anchored context.
		`:root .a{p:q} :root .b{@extend .a}`,
		`:root .a{p:q} .b{@extend .a}`,
		// child-combinator anchored extend.
		`.x > .a{p:q} .y > .b{@extend .a}`,
		// extend into :not / :is.
		`:not(.a){p:q} .b{@extend .a}`,
		`:is(.a, .c){p:q} .b{@extend .a}`,
		// transitive chains mixing combinators and compounds.
		`.a{p:q} .b .a{p:r} .c{@extend .a} .d{@extend .c}`,
		`.x ~ .a{p:q} .b{@extend .a} .c{@extend .b}`,
		// pseudo-element and id extenders.
		`.a{p:q} #b{@extend .a}`,
		`.a{p:q} .b::before{@extend .a}`,
		// duplicate/multi-target on a shared compound.
		`.a.b{p:q} .c{@extend .a} .d{@extend .b}`,
		// self-extend and duplicate extends (no-change / dedup arms).
		`.a{x:y} .a{@extend .a}`,
		`.a{x:y} .b{@extend .a} .b{@extend .a}`,
		`.a{x:y} .b .a{p:q} .c{@extend .a} .c{@extend .a}`,
		// placeholder transitive fan reaching a target through two paths.
		`%x{a:b} .y{@extend %x} .z{@extend .y} .w{@extend %x}`,
		// optional plus required extend of the same target.
		`.a{x:y} .b{@extend .a !optional} .c{@extend .a}`,
		// target that also appears inside :not (extend a no-op on some boxes).
		`.a{x:y} :not(.a){p:q} .b{@extend .a}`,
		// long transitive fan producing existing-extension re-extension.
		`.a{p:q} .b{@extend .a} .c{@extend .b} .d{@extend .a} .e{@extend .c}`,
		// multi-target compound with sibling-anchored extenders.
		`.a.b{p:q} .x ~ .c{@extend .a} .y + .d{@extend .b}`,
	}
	for _, src := range cases {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("deep extend case failed: %q: %v", src, err)
		}
	}
}
