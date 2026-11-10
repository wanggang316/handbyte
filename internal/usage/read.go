package usage

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"unsafe"
)

// The values come from <sys/attr.h>, <sys/stat.h>, <sys/vnode.h> and
// <sys/syscall.h>. The syscall package has no getattrlistbulk wrapper, and
// the release build does not use cgo.
const (
	sysGetattrlistbulk = 461

	attrCmnName        = 0x00000001
	attrCmnDevID       = 0x00000002
	attrCmnObjType     = 0x00000008
	attrCmnFlags       = 0x00040000
	attrCmnFileID      = 0x02000000
	attrCmnError       = 0x20000000
	attrCmnReturned    = 0x80000000
	attrDirMountStatus = 0x00000004
	attrFileLinkCount  = 0x00000001
	attrFileAllocSize  = 0x00000004
	attrFileDataLength = 0x00000200
	attrCmnExtCloneID  = 0x00000100
	attrCmnExtExtFlags = 0x00000200
	attrCmnExtCloneRef = 0x00001000
	fsoptCmnExtended   = 0x00000020

	dirMountPoint     = 0x00000001
	sfFirmlink        = 0x00800000
	efMayShareBlocks  = 0x00000001
	efSharesAllBlocks = 0x00000040

	vReg = 1
	vDir = 2
)

type attrList struct {
	bitmapCount uint16
	_           uint16
	common      uint32
	volume      uint32
	dir         uint32
	file        uint32
	fork        uint32
}

var listAttrs = attrList{
	bitmapCount: 5,
	common:      attrCmnReturned | attrCmnError | attrCmnName | attrCmnDevID | attrCmnObjType | attrCmnFlags | attrCmnFileID,
	dir:         attrDirMountStatus,
	file:        attrFileLinkCount | attrFileAllocSize | attrFileDataLength,
	fork:        attrCmnExtCloneID | attrCmnExtExtFlags | attrCmnExtCloneRef,
}

// The attributes that an entry needs so that lstat is not necessary.
const needCommon = attrCmnDevID | attrCmnObjType | attrCmnFlags | attrCmnFileID

// entry is one directory entry with the values of lstat that the scan uses.
type entry struct {
	name      string
	dir       bool
	regular   bool
	dev       uint64
	ino       uint64
	flags     uint32
	links     uint32
	allocated int64
	logical   int64
	clone     cloneState
}

// cloneState is how a file shares its blocks with other files on APFS.
type cloneState struct {
	// full is true when all blocks of the file are shared with at least one
	// other file. These files have the same id.
	full bool
	id   uint64
	// partial is true when some blocks can be shared and the file is not a
	// full clone.
	partial bool
}

var bulkBuffers = sync.Pool{New: func() any { b := make([]byte, 32<<10); return &b }}

// read lists a directory in name order. It returns the entries it could
// read and the first error. One getattrlistbulk call reads many entries.
func read(dir string) ([]entry, error) {
	fd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: dir, Err: err}
	}
	defer syscall.Close(fd)
	bp := bulkBuffers.Get().(*[]byte)
	defer bulkBuffers.Put(bp)
	buf := *bp
	var entries []entry
	var first error
	for {
		n, _, errno := syscall.Syscall6(sysGetattrlistbulk, uintptr(fd), uintptr(unsafe.Pointer(&listAttrs)),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), fsoptCmnExtended, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			if first == nil {
				first = &os.PathError{Op: "getattrlistbulk", Path: dir, Err: errno}
			}
			break
		}
		if n == 0 {
			break
		}
		rest := buf
		for range int(n) {
			size := binary.LittleEndian.Uint32(rest)
			e, ok := parseEntry(rest[:size])
			rest = rest[size:]
			if !ok {
				// A mount point or a firmlink gives the values of the
				// directory below it, and an entry can fail alone.
				if e, err = lstatEntry(dir, e.name); err != nil {
					if first == nil && !os.IsNotExist(err) {
						first = err
					}
					continue
				}
			}
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return entries, first
}

// parseEntry reads one record of getattrlistbulk. ok is false when the
// record does not give the values that lstat gives.
func parseEntry(rec []byte) (e entry, ok bool) {
	le := binary.LittleEndian
	common, dirAttrs, fileAttrs, fork := le.Uint32(rec[4:]), le.Uint32(rec[12:]), le.Uint32(rec[16:]), le.Uint32(rec[20:])
	p := rec[24:]
	failed := false
	if common&attrCmnError != 0 {
		failed = le.Uint32(p) != 0
		p = p[4:]
	}
	// The name reference is an offset from its own position and a length
	// that includes the closing NUL.
	offset, length := int(int32(le.Uint32(p))), int(le.Uint32(p[4:]))
	e.name = string(p[offset : offset+length-1])
	p = p[8:]
	if failed || common&needCommon != needCommon {
		return e, false
	}
	// dev_t is signed; widen it as Stat_t.Dev is widened.
	e.dev = uint64(int32(le.Uint32(p)))
	kind := le.Uint32(p[4:])
	e.flags = le.Uint32(p[8:])
	e.ino = le.Uint64(p[12:])
	p = p[20:]
	e.dir, e.regular = kind == vDir, kind == vReg
	if e.flags&sfFirmlink != 0 {
		return e, false
	}
	if e.dir {
		return e, dirAttrs&attrDirMountStatus != 0 && le.Uint32(p)&dirMountPoint == 0
	}
	const needFile = attrFileLinkCount | attrFileAllocSize | attrFileDataLength
	if fileAttrs&needFile != needFile {
		return e, false
	}
	e.links = le.Uint32(p)
	e.allocated = int64(le.Uint64(p[4:]))
	e.logical = int64(le.Uint64(p[12:]))
	e.clone = parseClone(fork, p[20:])
	return e, true
}

// parseClone reads the clone ID, the extended flags and the clone reference
// count, in this order, for the bits that are set in returned.
func parseClone(returned uint32, p []byte) cloneState {
	le := binary.LittleEndian
	if returned&attrCmnExtCloneID == 0 || returned&attrCmnExtExtFlags == 0 {
		return cloneState{}
	}
	c := cloneState{id: le.Uint64(p)}
	flags := le.Uint64(p[8:])
	if returned&attrCmnExtCloneRef != 0 {
		c.full = le.Uint32(p[16:]) > 1
	} else {
		// Without the reference count, use the flag. Each full clone has it.
		c.full = flags&efSharesAllBlocks != 0
	}
	c.partial = !c.full && flags&efMayShareBlocks != 0
	return c
}

func lstatEntry(dir, name string) (entry, error) {
	info, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return entry{}, err
	}
	e := entry{name: name, dir: info.IsDir(), regular: info.Mode().IsRegular(), logical: info.Size()}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		e.dev, e.ino, e.flags, e.links = uint64(st.Dev), st.Ino, st.Flags, uint32(st.Nlink)
		e.allocated = st.Blocks * 512
	}
	return e, nil
}
