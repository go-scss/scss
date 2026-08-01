// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestIndentedLoudCommentNormalization pins the indented-syntax loud-comment
// re-serialization (compile.go dartIndentedLoudComment + serialize.go
// emitComment): a free-form `/* ... */` comment written in the indented syntax
// is rebuilt with ` * ` continuation prefixes and re-indented exactly as
// dart-sass 1.102 does. Every expectation below is byte-for-byte what
// `sass --indented` emits.
func TestIndentedLoudCommentNormalization(t *testing.T) {
	cases := []struct{ src, want string }{
		// Empty first line: dropped, comment collapses onto the next line.
		{"/* \n  a */\n", "/* a */\n"},
		// Content after the close line keeps a ` * ` continuation.
		{"/* \n  a \n  */\n", "/* a \n * */\n"},
		// Unterminated comment: dart-sass appends the closing ` */`.
		{"/* \n  a\n", "/* a */\n"},
		{"/* a\n", "/* a */\n"},
		// A second comment after the close on the same line is dropped.
		{"/* */ /* */\n", "/* */\n"},
		// Single-line comment is emitted verbatim.
		{"/* hello */\n", "/* hello */\n"},
		// Blank lines inside the comment are preserved as bare ` *` lines.
		{
			"/* Preserves\n\n  empty\n\n\n  lines\n",
			"/* Preserves\n *\n * empty\n *\n *\n * lines */\n",
		},
		// Indentation deeper than the ` * ` prefix is preserved.
		{
			"/* Even\n      when\n     it starts\n",
			"/* Even\n *    when\n *   it starts */\n",
		},
		// Interpolation is evaluated within the comment body.
		{"/* x: #{1 + 1}\n  y */\n", "/* x: 2\n * y */\n"},
		// A comment nested inside a rule re-indents to the rule's depth.
		{"a\n  /* nested\n    deep */\n", "a {\n  /* nested\n  * deep */\n}\n"},
		// An unterminated comment ends when a following line dedents to its level.
		{"/* c\n  more\na\n  b: 1\n", "/* c\n * more */\na {\n  b: 1;\n}\n"},
	}
	for _, c := range cases {
		res, err := Render(c.src, true, false, nil)
		if err != nil {
			t.Fatalf("compile error for %q: %v", c.src, err)
		}
		if res.CSS != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, res.CSS)
		}
	}
}

// TestScssLoudCommentSerialization pins the SCSS-syntax loud-comment behaviour:
// CR/FF newline conversion inside the comment body, multi-line re-indentation to
// the comment's column, and sourceMappingURL/sourceURL dropping. Expectations
// match dart-sass 1.102 (modulo trailing blank lines, which the CSS comparison
// normalizes away).
func TestScssLoudCommentSerialization(t *testing.T) {
	cases := []struct{ src, want string }{
		// A bare CR inside the comment becomes an LF.
		{"/* foo\r * bar */\n", "/* foo\n * bar */\n"},
		// A CR LF pair collapses to a single LF.
		{"/* foo\r\n * bar */\n", "/* foo\n * bar */\n"},
		// A form feed becomes an LF.
		{"/* foo\f * bar */\n", "/* foo\n * bar */\n"},
		// A multi-line comment re-indents to its nesting depth.
		{
			".foo {\n    /* Foo\n Bar\nBaz */\n  a: b; }\n",
			".foo {\n  /* Foo\n   Bar\n  Baz */\n  a: b;\n}\n",
		},
		// sourceMappingURL / sourceURL comments are omitted from output (the blank
		// line the dropped comment leaves behind is trimmed by CSS comparison).
		{"a { b: c }\n/*# sourceMappingURL=whatever */\n", "a {\n  b: c;\n}\n\n"},
		{"a { b: c }\n/*# sourceURL=x */\n", "a {\n  b: c;\n}\n\n"},
		// A plain single-line comment with no special characters is unchanged.
		{"/* plain */\n", "/* plain */\n"},
	}
	for _, c := range cases {
		res, err := Render(c.src, false, false, nil)
		if err != nil {
			t.Fatalf("compile error for %q: %v", c.src, err)
		}
		if res.CSS != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, res.CSS)
		}
	}
}

// TestConvertCommentNewlines exercises every branch of the SCSS loud-comment
// newline normaliser directly.
func TestConvertCommentNewlines(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/* a */", "/* a */"},       // no CR/FF: returned unchanged
		{"a\rb", "a\nb"},             // bare CR -> LF
		{"a\r\nb", "a\nb"},           // CR LF -> LF (CR dropped)
		{"a\fb", "a\nb"},             // FF -> LF
		{"a\r", "a\n"},               // trailing CR -> LF
		{"a\r\n", "a\n"},             // trailing CR LF -> LF
		{"plain text", "plain text"}, // fast path, no conversion
	}
	for _, c := range cases {
		if got := convertCommentNewlines(c.in); got != c.want {
			t.Errorf("convertCommentNewlines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCommentMinIndentation covers every branch of the loud-comment minimum
// indentation scanner, including single-line (verbatim), interior blank lines,
// trailing whitespace-only lines, and the all-blank-after-first (-1) case.
func TestCommentMinIndentation(t *testing.T) {
	cases := []struct {
		in     string
		wantN  int
		wantOk bool
	}{
		{"/* a */", 0, false},            // no newline -> single line
		{"/* a\n  b */", 2, true},        // one continuation line at column 2
		{"/* a\n\n  b */", 2, true},      // interior blank line is skipped
		{"/* a\n    b\n  c */", 2, true}, // minimum of two continuations
		{"/* a\n  ", -1, true},           // trailing whitespace-only line
		{"/* a\n", -1, true},             // newline is the final character
		{"/* a\n\n", -1, true},           // only blank lines after the first
	}
	for _, c := range cases {
		gotN, gotOk := commentMinIndentation(c.in)
		if gotN != c.wantN || gotOk != c.wantOk {
			t.Errorf("commentMinIndentation(%q) = (%d, %v), want (%d, %v)",
				c.in, gotN, gotOk, c.wantN, c.wantOk)
		}
	}
}

// TestWriteCommentReindented exercises the re-indentation writer directly,
// including the trailing whitespace-to-space collapse and the defensive
// over-strip guard that the Render-level tests cannot reach.
func TestWriteCommentReindented(t *testing.T) {
	cases := []struct {
		text      string
		minIndent int
		depth     int
		want      string
	}{
		// Simple continuation re-indented to depth 1, minIndent 2.
		{"/* a\n  b */", 2, 1, "/* a\n  b */"},
		// minIndent 0 leaves indentation untouched, adds depth prefix.
		{"/* a\n b */", 0, 1, "/* a\n   b */"},
		// A trailing whitespace-only continuation collapses to a single space.
		{"/* a\n  ", 2, 0, "/* a "},
		// Over-large minIndent is clamped so no content is lost.
		{"/* a\nb */", 5, 0, "/* a\nb */"},
		// A blank line is preserved; the following line is stripped of minIndent
		// and, at depth 0, gains no replacement indentation.
		{"/* a\n\n  b */", 2, 0, "/* a\n\nb */"},
	}
	for _, c := range cases {
		s := &serializer{}
		s.writeCommentReindented(c.text, c.minIndent, c.depth)
		if got := s.sb.String(); got != c.want {
			t.Errorf("writeCommentReindented(%q, %d, %d) = %q, want %q",
				c.text, c.minIndent, c.depth, got, c.want)
		}
	}
}

// TestEmitCommentBranches covers emitComment's verbatim, sourcemap-drop and
// compressed paths directly.
func TestEmitCommentBranches(t *testing.T) {
	// Single-line comment: verbatim, newline-terminated in expanded output.
	s := &serializer{}
	s.emitComment(&cssComment{text: "/* x */"}, 0, false)
	if got := s.sb.String(); got != "/* x */\n" {
		t.Errorf("single-line: got %q", got)
	}

	// A trailing comment skips the leading indentation (the caller wrote the
	// separating space) but is otherwise identical.
	s = &serializer{}
	s.emitComment(&cssComment{text: "/* x */"}, 3, true)
	if got := s.sb.String(); got != "/* x */\n" {
		t.Errorf("trailing (no indent): got %q", got)
	}

	// sourceMappingURL comment is dropped entirely.
	s = &serializer{}
	s.emitComment(&cssComment{text: "/*# sourceMappingURL=z */"}, 0, false)
	if got := s.sb.String(); got != "" {
		t.Errorf("sourcemap drop: got %q", got)
	}

	// Compressed output emits a multi-line comment verbatim (no re-indent, no
	// trailing newline).
	s = &serializer{compressed: true}
	s.emitComment(&cssComment{text: "/* a\n  b */", col: 0}, 0, false)
	if got := s.sb.String(); got != "/* a\n  b */" {
		t.Errorf("compressed multi-line: got %q", got)
	}

	// A comment whose only continuation lines are blank (min == -1) is emitted
	// verbatim rather than re-indented.
	s = &serializer{}
	s.emitComment(&cssComment{text: "/* a\n\n*/", col: 0}, 0, false)
	if got := s.sb.String(); got != "/* a\n\n*/\n" {
		t.Errorf("all-blank continuation: got %q", got)
	}
}

// TestDartIndentedLoudCommentEdges covers dartIndentedLoudComment branches not
// reached through Render: a dedent that ends the comment, a comment consisting
// only of whitespace after `/*`, and interpolation with nested braces.
func TestDartIndentedLoudCommentEdges(t *testing.T) {
	cases := []struct {
		content      string
		parentIndent int
		want         string
	}{
		// A trailing newline whose next "line" is end-of-input dedents to 0 and
		// ends the comment, which is then auto-closed.
		{"/* a\n", 0, "/* a */"},
		// Only whitespace after `/*` with no newline: spaces are preserved and
		// the comment is auto-closed.
		{"/*  ", 0, "/*   */"},
		// Interpolation with nested braces is copied verbatim for re-parsing
		// (the brace-depth counter balances the inner `{ }`).
		{"/* #{ {x} } */", 0, "/* #{ {x} } */"},
		// A blank line whose following lines are all end-of-input dedents to the
		// parent level and ends the comment, which is then auto-closed.
		{"/* a\n\n", 0, "/* a */"},
	}
	for _, c := range cases {
		if got := dartIndentedLoudComment(c.content, c.parentIndent); got != c.want {
			t.Errorf("dartIndentedLoudComment(%q, %d) = %q, want %q",
				c.content, c.parentIndent, got, c.want)
		}
	}
}
