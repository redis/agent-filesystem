package main

import (
	"fmt"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
)

func TestNewNFSHandlerKeepsMacOSStyleWorkloadHandlesAlive(t *testing.T) {
	fs := memfs.New()
	handler := newNFSHandler(fs)

	paths := make([][]string, 0, 4096)
	for i := 0; i < 2000; i++ {
		dir := fmt.Sprintf("d%02d", i%20)
		sub := fmt.Sprintf("sub%02d", i%7)
		file := fmt.Sprintf("f%05d.md", i)
		paths = append(paths,
			[]string{"bench", dir},
			[]string{"bench", "._" + dir},
			[]string{"bench", dir, sub},
			[]string{"bench", dir, "._" + sub},
			[]string{"bench", dir, sub, file},
			[]string{"bench", dir, sub, "._" + file},
		)
	}

	handles := make([][]byte, 0, len(paths))
	for _, p := range paths {
		handles = append(handles, handler.ToHandle(fs, p))
	}

	for _, idx := range []int{0, 1, 2, len(handles) / 2, len(handles) - 1} {
		if _, _, err := handler.FromHandle(handles[idx]); err != nil {
			t.Fatalf("expected handle %d to remain valid after %d assignments: %v", idx, len(handles), err)
		}
	}
}
