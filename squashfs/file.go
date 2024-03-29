package squashfs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"
)

//prefixed with i, because the fields are named the same
//and we only have this interface to dedupe some code
//between extended and basic file, i don't want to have
//to always to through getters/setters
type SqfsFile interface {
	iBlocksStart() uint64
	iFileSize() uint64
	iFragmentBlockIndex() uint32
	iBlockOffset() uint32
	iBlockSizes() []uint32
	endsInFragment() bool
}

func (bf BasicFile) iBlocksStart() uint64        { return uint64(bf.BlocksStart) }
func (bf BasicFile) iFileSize() uint64           { return uint64(bf.FileSize) }
func (bf BasicFile) iFragmentBlockIndex() uint32 { return bf.FragmentBlockIndex }
func (bf BasicFile) iBlockOffset() uint32        { return bf.BlockOffset }
func (bf BasicFile) iBlockSizes() []uint32       { return bf.BlockSizes }

func (bf ExtendedFile) iBlocksStart() uint64        { return bf.BlocksStart }
func (bf ExtendedFile) iFileSize() uint64           { return bf.FileSize }
func (bf ExtendedFile) iFragmentBlockIndex() uint32 { return bf.FragmentBlockIndex }
func (bf ExtendedFile) iBlockOffset() uint32        { return bf.BlockOffset }
func (bf ExtendedFile) iBlockSizes() []uint32       { return bf.BlockSizes }

type File struct {
	bf         SqfsFile
	de         DirectoryEntry
	sqfs       *SquashFS
	databuffer []byte

	currentBlockId    int
	currentByteOffset int64
	haveReadFragment  bool
	nBytesRead        int
}

func fileFromBasicFile(s *SquashFS, bf BasicFile, de DirectoryEntry) *File {
	file := File{}
	file.bf = bf
	file.de = de
	file.sqfs = s

	return &file
}
func fileFromExtendedFile(s *SquashFS, bf ExtendedFile, de DirectoryEntry) *File {

	file := File{}
	file.bf = bf
	file.de = de
	file.sqfs = s

	return &file
}
func (f *File) Read(buf []byte) (int, error) {
	// go expects that we can just read n bytes from a stream here
	// but we deal with compressed blocks internally
	// and the last block may be a special case fragment
	// so this is kind of a headache for us.
	// build a state machine that keep tracks of which block we read
	// into a buffer, and then consume data from that buffer
	// once the buffer is empty, read the next block

	if len(f.databuffer) == 0 {
		var err error
		if f.currentBlockId < len(f.bf.iBlockSizes()) {
			f.databuffer, err = readBlock(f)
			if err != nil {
				return 0, err
			}
		} else if f.bf.endsInFragment() && !f.haveReadFragment {
			f.databuffer, err = readFragment(f)
			if err != nil {
				return 0, err
			}
		} else {
			return 0, io.EOF
		}
	}

	n := copy(buf, f.databuffer)
	f.databuffer = f.databuffer[n:]
	f.nBytesRead += n
	return n, nil
}

func readBlock(f *File) ([]byte, error) {
	sqfs := f.sqfs
	if _, err := sqfs.reader.Seek(int64(f.bf.iBlocksStart())+f.currentByteOffset, io.SeekStart); err != nil {
		return nil, err
	}
	sz := f.bf.iBlockSizes()[f.currentBlockId]
	f.currentBlockId++
	f.currentByteOffset += int64(sz)

	block := make([]byte, sz)
	_, err := io.ReadFull(sqfs.reader, block)
	if err != nil {
		return block, err
	}

	// TODO, check if even compressed? or maybe make uncompress a NOP if not
	b, err := uncompress(block)
	if err != nil {
		return block, err
	}

	return b, nil
}

func readFragment(f *File) ([]byte, error) {
	sqfs := f.sqfs

	if sqfs.superblock.Flags&NoFragments == NoFragments {
		fmt.Fprint(os.Stderr, "Superblock sais fragments aren't used. But File inode sais otherwise. idk man, kinda sus. Fightning the power and ignoring superblock!")
	}

	noffsets := sqfs.superblock.FragmentEntryCount / 512
	if sqfs.superblock.FragmentEntryCount%512 != 0 {
		noffsets++
	}

	offs := (f.bf.iFragmentBlockIndex() / 512) * 8 // u64 = 8byte
	if _, err := sqfs.reader.Seek(int64(sqfs.superblock.FragmentTableStart)+int64(offs), io.SeekStart); err != nil {
		return nil, err
	}

	var metablockoffset uint64
	if err := binary.Read(sqfs.reader, binary.LittleEndian, &metablockoffset); err != nil {
		return nil, err
	}

	block, _, err := sqfs.readOneMetaBlock(metablockoffset)
	if err != nil {
		return nil, err
	}
	// FragmentBlockEntry = 16 byte.
	boffset := (f.bf.iFragmentBlockIndex() % 512) * 16
	blockbuf := bytes.NewBuffer(block[boffset:])
	fblock := FragmentBlockEntry{}
	if err := binary.Read(blockbuf, binary.LittleEndian, &fblock); err != nil {
		return nil, err
	}

	size := fblock.Size & 0xFFFFFF
	isUncompressed := fblock.Size&0x1000000 != 0

	compressedBlock := make([]byte, size)
	if _, err := sqfs.reader.Seek(int64(fblock.Start), io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(sqfs.reader, compressedBlock); err != nil {
		return nil, err
	}
	var uncompressedBlock []byte
	if isUncompressed {
		uncompressedBlock = compressedBlock
	} else {
		uncompressedBlock, err = uncompress(compressedBlock)
		if err != nil {
			return nil, err
		}
	}
	f.haveReadFragment = true
	maxReadUntil := f.bf.iFileSize() - uint64(f.nBytesRead)
	return uncompressedBlock[f.bf.iBlockOffset() : uint64(f.bf.iBlockOffset())+maxReadUntil], nil
}

func (f File) Close() error {
	//no op, we don't hold per file handles
	return nil
}

func (f File) Stat() (fs.FileInfo, error) {
	return fileInfoFromDirEntry(f.sqfs, f.de), nil
}

type FileInfo struct {
	sqfs       *SquashFS
	dirEntry   DirectoryEntry
	statcalled bool //we lazily call stat and cache results. everything below this line.
	modTime    time.Time
	mode       uint32
	size       int64
	uid        uint16
	gid        uint16

	symlinkTarget     string
	symlinkTargetSize int64
}

// the default file info struct doesn't contain uid/gid, or info about symlinks
// and possibly other things. However Sys() let's us return anything.
// So I suppose callers will have to call Sys()
// and then dynamically check if it implements the SquashInfo interface.
type SquashInfo interface {
	Uid() uint16
	Gid() uint16
	SymlinkTarget() string
}

func fileInfoFromDirEntry(sqfs *SquashFS, d DirectoryEntry) FileInfo {
	fileInfo := FileInfo{}
	fileInfo.dirEntry = d
	fileInfo.sqfs = sqfs
	return fileInfo
}

func (f FileInfo) ModTime() time.Time {
	f.stat()
	return f.modTime
}

func (f FileInfo) Mode() fs.FileMode {
	f.stat()
	return fs.FileMode(f.mode)
}
func (f FileInfo) Size() int64 {
	f.stat()
	return f.size
}
func (f FileInfo) Sys() any {
	return f
}
func (f FileInfo) Uid() uint16 {
	f.stat()
	return f.uid
}
func (f FileInfo) Gid() uint16 {
	f.stat()
	return f.gid
}

func (f FileInfo) SymlinkTarget() string {
	f.stat()
	return f.symlinkTarget
}
func (f FileInfo) IsDir() bool {
	return f.dirEntry.dtype == tBasicDirectory || f.dirEntry.dtype == tExtendedDirectory
}

func (f FileInfo) Name() string {
	return f.dirEntry.name
}
func (f *FileInfo) stat() error {
	if f.statcalled {
		return nil
	}
	f.statcalled = true

	sqfs := f.sqfs
	header, inode, err := sqfs.readInode(uint64(f.dirEntry.InodeNumber), uint64(f.dirEntry.Offset), uint64(f.dirEntry.Start))
	if err != nil {
		return err
	}
	f.modTime = time.Unix(int64(header.ModifiedTime), 0)
	f.mode = uint32(f.dirEntry.Type()) | uint32(header.Permissions)

	f.uid = header.UidIdx
	f.gid = header.GidIdx

	switch node := inode.(type) {
	case BasicFile:
		f.size = int64(node.FileSize)
	case ExtendedFile:
		f.size = int64(node.FileSize)
	case Directory:
		f.size = 0
	case BasicSymlink:
		f.size = 0
		f.symlinkTarget = node.TargetPath
		f.symlinkTargetSize = int64(node.TargetSize)
	case ExtendedSymlink:
		f.size = 0
		f.symlinkTarget = node.TargetPath
		f.symlinkTargetSize = int64(node.TargetSize)
	default:
		return unimplemented(fmt.Sprintf("unhandled type %T in stat()", node))
	}

	return nil
}

func (d DirectoryEntry) Name() string {
	return d.name
}

func (d DirectoryEntry) IsDir() bool {
	return d.dtype == tBasicDirectory || d.dtype == tExtendedDirectory
}

func (d DirectoryEntry) Type() fs.FileMode {
	var mode fs.FileMode

	switch d.dtype {
	case tBasicDirectory, tExtendedDirectory:
		mode |= fs.ModeDir
	case tBasicSymlink, tExtendedSymlink:
		mode |= fs.ModeSymlink
	case tBasicFifo, tExtendedFifo:
		mode |= fs.ModeNamedPipe
	case tBasicSocket, tExtendedSocket:
		mode |= fs.ModeSocket
	case tBasicBlockDevice, tExtendedBlockDevice:
		mode |= fs.ModeDevice
	case tBasicCharDevice, tExtendedCharDevice:
		mode |= fs.ModeCharDevice
	default:
		mode |= fs.ModeIrregular
	}

	return mode.Type()
}
func (d DirectoryEntry) Info() (fs.FileInfo, error) {
	return fileInfoFromDirEntry(d.sqfs, d), nil
}

func (d Directory) Close() error {
	return nil //noop
}
func (d Directory) Read(p []byte) (int, error) {
	err := fs.PathError{}
	err.Op = "read"
	err.Err = errors.New("cannot Read() directory")
	return 0, &err
}
func (d Directory) Stat() (fs.FileInfo, error) {
	return DirInfo{dir: d, entry: d.pointingEntry}, nil
}

type DirInfo struct {
	dir   Directory
	entry DirectoryEntry
	// the entry pointing at this directory
	// the directory itself doesn't have a name and is just an inode
	// so i think multiple entries could point to the same directory
	// inode with different names, and the only way to have a name
	// is to keep track of which DirectoryEntry pointed to this directory
	// when the path was resolved
}

func (d DirInfo) Name() string {
	return d.dir.pointingEntry.name
}
func (d DirInfo) Size() int64 {
	return 0
}
func (d DirInfo) Mode() fs.FileMode {
	return d.dir.pointingEntry.Type()
}
func (d DirInfo) ModTime() time.Time {
	// directories have no mod time on the structs?!
	// superblock does though
	return time.Unix(int64(d.dir.pointingEntry.sqfs.superblock.ModificationTime), 0)
}
func (d DirInfo) IsDir() bool {
	return true
}
func (d DirInfo) Sys() any {
	return nil
}
