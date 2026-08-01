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

// ReferrerImporter is the referrer-aware importer form. In addition to the URL
// being loaded, it receives the canonical URL of the stylesheet issuing the load
// — the file whose code is currently being evaluated (a module's own URL for its
// top-level rules, or a mixin/@content block's defining file for a dynamic load,
// such as meta.load-css, nested inside it). Mirroring Dart Sass's
// Importer.canonicalize(url, baseUrl:), an importer should resolve url relative to
// referrer first, then against its configured load paths. referrer is empty for a
// load issued by the entry stylesheet, which has no canonical URL.
//
// forImport is true only for a legacy @import (Dart Sass's canonicalize(url,
// forImport:)); an importer should then prefer an import-only file — x.import.scss
// / _x.import.scss, or index.import.scss inside a directory — over the ordinary
// file of the same name. @use, @forward and meta.load-css pass false.
type ReferrerImporter func(url, referrer string, forImport bool) (source string, canonicalURL string, ok bool)

// Options configures a compilation.
type Options struct {
	Syntax    Syntax
	Style     OutputStyle
	LoadPaths []string
	// Importer, when set, resolves @use/@forward/@import URLs. When nil, a
	// filesystem importer based on LoadPaths (and the entry file's directory)
	// is used. It receives only the URL; relative resolution is against the
	// configured load paths. For referrer-relative resolution (Dart Sass's
	// default behaviour, where a load resolves relative to the file issuing it),
	// set ImporterWithReferrer instead.
	Importer Importer
	// ImporterWithReferrer, when set, takes precedence over Importer and receives
	// the referrer (the canonical URL of the file issuing the load) so it can
	// resolve relative-to-referrer first, then load paths — matching Dart Sass.
	// The built-in filesystem importer used when both are nil is referrer-aware.
	ImporterWithReferrer ReferrerImporter
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
	var imp engine.Importer
	switch {
	case opts.ImporterWithReferrer != nil:
		imp = engine.Importer(opts.ImporterWithReferrer)
	case opts.Importer != nil:
		// A legacy url-only importer ignores the referrer and resolves against its
		// own configured paths, preserving the pre-referrer public contract.
		legacy := opts.Importer
		imp = func(url, _ string, _ bool) (string, string, bool) { return legacy(url) }
	default:
		imp = fileImporter(opts.BaseDir, opts.LoadPaths)
	}
	res, err := engine.Render(source, opts.Syntax == SyntaxIndented, opts.Style == Compressed, imp)
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

// fileImporter builds a referrer-aware filesystem importer resolving partials and
// extensions relative to the file issuing the load first (Dart Sass's default),
// then across the entry directory and the configured load paths.
func fileImporter(baseDir string, loadPaths []string) engine.Importer {
	dirs := make([]string, 0, len(loadPaths)+1)
	if baseDir != "" {
		dirs = append(dirs, baseDir)
	} else {
		dirs = append(dirs, ".")
	}
	dirs = append(dirs, loadPaths...)
	return func(url, referrer string, forImport bool) (string, string, bool) {
		if strings.HasPrefix(url, "sass:") {
			return "", "", false
		}
		// Relative-to-referrer first: a load resolves against the directory of the
		// file that issued it before falling back to the configured load paths.
		if referrer != "" {
			for _, cand := range importCandidates(filepath.Dir(referrer), url, forImport) {
				if data, err := os.ReadFile(cand); err == nil {
					return string(data), cand, true
				}
			}
		}
		for _, dir := range dirs {
			for _, cand := range importCandidates(dir, url, forImport) {
				if data, err := os.ReadFile(cand); err == nil {
					return string(data), cand, true
				}
			}
		}
		return "", "", false
	}
}

// importCandidates lists, in precedence order, the on-disk files a load URL may
// resolve to within a single directory. When forImport is set (a legacy @import),
// an import-only file is preferred over the ordinary file of the same name, and a
// directory's index.import file over its plain index — matching Dart Sass's
// resolveImportPath under forImport.
func importCandidates(dir, url string, forImport bool) []string {
	base := filepath.Join(dir, url)
	name := filepath.Base(base)
	parent := filepath.Dir(base)
	exts := []string{".scss", ".sass", ".css"}
	if hasSassExt(url) {
		// An explicit-extension import prefers name.import.ext over name.ext.
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		stemName := strings.TrimSuffix(name, ext)
		var out []string
		if forImport {
			out = append(out, stem+".import"+ext)
			out = append(out, filepath.Join(parent, "_"+stemName+".import"+ext))
		}
		out = append(out, base)
		out = append(out, filepath.Join(parent, "_"+name))
		return out
	}
	var out []string
	if forImport {
		// Direct import-only file (x.import.scss / _x.import.scss), highest priority.
		for _, ext := range exts {
			out = append(out, base+".import"+ext)
			out = append(out, filepath.Join(parent, "_"+name+".import"+ext))
		}
	}
	for _, ext := range exts {
		out = append(out, base+ext)
		out = append(out, filepath.Join(parent, "_"+name+ext))
	}
	// index files: an import-only index (index.import.ext) precedes the plain index.
	if forImport {
		for _, ext := range exts {
			out = append(out, filepath.Join(base, "index.import"+ext))
			out = append(out, filepath.Join(base, "_index.import"+ext))
		}
	}
	for _, ext := range exts {
		out = append(out, filepath.Join(base, "index"+ext))
		out = append(out, filepath.Join(base, "_index"+ext))
	}
	return out
}

func hasSassExt(url string) bool {
	return strings.HasSuffix(url, ".scss") || strings.HasSuffix(url, ".sass") || strings.HasSuffix(url, ".css")
}
