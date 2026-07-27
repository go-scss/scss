// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

// Package scss is a pure-Go (CGO-free) Sass/SCSS compiler whose output tracks
// Dart Sass, the canonical reference implementation. It supports both the SCSS
// and the indented (.sass) syntaxes and the "expanded" and "compressed" output
// styles.
//
// The public surface intentionally mirrors the modern sass-embedded Dart Sass
// API shape (CompileString / Compile returning CSS + loaded URLs) so language
// bindings — such as the go-ruby-sass adapter — can wrap it directly.
package scss

import (
	"os"
	"path/filepath"
	"strings"

	engine "github.com/go-scss/scss/internal/scss"
)

// Syntax selects the input grammar.
type Syntax int

const (
	// SyntaxSCSS is the SCSS (braces/semicolons) grammar.
	SyntaxSCSS Syntax = iota
	// SyntaxIndented is the indented (.sass) grammar.
	SyntaxIndented
	// SyntaxCSS is plain CSS (parsed as SCSS without Sass features enabled).
	SyntaxCSS
)

// OutputStyle selects the serialization style.
type OutputStyle int

const (
	// Expanded is the human-readable multi-line style.
	Expanded OutputStyle = iota
	// Compressed is the minified style.
	Compressed
)

// Importer resolves an import URL to source text and a canonical URL.
type Importer func(url string) (source string, canonicalURL string, ok bool)

// Options configures a compilation.
type Options struct {
	Syntax    Syntax
	Style     OutputStyle
	LoadPaths []string
	// Importer, when set, resolves @use/@forward/@import URLs. When nil, a
	// filesystem importer based on LoadPaths (and the entry file's directory)
	// is used.
	Importer Importer
	// baseDir is the directory used to resolve relative imports (set internally
	// by Compile; may be set by callers of CompileString).
	BaseDir string
}

// CompileResult is the outcome of a successful compilation.
type CompileResult struct {
	CSS        string
	LoadedURLs []string
	SourceMap  string // source maps are not yet emitted (see package residuals)
}

// CompileString compiles Sass/SCSS source text to CSS.
func CompileString(source string, opts *Options) (*CompileResult, error) {
	if opts == nil {
		opts = &Options{}
	}
	imp := opts.Importer
	if imp == nil {
		imp = fileImporter(opts.BaseDir, opts.LoadPaths)
	}
	res, err := engine.Render(source, opts.Syntax == SyntaxIndented, opts.Style == Compressed, engine.Importer(imp))
	if err != nil {
		return nil, err
	}
	return &CompileResult{CSS: res.CSS, LoadedURLs: res.LoadedURLs}, nil
}

// Compile reads and compiles a Sass/SCSS file.
func Compile(path string, opts *Options) (*CompileResult, error) {
	if opts == nil {
		opts = &Options{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	o := *opts
	if strings.HasSuffix(path, ".sass") {
		o.Syntax = SyntaxIndented
	} else if strings.HasSuffix(path, ".css") {
		o.Syntax = SyntaxCSS
	}
	o.BaseDir = filepath.Dir(path)
	res, err := CompileString(string(data), &o)
	if err != nil {
		return nil, err
	}
	res.LoadedURLs = append([]string{path}, res.LoadedURLs...)
	return res, nil
}

// fileImporter builds a filesystem importer resolving partials and extensions
// across the entry directory and the configured load paths.
func fileImporter(baseDir string, loadPaths []string) Importer {
	dirs := make([]string, 0, len(loadPaths)+1)
	if baseDir != "" {
		dirs = append(dirs, baseDir)
	} else {
		dirs = append(dirs, ".")
	}
	dirs = append(dirs, loadPaths...)
	return func(url string) (string, string, bool) {
		if strings.HasPrefix(url, "sass:") {
			return "", "", false
		}
		for _, dir := range dirs {
			for _, cand := range importCandidates(dir, url) {
				if data, err := os.ReadFile(cand); err == nil {
					return string(data), cand, true
				}
			}
		}
		return "", "", false
	}
}

func importCandidates(dir, url string) []string {
	base := filepath.Join(dir, url)
	name := filepath.Base(base)
	parent := filepath.Dir(base)
	var out []string
	exts := []string{".scss", ".sass", ".css"}
	if hasSassExt(url) {
		out = append(out, base)
		out = append(out, filepath.Join(parent, "_"+name))
		return out
	}
	for _, ext := range exts {
		out = append(out, base+ext)
		out = append(out, filepath.Join(parent, "_"+name+ext))
	}
	// index files
	for _, ext := range exts {
		out = append(out, filepath.Join(base, "index"+ext))
		out = append(out, filepath.Join(base, "_index"+ext))
	}
	return out
}

func hasSassExt(url string) bool {
	return strings.HasSuffix(url, ".scss") || strings.HasSuffix(url, ".sass") || strings.HasSuffix(url, ".css")
}
