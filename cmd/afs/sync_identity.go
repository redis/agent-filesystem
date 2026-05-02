package main

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"syscall"
)

func localFileIdentity(info fs.FileInfo) string {
	if info == nil {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino)
}

func localFileIdentityFromPath(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	return localFileIdentity(info)
}

func storedLocalIdentity(entry SyncEntry) string {
	return strings.TrimSpace(entry.LocalIdentity)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
