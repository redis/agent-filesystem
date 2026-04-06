package main

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	nfs "github.com/willscott/go-nfs"
	nfsc "github.com/willscott/go-nfs-client/nfs"
	rpc "github.com/willscott/go-nfs-client/nfs/rpc"
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

func TestNewNFSHandlerRenamesHandlesAcrossPaths(t *testing.T) {
	fs := memfs.New()
	handler := newNFSHandler(fs)

	renamer, ok := any(handler).(nfs.HandleRenamer)
	if !ok {
		t.Fatal("expected handler to support handle rename")
	}

	fileHandle := handler.ToHandle(fs, []string{"bench", "from.txt"})
	if err := renamer.RenameHandle(fs, []string{"bench", "from.txt"}, []string{"bench", "to.txt"}); err != nil {
		t.Fatalf("rename file handle: %v", err)
	}
	_, gotPath, err := handler.FromHandle(fileHandle)
	if err != nil {
		t.Fatalf("fromhandle file: %v", err)
	}
	if fmt.Sprint(gotPath) != fmt.Sprint([]string{"bench", "to.txt"}) {
		t.Fatalf("file handle path = %v, want [bench to.txt]", gotPath)
	}

	childHandle := handler.ToHandle(fs, []string{"bench", "dir", "child.txt"})
	if err := renamer.RenameHandle(fs, []string{"bench", "dir"}, []string{"bench", "renamed"}); err != nil {
		t.Fatalf("rename dir handle: %v", err)
	}
	_, gotPath, err = handler.FromHandle(childHandle)
	if err != nil {
		t.Fatalf("fromhandle child: %v", err)
	}
	if fmt.Sprint(gotPath) != fmt.Sprint([]string{"bench", "renamed", "child.txt"}) {
		t.Fatalf("child handle path = %v, want [bench renamed child.txt]", gotPath)
	}
}

func TestNewNFSHandlerInvalidatesVerifiers(t *testing.T) {
	fs := memfs.New()
	handler := newNFSHandler(fs)

	invalidator, ok := any(handler).(nfs.VerifierInvalidator)
	if !ok {
		t.Fatal("expected handler to support verifier invalidation")
	}

	cacheHelper, ok := handler.Handler.(interface {
		VerifierFor(path string, contents []os.FileInfo) uint64
		DataForVerifier(path string, verifier uint64) []os.FileInfo
	})
	if !ok {
		t.Fatalf("wrapped handler type = %T, want verifier cache support", handler.Handler)
	}

	file, err := fs.Create("/note.txt")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	_ = file.Close()

	info, err := fs.Stat("/note.txt")
	if err != nil {
		t.Fatalf("stat note: %v", err)
	}
	entries := []os.FileInfo{info}
	verifier := cacheHelper.VerifierFor("", entries)
	if got := cacheHelper.DataForVerifier("", verifier); len(got) != 1 || got[0].Name() != "note.txt" {
		t.Fatalf("cached verifier lookup = %v, want [note.txt]", got)
	}

	invalidator.InvalidateVerifier("")
	if got := cacheHelper.DataForVerifier("", verifier); got != nil {
		t.Fatalf("verifier lookup after invalidation = %v, want nil", got)
	}
}

func TestOpenHandleRemainsReadableAfterRename(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	fs := memfs.New()
	f, err := fs.Create("/from.txt")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := f.Write([]byte("hello")); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	_ = f.Close()

	handler := newNFSHandler(fs)
	done := make(chan error, 1)
	go func() {
		done <- nfs.Serve(listener, handler)
	}()
	defer func() {
		_ = listener.Close()
		<-done
	}()

	conn, err := rpc.DialTCP(listener.Addr().Network(), listener.Addr().String(), false)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var mounter nfsc.Mount
	mounter.Client = conn
	target, err := mounter.Mount("/", rpc.AuthNull)
	if err != nil {
		t.Fatalf("mount: %v", err)
	}
	defer func() { _ = mounter.Unmount() }()

	openFile, err := target.Open("/from.txt")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer openFile.Close()

	if err := target.Rename("/from.txt", "/to.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	buf := make([]byte, 5)
	n, err := openFile.Read(buf)
	if err != nil {
		t.Fatalf("read after rename: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("read after rename = %q, want hello", string(buf[:n]))
	}

	info, _, err := target.Lookup("/to.txt", false)
	if err != nil {
		t.Fatalf("lookup new path: %v", err)
	}
	if info == nil || info.Size() != 5 {
		t.Fatalf("lookup new path returned %+v", info)
	}

	if _, _, err := target.Lookup("/from.txt", false); !os.IsNotExist(err) {
		t.Fatalf("expected old path to be missing, got %v", err)
	}
}
