// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file gates the compiler against the canonical Sass conformance suite,
// github.com/sass/sass-spec. That suite ships as HRX archives (*.hrx): a single
// text file bundling many virtual files (input.scss / output.css / error /
// warning / partials) separated by "<===>" boundary lines. Dart Sass is the
// reference implementation, so each case's output.css is the ground truth.
//
// The full suite (thousands of cases) is not vendored. Point SASS_SPEC_PATH at a
// checkout of sass-spec to run the whole differential; otherwise the full-suite
// test skips and CI relies on the frozen golden subset in testdata/spec (see
// spec_golden_test.go).

// hrxParse splits an HRX archive into a map of virtual path -> file contents.
func hrxParse(data string) map[string]string {
	files := map[string]string{}
	var cur string
	haveCur := false
	var buf []string
	flush := func() {
		if haveCur {
			files[cur] = strings.Join(buf, "\n")
		}
		buf = buf[:0]
	}
	for _, ln := range strings.Split(data, "\n") {
		if strings.HasPrefix(ln, "<===>") {
			flush()
			rest := strings.TrimSpace(strings.TrimPrefix(ln, "<===>"))
			cur = rest
			haveCur = rest != "" // a bare <===> introduces a comment section
			continue
		}
		buf = append(buf, ln)
	}
	flush()
	return files
}

// specCase is a single success case: an input that should compile to output.
type specCase struct {
	name     string // hrxRel + "::" + caseDir
	input    string
	indented bool
	expected string
	files    map[string]string // full virtual FS of the owning HRX (for imports)
	caseDir  string            // virtual dir of the input (for import resolution)
	specDir  string            // on-disk spec load-path root
}

// importCandidateNames returns partial/extension/index candidates for url.
func importCandidateNames(joined, url string) []string {
	name := path.Base(joined)
	parent := path.Dir(joined)
	exts := []string{".scss", ".sass", ".css"}
	hasExt := strings.HasSuffix(url, ".scss") || strings.HasSuffix(url, ".sass") || strings.HasSuffix(url, ".css")
	var cands []string
	if hasExt {
		cands = append(cands, joined, path.Join(parent, "_"+name))
	} else {
		for _, e := range exts {
			cands = append(cands, joined+e, path.Join(parent, "_"+name+e))
		}
		for _, e := range exts {
			cands = append(cands, path.Join(joined, "index"+e), path.Join(joined, "_index"+e))
		}
	}
	return cands
}

// hrxImporter resolves @use/@forward/@import first against the HRX's own virtual
// file map (relative to the importing case's directory), then against real files
// on disk rooted at specDir — sass-spec runs with the spec tree as a load path,
// and some shared partials (e.g. _utils.scss) live on disk beside the archive.
func hrxImporter(files map[string]string, baseDir, specDir string) Importer {
	return func(url string) (string, string, bool) {
		if strings.HasPrefix(url, "sass:") {
			return "", "", false
		}
		// 1) virtual FS, relative to the case dir.
		joined := path.Clean(path.Join(baseDir, url))
		for _, c := range importCandidateNames(joined, url) {
			c = strings.TrimPrefix(c, "./")
			if src, ok := files[c]; ok {
				return src, c, true
			}
		}
		// 2) disk, rooted at the spec load path (url is spec-root relative).
		if specDir != "" {
			diskJoined := filepath.Clean(filepath.Join(specDir, filepath.FromSlash(url)))
			for _, c := range importCandidateNames(diskJoined, url) {
				if data, err := os.ReadFile(c); err == nil {
					return string(data), c, true
				}
			}
		}
		return "", "", false
	}
}

// collectCases parses one HRX archive and returns its runnable success cases.
func collectCases(hrxRel, data, specDir string) []specCase {
	files := hrxParse(data)
	var cases []specCase
	for p, src := range files {
		var inputExt string
		switch {
		case strings.HasSuffix(p, "input.scss"):
			inputExt = "input.scss"
		case strings.HasSuffix(p, "input.sass"):
			inputExt = "input.sass"
		default:
			continue
		}
		dir := strings.TrimSuffix(p, inputExt)
		exp, ok := files[dir+"output.css"]
		if !ok {
			continue // error/warning-only case: not an output differential
		}
		// Skip cases explicitly marked TODO/ignored for dart-sass.
		if opt, ok := files[dir+"options.yml"]; ok {
			if strings.Contains(opt, "dart-sass") &&
				(strings.Contains(opt, ":todo:") || strings.Contains(opt, ":ignore_for:")) {
				continue
			}
		}
		cases = append(cases, specCase{
			name:     hrxRel + "::" + strings.TrimSuffix(dir, "/"),
			input:    src,
			indented: inputExt == "input.sass",
			expected: exp,
			files:    files,
			caseDir:  strings.TrimSuffix(dir, "/"),
			specDir:  specDir,
		})
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].name < cases[j].name })
	return cases
}

// normalizeCSS strips trailing whitespace for tolerant comparison.
func normalizeCSS(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// runCase compiles one case and reports whether the output matches expected.
func runCase(c specCase) (got string, err error) {
	return runCaseWith(c, hrxImporter(c.files, c.caseDir, c.specDir))
}

func runCaseWith(c specCase, imp Importer) (got string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errFromRecover(r)
		}
	}()
	syn := SyntaxSCSS
	if c.indented {
		syn = SyntaxIndented
	}
	res, e := CompileString(c.input, &Options{Syntax: syn, Style: Expanded, Importer: imp})
	if e != nil {
		return "", e
	}
	return res.CSS, nil
}

// selfContained reports whether a passing case compiles to the expected output
// with no file imports at all (only built-in sass: modules). Such single-file
// cases can be frozen verbatim into the in-repo golden corpus, so CI needs no
// sass-spec checkout and no partials to carry along.
func selfContained(c specCase) bool {
	loadedFile := false
	imp := func(url string) (string, string, bool) {
		if !strings.HasPrefix(url, "sass:") {
			loadedFile = true
		}
		return hrxImporter(c.files, c.caseDir, c.specDir)(url)
	}
	got, err := runCaseWith(c, imp)
	return err == nil && !loadedFile && normalizeCSS(got) == normalizeCSS(c.expected)
}

func errFromRecover(r any) error {
	if e, ok := r.(error); ok {
		return e
	}
	return &panicErr{r}
}

type panicErr struct{ v any }

func (p *panicErr) Error() string { return "panic" }

// walkSpec loads every case under dir.
func walkSpec(t *testing.T, dir string) []specCase {
	t.Helper()
	var all []specCase
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".hrx") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		all = append(all, collectCases(rel, string(data), dir)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk spec: %v", err)
	}
	return all
}

// TestSassSpecFull runs the entire sass-spec suite when SASS_SPEC_PATH is set.
// It reports a pass rate and never fails on individual divergences (the frozen
// golden subset is the CI gate); its purpose is the compat oracle X/N.
func TestSassSpecFull(t *testing.T) {
	root := os.Getenv("SASS_SPEC_PATH")
	if root == "" {
		t.Skip("set SASS_SPEC_PATH to a sass-spec checkout to run the full conformance suite")
	}
	specDir := filepath.Join(root, "spec")
	if _, err := os.Stat(specDir); err != nil {
		specDir = root
	}
	cases := walkSpec(t, specDir)
	if len(cases) == 0 {
		t.Fatalf("no spec cases found under %s", specDir)
	}
	var pass, fail int
	bucket := map[string][2]int{} // top dir -> [pass,fail]
	var report strings.Builder
	for _, c := range cases {
		got, err := runCase(c)
		top := strings.SplitN(c.name, "/", 2)[0]
		b := bucket[top]
		if err == nil && normalizeCSS(got) == normalizeCSS(c.expected) {
			pass++
			b[0]++
		} else {
			fail++
			b[1]++
			if err != nil {
				report.WriteString("FAIL " + c.name + ": error: " + err.Error() + "\n")
			} else {
				report.WriteString("FAIL " + c.name + ": mismatch\n<<<GOT\n" + got + "GOT\n>>>WANT\n" + c.expected + "WANT\n")
			}
		}
		bucket[top] = b
	}
	if rf := os.Getenv("SASS_SPEC_REPORT"); rf != "" {
		_ = os.WriteFile(rf, []byte(report.String()), 0o644)
	}
	total := pass + fail
	t.Logf("sass-spec success-case differential: %d/%d passed (%.2f%%)", pass, total, 100*float64(pass)/float64(total))
	if dir := os.Getenv("SASS_SPEC_FREEZE"); dir != "" {
		freezeGolden(t, dir, cases)
	}
	tops := make([]string, 0, len(bucket))
	for k := range bucket {
		tops = append(tops, k)
	}
	sort.Strings(tops)
	for _, k := range tops {
		b := bucket[k]
		t.Logf("  %-28s %5d/%-5d", k, b[0], b[0]+b[1])
	}
}

// goldenBucket groups a case for freezing. The selector module, the @extend
// engine and the media-query subtree get dedicated buckets so the frozen corpus
// exercises them thoroughly; everything else buckets by top-level directory.
func goldenBucket(name string) string {
	switch {
	case strings.Contains(name, "core_functions/selector/"):
		return "selector"
	case strings.Contains(name, "/extend/") || strings.HasPrefix(name, "directives/extend"):
		return "extend"
	case strings.Contains(name, "css/media"):
		return "media"
	default:
		return strings.SplitN(name, "/", 2)[0]
	}
}

// freezeGolden writes a representative, self-contained, passing subset of the
// suite into dir as per-bucket HRX archives, forming the in-repo golden corpus.
func freezeGolden(t *testing.T, dir string, cases []specCase) {
	t.Helper()
	// The selector/extend/media buckets get a high cap so the frozen corpus
	// covers the full ported algorithms (superselector, unify, weave, extend and
	// media merge); other buckets stay representative.
	cap := func(bucket string) int {
		switch bucket {
		case "selector", "extend", "media":
			return 100000
		default:
			return 60
		}
	}
	byBucket := map[string][]specCase{}
	for _, c := range cases {
		top := goldenBucket(c.name)
		if len(byBucket[top]) >= cap(top) {
			continue
		}
		if !selfContained(c) {
			continue
		}
		byBucket[top] = append(byBucket[top], c)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	total := 0
	for bucket, cs := range byBucket {
		var sb strings.Builder
		for i, c := range cs {
			in := "input.scss"
			if c.indented {
				in = "input.sass"
			}
			fmt.Fprintf(&sb, "<===> case%d/%s\n%s\n\n", i, in, strings.TrimRight(c.input, "\n"))
			fmt.Fprintf(&sb, "<===> case%d/output.css\n%s\n\n", i, strings.TrimRight(c.expected, "\n"))
			total++
		}
		fn := filepath.Join(dir, strings.ReplaceAll(bucket, "/", "_")+".hrx")
		if err := os.WriteFile(fn, []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("froze %d self-contained golden cases across %d buckets into %s", total, len(byBucket), dir)
}

// TestSassSpecGolden runs the frozen in-repo sass-spec subset (testdata/spec).
// Every frozen case must pass exactly — this is the CI conformance gate that
// runs without a sass-spec checkout. The corpus is regenerated by running
// TestSassSpecFull with SASS_SPEC_FREEZE=testdata/spec against a checkout.
func TestSassSpecGolden(t *testing.T) {
	dir := "testdata/spec"
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no frozen golden corpus at testdata/spec")
	}
	cases := walkSpec(t, dir)
	if len(cases) == 0 {
		t.Fatal("frozen golden corpus is empty")
	}
	var fails int
	for _, c := range cases {
		got, err := runCase(c)
		if err != nil {
			t.Errorf("%s: compile error: %v", c.name, err)
			fails++
			continue
		}
		if normalizeCSS(got) != normalizeCSS(c.expected) {
			t.Errorf("%s: output mismatch\n got: %q\nwant: %q", c.name, got, c.expected)
			fails++
		}
	}
	t.Logf("frozen golden conformance: %d/%d cases pass", len(cases)-fails, len(cases))
}
