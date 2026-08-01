// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestImportBodyBlankLines locks down that a legacy @import inlined into a style
// rule contributes rule-body content: the nested rules it produces are NOT
// blank-separated (dart never blanks rule-body siblings), whereas an @import at
// the stylesheet top level still yields blank-separated top-level rules.
// Expectations are byte-exact against dart-sass 1.102.
func TestImportBodyBlankLines(t *testing.T) {
	// Inlined into a rule: no blank between the two resulting nested rules, even
	// when the second comes from a control directive in the imported file.
	nested := renderWith(t, ".wrap { @import \"m\"; }\n", map[string]string{
		"_m.scss": ".okay { background: green; }\n@if true { .broken { background: red; } }\n",
	})
	wantNested := ".wrap .okay {\n  background: green;\n}\n.wrap .broken {\n  background: red;\n}\n"
	if nested != wantNested {
		t.Errorf("nested @import:\n want: %q\n  got: %q", wantNested, nested)
	}

	// At the top level the imported rules stay blank-separated top-level siblings.
	top := renderWith(t, "@import \"two\";\n.z { z: 3; }\n", map[string]string{
		"_two.scss": "a { x: 1; }\nb { y: 2; }\n",
	})
	wantTop := "a {\n  x: 1;\n}\n\nb {\n  y: 2;\n}\n\n.z {\n  z: 3;\n}\n"
	if top != wantTop {
		t.Errorf("top-level @import:\n want: %q\n  got: %q", wantTop, top)
	}
}
