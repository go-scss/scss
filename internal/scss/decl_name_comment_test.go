// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestDeclarationNameLoudComment covers a loud comment attached to a property
// name (sass-spec libsass-closed-issues/issue_1422). dart-sass keeps a comment
// glued directly to the name identifier (`foo/*c*/:` -> `foo/*c*/`) as part of
// the property name, but folds a comment separated from the name by whitespace
// (`foo /*c*/:` -> `foo`) away like ordinary whitespace. Comments in the value
// are always dropped. Each expected output is byte-verified against dart-sass
// 1.102.
func TestDeclarationNameLoudComment(t *testing.T) {
	cases := []struct{ in, want string }{
		// The issue_1422 fixture: a glued name comment survives; a spaced one does
		// not; value comments are dropped either way.
		{".foo {\n  /*foo*/foo/*foo*/: /*foo*/bar/*foo*/;\n" +
			"  /*foo*/ foo /*foo*/ : /*foo*/ bar /*foo*/;\n}\n",
			".foo {\n  /*foo*/\n  foo/*foo*/: bar;\n  /*foo*/\n  foo: bar;\n}\n"},
		// A comment glued right after the name is kept.
		{".foo { bar/*c*/: 1; }", ".foo {\n  bar/*c*/: 1;\n}\n"},
		// Whitespace before the comment folds it away (the not-glued branch).
		{".foo { bar /*c*/: 1; }", ".foo {\n  bar: 1;\n}\n"},
		// A trailing space after a glued comment does not detach it.
		{".foo { bar/*c*/ : 1; }", ".foo {\n  bar/*c*/: 1;\n}\n"},
	}
	for _, c := range cases {
		res, err := Render(c.in, false, false, nil)
		if err != nil {
			t.Errorf("declaration %q: unexpected error: %v", c.in, err)
			continue
		}
		if res.CSS != c.want {
			t.Errorf("declaration %q:\n got %q\nwant %q", c.in, res.CSS, c.want)
		}
	}
}
