// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// Result holds a compilation result.
type Result struct {
	CSS        string
	LoadedURLs []string
	Warnings   []string
}

// Render compiles Sass/SCSS source to CSS.
func Render(source string, indented bool, compressed bool, importer Importer) (res Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = rethrowIfNotSass(r)
		}
	}()
	if indented {
		source = convertIndented(source)
	}
	stmts, perr := parseStylesheet(source)
	if perr != nil {
		return Result{}, perr
	}
	e := newEvaluator(importer)
	e.run(stmts)
	css := serialize(e.root, compressed)
	return Result{CSS: css, LoadedURLs: e.loadedURLs, Warnings: e.warnings}, nil
}

// convertIndented converts indented (.sass) syntax to bracketed SCSS.
func convertIndented(src string) string {
	lines := strings.Split(src, "\n")
	type entry struct{ indent int }
	var stack []entry
	var out []string
	indentOf := func(s string) int {
		n := 0
		for _, c := range s {
			if c == ' ' {
				n++
			} else if c == '\t' {
				n += 8
			} else {
				break
			}
		}
		return n
	}
	isBlank := func(s string) bool { return strings.TrimSpace(s) == "" }

	nextContentIndent := func(from int) int {
		for j := from; j < len(lines); j++ {
			if !isBlank(lines[j]) && !strings.HasPrefix(strings.TrimSpace(lines[j]), "//") {
				return indentOf(lines[j])
			}
		}
		return -1
	}
	closeTo := func(indent int) {
		for len(stack) > 0 && indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
			out = append(out, strings.Repeat("  ", len(stack))+"}")
		}
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if isBlank(line) {
			continue
		}
		content := strings.TrimSpace(line)
		indent := indentOf(line)
		if strings.HasPrefix(content, "//") {
			closeTo(indent)
			out = append(out, strings.Repeat("  ", len(stack))+content)
			continue
		}
		closeTo(indent)
		ni := nextContentIndent(i + 1)
		pad := strings.Repeat("  ", len(stack))
		if ni > indent {
			out = append(out, pad+content+" {")
			stack = append(stack, entry{indent: indent})
		} else {
			out = append(out, pad+content+";")
		}
	}
	for len(stack) > 0 {
		stack = stack[:len(stack)-1]
		out = append(out, strings.Repeat("  ", len(stack))+"}")
	}
	return strings.Join(out, "\n")
}
