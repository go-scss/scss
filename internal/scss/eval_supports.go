// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

// serializeSupportsCond renders a parsed @supports condition to its canonical
// CSS text, mirroring dart-sass's _visitSupportsCondition. Declaration values
// are evaluated (so SassScript such as `1 + 1` resolves), redundant parentheses
// are dropped, and comments/whitespace are normalized by the parser.
func (e *evaluator) serializeSupportsCond(c SupportsCond) string {
	switch n := c.(type) {
	case *SupportsOperation:
		return e.parenthesizeSupports(n.Left, n.Op) + " " + n.Op + " " + e.parenthesizeSupports(n.Right, n.Op)
	case *SupportsNegation:
		return "not " + e.parenthesizeSupports(n.Cond, "")
	case *SupportsInterp:
		return e.stringifyInterp(e.evalExpr(n.Expr))
	case *SupportsDecl:
		old := e.inSupportsDecl
		e.inSupportsDecl = true
		defer func() { e.inSupportsDecl = old }()
		name := e.stringify(e.evalExpr(n.Name))
		if n.Custom {
			return "(" + name + ":" + e.resolveInterp(n.RawValue) + ")"
		}
		return "(" + name + ": " + e.stringify(e.evalExpr(n.Value)) + ")"
	case *SupportsFunc:
		return e.resolveInterp(n.Name) + "(" + e.resolveInterp(n.Args) + ")"
	default: // *SupportsAnything
		return "(" + e.resolveInterp(c.(*SupportsAnything).Contents) + ")"
	}
}

// parenthesizeSupports mirrors dart-sass _parenthesize: an operand is wrapped in
// parentheses when it is a negation, or an operation whose operator differs from
// the surrounding one (or when there is no surrounding operator).
func (e *evaluator) parenthesizeSupports(c SupportsCond, operator string) string {
	switch n := c.(type) {
	case *SupportsNegation:
		return "(" + e.serializeSupportsCond(n) + ")"
	case *SupportsOperation:
		if operator == "" || operator != n.Op {
			return "(" + e.serializeSupportsCond(n) + ")"
		}
	}
	return e.serializeSupportsCond(c)
}
