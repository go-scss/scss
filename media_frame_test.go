// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// These cases pin the @media/@supports/@at-root frame model to dart-sass's
// behaviour: query merging while bubbling, the parent-media split that keeps
// residual content in source order (sass/dart-sass#777), @supports staying
// inside its enclosing @media, media context surviving a @supports boundary for
// merge purposes, and the default @at-root (`without: rule`) preserving media.
func TestMediaFrameModel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "parent media splits around a bubbled child",
			in: "@media (a: b) {\n" +
				"  x { p: q }\n" +
				"  @media (c: d) { e { f: g } }\n" +
				"  h { i: j }\n" +
				"}\n",
			want: "@media (a: b) {\n  x {\n    p: q;\n  }\n}\n" +
				"@media (a: b) and (c: d) {\n  e {\n    f: g;\n  }\n}\n" +
				"@media (a: b) {\n  h {\n    i: j;\n  }\n}\n",
		},
		{
			name: "bubbled child is first, residual follows",
			in: "@media (a: b) {\n" +
				"  @media (c: d) { e { f: g } }\n" +
				"  h { i: j }\n" +
				"}\n",
			want: "@media (a: b) and (c: d) {\n  e {\n    f: g;\n  }\n}\n" +
				"@media (a: b) {\n  h {\n    i: j;\n  }\n}\n",
		},
		{
			name: "supports stays inside enclosing media",
			in:   "@media screen {\n  @supports (a: b) { c { d: e } }\n}\n",
			want: "@media screen {\n  @supports (a: b) {\n    c {\n      d: e;\n    }\n  }\n}\n",
		},
		{
			name: "media context survives a supports boundary for merging",
			in: "@media (a: b) {\n" +
				"  @supports (x: y) {\n" +
				"    @media (c: d) { e { f: g } }\n" +
				"  }\n" +
				"}\n",
			want: "@media (a: b) {\n  @supports (x: y) {\n" +
				"    @media (a: b) and (c: d) {\n      e {\n        f: g;\n      }\n    }\n  }\n}\n",
		},
		{
			name: "at-root keeps media, drops style rule",
			in:   "@media screen {\n  .foo {\n    @at-root .bar { a: b }\n  }\n}\n",
			want: "@media screen {\n  .bar {\n    a: b;\n  }\n}\n",
		},
		{
			name: "at-root in bubbled media keeps media",
			in:   ".foo {\n  @media screen {\n    @at-root .bar { a: b }\n  }\n}\n",
			want: "@media screen {\n  .bar {\n    a: b;\n  }\n}\n",
		},
		{
			name: "blank precedes a bubbled media only after a style rule",
			in: "p { x: y }\n" +
				"@media (a: b) {\n  @media (c: d) { z { w: v } }\n}\n",
			want: "p {\n  x: y;\n}\n\n" +
				"@media (a: b) and (c: d) {\n  z {\n    w: v;\n  }\n}\n",
		},
		{
			name: "no blank between adjacent bubbled media after a media sibling",
			in: "@media (a: b) {\n" +
				"  x { p: q }\n" +
				"  @media (c: d) { e { f: g } }\n" +
				"}\n" +
				"@media (m: n) { y { o: p } }\n",
			want: "@media (a: b) {\n  x {\n    p: q;\n  }\n}\n" +
				"@media (a: b) and (c: d) {\n  e {\n    f: g;\n  }\n}\n" +
				"@media (m: n) {\n  y {\n    o: p;\n  }\n}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := CompileString(tc.in, &Options{Style: Expanded})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if res.CSS != tc.want {
				t.Errorf("mismatch\n--- got ---\n%s\n--- want ---\n%s", res.CSS, tc.want)
			}
		})
	}
}
