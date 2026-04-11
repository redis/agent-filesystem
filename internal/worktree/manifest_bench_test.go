package worktree

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func buildSyntheticTree(b *testing.B, dirs, filesPerDir, size int) string {
	b.Helper()
	root := b.TempDir()
	for d := 0; d < dirs; d++ {
		sub := filepath.Join(root, fmt.Sprintf("pkg%03d", d))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			b.Fatal(err)
		}
		for f := 0; f < filesPerDir; f++ {
			data := bytes.Repeat([]byte{byte((d + f) & 0xff)}, size+(f*13))
			name := filepath.Join(sub, fmt.Sprintf("f%04d.bin", f))
			if err := os.WriteFile(name, data, 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	return root
}

func benchmarkBuildManifest(b *testing.B, dirs, filesPerDir, size, workers int) {
	root := buildSyntheticTree(b, dirs, filesPerDir, size)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _, err := BuildManifestFromDirectory(root, "bench", "initial", BuildManifestOptions{Workers: workers})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildManifest_Small_Serial(b *testing.B) {
	benchmarkBuildManifest(b, 16, 32, 2*1024, 1)
}

func BenchmarkBuildManifest_Small_Parallel(b *testing.B) {
	benchmarkBuildManifest(b, 16, 32, 2*1024, 0)
}

func BenchmarkBuildManifest_Medium_Serial(b *testing.B) {
	benchmarkBuildManifest(b, 64, 64, 8*1024, 1)
}

func BenchmarkBuildManifest_Medium_Parallel(b *testing.B) {
	benchmarkBuildManifest(b, 64, 64, 8*1024, 0)
}

func BenchmarkBuildManifest_Large_Serial(b *testing.B) {
	benchmarkBuildManifest(b, 128, 128, 16*1024, 1)
}

func BenchmarkBuildManifest_Large_Parallel(b *testing.B) {
	benchmarkBuildManifest(b, 128, 128, 16*1024, 0)
}
