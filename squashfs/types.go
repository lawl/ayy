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

const UncompressedInodes = 0x0001
const UncompressedData = 0x0002
const Check = 0x0004
const UncompressedFragments = 0x0008
const NoFragments = 0x0010
const AlwaysFragments = 0x0020
const Duplicates = 0x0040
const Exportable = 0x0080
const UncompressedXAttrs = 0x0100
const NoXAttrs = 0x0200
const CompressorOptions = 0x0400
const UncompressedIds = 0x0800

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
	BlockIdx          uint32
	HardLinkCount     uint32
	FileSize          uint16
	BlockOffset       uint16
	ParentInodeNumber uint32
}

type FragmentBlockEntry struct {
	Start  uint64
	Size   uint32
	Unused uint32
}
