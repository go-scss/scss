// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// This file gates the compiler against the canonical Sass conformance suite,
// github.com/sass/sass-spec. That suite ships as HRX archives (*.hrx): a single
// text file bundling many virtual files (input.scss / output.css / error /
// warning / partials) separated by "<===>" boundary lines. Dart Sass is the
// reference implementation, so each case's dart-applicable expected output is
// the ground truth.
//
// Scoring fidelity. sass-spec deliberately contains cases that a given
// implementation is not expected to pass. The official runner honours a
// per-directory annotation system (options.yml, recursively merged from the
// suite root down to the case) and per-impl expected-output overrides. This
// harness reproduces that logic for the dart-sass implementation so the pass
// rate is scored the way the official runner scores dart-sass:
//
//   - :ignore_for: [dart-sass]  -> case is EXCLUDED from the denominator.
//   - :todo: [ ... dart-sass ]  -> case is EXCLUDED (skipped, not a failure).
//   - output-dart-sass.css / error-dart-sass  -> override the plain expectation.
//   - a case with only an `error`/`error-dart-sass` expectation is an error
//     case, not part of the success-output differential.
//
// A case that sass-spec marks not-applicable-to-dart-sass is thus removed from
// the denominator rather than counted as a failure, exactly as the official
// runner does when scoring dart-sass.
//
// The full suite (thousands of cases) is not vendored. Point SASS_SPEC_PATH at a
// checkout of sass-spec to run the whole differential; otherwise the full-suite
// test skips and CI relies on the frozen golden subset in testdata/spec.

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

// specOptions is the subset of sass-spec's options.yml metadata that affects how
// dart-sass is scored. Lists hold implementation identifiers (often shorthand
// GitHub issue references such as "sass/dart-sass#1347"); membership is by
// substring, matching the official runner's SpecOptions.hasForImpl.
type specOptions struct {
	ignoreFor   []string
	todo        []string
	warningTodo []string
	precision   int // 0 == unset (dart default is 10)
}

// merge concatenates the impl lists (parent then child) and lets a child
// precision override the parent's, mirroring SpecOptions.merge.
func (o specOptions) merge(c specOptions) specOptions {
	out := specOptions{
		ignoreFor:   append(append([]string{}, o.ignoreFor...), c.ignoreFor...),
		todo:        append(append([]string{}, o.todo...), c.todo...),
		warningTodo: append(append([]string{}, o.warningTodo...), c.warningTodo...),
		precision:   o.precision,
	}
	if c.precision != 0 {
		out.precision = c.precision
	}
	return out
}

func hasImpl(list []string, impl string) bool {
	for _, it := range list {
		if strings.Contains(it, impl) {
			return true
		}
	}
	return false
}

// parseOptionsYAML reads the small YAML dialect used by sass-spec options.yml:
// top-level ":key:" entries whose value is either an inline scalar (precision)
// or a block/flow list of implementation identifiers.
func parseOptionsYAML(content string) specOptions {
	var o specOptions
	cur := ""
	for _, raw := range strings.Split(content, "\n") {
		ln := strings.TrimRight(raw, "\r")
		t := strings.TrimSpace(ln)
		if t == "" || t == "---" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "- ") || t == "-" {
			item := strings.TrimSpace(strings.TrimPrefix(t, "-"))
			appendItem(&o, cur, item)
			continue
		}
		if strings.HasPrefix(t, ":") {
			idx := strings.Index(t[1:], ":")
			if idx < 0 {
				continue
			}
			key := t[:idx+2] // include leading and trailing colon
			val := strings.TrimSpace(t[idx+2:])
			cur = key
			switch key {
			case ":precision:":
				if n, err := strconv.Atoi(val); err == nil {
					o.precision = n
				}
			default:
				// Inline flow list, e.g. ":ignore_for: [dart-sass]".
				if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
					for _, it := range strings.Split(strings.Trim(val, "[]"), ",") {
						appendItem(&o, key, strings.TrimSpace(it))
					}
				} else if val != "" {
					appendItem(&o, key, val)
				}
			}
		}
	}
	return o
}

func appendItem(o *specOptions, key, item string) {
	if item == "" {
		return
	}
	switch key {
	case ":ignore_for:":
		o.ignoreFor = append(o.ignoreFor, item)
	case ":todo:":
		o.todo = append(o.todo, item)
	case ":warning_todo:":
		o.warningTodo = append(o.warningTodo, item)
	}
}

// ancestorDirs returns dir and every ancestor virtual directory up to the HRX
// root, ordered root-first, each with a trailing slash ("" is the root).
func ancestorDirs(dir string) []string {
	dir = strings.TrimSuffix(dir, "/")
	var parts []string
	if dir != "" {
		parts = strings.Split(dir, "/")
	}
	out := []string{""}
	acc := ""
	for _, p := range parts {
		acc = acc + p + "/"
		out = append(out, acc)
	}
	return out
}

// hrxOptionsFor merges the on-disk base options with every options.yml lying on
// the path from the HRX root down to the case directory (root-first).
func hrxOptionsFor(files map[string]string, base specOptions, caseDir string) specOptions {
	eff := base
	for _, d := range ancestorDirs(caseDir) {
		if content, ok := files[d+"options.yml"]; ok {
			eff = eff.merge(parseOptionsYAML(content))
		}
	}
	return eff
}

// dartMode reports how dart-sass treats a case: "ignore" (never applicable),
// "todo" (expected to pass eventually, skipped today), or "" (scored normally).
func dartMode(o specOptions) string {
	if hasImpl(o.ignoreFor, "dart-sass") {
		return "ignore"
	}
	if hasImpl(o.todo, "dart-sass") {
		return "todo"
	}
	return ""
}

// specCase is a single differential case.
type specCase struct {
	name      string // hrxRel + "::" + caseDir
	input     string
	indented  bool
	expected  string            // dart-applicable expected output (empty for error cases)
	files     map[string]string // full virtual FS of the owning HRX (for imports)
	caseDir   string            // virtual dir of the input (for import resolution)
	specDir   string            // on-disk spec load-path root
	hrxPrefix string            // spec-root-relative dir the HRX materialises into
	mode      string            // dartMode: "", "ignore", or "todo"
	errCase   bool              // dart expects a compile error, not output
	precision int
}

// applicable reports whether a case counts toward the success-output
// differential denominator when scoring dart-sass. A case only enters the case
// list when it carries a dart expectation, so a scored case always has an output
// expectation (an empty one is legitimate: side-effect specs such as
// math.random assert via @error and expect empty output.css).
func (c specCase) applicable() bool { return c.mode == "" && !c.errCase }

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

// hrxImporter resolves @use/@forward/@import the way the official sass-spec
// runner does. The runner materialises the whole spec tree to a root and invokes
// the compiler with cwd at the case directory and --load-path=<spec-root>. So a
// url resolves (1) relative to the importing case's directory, then (2) relative
// to the load-path (spec root) — which, for a within-archive partial, means the
// HRX's own virtual files keyed under hrxPrefix. A final on-disk fallback covers
// the handful of shared partials that live beside an archive on disk.
func hrxImporter(files map[string]string, baseDir, specDir, hrxPrefix string) Importer {
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
		// 2) load-path (spec root) relative: within-archive partials are stored
		// under hrxPrefix, so strip that prefix to get the virtual key.
		if hrxPrefix != "" && strings.HasPrefix(url, hrxPrefix+"/") {
			rel := strings.TrimPrefix(url, hrxPrefix+"/")
			for _, c := range importCandidateNames(rel, url) {
				c = strings.TrimPrefix(c, "./")
				if src, ok := files[c]; ok {
					return src, c, true
				}
			}
		}
		// 3) disk, relative to the case's on-disk location (specDir/hrxPrefix/
		// caseDir) — resolves "../foo" references to real partials that live on
		// disk beside an archive, e.g. w3c/_test-hue.scss.
		if specDir != "" && hrxPrefix != "" {
			caseOnDisk := filepath.Join(specDir, filepath.FromSlash(hrxPrefix), filepath.FromSlash(baseDir))
			diskJoined := filepath.Clean(filepath.Join(caseOnDisk, filepath.FromSlash(url)))
			for _, c := range importCandidateNames(diskJoined, url) {
				if data, err := os.ReadFile(c); err == nil {
					return string(data), c, true
				}
			}
		}
		// 4) disk, rooted at the spec load path (url is spec-root relative).
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

// dartExpectation selects the dart-sass expectation for a case directory: an
// impl-specific override wins over the plain file, and an output expectation is
// a success case while an error expectation is not. It returns the expected
// output (empty for error/absent) and whether the case expects an error.
func dartExpectation(files map[string]string, dir string) (expected string, errCase bool, ok bool) {
	switch {
	case has(files, dir+"output-dart-sass.css"):
		return files[dir+"output-dart-sass.css"], false, true
	case has(files, dir+"error-dart-sass"):
		return "", true, true
	case has(files, dir+"output.css"):
		return files[dir+"output.css"], false, true
	case has(files, dir+"error"):
		return "", true, true
	}
	return "", false, false
}

func has(files map[string]string, k string) bool { _, ok := files[k]; return ok }

// collectCases parses one HRX archive and returns its cases, each tagged with
// the dart-sass mode and expectation. baseOpts carries the options inherited
// from on-disk parent directories of the archive.
func collectCases(hrxRel, data, specDir string, baseOpts specOptions) []specCase {
	files := hrxParse(data)
	hrxPrefix := filepath.ToSlash(strings.TrimSuffix(hrxRel, ".hrx"))
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
		exp, errCase, ok := dartExpectation(files, dir)
		if !ok {
			continue // no expectation at all: nothing to differentiate
		}
		opts := hrxOptionsFor(files, baseOpts, strings.TrimSuffix(dir, "/"))
		cases = append(cases, specCase{
			name:      hrxRel + "::" + strings.TrimSuffix(dir, "/"),
			input:     src,
			indented:  inputExt == "input.sass",
			expected:  exp,
			files:     files,
			caseDir:   strings.TrimSuffix(dir, "/"),
			specDir:   specDir,
			hrxPrefix: hrxPrefix,
			mode:      dartMode(opts),
			errCase:   errCase,
			precision: opts.precision,
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

// runCase compiles one case with go-scss and reports the output.
func runCase(c specCase) (got string, err error) {
	return runCaseWith(c, hrxImporter(c.files, c.caseDir, c.specDir, c.hrxPrefix))
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
		return hrxImporter(c.files, c.caseDir, c.specDir, c.hrxPrefix)(url)
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

// diskOptionsFor walks from specDir down to the directory holding hrxPath and
// merges any on-disk options.yml encountered (root-first).
func diskOptionsFor(specDir, hrxPath string) specOptions {
	var eff specOptions
	rel, err := filepath.Rel(specDir, filepath.Dir(hrxPath))
	if err != nil {
		return eff
	}
	dir := specDir
	segs := []string{""}
	if rel != "." {
		segs = append(segs, strings.Split(rel, string(filepath.Separator))...)
	}
	for _, s := range segs {
		if s != "" {
			dir = filepath.Join(dir, s)
		}
		if data, err := os.ReadFile(filepath.Join(dir, "options.yml")); err == nil {
			eff = eff.merge(parseOptionsYAML(string(data)))
		}
	}
	return eff
}

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
		all = append(all, collectCases(rel, string(data), dir, diskOptionsFor(dir, p))...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk spec: %v", err)
	}
	return all
}

// topBucket returns the first path segment of a case name (before the HRX rel).
func topBucket(name string) string { return strings.SplitN(name, "/", 2)[0] }

// TestSassSpecFull runs the entire sass-spec suite when SASS_SPEC_PATH is set.
// It scores go-scss the way the official runner scores dart-sass (honouring the
// options.yml annotation system and per-impl expectations) and reports a pass
// rate over the dart-applicable success cases. It never fails on individual
// divergences — the frozen golden subset is the CI gate; its purpose is the
// compat oracle X/N and the roadmap-by-subtree breakdown.
//
// Set SASS_SPEC_ORACLE=/path/to/sass to additionally run dart-sass itself
// through this same harness and report its ceiling score, which empirically
// defines the applicable set and validates harness fidelity.
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

	var applicable, ignored, todo, errCases int
	var pass, fail int
	bucket := map[string][3]int{} // top dir -> [pass, fail, applicable]
	var report strings.Builder
	for _, c := range cases {
		switch {
		case c.mode == "ignore":
			ignored++
			continue
		case c.mode == "todo":
			todo++
			continue
		case c.errCase:
			errCases++
			continue
		}
		applicable++
		got, err := runCase(c)
		top := topBucket(c.name)
		b := bucket[top]
		b[2]++
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

	t.Logf("corrected harness (scored as dart-sass): %d total cases", len(cases))
	t.Logf("  excluded: %d :ignore_for dart-sass, %d :todo dart-sass, %d error-only cases", ignored, todo, errCases)
	t.Logf("go-scss success-output differential: %d/%d applicable passed (%.2f%%)",
		pass, applicable, 100*float64(pass)/float64(applicable))

	tops := make([]string, 0, len(bucket))
	for k := range bucket {
		tops = append(tops, k)
	}
	sort.Strings(tops)
	for _, k := range tops {
		b := bucket[k]
		t.Logf("  %-28s %5d/%-5d (%d fail)", k, b[0], b[2], b[1])
	}

	if oracle := os.Getenv("SASS_SPEC_ORACLE"); oracle != "" {
		runDartOracle(t, oracle, cases)
	}
	if dir := os.Getenv("SASS_SPEC_FREEZE"); dir != "" {
		freezeGolden(t, dir, cases)
	}
}

// runDartOracle runs the real dart-sass binary through this same harness over
// every dart-applicable success case and reports its pass/total — the honest
// 100% ceiling. Any applicable case dart itself "fails" here is a harness-
// fidelity bug or a genuinely contested case, and is logged for inspection.
func runDartOracle(t *testing.T, sassBin string, cases []specCase) {
	var apps []specCase
	for _, c := range cases {
		if c.applicable() {
			apps = append(apps, c)
		}
	}
	type result struct {
		name string
		ok   bool
		miss string
	}
	results := make([]result, len(apps))
	sem := make(chan struct{}, 12)
	var wg sync.WaitGroup
	for i := range apps {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			got, err := dartCompile(sassBin, apps[i])
			ok := err == nil && normalizeCSS(got) == normalizeCSS(apps[i].expected)
			r := result{name: apps[i].name, ok: ok}
			if !ok {
				if err != nil {
					r.miss = "error: " + err.Error()
				} else {
					r.miss = "mismatch"
				}
			}
			results[i] = r
		}(i)
	}
	wg.Wait()

	pass := 0
	var misses strings.Builder
	for _, r := range results {
		if r.ok {
			pass++
		} else {
			misses.WriteString("DART-MISS " + r.name + ": " + r.miss + "\n")
		}
	}
	t.Logf("dart-sass ceiling through same harness: %d/%d applicable passed (%.2f%%)",
		pass, len(apps), 100*float64(pass)/float64(len(apps)))
	if rf := os.Getenv("SASS_SPEC_ORACLE_REPORT"); rf != "" {
		_ = os.WriteFile(rf, []byte(misses.String()), 0o644)
	}
}

// dartCompile materialises a case's virtual filesystem to a temp directory and
// runs the dart-sass binary exactly as the official runner does: cwd is the
// case directory, args are [--precision N?] <inputfile>, stdout is the output.
func dartCompile(sassBin string, c specCase) (string, error) {
	tmp, err := os.MkdirTemp("", "sass-oracle-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	// Materialise the archive under its spec-root-relative prefix so that both
	// case-relative and load-path-relative (@use 'a/b/utils') imports resolve
	// exactly as they do for the official runner, which sets --load-path=<root>.
	base := filepath.Join(tmp, filepath.FromSlash(c.hrxPrefix))
	// Copy any real on-disk loose files (non-archive shared partials such as
	// w3c/_test-hue.scss) that live in the archive's on-disk parent directory,
	// so "../partial" references resolve exactly as for the official runner.
	if c.specDir != "" && c.hrxPrefix != "" {
		parent := path.Dir(c.hrxPrefix)
		realParent := filepath.Join(c.specDir, filepath.FromSlash(parent))
		if entries, err := os.ReadDir(realParent); err == nil {
			for _, e := range entries {
				if e.IsDir() || strings.HasSuffix(e.Name(), ".hrx") {
					continue
				}
				if data, err := os.ReadFile(filepath.Join(realParent, e.Name())); err == nil {
					dst := filepath.Join(tmp, filepath.FromSlash(parent), e.Name())
					_ = os.MkdirAll(filepath.Dir(dst), 0o755)
					_ = os.WriteFile(dst, data, 0o644)
				}
			}
		}
	}
	for vp, content := range c.files {
		fp := filepath.Join(base, filepath.FromSlash(vp))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	inputName := "input.scss"
	if c.indented {
		inputName = "input.sass"
	}
	args := []string{"--load-path=" + tmp, "--load-path=" + c.specDir}
	if c.precision != 0 {
		args = append(args, "--precision", strconv.Itoa(c.precision))
	}
	args = append(args, inputName)
	cmd := exec.Command(sassBin, args...)
	cmd.Dir = filepath.Join(base, filepath.FromSlash(c.caseDir))
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
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
// Only dart-applicable success cases are eligible.
func freezeGolden(t *testing.T, dir string, cases []specCase) {
	t.Helper()
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
		if !c.applicable() {
			continue
		}
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
