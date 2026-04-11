package nfs

import (
	"errors"
	"hash/fnv"
	"io"
	"math"
	"os"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/willscott/go-nfs-client/nfs/xdr"
	"github.com/willscott/go-nfs/file"
)

// FileAttribute holds metadata about a filesystem object
type FileAttribute struct {
	Type                FileType
	FileMode            uint32
	Nlink               uint32
	UID                 uint32
	GID                 uint32
	Filesize            uint64
	Used                uint64
	SpecData            [2]uint32
	FSID                uint64
	Fileid              uint64
	Atime, Mtime, Ctime FileTime
}

// FileType represents a NFS File Type
type FileType uint32

// Enumeration of NFS FileTypes
const (
	FileTypeRegular FileType = iota + 1
	FileTypeDirectory
	FileTypeBlock
	FileTypeCharacter
	FileTypeLink
	FileTypeSocket
	FileTypeFIFO
)

func (f FileType) String() string {
	switch f {
	case FileTypeRegular:
		return "Regular"
	case FileTypeDirectory:
		return "Directory"
	case FileTypeBlock:
		return "Block Device"
	case FileTypeCharacter:
		return "Character Device"
	case FileTypeLink:
		return "Symbolic Link"
	case FileTypeSocket:
		return "Socket"
	case FileTypeFIFO:
		return "FIFO"
	default:
		return "Unknown"
	}
}

// Mode provides the OS interpreted mode of the file attributes
func (f *FileAttribute) Mode() os.FileMode {
	return os.FileMode(f.FileMode)
}

// FileCacheAttribute is the subset of FileAttribute used by
// wcc_attr
type FileCacheAttribute struct {
	Filesize     uint64
	Mtime, Ctime FileTime
}

// AsCache provides the wcc view of the file attributes
func (f FileAttribute) AsCache() *FileCacheAttribute {
	wcc := FileCacheAttribute{
		Filesize: f.Filesize,
		Mtime:    f.Mtime,
		Ctime:    f.Ctime,
	}
	return &wcc
}

// ToFileAttribute creates an NFS fattr3 struct from an OS.FileInfo
func ToFileAttribute(info os.FileInfo, filePath string) *FileAttribute {
	f := FileAttribute{}

	m := info.Mode()
	f.FileMode = uint32(m)
	if info.IsDir() {
		f.Type = FileTypeDirectory
	} else if m&os.ModeSymlink != 0 {
		f.Type = FileTypeLink
	} else if m&os.ModeCharDevice != 0 {
		f.Type = FileTypeCharacter
	} else if m&os.ModeDevice != 0 {
		f.Type = FileTypeBlock
	} else if m&os.ModeSocket != 0 {
		f.Type = FileTypeSocket
	} else if m&os.ModeNamedPipe != 0 {
		f.Type = FileTypeFIFO
	} else {
		f.Type = FileTypeRegular
	}
	// The number of hard links to the file.
	f.Nlink = 1

	if a := file.GetInfo(info); a != nil {
		f.Nlink = a.Nlink
		f.UID = a.UID
		f.GID = a.GID
		f.SpecData = [2]uint32{a.Major, a.Minor}
		f.Fileid = a.Fileid
	} else {
		hasher := fnv.New64()
		_, _ = hasher.Write([]byte(filePath))
		f.Fileid = hasher.Sum64()
	}

	f.Filesize = uint64(info.Size())
	f.Used = uint64(info.Size())
	f.Atime = ToNFSTime(info.ModTime())
	f.Mtime = f.Atime
	f.Ctime = f.Atime
	return &f
}

// tryStat attempts to create a FileAttribute from a path.
func tryStat(fs billy.Filesystem, path []string) *FileAttribute {
	fullPath := fs.Join(path...)
	attrs, err := fs.Lstat(fullPath)
	if err != nil || attrs == nil {
		Log.Errorf("err loading attrs for %s: %v", fs.Join(path...), err)
		return nil
	}
	return ToFileAttribute(attrs, fullPath)
}

// WriteWcc writes the `wcc_data` representation of an object.
func WriteWcc(writer io.Writer, pre *FileCacheAttribute, post *FileAttribute) error {
	if pre == nil {
		if err := xdr.Write(writer, uint32(0)); err != nil {
			return err
		}
	} else {
		if err := xdr.Write(writer, uint32(1)); err != nil {
			return err
		}
		if err := xdr.Write(writer, *pre); err != nil {
			return err
		}
	}
	if post == nil {
		if err := xdr.Write(writer, uint32(0)); err != nil {
			return err
		}
	} else {
		if err := xdr.Write(writer, uint32(1)); err != nil {
			return err
		}
		if err := xdr.Write(writer, *post); err != nil {
			return err
		}
	}
	return nil
}

// WritePostOpAttrs writes the `post_op_attr` representation of a files attributes
func WritePostOpAttrs(writer io.Writer, post *FileAttribute) error {
	if post == nil {
		if err := xdr.Write(writer, uint32(0)); err != nil {
			return err
		}
	} else {
		if err := xdr.Write(writer, uint32(1)); err != nil {
			return err
		}
		if err := xdr.Write(writer, *post); err != nil {
			return err
		}
	}
	return nil
}

// SetFileAttributes represents a command to update some metadata
// about a file.
type SetFileAttributes struct {
	SetMode  *uint32
	SetUID   *uint32
	SetGID   *uint32
	SetSize  *uint64
	SetAtime *time.Time
	SetMtime *time.Time
}

// BatchSetAttrer is an optional extension of billy.Change that collapses
// the Chmod + Lchown + Chtimes triple invoked by SetFileAttributes.Apply
// into a single call. Filesystems that implement BatchSetAttrer are auto-
// detected via a type assertion; filesystems that do not still get the
// sequential fallback. Nil pointers mean "do not change this field"; an
// all-nil call must return nil.
//
// The fast path avoids three separate backend round trips per NFS CREATE
// and SETATTR RPC; for Redis-backed filesystems that is often ~40-60 ms
// off every created file.
type BatchSetAttrer interface {
	SetAttrs(
		name string,
		mode *os.FileMode,
		uid *int,
		gid *int,
		atime *time.Time,
		mtime *time.Time,
	) error
}

// Apply uses a `Change` implementation to set defined attributes on a
// provided file. When changer also satisfies BatchSetAttrer, the
// mode / uid / gid / atime / mtime portion of the update is dispatched
// in a single call; otherwise the legacy sequential Chmod + Lchown +
// Chtimes path runs. Either way, a completely empty diff (all requested
// fields already match curr) skips the call entirely. SetSize runs
// independently after the attribute block because it lives on a
// different code path (fs.OpenFile + fp.Truncate).
func (s *SetFileAttributes) Apply(changer billy.Change, fs billy.Filesystem, file string) error {
	curOS, err := fs.Lstat(file)
	if errors.Is(err, os.ErrNotExist) {
		return &NFSStatusError{NFSStatusNoEnt, os.ErrNotExist}
	} else if errors.Is(err, os.ErrPermission) {
		return &NFSStatusError{NFSStatusAccess, os.ErrPermission}
	} else if err != nil {
		return nil
	}
	curr := ToFileAttribute(curOS, file)

	// Build the attribute diff. A nil output pointer means "no change".
	// Each branch matches the no-op checks the legacy code had so
	// behavior is preserved.
	var (
		modeOut  *os.FileMode
		uidOut   *int
		gidOut   *int
		atimeOut *time.Time
		mtimeOut *time.Time
	)
	if s.SetMode != nil {
		want := os.FileMode(*s.SetMode) & os.ModePerm
		if want != curr.Mode().Perm() {
			m := want
			modeOut = &m
		}
	}
	if s.SetUID != nil || s.SetGID != nil {
		euid := curr.UID
		if s.SetUID != nil {
			euid = *s.SetUID
		}
		egid := curr.GID
		if s.SetGID != nil {
			egid = *s.SetGID
		}
		if euid != curr.UID {
			u := int(euid)
			uidOut = &u
		}
		if egid != curr.GID {
			g := int(egid)
			gidOut = &g
		}
	}
	if s.SetAtime != nil || s.SetMtime != nil {
		atime := curr.Atime.Native()
		if s.SetAtime != nil {
			atime = s.SetAtime
		}
		mtime := curr.Mtime.Native()
		if s.SetMtime != nil {
			mtime = s.SetMtime
		}
		if atime != nil && (curr.Atime.Native() == nil || *atime != *curr.Atime.Native()) {
			t := *atime
			atimeOut = &t
		}
		if mtime != nil && (curr.Mtime.Native() == nil || *mtime != *curr.Mtime.Native()) {
			t := *mtime
			mtimeOut = &t
		}
	}

	diffEmpty := modeOut == nil && uidOut == nil && gidOut == nil &&
		atimeOut == nil && mtimeOut == nil
	if !diffEmpty {
		if batcher, ok := changer.(BatchSetAttrer); ok {
			// Fast path: one call, 1 RTT on Redis-backed backends.
			if err := batcher.SetAttrs(file, modeOut, uidOut, gidOut, atimeOut, mtimeOut); err != nil {
				if errors.Is(err, os.ErrPermission) {
					return &NFSStatusError{NFSStatusAccess, os.ErrPermission}
				}
				return err
			}
		} else {
			// Legacy fallback: sequential Chmod + Lchown + Chtimes for
			// any billy.Change that doesn't implement the batched
			// interface (e.g. memfs and upstream go-nfs consumers).
			if modeOut != nil {
				if changer == nil {
					return &NFSStatusError{NFSStatusNotSupp, os.ErrPermission}
				}
				if err := changer.Chmod(file, *modeOut); err != nil {
					if errors.Is(err, os.ErrPermission) {
						return &NFSStatusError{NFSStatusAccess, os.ErrPermission}
					}
					return err
				}
			}
			if uidOut != nil || gidOut != nil {
				if changer == nil {
					return &NFSStatusError{NFSStatusNotSupp, os.ErrPermission}
				}
				// Lchown takes both uid and gid. Fall back to curr for
				// whichever side wasn't explicitly changed.
				eu := int(curr.UID)
				if uidOut != nil {
					eu = *uidOut
				}
				eg := int(curr.GID)
				if gidOut != nil {
					eg = *gidOut
				}
				if err := changer.Lchown(file, eu, eg); err != nil {
					if errors.Is(err, os.ErrPermission) {
						return &NFSStatusError{NFSStatusAccess, os.ErrPermission}
					}
					return err
				}
			}
			if atimeOut != nil || mtimeOut != nil {
				if changer == nil {
					return &NFSStatusError{NFSStatusNotSupp, os.ErrPermission}
				}
				var at, mt time.Time
				if a := curr.Atime.Native(); a != nil {
					at = *a
				}
				if m := curr.Mtime.Native(); m != nil {
					mt = *m
				}
				if atimeOut != nil {
					at = *atimeOut
				}
				if mtimeOut != nil {
					mt = *mtimeOut
				}
				if err := changer.Chtimes(file, at, mt); err != nil {
					if errors.Is(err, os.ErrPermission) {
						return &NFSStatusError{NFSStatusAccess, err}
					}
					return err
				}
			}
		}
	}

	// SetSize lives on a separate code path (fs.OpenFile + fp.Truncate)
	// and runs regardless of whether the attribute block did anything.
	// This preserves the historical ordering: attributes first, then
	// truncate.
	if s.SetSize != nil {
		if curr.Mode()&os.ModeSymlink != 0 {
			return &NFSStatusError{NFSStatusNotSupp, os.ErrInvalid}
		}
		fp, err := fs.OpenFile(file, os.O_WRONLY|os.O_EXCL, 0)
		if errors.Is(err, os.ErrPermission) {
			return &NFSStatusError{NFSStatusAccess, err}
		} else if err != nil {
			return err
		}
		if *s.SetSize > math.MaxInt64 {
			return &NFSStatusError{NFSStatusInval, os.ErrInvalid}
		}
		if err := fp.Truncate(int64(*s.SetSize)); err != nil {
			return err
		}
		if err := fp.Close(); err != nil {
			return err
		}
	}

	return nil
}

// Mode returns a mode if specified or the provided default mode.
func (s *SetFileAttributes) Mode(def os.FileMode) os.FileMode {
	if s.SetMode != nil {
		return os.FileMode(*s.SetMode) & os.ModePerm
	}
	return def
}

// ReadSetFileAttributes reads an sattr3 xdr stream into a go struct.
func ReadSetFileAttributes(r io.Reader) (*SetFileAttributes, error) {
	attrs := SetFileAttributes{}
	hasMode, err := xdr.ReadUint32(r)
	if err != nil {
		return nil, err
	}
	if hasMode != 0 {
		mode, err := xdr.ReadUint32(r)
		if err != nil {
			return nil, err
		}
		attrs.SetMode = &mode
	}
	hasUID, err := xdr.ReadUint32(r)
	if err != nil {
		return nil, err
	}
	if hasUID != 0 {
		uid, err := xdr.ReadUint32(r)
		if err != nil {
			return nil, err
		}
		attrs.SetUID = &uid
	}
	hasGID, err := xdr.ReadUint32(r)
	if err != nil {
		return nil, err
	}
	if hasGID != 0 {
		gid, err := xdr.ReadUint32(r)
		if err != nil {
			return nil, err
		}
		attrs.SetGID = &gid
	}
	hasSize, err := xdr.ReadUint32(r)
	if err != nil {
		return nil, err
	}
	if hasSize != 0 {
		var size uint64
		attrs.SetSize = &size
		if err := xdr.Read(r, &size); err != nil {
			return nil, err
		}
	}
	aTime, err := xdr.ReadUint32(r)
	if err != nil {
		return nil, err
	}
	if aTime == 1 {
		now := time.Now()
		attrs.SetAtime = &now
	} else if aTime == 2 {
		t := FileTime{}
		if err := xdr.Read(r, &t); err != nil {
			return nil, err
		}
		attrs.SetAtime = t.Native()
	}
	mTime, err := xdr.ReadUint32(r)
	if err != nil {
		return nil, err
	}
	if mTime == 1 {
		now := time.Now()
		attrs.SetMtime = &now
	} else if mTime == 2 {
		t := FileTime{}
		if err := xdr.Read(r, &t); err != nil {
			return nil, err
		}
		attrs.SetMtime = t.Native()
	}
	return &attrs, nil
}
