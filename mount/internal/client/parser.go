package client

import (
	"fmt"
	"strconv"
)

// StatResult holds parsed FS.STAT response.
type StatResult struct {
	Inode uint64
	Type  string // "file", "dir", "symlink"
	Mode  uint32 // POSIX permission bits
	UID   uint32
	GID   uint32
	Size  int64
	Ctime int64 // milliseconds since epoch
	Mtime int64
	Atime int64
}

// LsEntry holds one entry from FS.LS LONG.
type LsEntry struct {
	Inode uint64
	Name  string
	Type  string
	Mode  uint32
	UID   uint32
	GID   uint32
	Size  int64
	Mtime int64
}

// InfoResult holds parsed FS.INFO response.
type InfoResult struct {
	Files          int64
	Directories    int64
	Symlinks       int64
	TotalDataBytes int64
	TotalInodes    int64
}

type FileLock struct {
	Start uint64
	End   uint64
	Type  uint32
	PID   uint32
}

// parseStat parses the flat [field, value, ...] array from FS.STAT.
func parseStat(res []interface{}) (*StatResult, error) {
	if len(res) < 16 {
		return nil, fmt.Errorf("unexpected STAT response length: %d", len(res))
	}

	m := make(map[string]interface{}, 8)
	for i := 0; i+1 < len(res); i += 2 {
		key, ok := res[i].(string)
		if !ok {
			continue
		}
		m[key] = res[i+1]
	}

	sr := &StatResult{}
	sr.Inode = uint64(toInt64(m["inode"]))
	sr.Type = toString(m["type"])
	sr.Mode = parseOctalMode(toString(m["mode"]))
	sr.UID = uint32(toInt64(m["uid"]))
	sr.GID = uint32(toInt64(m["gid"]))
	sr.Size = toInt64(m["size"])
	sr.Ctime = toInt64(m["ctime"])
	sr.Mtime = toInt64(m["mtime"])
	sr.Atime = toInt64(m["atime"])
	return sr, nil
}

// parseLsLong parses the array of [name, type, mode, size, mtime] arrays.
func parseLsLong(res []interface{}) ([]LsEntry, error) {
	entries := make([]LsEntry, 0, len(res))
	for _, item := range res {
		arr, ok := item.([]interface{})
		if !ok || len(arr) < 5 {
			continue
		}
		e := LsEntry{
			Inode: uint64(toInt64(valueAt(arr, 5))),
			Name:  toString(arr[0]),
			Type:  toString(arr[1]),
			Mode:  parseOctalMode(toString(arr[2])),
			UID:   uint32(toInt64(valueAt(arr, 6))),
			GID:   uint32(toInt64(valueAt(arr, 7))),
			Size:  toInt64(arr[3]),
			Mtime: toInt64(arr[4]),
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// parseInfo parses the flat [field, value, ...] array from FS.INFO.
func parseInfo(res []interface{}) (*InfoResult, error) {
	if len(res) < 10 {
		return nil, fmt.Errorf("unexpected INFO response length: %d", len(res))
	}

	m := make(map[string]interface{}, 5)
	for i := 0; i+1 < len(res); i += 2 {
		key, ok := res[i].(string)
		if !ok {
			continue
		}
		m[key] = res[i+1]
	}

	return &InfoResult{
		Files:          toInt64(m["files"]),
		Directories:    toInt64(m["directories"]),
		Symlinks:       toInt64(m["symlinks"]),
		TotalDataBytes: toInt64(m["total_data_bytes"]),
		TotalInodes:    toInt64(m["total_inodes"]),
	}, nil
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func valueAt(values []interface{}, idx int) interface{} {
	if idx < 0 || idx >= len(values) {
		return nil
	}
	return values[idx]
}

func toInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	default:
		return 0
	}
}

func parseOctalMode(s string) uint32 {
	n, _ := strconv.ParseUint(s, 8, 32)
	return uint32(n)
}

// WcResult holds word/line/char counts from a Wc operation.
type WcResult struct {
	Lines int64
	Words int64
	Chars int64
}

// TreeEntry holds one entry from a Tree walk.
type TreeEntry struct {
	Path  string
	Type  string // "file", "dir", "symlink"
	Depth int
}

// GrepMatch holds one grep result.
type GrepMatch struct {
	Path    string
	LineNum int64
	Line    string
}
