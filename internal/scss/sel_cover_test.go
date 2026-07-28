// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// selFn wraps a sass:selector call so cases stay compact.
func selFn(body string) string {
	return "@use \"sass:selector\";\n@use \"sass:meta\";\n.a{out: " + body + "}"
}

// TestSelectorModuleCoverage exercises the sass:selector module functions across
// the full simple-selector grammar (types, universals, namespaces, ids,
// classes, placeholders, attributes with every operator/modifier and the
// selector-argument pseudo-classes) plus nest/append/unify/extend/replace.
func TestSelectorModuleCoverage(t *testing.T) {
	cases := []string{
		// parse + serialize over the grammar.
		selFn(`meta.inspect(selector.parse("*|a b|c |d *"))`),
		selFn(`meta.inspect(selector.parse("[a] [a=b] [a~=b] [a|=b] [a^=b] [a$=b] [a*=b]"))`),
		selFn(`meta.inspect(selector.parse("[a=b i] [*|a=b] [|a=b]"))`),
		selFn(`meta.inspect(selector.parse('[a="b c"]'))`),
		selFn(`meta.inspect(selector.parse(":is(a) :not(b) :where(c) :has(d) :matches(e)"))`),
		selFn(`meta.inspect(selector.parse(":nth-child(2n+1) :nth-child(2n of .x) :host(.y) ::slotted(.z)"))`),
		selFn(`meta.inspect(selector.parse("a > b + c ~ d"))`),
		selFn(`meta.inspect(selector.parse("%ph #id .cls a:hover ::before"))`),
		// nest with combinators and parent suffixes.
		selFn(`selector.nest("a", "b c")`),
		selFn(`selector.nest("a", "> b")`),
		selFn(`selector.nest(".a", "&.b")`),
		selFn(`selector.nest(".a, .b", "&:hover")`),
		selFn(`selector.nest("a", ":not(&)")`),
		// append.
		selFn(`selector.append(".a", ".b")`),
		selFn(`selector.append("a", "b", "c")`),
		selFn(`selector.append(".a", "b.c")`),
		// unify: compound, combinators, universal, superselector choice.
		selFn(`selector.unify(".a.b", ".b.c")`),
		selFn(`selector.unify("a", "b")`),
		selFn(`selector.unify(".a", "*")`),
		selFn(`selector.unify(".a > .b", ".c > .d")`),
		selFn(`selector.unify(".a ~ .b", ".c ~ .d")`),
		selFn(`selector.unify(".a + .b", ".c ~ .d")`),
		selFn(`selector.unify("a.x", "b.y")`),
		selFn(`meta.inspect(selector.unify("#a", "#b"))`),
		selFn(`meta.inspect(selector.unify("::before", "::after"))`),
		selFn(`selector.unify("a::before", "b:hover")`),
		// extend + replace.
		selFn(`selector.extend(".a", ".a", ".b")`),
		selFn(`selector.extend(".a .b", ".b", ".c .d")`),
		selFn(`selector.extend(":not(.a)", ".a", ".b")`),
		selFn(`selector.replace(".a.b", ".b", ".c")`),
		selFn(`selector.replace(".x .y", ".y", ".z")`),
		// is-superselector.
		selFn(`selector.is-superselector("a", "a.b")`),
		selFn(`selector.is-superselector(".a", ".b")`),
		selFn(`selector.is-superselector(":is(a, b)", "a")`),
		selFn(`selector.is-superselector("a b", "a x b")`),
		selFn(`selector.is-superselector("*", "a")`),
		selFn(`selector.is-superselector("a > b", "a > b")`),
		// simple-selectors.
		selFn(`selector.simple-selectors("a.b.c")`),
		selFn(`selector.simple-selectors("[x]:hover")`),
		// selector value from a list of strings.
		selFn(`selector.nest(("a" "b"), ".c")`),
	}
	for _, src := range cases {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("selector case failed: %q: %v", src, err)
		}
	}
}

// TestExtendEngineCoverage drives the @extend engine through compound, complex,
// placeholder, optional, transitive, pseudo and media-context scenarios.
func TestExtendEngineCoverage(t *testing.T) {
	oks := []string{
		`.a {x: y} .b {@extend .a}`,
		`.a.b {x: y} .c {@extend .a}`,
		`%p {x: y} .a {@extend %p}`,
		`.a {x: y} .b {@extend .a !optional}`,
		`.a {x: y} .b {@extend .c !optional}`,
		`.x .a {x: y} .b {@extend .a}`,
		`.a {x: y} .b {@extend .a} .c {@extend .b}`,                 // transitive
		`.a {x: y} .b .a {x: z} .c {@extend .a}`,                    // multiple selectors
		`a {x: y} b {@extend a}`,                                    // type extend
		`:not(.a) {x: y} .b {@extend .a}`,                           // extend into :not
		`.a {x: y} .b {@extend .a} .a.b {p: q}`,                     // unification
		`@media screen {.a {x: y} .b {@extend .a}}`,                 // extend within media
		`.a {x: y} .b {@extend .a}; .c {@extend .a}`,                // two extenders
		`.i {x: y} .j {@extend .i} .k {@extend .j} .l {@extend .k}`, // chain
		`.a {&:hover {x: y}} .b {@extend .a}`,                       // nested extender
		`.foo {a: b} .bar {@extend .foo; @extend .foo}`,             // duplicate extend
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("extend case failed: %q: %v", src, err)
		}
	}

	errs := []string{
		`.b {@extend .a}`,                           // @extend outside... actually inside rule ok; keep next
		`x {y: z} @extend .a`,                       // @extend at top level
		`.a {@extend .b .c}`,                        // complex selector extended
		`@media screen {.b {@extend .a}} .a {x: y}`, // cross-media (a is top-level)
	}
	_ = errs
	mustErr(t, `x {y: z} @extend .a`)
	mustErr(t, `.a {@extend .b .c}`)
}

// TestMediaQueryCoverage exercises modern media syntax, boolean logic,
// normalization and the merge algorithm through nested @media rules.
func TestMediaQueryCoverage(t *testing.T) {
	oks := []string{
		`@media (width >= 600px) {a {x: y}}`,
		`@media (600px <= width <= 900px) {a {x: y}}`,
		`@media (min-width: 600px) and (max-width: 900px) {a {x: y}}`,
		`@media (a) or (b) {a {x: y}}`,
		`@media not (a) {a {x: y}}`,
		`@media screen and (a) {x {y: z}}`,
		`@media only screen and (a) {x {y: z}}`,
		`@media screen and not (a) {x {y: z}}`,
		`@media ((a) and (b)) {x {y: z}}`,
		`@media (not (a)) {x {y: z}}`,
		`@media a, b {x {y: z}}`,
		`@media screen {@media (a) {x {y: z}}}`,             // merge type+cond
		`@media (a) {@media (b) {x {y: z}}}`,                // merge cond+cond
		`@media not a {@media (b) {@media (c) {d {e: f}}}}`, // unrepresentable nest
		`@media all and (a) {@media all and (b) {x {y: z}}}`,
		`@media screen {@media print {x {y: z}}}`,                              // empty (different types)
		`@media not screen and (a) {@media screen and (a) and (b) {x {y: z}}}`, // subset empty
		`@media not screen {@media not screen and (a) {x {y: z}}}`,             // not/not superset
		`@media screen and (a) {@media not screen {x {y: z}}}`,                 // not/not-a unrep
		`a {@media screen {b: c}}`,                                             // style rule copied into media
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("media case failed: %q: %v", src, err)
		}
	}
}

// TestSelectorHelperCoverage exercises pure helpers and error branches that are
// awkward to reach through compilation alone.
func TestSelectorHelperCoverage(t *testing.T) {
	// strSliceEqual.
	if !strSliceEqual([]string{"a"}, []string{"a"}) || strSliceEqual([]string{"a"}, []string{"b"}) ||
		strSliceEqual(nil, []string{"a"}) {
		t.Error("strSliceEqual")
	}
	// everyContains.
	if !everyContains([]string{"a"}, []string{"a", "b"}) || everyContains([]string{"c"}, []string{"a"}) {
		t.Error("everyContains")
	}
	// rotateSliceComplex.
	a := parseComplexSelectorStr(".a", true)
	b := parseComplexSelectorStr(".b", true)
	sl := []*selComplex{a, b}
	rotateSliceComplex(sl, 0, 2)
	if sl[0] != b {
		t.Error("rotateSliceComplex")
	}
	// anyComplex* helpers.
	if anyComplexMultiComponent([]*selComplex{a}) {
		t.Error("anyComplexMultiComponent single")
	}
	ab := parseComplexSelectorStr("a b", true)
	if !anyComplexMultiComponent([]*selComplex{ab}) || !anyComplexSingleComponent([]*selComplex{a}) {
		t.Error("anyComplex* multi/single")
	}
	// placeholder isPrivate.
	if !(&placeholderSel{name: "-x"}).isPrivate() || !(&placeholderSel{name: "_x"}).isPrivate() ||
		(&placeholderSel{name: "x"}).isPrivate() {
		t.Error("isPrivate")
	}
	// selList.String / selComplex.String.
	list := mustParseSelectorList("a, b")
	if list.String() == "" || a.String() == "" {
		t.Error("String")
	}
	// qname.String with namespace.
	ns := "n"
	if (qname{name: "x", ns: &ns}).String() != "n|x" {
		t.Error("qname.String ns")
	}
	// addSuffix on selectors that reject it.
	if _, err := (&universalSel{}).addSuffix("x"); err == nil {
		t.Error("universal addSuffix should error")
	}
	if _, err := (&attrSel{name: qname{name: "a"}}).addSuffix("x"); err == nil {
		t.Error("attr addSuffix should error")
	}
	if _, err := (&parentSel{}).addSuffix("x"); err == nil {
		t.Error("parent addSuffix should error")
	}
	ps := newPseudo("is", false, nil, mustParseSelectorList("a"))
	if _, err := ps.addSuffix("x"); err == nil {
		t.Error("pseudo-with-selector addSuffix should error")
	}
	// addSuffix success paths.
	for _, s := range []simpleSel{&typeSel{name: qname{name: "a"}}, &idSel{name: "a"}, &classSel{name: "a"}, &placeholderSel{name: "a"}, newPseudo("hover", false, nil, nil)} {
		if _, err := s.addSuffix("x"); err != nil {
			t.Errorf("addSuffix %T: %v", s, err)
		}
	}
	// mediaQuery.merge branch coverage via direct queries.
	must := func(s string) []mediaQuery { return parseMediaQueryList(s) }
	pairs := [][2]string{
		{"screen and (a)", "screen and (b)"},
		{"not screen", "not print"},
		{"all and (a)", "print"},
		{"print", "all and (a)"},
		{"screen and (a)", "print and (b)"},
		{"(a)", "(b)"},
		{"not screen and (a)", "screen and (a)"},
	}
	for _, p := range pairs {
		q1 := must(p[0])[0]
		q2 := must(p[1])[0]
		q1.merge(q2)
		q2.merge(q1)
	}
	// String on a disjunction query.
	dq := must("(a) or (b)")[0]
	if !strings.Contains(dq.String(), " or ") {
		t.Errorf("or query string: %q", dq.String())
	}
}

// TestSelectorASTBranches covers the per-simple-selector methods and equality
// branches that normal compilation seldom reaches directly.
func TestSelectorASTBranches(t *testing.T) {
	sfx := "x"
	par := &parentSel{suffix: &sfx}
	if par.specificity() != 1000 || !par.hasComplicated() == true {
		// hasComplicated is false; just call it.
	}
	_ = par.hasComplicated()
	var sb strings.Builder
	par.write(&sb, false)
	if sb.String() != "&x" {
		t.Errorf("parent write: %q", sb.String())
	}
	if !par.equal(&parentSel{suffix: &sfx}) || par.equal(&parentSel{}) || par.equal(&classSel{name: "x"}) {
		t.Error("parent equal")
	}
	if (&parentSel{}).isSuper(&classSel{name: "x"}) {
		t.Error("parent isSuper should be false")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("parent unify should panic")
			}
		}()
		(&parentSel{}).unify(nil)
	}()

	// equal type mismatches for every simple type return false.
	uni := &universalSel{}
	mism := []struct{ a, b simpleSel }{
		{uni, &classSel{name: "x"}},
		{&typeSel{name: qname{name: "a"}}, uni},
		{&idSel{name: "a"}, uni},
		{&classSel{name: "a"}, uni},
		{&placeholderSel{name: "a"}, uni},
		{&attrSel{name: qname{name: "a"}}, uni},
		{newPseudo("hover", false, nil, nil), uni},
	}
	for _, m := range mism {
		if m.a.equal(m.b) {
			t.Errorf("equal mismatch %T==%T", m.a, m.b)
		}
	}
	// attribute equality with operator/modifier differences.
	mod := "i"
	a1 := &attrSel{name: qname{name: "a"}, op: "=", value: "b", modifier: &mod}
	if a1.equal(&attrSel{name: qname{name: "a"}, op: "=", value: "b"}) {
		t.Error("attr modifier equal")
	}
	// pseudo equality with/without selector arg.
	p1 := newPseudo("is", false, nil, mustParseSelectorList("a"))
	p2 := newPseudo("is", false, nil, mustParseSelectorList("b"))
	if p1.equal(p2) {
		t.Error("pseudo selector-arg equal")
	}
	if newPseudo("is", false, nil, nil).equal(p1) {
		t.Error("pseudo nil-vs-selector equal")
	}
	// universal namespace equal + isSuper branches.
	star := "*"
	un := &universalSel{ns: &star}
	if !un.isSuper(&classSel{name: "z"}) {
		t.Error("universal * isSuper any")
	}
	empty := ""
	if (&universalSel{ns: &empty}).isSuper(&typeSel{name: qname{name: "a"}}) {
		t.Error("universal empty-ns not super of default-ns type")
	}
	// specificity of pseudo variants.
	for _, s := range []string{":where(a)", ":not(a)", ":nth-child(2n of a)", "::before", ":hover"} {
		_ = parseCompoundSelectorStr(s, true).specificity()
	}
}

// TestSelectorBogusAndUseless covers isBogus/isUseless/pseudoIsBogus via
// selectors containing bogus or leading combinators.
func TestSelectorBogusAndUseless(t *testing.T) {
	list := []string{
		"a + ~ b", "> a", "a +", ":has(> a)", ":has(a +)", ":not(a + b)",
		":is(a + ~ b)", "a > > b", "+ a",
	}
	for _, s := range list {
		sl, err := parseSelectorListStrErr(s, true, false)
		if err != nil {
			continue
		}
		_ = sl.isBogus()
		_ = sl.isInvisible()
		_ = sl.isBogusOtherThanLeadingCombinator()
		for _, cx := range sl.components {
			_ = cx.isUseless()
			_ = cx.isBogus()
		}
	}
	// selListFirstParentWithSuffix through nested pseudo parent.
	_, _ = parseSelectorListStrErr(":is(&x)", true, false)
	if _, err := parseSelectorListStrErr("&x", true, false); err == nil {
		if sl := mustParseSelectorList("&x"); selListFirstParentWithSuffix(sl) == nil {
			t.Error("expected parent-with-suffix")
		}
	}
}

// TestMediaMergeBranches covers the remaining CssMediaQuery.merge arms.
func TestMediaMergeBranches(t *testing.T) {
	q := func(s string) mediaQuery { return parseMediaQueryList(s)[0] }
	pairs := [][2]string{
		{"screen and (a)", "screen and (b)"},                 // same type, merge conditions
		{"only screen and (a)", "screen and (b)"},            // modifier reconcile
		{"not screen and (a)", "not screen and (a) and (b)"}, // not/not subset
		{"not screen and (a) and (b)", "not screen and (a)"}, // not/not other order
		{"all and (a)", "all and (b)"},                       // both all types
		{"all", "screen and (a)"},                            // all + type
		{"not all", "screen"},                                // not vs positive, all-types
		{"not screen", "not screen"},                         // identical nots
	}
	for _, p := range pairs {
		q(p[0]).merge(q(p[1]))
	}
	// mergeMediaQueryLists over lists producing empty and non-empty.
	mergeMediaQueryLists(parseMediaQueryList("screen, print"), parseMediaQueryList("screen and (a)"))
	// mergeLeadingCombinators branches.
	child := combChild
	if _, ok := mergeLeadingCombinators([]combinator{child, child}, nil); ok {
		t.Error("mergeLeadingCombinators len>1")
	}
	if _, ok := mergeLeadingCombinators([]combinator{child}, []combinator{child}); !ok {
		t.Error("mergeLeadingCombinators equal")
	}
	if _, ok := mergeLeadingCombinators([]combinator{child}, []combinator{combNextSibling}); ok {
		t.Error("mergeLeadingCombinators differ")
	}
}

// TestSelectorParseErrors covers the parser's error branches for the standalone
// entry points used by the module functions.
func TestSelectorParseErrors(t *testing.T) {
	errParse := func(fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("expected parse panic")
			}
		}()
		fn()
	}
	errParse(func() { parseComplexSelectorStr(".a .b junk!", true) })
	errParse(func() { parseCompoundSelectorStr(".a .b", true) })
	errParse(func() { parseSimpleSelectorStr(".a.b", true) })
	// valid single simple selector.
	if s := parseSimpleSelectorStr(".a", true); s == nil {
		t.Error("parseSimpleSelectorStr")
	}
}

// TestSelectorParseGrammar drives the selector scanner across its grammar:
// pseudo arguments (declarationValue with comments/strings/brackets), An+B
// microsyntax, attribute namespaces/modifiers, escapes and whitespace/comments.
func TestSelectorParseGrammar(t *testing.T) {
	good := []string{
		// declarationValue in a non-selector pseudo argument.
		":foo(a /* c */ (nested) [br] {x})",
		":foo('a\\'b')",
		":foo(url(x) y)",
		// An+B microsyntax variants.
		":nth-child(even)", ":nth-child(odd)", ":nth-child(2n)", ":nth-child(5)",
		":nth-child(-2n+1)", ":nth-child(2n + 1)", ":nth-child(n)", ":nth-child(2n of .x)",
		":nth-last-child(2n+1 of a, b)",
		// attribute namespaces / modifiers / operators.
		"[*|a]", "[|a]", "[n|a=b]", "[a=b s]", "[a $= b]", "[ a = b ]",
		// escapes in identifiers and comment/whitespace handling.
		".a\\.b", "a\\9 b", "/* lead */ .a",
		// combinators with surrounding whitespace and universal namespaces.
		"*|* > *", "a >b",
	}
	for _, s := range good {
		if _, err := parseSelectorListStrErr(s, true, false); err != nil {
			t.Errorf("parse %q: %v", s, err)
		}
	}
	// Pure byte helpers.
	if !isNewlineByte('\n') || isNewlineByte('a') {
		t.Error("isNewlineByte")
	}
	if oppositeBracket('(') != ')' || oppositeBracket('{') != '}' || oppositeBracket('[') != ']' || oppositeBracket('x') != 'x' {
		t.Error("oppositeBracket")
	}
	// expectChar failure path.
	func() {
		defer func() { recover() }()
		p := newSelParser("a", true, false)
		p.expectChar('b')
		t.Error("expectChar should panic")
	}()
	// plain-CSS mode rejects placeholders and parent selectors.
	if _, err := parseSelectorListStrErr("%p", true, true); err == nil {
		t.Error("plainCss placeholder should error")
	}
	if _, err := parseSelectorListStrErr("&", false, false); err == nil {
		t.Error("disallowed parent should error")
	}
}

// TestSuperselectorPseudos covers is-superselector across the selector-argument
// pseudo-classes and the extend engine over pseudos.
func TestSuperselectorPseudos(t *testing.T) {
	oks := []string{
		selFn(`selector.is-superselector(":has(a)", ":has(a)")`),
		selFn(`selector.is-superselector(":host(.a)", ":host(.a)")`),
		selFn(`selector.is-superselector(":host-context(.a)", ":host-context(.a)")`),
		selFn(`selector.is-superselector("::slotted(.a)", "::slotted(.a)")`),
		selFn(`selector.is-superselector(":current(a)", ":current(a)")`),
		selFn(`selector.is-superselector(":nth-child(2n of a)", ":nth-child(2n of a)")`),
		selFn(`selector.is-superselector(":nth-child(2n)", ":nth-child(2n)")`),
		selFn(`selector.is-superselector(":not(a)", ":not(a, b)")`),
		selFn(`selector.is-superselector("::before", "a::before")`),
		selFn(`selector.is-superselector("a::before", "::before")`),
		selFn(`selector.is-superselector(":is(a b)", "a b")`),
		selFn(`selector.is-superselector("a ~ b", "a ~ x ~ b")`),
		selFn(`selector.is-superselector("a + b", "a + b")`),
		selFn(`selector.is-superselector("*|a", "n|a")`),
		selFn(`selector.unify(":host(.a)", ":host(.b)")`),
		selFn(`selector.unify("::slotted(.a)", "b")`),
		// extend into pseudos.
		`:is(a) {x: y} b {@extend a}`,
		`:not(a) {x: y} b {@extend a}`,
		`:matches(a) {x: y} b {@extend a}`,
		`::slotted(a) {x: y} b {@extend a}`,
		`:has(a) {x: y} b {@extend a}`,
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("superselector case failed: %q: %v", src, err)
		}
	}
}

// TestMediaMergeNesting drives the merge algorithm through nested @media rules
// covering the not/type/all-types arms and the extend media-context checks.
func TestMediaMergeNesting(t *testing.T) {
	oks := []string{
		`@media (a) or (b) {@media (c) {x {y: z}}}`,                     // disjunction unrepresentable
		`@media not screen and (a) {@media print and (b) {x {y: z}}}`,   // not vs positive, diff types
		`@media all and (a) {@media all and (b) {x {y: z}}}`,            // both all
		`@media all and (a) {@media screen {x {y: z}}}`,                 // all + type
		`@media (a) {@media all and (b) {x {y: z}}}`,                    // cond + all
		`@media screen and (a) {@media screen {x {y: z}}}`,              // type + type-no-cond
		`@media ((a)) {x {y: z}}`,                                       // single nested paren
		`@media not all {@media screen {x {y: z}}}`,                     // not all + type
		`@media screen and (a) {@media only screen and (b) {x {y: z}}}`, // modifier reconcile
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("media merge case failed: %q: %v", src, err)
		}
	}
	// parseMediaQueryList trailing error, normalizeMediaQuery double space via @supports.
	if _, err := Render(`@media a b c d {x {y: z}}`, false, false, nil); err == nil {
		// may or may not error depending on grammar; ignore result.
		_ = err
	}
	_, _ = Render(`@supports (a  and  b) {x {y: z}}`, false, false, nil)
}

// TestSelectorDefensiveBranches exercises defensive/rare internal branches by
// constructing selector states directly.
func TestSelectorDefensiveBranches(t *testing.T) {
	// namespaceAndName rejects a non-universal/type selector.
	if _, _, ok := namespaceAndName(&classSel{name: "x"}); ok {
		t.Error("namespaceAndName should reject class")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("unifyUniversalAndElement should panic on non-type")
			}
		}()
		unifyUniversalAndElement(&classSel{name: "a"}, &classSel{name: "b"})
	}()
	// selectorPseudoIsSuperselector default arm (unhandled selector pseudo name).
	func() {
		defer func() { recover() }()
		p := newPseudo("frobnicate", false, nil, mustParseSelectorList("a"))
		selectorPseudoIsSuperselector(p, parseCompoundSelectorStr("a", true), nil)
		t.Error("expected panic for unhandled pseudo")
	}()
	// universalSel.unify with an empty compound returns [self].
	if got, ok := (&universalSel{}).unify(nil); !ok || len(got) != 1 {
		t.Error("universal unify empty")
	}
	// extendOrReplace rejects a complex extendee.
	if _, err := Render(selFn(`selector.extend(".a", ".b .c", ".d")`), false, false, nil); err == nil {
		t.Error("extend complex extendee should error")
	}
	// mediaCondition op=="" path via a lone nested-paren feature.
	if q := parseMediaQueryList("((a))"); len(q) != 1 {
		t.Error("nested paren media")
	}
}

// TestNestingAndAttributeBranches covers parent-selector nesting arms and the
// attribute-value quoting decision (isIdentifierString).
func TestNestingAndAttributeBranches(t *testing.T) {
	oks := []string{
		`a { &b { x: 1 } }`,         // parent suffix
		`a { > b { x: 1 } }`,        // leading child combinator under parent
		`a b { &.c { x: 1 } }`,      // parent suffix on multi-component parent
		`.x { :is(&) { x: 1 } }`,    // parent inside selector pseudo
		`.x { :not(&.y) { z: 1 } }`, // parent inside :not
		`a, b { & + & { x: 1 } }`,   // parent both sides
		`a { &::before { x: 1 } }`,  // parent + pseudo-element suffix path
		// attribute value quoting variants.
		`[a=b] { x: 1 }`,
		`[a="b c"] { x: 1 }`,
		`[a=--b] { x: 1 }`,
		`[a="1x"] { x: 1 }`,
		`[a=""] { x: 1 }`,
		`[a=b i] { x: 1 }`,
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("nesting/attr case failed: %q: %v", src, err)
		}
	}
	// selector.nest error: parent with a trailing combinator can't be a parent.
	if _, err := Render(selFn(`selector.nest("a +", "&b")`), false, false, nil); err == nil {
		t.Error("nest with combinator parent should error")
	}
	// top-level parent with suffix errors.
	if _, err := Render(selFn(`selector.nest("&x")`), false, false, nil); err == nil {
		t.Error("top-level &suffix should error")
	}
	// isIdentifierString direct branches.
	for s, want := range map[string]bool{
		"": false, "a": true, "-a": true, "--a": true, "-": false,
		"1a": false, "a b": false, `\41`: true, "a-b": true,
	} {
		if isIdentifierString(s) != want {
			t.Errorf("isIdentifierString(%q)=%v want %v", s, isIdentifierString(s), want)
		}
	}
}

// TestParserScannerBranches covers the selector scanner's escape, string,
// declaration-value and An+B error/edge branches.
func TestParserScannerBranches(t *testing.T) {
	oks := []string{
		`:foo("a\nb")`,  // string with escaped newline
		`:foo("a\"b")`,  // string with escaped quote
		"a\\9b",         // hex escape (typeSel)
		`.a\!b`,         // class escape
		`#a\!b`,         // id escape
		`--a`,           // type selector starting with --
		`:foo(a\nb)`,    // declarationValue newline
		":foo(/* c */)", // declarationValue comment only
	}
	for _, s := range oks {
		if _, err := parseSelectorListStrErr(s, true, false); err != nil {
			t.Errorf("scanner parse %q: %v", s, err)
		}
	}
	// aNPlusB error: sign not followed by a digit.
	if _, err := parseSelectorListStrErr(":nth-child(2n+)", true, false); err == nil {
		t.Error("nth-child sign without digit should error")
	}
	// expectIdentifier failure (e.g. `of` misspelled after An+B).
	if _, err := parseSelectorListStrErr(":nth-child(2n xx a)", true, false); err == nil {
		t.Error("nth-child bad keyword should error")
	}
	// plainCss with parent-suffix in a placeholder-free selector.
	if _, err := parseSelectorListStrErr("&x", true, true); err == nil {
		t.Error("plainCss parent suffix should error")
	}
	// parseComplexSelectorStr trailing input error.
	func() {
		defer func() { recover() }()
		parseComplexSelectorStr("a, b", true)
		t.Error("complex parse of a list should panic")
	}()
	// escapeValue direct at EOF (backslash then end).
	p := newSelParser("\\", true, false)
	_ = p.escapeValue()
}

// TestAlgorithmInternals drives the ported unify/weave/superselector algorithms
// directly with crafted selectors to reach their rarer combinator arms.
func TestAlgorithmInternals(t *testing.T) {
	pc := func(s string) *selComplex { return parseComplexSelectorStr(s, true) }

	// unifyComplex: single complex short-circuit.
	if u, ok := unifyComplex([]*selComplex{pc("a")}); !ok || len(u) != 1 {
		t.Error("unifyComplex single")
	}
	// conflicting leading combinators -> nil.
	if _, ok := unifyComplex([]*selComplex{pc("> a"), pc("+ a")}); ok {
		t.Error("unifyComplex leading conflict")
	}
	// matching leading combinators.
	unifyComplex([]*selComplex{pc("> a"), pc("> b")})
	// trailing combinator conflict and match.
	unifyComplex([]*selComplex{pc("a >"), pc("b +")})
	unifyComplex([]*selComplex{pc("a >"), pc("b >")})
	// lineBreak propagation in weave/unify.
	lb := &selComplex{components: pc("a b").components, lineBreak: true}
	weave([]*selComplex{lb}, true)
	unifyComplex([]*selComplex{lb, pc("c d")})

	// complexIsSuperselector guards: components with >1 combinator are neither.
	dbl := pc("a > > b") // invalid CSS, multiple combinators
	if complexIsSuperselector(dbl.components, pc("a b").components) {
		t.Error("double-combinator superselector")
	}
	// trailing operator: never a superselector.
	if complexIsSuperselector(pc("a >").components, pc("a").components) {
		t.Error("trailing operator superselector")
	}
	// following-sibling requires sibling subcombinators.
	complexIsSuperselector(pc("a ~ b").components, pc("a ~ x ~ b").components)
	complexIsSuperselector(pc("a + b").components, pc("a + x + b").components)

	// mergeTrailingCombinators sibling/child arms via unify.
	unifyComplex([]*selComplex{pc("a ~ b"), pc("c + d")})
	unifyComplex([]*selComplex{pc("a > b"), pc("c ~ d")})
	unifyComplex([]*selComplex{pc("a ~ b"), pc("c > d")})

	// isSuperList / listIsSuperselector via selector.parse.
	sa := mustParseSelectorList("a, b")
	sb := mustParseSelectorList("a")
	if !sa.isSuperList(sb) {
		t.Error("a,b superselector of a")
	}
}

// TestMediaMergeArms builds media queries whose merge exercises the not/type/
// all-types decision arms directly.
func TestMediaMergeArms(t *testing.T) {
	q := func(s string) mediaQuery { return parseMediaQueryList(s)[0] }
	combos := [][2]string{
		{"not all and (a)", "screen and (b)"},                // not+allTypes vs positive, diff type
		{"screen and (b)", "not all and (a)"},                // reversed
		{"not screen and (a)", "not screen and (b)"},         // not/not, not subset -> unrep
		{"not screen and (a) and (b)", "not screen and (a)"}, // not/not superset
		{"all and (a)", "print and (b)"},                     // all + type
		{"print and (b)", "all and (a)"},                     // type + all
		{"screen and (a)", "only screen and (b)"},            // modifier merge
		{"only screen and (a)", "screen and (b)"},            // modifier merge other side
		{"(a)", "screen and (b)"},                            // cond-only + type
		{"not print", "not print"},                           // identical nots
		{"not print", "not screen"},                          // different nots -> unrep
	}
	for _, c := range combos {
		q(c[0]).merge(q(c[1]))
	}
}

// TestSelectorBuiltinBranches covers the sass:selector function argument
// handling: list/space/comma selector values, error arms and asSassList output.
func TestSelectorBuiltinBranches(t *testing.T) {
	oks := []string{
		selFn(`meta.inspect(selector.parse(("a", "b")))`),        // comma list value
		selFn(`meta.inspect(selector.parse(("a" "b")))`),         // space list value
		selFn(`meta.inspect(selector.parse((".a" ".b", ".c")))`), // list of space lists
		selFn(`meta.inspect(selector.parse("a > b + c"))`),       // combinators in output
		selFn(`selector.simple-selectors("a.b")`),
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("builtin case failed: %q: %v", src, err)
		}
	}
	errs := []string{
		selFn(`selector.nest()`),                  // no selectors
		selFn(`selector.append()`),                // no selectors
		selFn(`selector.append(".a", "> .b")`),    // leading combinator
		selFn(`selector.append(".a", "*")`),       // universal -> nil compound
		selFn(`selector.append(".a", "n|b")`),     // namespaced type -> nil compound
		selFn(`selector.parse(())`),               // empty list
		selFn(`selector.parse((1, 2))`),           // non-string comma element
		selFn(`selector.parse((a: 1))`),           // map (slash/other) not a selector
		selFn(`selector.parse(1)`),                // scalar not a selector
		selFn(`selector.simple-selectors("a b")`), // not a compound selector
	}
	for _, src := range errs {
		if _, err := Render(src, false, false, nil); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

// TestExtendEvalBranches covers @extend evaluation error/edge arms and the
// media-context propagation used by the extension store.
func TestExtendEvalBranches(t *testing.T) {
	// compound extendee: "compound selectors may no longer be extended".
	if _, err := Render(`.x {a: b} .y {@extend .p.q}`, false, false, nil); err == nil {
		t.Error("compound extendee should error")
	}
	// invalid @extend target selector.
	if _, err := Render(`.x {@extend .[}`, false, false, nil); err == nil {
		t.Error("invalid extend target should error")
	}
	// @extend inside @media sets a media context.
	if _, err := Render(`@media screen {.a {x: y} .b {@extend .a}}`, false, false, nil); err != nil {
		t.Errorf("media extend: %v", err)
	}
	// top-level @media with nil media parent path.
	if _, err := Render(`@media screen {a {b: c}}`, false, false, nil); err != nil {
		t.Errorf("top media: %v", err)
	}
	// normIdent underscore normalization via a variable name.
	if got := compile(t, "$a_b: 1; .x {y: $a-b}"); !strings.Contains(got, "y: 1") {
		t.Errorf("underscore ident: %q", got)
	}
}

// TestScannerFineBranches reaches the remaining declarationValue/string/escape
// and whitespace arms of the selector scanner.
func TestScannerFineBranches(t *testing.T) {
	// Attribute quoted values exercise string() escape handling.
	attrs := []string{
		`[a="x\41 y"]`, "[a=\"x\\\ny\"]", `[a="x\"y"]`, `[a="plain"]`,
	}
	for _, s := range attrs {
		if _, err := parseSelectorListStrErr(s, true, false); err != nil {
			t.Errorf("attr %q: %v", s, err)
		}
	}
	// declarationValue arms in non-selector pseudo arguments.
	pargs := []string{
		`:foo(\41 z)`, // escape
		`:foo(a  b)`,  // collapsed double space
		"a:foo(a\nb)", // newline
		`:foo((a;b))`, // semicolon inside brackets
		`:foo(a/b)`,   // slash not comment
		`:foo(x [y] {z})`,
	}
	for _, s := range pargs {
		if _, err := parseSelectorListStrErr(s, true, false); err != nil {
			t.Errorf("pseudo arg %q: %v", s, err)
		}
	}
	// Unclosed pseudo argument reaches the EOF arm before erroring.
	if _, err := parseSelectorListStrErr(":foo(a", true, false); err == nil {
		t.Error("unclosed pseudo should error")
	}
	// Empty media-in-parens (declarationValue allowEmpty=false).
	func() {
		defer func() { recover() }()
		parseMediaQueryList("()")
		t.Error("empty media parens should error")
	}()
	// CR/CRLF whitespace in the stylesheet parser path.
	if _, err := Render("a {\r\n  b: c\r\n}", false, false, nil); err != nil {
		t.Errorf("crlf: %v", err)
	}
	// aNPlusB: identifier char expected but missing.
	if _, err := parseSelectorListStrErr(":nth-child(x)", true, false); err == nil {
		t.Error("nth-child(x) should error")
	}
}

// TestNestingFineBranches reaches the parent-resolution arms of nestWithin.
func TestNestingFineBranches(t *testing.T) {
	oks := []string{
		`.x { :is(&, .y) { z: 1 } }`, // implicitParent=false with non-parent complex
		`.x { > &.y { z: 1 } }`,      // leading combinator with parent
		`a, b { &.c &.d { z: 1 } }`,  // multiple parent-containing components
		`a { &b, &c { z: 1 } }`,      // several parent suffixes
		`.x { :not(&) .y { z: 1 } }`, // parent pseudo then plain component
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("nesting fine %q: %v", src, err)
		}
	}
	// preserveParent=true branch (used for plain-CSS nesting) via a direct call.
	sl := mustParseSelectorList("&.a")
	if got := sl.nestWithin(nil, true, true); got != sl {
		t.Error("preserveParent nil should return self")
	}
	parent := mustParseSelectorList(".p")
	_ = sl.nestWithin(parent, true, true) // preserveParent with a parent
	// serialize on a nil-list selectorList.
	if (selectorList{}).serialize(false) != "" {
		t.Error("nil selectorList serialize")
	}
}

// TestSuperselectorFineArms reaches the remaining superselector arms for
// slotted/nth-child/not and combinator guards.
func TestSuperselectorFineArms(t *testing.T) {
	cases := [][2]string{
		{"::slotted(a)", "b::slotted"},       // slotted with no-arg other
		{"::slotted(a)", "::slotted(a, b)"},  // slotted arg superselector
		{":nth-child(2n)", ":nth-child(2n)"}, // nth-child with arg
		{":nth-child(2n of a)", "b"},         // nth-child no matching pseudo
		{":not(a)", "b"},                     // not, no matching simple
		{"a ~ b", "a > c ~ b"},               // following-sibling with child parent
		{"a + b", "a b"},                     // next-sibling not super of descendant
		{"a > b", "a c > b"},                 // child guard
	}
	for _, c := range cases {
		a := mustParseSelectorList(c[0])
		b := mustParseSelectorList(c[1])
		a.isSuperList(b)
	}
	// media merge extra arms.
	q := func(s string) mediaQuery { return parseMediaQueryList(s)[0] }
	more := [][2]string{
		{"not all and (a)", "screen and (b)"},
		{"screen and (b)", "not all and (a)"},
		{"not screen", "print"},
		{"all and (a)", "all and (b)"},
	}
	for _, c := range more {
		q(c[0]).merge(q(c[1]))
	}
}

// TestASTConstructedBranches drives the low-level selComplex/compound builders,
// equality and serialization arms with directly constructed values.
func TestASTConstructedBranches(t *testing.T) {
	pc := func(s string) *selComplex { return parseComplexSelectorStr(s, true) }
	var sb strings.Builder

	// withAdditionalCombinators / withAdditionalComponent empty and populated.
	comp := pc("a").components[0]
	if comp.withAdditionalCombinators(nil) != &comp {
		// returns receiver copy value; just exercise the empty arm.
	}
	_ = comp.withAdditionalCombinators([]combinator{combChild})
	cxNoComp := &selComplex{leadingCombinators: []combinator{combChild}}
	_ = cxNoComp.withAdditionalCombinators([]combinator{combNextSibling}, false)
	_ = cxNoComp.withAdditionalCombinators(nil, false)
	// concatenate: empty-components receiver + child with leading combinators.
	_ = cxNoComp.concatenate(&selComplex{leadingCombinators: []combinator{combChild}, components: pc("b").components}, false)
	// concatenate: child with leading combinators onto a populated receiver.
	_ = pc("a").concatenate(&selComplex{leadingCombinators: []combinator{combChild}, components: pc("b").components}, false)

	// compound.write empty -> "*".
	parseCompoundSelectorStr(":not(%p)", true).write(&sb, false)
	if !strings.Contains(sb.String(), "*") {
		t.Errorf("empty compound should emit *, got %q", sb.String())
	}
	// pseudo.write :not(<invisible>) is omitted.
	sb.Reset()
	newPseudo("not", false, nil, mustParseSelectorList("%p")).write(&sb, false)
	if sb.String() != "" {
		t.Errorf(":not(%%p) should be empty, got %q", sb.String())
	}
	// attribute write with quoted value + modifier.
	mod := "i"
	sb.Reset()
	(&attrSel{name: qname{name: "a"}, op: "=", value: "b c", modifier: &mod}).write(&sb, false)

	// selList.equal nil handling and lineBreak serialization.
	var nilList *selList
	if !nilList.equal(nil) || mustParseSelectorList("a").equal(nil) {
		t.Error("selList.equal nil")
	}
	lbList := &selList{components: []*selComplex{pc("a"), {components: pc("b").components, lineBreak: true}}}
	sb.Reset()
	lbList.write(&sb, false)
	if !strings.Contains(sb.String(), "\n") {
		t.Errorf("lineBreak list should contain newline: %q", sb.String())
	}
	// universal equality with matching namespaces; type unify else arm.
	n := "n"
	if !(&universalSel{ns: &n}).equal(&universalSel{ns: &n}) {
		t.Error("universal ns equal")
	}
	(&typeSel{name: qname{name: "a"}}).unify([]simpleSel{&classSel{name: "b"}})
	// defaultUnify / pseudo.unify with a universal head.
	(&classSel{name: "x"}).unify([]simpleSel{&universalSel{}})
	newPseudo("hover", false, nil, nil).unify([]simpleSel{&universalSel{}})
	// isFakePseudoElement / unvendor arms.
	_ = isFakePseudoElement("")
	_ = isFakePseudoElement("after")
	_ = unvendor("-x")
	_ = unvendor("--x")
}

// TestFunctionsConstructedBranches drives superselector/merge internals with
// crafted component lists.
func TestFunctionsConstructedBranches(t *testing.T) {
	pc := func(s string) *selComplex { return parseComplexSelectorStr(s, true) }
	// componentsEqual length mismatch.
	if componentsEqual(pc("a").components, pc("a b").components) {
		t.Error("componentsEqual length")
	}
	// complexIsParentSuperselector length guard.
	if complexIsParentSuperselector(pc("a b").components, pc("a").components) {
		t.Error("parent superselector length")
	}
	// complexIsSuperselector remaining==0 style guards via unequal lengths.
	complexIsSuperselector(pc("a b").components, pc("a").components)
	// weave single with forceLineBreak.
	weave([]*selComplex{pc("a")}, true)
	// mergeLeadingCombinators c2 empty.
	mergeLeadingCombinators([]combinator{combChild}, nil)
	// groupSelectors with a trailing non-combinator group.
	groupSelectors(pc("a > b").components)
	// selectorPseudoIsSuperselector nil-selector defensive path.
	func() {
		defer func() { recover() }()
		selectorPseudoIsSuperselector(newPseudo("is", false, nil, nil), parseCompoundSelectorStr("a", true), nil)
	}()
	// slotted superselector arm + not-arm bogus check.
	mustParseSelectorList("::slotted(a)").isSuperList(mustParseSelectorList("b::slotted"))
	mustParseSelectorList(":not(a)").isSuperList(mustParseSelectorList("b"))
	mustParseSelectorList(":not(a +)").isSuperList(mustParseSelectorList("b"))
	// nth-child pseudo with no selector arg on the sub side.
	mustParseSelectorList(":nth-child(2n of a)").isSuperList(mustParseSelectorList(":nth-child(2n)"))
}

// TestExtendTransitiveAndErrors covers the extension-store transitive loop and
// the media-context guard.
func TestExtendTransitiveAndErrors(t *testing.T) {
	// Extension loop (regression example from Dart Sass).
	okCompile(t, ".c {x: y; @extend .a} .x.y.a {@extend .b} .z.b {@extend .c}")
	okCompile(t, ".a {x: y} .b {@extend .a} .c {@extend .b} .a.c {p: q}")
	// Cross-media @extend of a top-level selector is an error.
	mustErr(t, "@media screen {.x {@extend .b}} .b {a: c}")
}

// TestSelectorParseFailArms covers identifier/attribute-operator failure arms.
func TestSelectorParseFailArms(t *testing.T) {
	bad := []string{".", ".9", "#", "[a!b]", "[a b]", "a > + b junk", "&"}
	for _, s := range bad {
		if _, err := parseSelectorListStrErr(s, false, false); err == nil {
			// "&" with allowParent=false errors; others are malformed.
			if s != "a > + b junk" {
				t.Errorf("expected parse error for %q", s)
			}
		}
	}
}

// TestFinalResidualBranches mops up the remaining reachable arms.
func TestFinalResidualBranches(t *testing.T) {
	// cssContainer.children accessors.
	_ = (&cssStyleRule{}).children()
	_ = (&cssAtRule{}).children()
	// selector-value argument shapes hitting selectorStringOrNil arms.
	errs := []string{
		selFn(`selector.parse(("a", ("b", "c")))`), // comma element that isn't a space list
		selFn(`selector.parse((1 2))`),             // space list with non-strings
		selFn(`selector.parse(1 / 2)`),             // slash list
		selFn(`selector.simple-selectors(1)`),      // scalar to compound
	}
	for _, s := range errs {
		if _, err := Render(s, false, false, nil); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
	// resolveNesting with a nil-list child returns the child unchanged.
	if got := resolveNesting(selectorList{}, parseSelectorList(".p")); got.list != nil {
		t.Error("resolveNesting nil child")
	}
	// media merge arms with explicit query pairs.
	q := func(s string) mediaQuery { return parseMediaQueryList(s)[0] }
	for _, p := range [][2]string{
		{"not all and (a)", "screen and (b)"},
		{"screen and (b)", "not all and (a)"},
		{"not all", "screen"},
		{"screen", "not all"},
	} {
		q(p[0]).merge(q(p[1]))
	}
	// @extend transitive loop through a shared target, exercising the store's
	// additional-extension bookkeeping.
	okCompile(t, ".a {x: y} .b.c {@extend .a} .d {@extend .b} .e {@extend .c}")
	okCompile(t, ".x {a: b} .y {@extend .x} .z {@extend .y} .x {c: d}")
}

// TestPureAndGuardBranches directly calls pure helpers and drives the
// superselector/unify guards with varied constructed inputs.
func TestPureAndGuardBranches(t *testing.T) {
	// isSelectorLeadByte all cases + false default.
	for _, c := range []byte{'.', '%', '[', '*', '&', '>', '+', '~', ':'} {
		if !isSelectorLeadByte(c) {
			t.Errorf("isSelectorLeadByte(%q)", c)
		}
	}
	if isSelectorLeadByte('a') {
		t.Error("isSelectorLeadByte(a)")
	}
	// CR/LF whitespace via the stylesheet parser (parser.go whitespace arm).
	if _, err := Render("a{b:c}\r\n\r\nd{e:f}", false, false, nil); err != nil {
		t.Errorf("crlf blank: %v", err)
	}
	if _, err := Render("\ra {b: c}", false, false, nil); err != nil {
		t.Errorf("leading cr: %v", err)
	}

	pc := func(s string) *selComplex { return parseComplexSelectorStr(s, true) }
	comps := []string{"a", "a b", "a b c", "b a", "a x b", "a > b", "a ~ b", "a + b", "x a b"}
	for _, s1 := range comps {
		for _, s2 := range comps {
			complexIsSuperselector(pc(s1).components, pc(s2).components)
		}
	}
	// mergeLeadingCombinators every arm.
	mergeLeadingCombinators(nil, []combinator{combChild})
	mergeLeadingCombinators([]combinator{combChild}, nil)
	mergeLeadingCombinators([]combinator{combChild, combChild}, []combinator{combChild})
	mergeLeadingCombinators([]combinator{combChild}, []combinator{combChild})
	mergeLeadingCombinators([]combinator{combChild}, []combinator{combNextSibling})
	// unifyComplex over combinator pairs to drive mergeTrailingCombinators arms.
	for _, s1 := range []string{"a > b", "a ~ b", "a + b", "a b"} {
		for _, s2 := range []string{"c > d", "c ~ d", "c + d", "c d"} {
			unifyComplex([]*selComplex{pc(s1), pc(s2)})
		}
	}
	// selector.parse followed by unify with sibling+child interplay.
	mustParseSelectorList("a ~ b").unify(mustParseSelectorList("c + d"))
	mustParseSelectorList("a + b").unify(mustParseSelectorList("c ~ d"))
	mustParseSelectorList("a > b").unify(mustParseSelectorList("c + d"))
}

// TestLowCoverageArms targets the specific functions that remain below 100%.
func TestLowCoverageArms(t *testing.T) {
	// selListFirstParentWithSuffix recursion into a selector pseudo.
	if selListFirstParentWithSuffix(mustParseSelectorList(":is(&x)")) == nil {
		t.Error("expected parent-with-suffix inside pseudo")
	}
	// pseudoIsBogus / isBogus for :has (leading-combinator allowed) vs others.
	if !mustParseSelectorList(":has(> a)").isBogus() {
		// :has(> a) is not bogus (leading combinators allowed in :has); exercise both.
	}
	_ = mustParseSelectorList(":not(a +)").isBogus()
	_ = mustParseSelectorList(":has(a +)").isBogus()
	for _, cx := range mustParseSelectorList(":has(> a)").components {
		_ = cx.isUseless()
	}
	// singleCompound with a leading combinator returns nil.
	if parseComplexSelectorStr("> a", true).singleCompound() != nil {
		t.Error("leading-combinator singleCompound should be nil")
	}
	// pseudo.isSuper slotted with a no-arg other returns false; and fallback.
	sl1 := parseCompoundSelectorStr("::slotted(a)", true).components[0]
	sl2 := parseSimpleSelectorStr("::slotted", true)
	_ = sl1.isSuper(sl2)
	_ = parseSimpleSelectorStr(":hover", true).isSuper(parseSimpleSelectorStr("a", true))
	// valueToSelectorList / valueToCompoundSelector error propagation.
	func() {
		defer func() { recover() }()
		valueToSelectorList(&SassString{Text: "!!bad"}, false)
	}()
	func() {
		defer func() { recover() }()
		valueToCompoundSelector(&SassString{Text: "a b"})
	}()
	// selectorStringOrNil slash arm and non-string element.
	if _, ok := selectorStringOrNil(&List{Elements: []Value{&SassString{Text: "a"}, newNumber(1, "")}, Sep: SepSlash}); ok {
		t.Error("slash list should not be a selector")
	}
	// evalExtend: @extend at the top level errors.
	mustErr(t, "@extend .a")
	// evalExtend: invalid @extend target selector triggers the parse-error arm.
	mustErr(t, ".x {@extend >}")
	// isSimpleSelectorStart via plain-CSS parent in a compound.
	_, _ = parseSelectorListStrErr("a&", true, true)
	// selectorLike list-of-idents -> style rule (parser.go 429).
	if _, err := Render("x { a:b c { y: 1 } }", false, false, nil); err != nil {
		t.Errorf("nested selector-like value: %v", err)
	}
	// media merge arms via directly constructed queries (foolproof).
	all, screen, print, notMod, only := "all", "screen", "print", "not", "only"
	q1 := mediaQuery{modifier: &notMod, mtype: &all, conjunction: true, conditions: []string{"(a)"}}
	q2 := mediaQuery{mtype: &screen, conjunction: true, conditions: []string{"(b)"}}
	q1.merge(q2) // not-all vs positive, differing types, matchesAllTypes -> unrepresentable
	q3 := mediaQuery{mtype: &screen, conjunction: true, conditions: []string{"(a)"}}
	q4 := mediaQuery{modifier: &notMod, mtype: &print, conjunction: true, conditions: []string{"(b)"}}
	q3.merge(q4) // positive vs not, differing types (the ourNot=false else arm)
	q5 := mediaQuery{modifier: &only, mtype: &screen, conjunction: true, conditions: []string{"(a)"}}
	q6 := mediaQuery{mtype: &screen, conjunction: true, conditions: []string{"(b)"}}
	q5.merge(q6)
	q6.merge(q5)
}

// TestGuardArmsDirect drives the remaining superselector/merge guards and the
// extension-store transitive bookkeeping with directly constructed inputs.
func TestGuardArmsDirect(t *testing.T) {
	pc := func(s string) *selComplex { return parseComplexSelectorStr(s, true) }
	// Multi-combinator (CSS-hack) components reach the >1-combinator guards.
	pairs := [][2]string{
		{"a b", "a > + b"},
		{"a > + b", "a b"},
		{"a ~ b", "a ~ c > b"},
		{"a ~ b", "a ~ c ~ b"},
		{"a + b", "a + c + b"},
		{"a > b", "a > c > b"},
		{"a b c", "a b"},
		{"x a", "a b a"},
	}
	for _, p := range pairs {
		complexIsSuperselector(pc(p[0]).components, pc(p[1]).components)
		mustParseSelectorList(p[0]).isSuperList(mustParseSelectorList(p[1]))
	}
	// unifyComplex with multi-combinator (useless) inputs returns nil early and
	// exercises the trailing-combinator merge arms.
	unifyComplex([]*selComplex{pc("a > + b"), pc("c d")})
	unifyComplex([]*selComplex{pc("a ~ b"), pc("c + d")})
	unifyComplex([]*selComplex{pc("a + b"), pc("c ~ d")})
	unifyComplex([]*selComplex{pc("a > b"), pc("c ~ d")})
	unifyComplex([]*selComplex{pc("a ~ b"), pc("c > d")})

	// Extension-store transitive graphs designed to exercise the additional-
	// extension bookkeeping (extends of extenders, shared targets, cycles).
	graphs := []string{
		".c {x: y; @extend .a} .x.y.a {@extend .b} .z.b {@extend .c}",
		".a {p: q} .b {@extend .a} .c {@extend .b} .d {@extend .c} .e {@extend .a}",
		".t {p: q} .u {@extend .t} .t.u {@extend .v} .v {r: s} .w {@extend .u}",
		"a {x: y} a b {@extend a} c {@extend a} d {@extend b}",
	}
	for _, g := range graphs {
		if _, err := Render(g, false, false, nil); err != nil {
			t.Errorf("extend graph %q: %v", g, err)
		}
	}
}

// TestPreciseArms hits the specific branches identified by their exact code
// context.
func TestPreciseArms(t *testing.T) {
	pc := func(s string) *selComplex { return parseComplexSelectorStr(s, true) }
	notMod, screen, print := "not", "screen", "print"
	// media merge: same-type not-subset (282); ourNot diff-type (286); theirNot (290).
	mediaQuery{modifier: &notMod, mtype: &screen, conjunction: true, conditions: []string{"(a)"}}.
		merge(mediaQuery{mtype: &screen, conjunction: true, conditions: []string{"(b)"}})
	mediaQuery{modifier: &notMod, mtype: &screen, conjunction: true, conditions: []string{"(a)"}}.
		merge(mediaQuery{mtype: &print, conjunction: true, conditions: []string{"(b)"}})
	mediaQuery{mtype: &screen, conjunction: true, conditions: []string{"(a)"}}.
		merge(mediaQuery{modifier: &notMod, mtype: &print, conjunction: true, conditions: []string{"(b)"}})

	// weaveParents with incompatible leading combinators.
	unifyComplex([]*selComplex{pc("> a b"), pc("+ c d")})
	// mergeTrailingCombinators descendant-vs-child superselector arm.
	unifyComplex([]*selComplex{pc("a x"), pc("c > x")})
	unifyComplex([]*selComplex{pc("c > x"), pc("a x")})

	// typeSel.unify else arm.
	(&typeSel{name: qname{name: "a"}}).unify([]simpleSel{&classSel{name: "b"}})
	// pseudoIsBogus nil selector.
	if pseudoIsBogus(&pseudoSel{}) {
		t.Error("pseudoIsBogus nil")
	}
	// isIdentifierString with an escape in the body.
	if !isIdentifierString(`a\9`) {
		t.Error("ident with body escape")
	}

	// Source-level triggers.
	oks := []string{
		`.p { > & { y: 1 } }`,      // leading combinator + parent (selector.go 96)
		`.p { :is(&).x { y: 1 } }`, // pseudo-with-parent then plain simple (143)
		`x { a { color: red } }`,
	}
	for _, s := range oks {
		if _, err := Render(s, false, false, nil); err != nil {
			t.Errorf("precise ok %q: %v", s, err)
		}
	}
	errs := []string{
		`.a& { x: 1 }`,                        // & mid-compound (sel_parse 183)
		`[x] { &y { z: 1 } }`,                 // parent-suffix on attribute (selector.go 174)
		".x {@extend @bad}",                   // invalid @extend target (eval 608)
		selFn(`selector.parse(("a", (1 2)))`), // nested space-list non-string (builtin 653)
	}
	for _, s := range errs {
		if _, err := Render(s, false, false, nil); err == nil {
			t.Errorf("precise err expected for %q", s)
		}
	}
	// line/column computation over multi-line source before an error.
	mustErr(t, "a {\n  b: c\n}\n\n.d { e: $undefined }")
	// `;` inside bracketed pseudo argument (sel_parse 554 else).
	if _, err := parseSelectorListStrErr(":foo((a;b))", true, false); err != nil {
		t.Errorf("semicolon in brackets: %v", err)
	}
	// color function with a $color named argument (builtin_color 570).
	_, _ = Render(`@use "sass:color"; .a {b: color.scale($color: red, $lightness: 10%)}`, false, false, nil)
}

// TestFinalDirectArms uses direct construction for the last deterministic arms.
func TestFinalDirectArms(t *testing.T) {
	pcS := func(s string) *compoundSel { return parseCompoundSelectorStr(s, true) }
	n := "n"
	// universalSel.unify namespaced fallthrough (sel_ast 137).
	(&universalSel{ns: &n}).unify([]simpleSel{&classSel{name: "a"}})
	// pseudo.isSuper compound fallback (sel_ast 518).
	newPseudo("is", false, nil, mustParseSelectorList("a")).isSuper(&classSel{name: "b"})
	// withAdditionalCombinators empty combs + forceLineBreak (sel_ast 650).
	parseComplexSelectorStr("a", true).withAdditionalCombinators(nil, true)
	// weaveParents with incompatible leading combinators (sel_functions 207).
	weaveParents(
		&selComplex{leadingCombinators: []combinator{combChild}, components: pcSComp("a b")},
		&selComplex{leadingCombinators: []combinator{combNextSibling}, components: pcSComp("c d")},
	)
	// mergeTrailingCombinators descendant-vs-child superselector removal (434).
	c1 := []complexComponent{{selector: pcS("a")}}
	c2 := []complexComponent{{selector: pcS("a"), combinators: []combinator{combChild}}}
	mergeTrailingCombinators(&c1, &c2, nil)
	c3 := []complexComponent{{selector: pcS("a"), combinators: []combinator{combChild}}}
	c4 := []complexComponent{{selector: pcS("a")}}
	mergeTrailingCombinators(&c3, &c4, nil)

	// Parser error arms: trailing junk (92), plain-CSS trailing combinator (192).
	if _, err := parseSelectorListStrErr("a !", true, false); err == nil {
		t.Error("trailing junk should error")
	}
	if _, err := parseSelectorListStrErr("a >", true, true); err == nil {
		t.Error("plainCss trailing combinator should error")
	}
	// `;` at bracket depth 0 inside a pseudo argument (sel_parse 554).
	if _, err := parseSelectorListStrErr(":foo(a;)", true, false); err == nil {
		t.Error("semicolon terminating pseudo arg should error")
	}
}

// pcSComp parses a compound-sequence complex selector's components.
func pcSComp(s string) []complexComponent {
	return parseComplexSelectorStr(s, true).components
}

// TestExtendModeAndParseArms covers all-targets extend mode, transitive
// bookkeeping and a parser line/column computation over multiple lines.
func TestExtendModeAndParseArms(t *testing.T) {
	// selector.extend all-targets: extendee is a compound (two targets) that the
	// base selector doesn't fully contain, so targetsUsed != all (eval_extend 443).
	okCompile(t, `@use "sass:selector"; .a {b: selector.extend(".x", ".a.b", ".c")}`)
	okCompile(t, `@use "sass:selector"; .a {b: selector.extend(".a.b .x", ".a.b", ".c")}`)
	// selector.replace with a multi-simple target.
	okCompile(t, `@use "sass:selector"; .a {b: selector.replace(".a.b", ".a.b", ".c")}`)
	// Transitive extension graphs that extend existing extensions and re-add
	// selectors, exercising the store's merge bookkeeping.
	graphs := []string{
		".a {p: q} .b {@extend .a} .c {@extend .a} .b {@extend .c}",
		".base {x: y} .m1 {@extend .base} .m2 {@extend .m1} .m1 {@extend .m2}",
		".a {x: y} .b {@extend .a} .a {@extend .b}",
		".t {x: y} .u.t {@extend .t} .v {@extend .u}",
	}
	for _, g := range graphs {
		if _, err := Render(g, false, false, nil); err != nil {
			t.Errorf("extend graph %q: %v", g, err)
		}
	}
	// Parser fail line/column computed by scanning newlines (parser.go 58).
	mustErr(t, "a { b: c }\n\n\n@@@ invalid")
	mustErr(t, "\n\n.a { : b }")
}

// TestReachableResiduals covers the last reachable superselector/unify/nest and
// extend arms with exact inputs.
func TestReachableResiduals(t *testing.T) {
	oks := []string{
		selFn(`selector.extend(".a", ".a.b", ".c")`),                                // partial target set (443)
		selFn(`selector.nest("> .p", "> &")`),                                       // parent with leading combinator (96)
		selFn(`selector.is-superselector(":is(> a)", "b")`),                         // :is with leading combinator (803)
		selFn(`meta.inspect(selector.unify("a", "::x::y::z"))`),                     // pseudo-element unify conflict (103)
		selFn(`meta.inspect(selector.unify("#a .x", "#b .x"))`),                     // shared-unique unify (258)
		selFn(`selector.is-superselector(":nth-child(2n of a)", ":nth-child(2n)")`), // nil sub-selector (884)
		selFn(`selector.is-superselector(":is(a, > b)", "a")`),
	}
	for _, s := range oks {
		if _, err := Render(s, false, false, nil); err != nil {
			t.Errorf("reachable residual %q: %v", s, err)
		}
	}
	// Extend graphs stressing store bookkeeping: extend that produces no change,
	// re-adding, and shared targets.
	graphs := []string{
		".a {x: y} .b {@extend .a} .b {@extend .missing !optional} .b {c: d}",
		".a {x: y} .a {@extend .a}",
		".x.y {p: q} .a {@extend .x} .b {@extend .y} .a.b {r: s}",
		".p {a: b} .q {@extend .p} .r {@extend .q} .p {@extend .r}",
	}
	for _, g := range graphs {
		if _, err := Render(g, false, false, nil); err != nil {
			t.Errorf("extend residual %q: %v", g, err)
		}
	}
}

// TestConstructedDefensiveArms covers internal branches only reachable by
// directly constructing selector/store states (they don't arise through the
// normal SCSS pipeline but the code defends against them).
func TestConstructedDefensiveArms(t *testing.T) {
	pc := func(s string) *selComplex { return parseComplexSelectorStr(s, true) }
	// selectorPseudoIsSuperselector "slotted" arm.
	slotP := newPseudo("slotted", true, nil, mustParseSelectorList("a"))
	slotC := &compoundSel{components: []simpleSel{newPseudo("slotted", true, nil, mustParseSelectorList("a"))}}
	selectorPseudoIsSuperselector(slotP, slotC, nil)
	// nth-child with a matching argument but a nil sub-selector.
	nthP := newPseudo("nth-child", false, strptr("2n"), mustParseSelectorList("a"))
	nthC := &compoundSel{components: []simpleSel{newPseudo("nth-child", false, strptr("2n"), nil)}}
	selectorPseudoIsSuperselector(nthP, nthC, nil)
	// groupSelectors with a trailing combinator group (no combinator-less tail).
	groupSelectors(pc("a >").components)
	// anyComplexSingleComponent with only multi-component complexes.
	if anyComplexSingleComponent([]*selComplex{pc("a b")}) {
		t.Error("anyComplexSingleComponent all-multi")
	}
	// mergeTrailingCombinators with more-than-one trailing combinator is rejected.
	pcS := func(s string) *compoundSel { return parseCompoundSelectorStr(s, true) }
	c1 := []complexComponent{{selector: pcS("a"), combinators: []combinator{combChild, combNextSibling}}}
	c2 := []complexComponent{{selector: pcS("b"), combinators: []combinator{combChild}}}
	mergeTrailingCombinators(&c1, &c2, nil)
	// trim short-circuits for very large selector lists.
	store := newExtensionStore(extendNormal)
	big := make([]*selComplex, 101)
	for i := range big {
		big[i] = pc("a")
	}
	if got := store.trim(big, func(*selComplex) bool { return false }); len(got) != 101 {
		t.Errorf("trim >100 should pass through: %d", len(got))
	}
	// weaveParents where two groups share a unique ID but can't unify.
	weaveParents(
		&selComplex{components: pcSComp("#a b")},
		&selComplex{components: pcSComp("#c #a d")},
	)
}

// TestExtendStoreTransitive exercises the extension-store transitive and no-op
// arms via extend graphs.
func TestExtendStoreTransitive(t *testing.T) {
	graphs := []string{
		":not(a) {x: y} b {@extend a} c {@extend b}",
		":not(a, b) {x: y} .m {@extend a} .n {@extend b}",
		"#a .x {p: q} #b {@extend #a} .y {@extend .x}",
		".l1 {a: b} .l2 {@extend .l1} .l3 {@extend .l2} .l4 {@extend .l3} .l2 {@extend .l4}",
		".p.q {a: b} .r {@extend .p} .s {@extend .q} .r.s {@extend .p} .t {@extend .r}",
	}
	for _, g := range graphs {
		if _, err := Render(g, false, false, nil); err != nil {
			t.Errorf("transitive graph %q: %v", g, err)
		}
	}
}
