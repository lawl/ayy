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

type File struct {
	bf         BasicFile
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
		if f.currentBlockId < len(f.bf.BlockSizes) {
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
	if _, err := sqfs.reader.Seek(int64(f.bf.BlocksStart)+f.currentByteOffset, io.SeekStart); err != nil {
		return nil, err
	}
	sz := f.bf.BlockSizes[f.currentBlockId]
	f.currentBlockId++
	f.currentByteOffset += int64(sz)

	block := make([]byte, sz)
	n, err := sqfs.reader.Read(block)
	if err != nil {
		return block, err
	}
	if n != int(sz) {
		return nil, errors.New(fmt.Sprintf("read failure: n != sz -> %d != %d", n, sz))
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

	offs := (f.bf.FragmentBlockIndex / 512) * 8 // u64 = 8byte
	if _, err := sqfs.reader.Seek(int64(sqfs.superblock.FragmentTableStart)+int64(offs), io.SeekStart); err != nil {
		return nil, err
	}

	var metablockoffset uint64
	if err := binary.Read(sqfs.reader, binary.LittleEndian, &metablockoffset); err != nil {
		return nil, err
	}

	block, _, err := sqfs.readMetadataBlockSingle(metablockoffset)
	if err != nil {
		return nil, err
	}
	// FragmentBlockEntry = 16 byte.
	boffset := (f.bf.FragmentBlockIndex % 512) * 16
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
	if _, err := sqfs.reader.Read(compressedBlock); err != nil {
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
	maxReadUntil := f.bf.FileSize - uint32(f.nBytesRead)
	return uncompressedBlock[f.bf.BlockOffset : f.bf.BlockOffset+maxReadUntil], nil
}

func (f File) Close() error {
	//no op, we don't hold per file handles
	return nil
}

func (f File) Stat() (fs.FileInfo, error) {
	return fileInfoFromDirEntry(f.sqfs, f.de), nil
}

func (s *SquashFS) ReadFile(bf BasicFile) ([]byte, error) {
	r := s.reader
	buf := make([]byte, bf.FileSize)
	curOffset := 0
	if _, err := r.Seek(int64(bf.BlocksStart), io.SeekStart); err != nil {
		return nil, err
	}
	for _, sz := range bf.BlockSizes {
		block := make([]byte, sz)
		n, err := r.Read(block)
		if err != nil {
			return buf, err
		}
		if n != int(sz) {
			return nil, errors.New(fmt.Sprintf("read failure: n != sz -> %d != %d", n, sz))
		}
		// TODO, check if even compressed? or maybe make uncompress a NOP if not
		var b []byte
		if s.superblock.Flags&UncompressedData == UncompressedData {
			b = block
		} else {
			b, err = uncompress(block)
			if err != nil {
				return buf, err
			}
		}

		copy(buf[curOffset:], b)
		curOffset += len(b)

		if curOffset >= len(buf) {
			return buf, io.EOF
		}
	}

	if bf.endsInFragment() {
		//Each metadata block holds 512 FragmentBlockEntries
		noffsets := s.superblock.FragmentEntryCount / 512
		if s.superblock.FragmentEntryCount%512 != 0 {
			noffsets++
		}

		offs := (bf.FragmentBlockIndex / 512) * 8 // u64 = 8byte
		if _, err := s.reader.Seek(int64(s.superblock.FragmentTableStart)+int64(offs), io.SeekStart); err != nil {
			return nil, err
		}

		var metablockoffset uint64
		if err := binary.Read(s.reader, binary.LittleEndian, &metablockoffset); err != nil {
			return nil, err
		}

		block, _, err := s.readMetadataBlockSingle(metablockoffset)
		if err != nil {
			return nil, err
		}
		// FragmentBlockEntry = 16 byte.
		boffset := (bf.FragmentBlockIndex % 512) * 16
		blockbuf := bytes.NewBuffer(block[boffset:])
		fblock := FragmentBlockEntry{}
		if err := binary.Read(blockbuf, binary.LittleEndian, &fblock); err != nil {
			return nil, err
		}
		compressedBlock := make([]byte, fblock.Size)
		if _, err := s.reader.Seek(int64(fblock.Start), io.SeekStart); err != nil {
			return nil, err
		}
		if _, err := s.reader.Read(compressedBlock); err != nil {
			return nil, err
		}
		uncompressedBlock, err := uncompress(compressedBlock)
		if err != nil {
			return nil, err
		}

		copy(buf[curOffset:], uncompressedBlock[bf.BlockOffset:])

		curOffset += len(uncompressedBlock)
	}

	return buf, nil
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
	case []DirectoryEntry:
		f.size = 0
	case BasicSymlink:
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
