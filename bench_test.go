// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-scss/scss"
)

// readBench loads a committed benchmark corpus file from bench/corpus.
func readBench(b *testing.B, name string) string {
	b.Helper()
	data, err := os.ReadFile(filepath.Join("bench", "corpus", name))
	if err != nil {
		b.Fatal(err)
	}
	return string(data)
}

func benchCompile(b *testing.B, src string, style scss.OutputStyle) {
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	opts := &scss.Options{Style: style}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := scss.CompileString(src, opts); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGeneratedExpanded compiles the synthetic heavy stylesheet
// (bench/corpus/generated.scss) in expanded style. This is the whole-pipeline
// benchmark used for CPU/mem profiling.
func BenchmarkGeneratedExpanded(b *testing.B) {
	benchCompile(b, readBench(b, "generated.scss"), scss.Expanded)
}

// BenchmarkGeneratedCompressed compiles the synthetic heavy stylesheet in
// compressed style.
func BenchmarkGeneratedCompressed(b *testing.B) {
	benchCompile(b, readBench(b, "generated.scss"), scss.Compressed)
}
