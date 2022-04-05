package squashfs

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

type SquashFS struct {
	reader     *io.SectionReader
	superblock Superblock
}

type Superblock struct {
	Magic               uint32
	InodeCount          uint32
	ModificationTime    uint32
	BlockSize           uint32
	FragmentEntryCount  uint32
	CompressionId       uint16
	BlockLog            uint16
	Flags               uint16
	IdCount             uint16
	VersionMajor        uint16
	VersionMinor        uint16
	RootInodeRef        uint64
	BytesUsed           uint64
	IdTableStart        uint64
	XattrIdTableStart   uint64
	InodeTableStart     uint64
	DirectoryTableStart uint64
	FragmentTableStart  uint64
	ExportTableStart    uint64
}

type unimplementedError struct {
	msg string
}

func (e unimplementedError) Error() string {
	return e.msg
}

func unimplemented(msg string) error {
	fmt.Fprintf(os.Stderr, "Called unimplemented thing '%s'\n", msg)
	debug.PrintStack()
	return unimplementedError{msg: "Unimplemented: " + msg}
}

const (
	UncompressedInodes    = 0x0001
	UncompressedData      = 0x0002
	Check                 = 0x0004
	UncompressedFragments = 0x0008
	NoFragments           = 0x0010
	AlwaysFragments       = 0x0020
	Duplicates            = 0x0040
	Exportable            = 0x0080
	UncompressedXAttrs    = 0x0100
	NoXAttrs              = 0x0200
	CompressorOptions     = 0x0400
	UncompressedIds       = 0x0800
)

const (
	tNone = iota
	tBasicDirectory
	tBasicFile
	tBasicSymlink
	tBasicBlockDevice
	tBasicCharDevice
	tBasicFifo
	tBasicSocket
	tExtendedDirectory
	tExtendedFile
	tExtendedSymlink
	tExtendedBlockDevice
	tExtendedCharDevice
	tExtendedFifo
	tExtendedSocket
)

type InodeHeader struct {
	InodeType    uint16
	Permissions  uint16
	UidIdx       uint16
	GidIdx       uint16
	ModifiedTime uint32
	InodeNumber  uint32
}

type DirectoryHeader struct {
	Count       uint32
	Start       uint32
	InodeNumber uint32
}

type DirectoryEntry struct {
	Offset      uint16
	InodeOffset int16
	InodeNumber uint32
	Start       uint32
	dtype       uint16
	NameSize    uint16
	name        string
	sqfs        *SquashFS
}

type BasicSymlink struct {
	HardLinkCount uint32
	TargetSize    uint32
	TargetPath    string
}

type BasicFile struct {
	BlocksStart        uint32
	FragmentBlockIndex uint32
	BlockOffset        uint32
	FileSize           uint32
	BlockSizes         []uint32
}

func (f BasicFile) endsInFragment() bool {
	return f.FragmentBlockIndex != 0xFFFFFFFF
}

type BasicDirectory struct {
	BlockStart        uint32
	HardLinkCount     uint32
	FileSize          uint16
	BlockOffset       uint16
	ParentInodeNumber uint32
}

type ExtendedDirectory struct {
	HardLinkCount     uint32
	FileSize          uint32
	BlockStart        uint32
	ParentInodeNumber uint32
	IndexCount        uint16
	BlockOffset       uint16
	XattrIdx          uint32
	Index             []DirectoryIndex
}

type DirectoryIndex struct {
	Index    uint32
	Start    uint32
	NameSize uint32
	Name     string
}

type FragmentBlockEntry struct {
	Start  uint64
	Size   uint32
	Unused uint32
}

type ExtendedFile struct {
	BlocksStart        uint64
	FileSize           uint64
	Sparse             uint64
	HardLinkCount      uint32
	FragmentBlockIndex uint32
	BlockOffset        uint32
	XattrIdx           uint32
	BlockSizes         []uint32
}

func (f ExtendedFile) endsInFragment() bool {
	return f.FragmentBlockIndex != 0xFFFFFFFF
}

type ExtendedSymlink struct {
	HardLinkCount uint32
	TargetSize    uint32
	TargetPath    string
	XattrIdx      uint32
}
