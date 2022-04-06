package squashfs

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// docs: https://dr-emann.github.io/squashfs

const maxBlockSz = ^uint16(1 << 15)

func New(reader *io.SectionReader) (*SquashFS, error) {

	sqfs := SquashFS{}

	superblock := Superblock{}
	if err := binary.Read(reader, binary.LittleEndian, &superblock); err != nil {
		return nil, err
	}

	if superblock.Magic != 0x73717368 {
		return nil, errors.New("not a squashfs archive, magic bytes dont match")
	}
	if superblock.CompressionId != 1 {
		return nil, unimplemented("compression type other than gzip")
	}
	if log2(superblock.BlockSize) != uint32(superblock.BlockLog) {
		return nil, errors.New("Corrupt archive: BlogLog does not match log2(BlockSize)")
	}
	if superblock.VersionMajor != 4 || superblock.VersionMinor != 0 {
		return nil, errors.New(fmt.Sprintf("SquashFS archive is not version 4.0, is: %d.%d", superblock.VersionMajor, superblock.VersionMinor))
	}
	if superblock.Flags&CompressorOptions == CompressorOptions {
		return nil, unimplemented("CompressorOptions")
	}
	if superblock.Flags&UncompressedInodes == UncompressedInodes {
		return nil, unimplemented("UncompressedInodes")
	}
	if superblock.Flags&UncompressedData == UncompressedData {
		return nil, unimplemented("UncompressedData")
	}
	if superblock.Flags&UncompressedFragments == UncompressedFragments {
		return nil, unimplemented("UncompressedData")
	}

	sqfs.reader = reader
	sqfs.superblock = superblock
	return &sqfs, nil
}

func log2(num uint32) uint32 {
	var n uint32

	for ; num != 0; num >>= 1 {
		n++
	}
	n--
	return n
}

func (s *SquashFS) Open(name string) (fs.File, error) {
	var retErr fs.PathError
	retErr.Op = "open"
	if !fs.ValidPath(name) {
		retErr.Err = fs.ErrInvalid
		return nil, &retErr
	}

	dirname, filename := path.Split(name)

	dir, err := resolveDirectory(s, dirname)
	if err != nil {
		return nil, err
	}

	for _, entry := range dir.entries {
		if entry.name == filename {
			_, iNode, err := s.readInode(uint64(entry.InodeNumber), uint64(entry.Offset), uint64(entry.Start))
			if err != nil {
				return nil, err
			}
			switch node := iNode.(type) {
			case BasicFile:
				f := fileFromBasicFile(s, node, entry)
				return f, nil
				//the extended info of files isn't actually read yet
				//but we read the basic info from extended files/dirs
			case ExtendedFile:
				f := fileFromExtendedFile(s, node, entry)
				return f, nil
			case BasicSymlink:
				target := node.TargetPath
				if !filepath.IsAbs(node.TargetPath) {
					target = filepath.Join(dirname, node.TargetPath)
				}
				return s.Open(target)
			case ExtendedSymlink:
				target := node.TargetPath
				if !filepath.IsAbs(node.TargetPath) {
					target = filepath.Join(dirname, node.TargetPath)
				}
				return s.Open(target)
			case Directory:
				return node, nil
			default:
				return nil, unimplemented(fmt.Sprintf("expected file to open() to be BasicFile or BasicSymlink, is %T", iNode))
			}

		}
	}

	return nil, &retErr
}
func resolveDirectory(s *SquashFS, dirname string) (Directory, error) {
	pathFragments := strings.Split(dirname, string(os.PathSeparator))
	dir, err := s.rootDir()
	if err != nil {
		return dir, err
	}
	var parent DirectoryEntry
	parent = dir.entries[0] // "." of rootDir()

	for _, f := range pathFragments {
		for _, entry := range dir.entries {
			if entry.name == f {
				_, dirInode, err := s.readInode(uint64(entry.InodeNumber), uint64(entry.Offset), uint64(entry.Start))
				if err != nil {
					return dir, err
				}
				this := entry
				this.name = "."
				parent.name = ".."
				var dirList []DirectoryEntry
				dirList = append(dirList, this)
				dirList = append(dirList, parent)
				parent = entry

				tmpDir, ok := dirInode.(Directory)

				if !ok {
					var retErr fs.PathError
					retErr.Op = "open"
					retErr.Err = errors.New(fmt.Sprintf("Inode is not a list of DirectoryEntry but %T", dirInode))
					return dir, &retErr
				}
				dir.entries = append(dirList, tmpDir.entries...)
			}
		}
	}
	return dir, nil
}
func (s *SquashFS) rootDir() (Directory, error) {

	h, entries, err := s.readInode(
		s.superblock.RootInodeRef,
		(s.superblock.RootInodeRef & 0xFFFF),
		(s.superblock.RootInodeRef&0xFFFFFFFF0000)>>16)
	if err != nil {
		return Directory{}, err
	}
	de, ok := entries.(Directory)
	if !ok {
		return de, errors.New("read inode, expected directory entry got something else")
	}
	var dirList []DirectoryEntry
	this := DirectoryEntry{
		dtype:       h.InodeType,
		name:        ".",
		sqfs:        s,
		InodeNumber: h.InodeNumber,
		Offset:      uint16(s.superblock.RootInodeRef & 0xFFFF),
		Start:       uint32((s.superblock.RootInodeRef & 0xFFFFFFFF0000) >> 16)}
	dirList = append(dirList, this)
	parent := this
	parent.name = ".."
	dirList = append(dirList, parent)
	de.entries = append(dirList, de.entries...)
	return de, nil
}

func (s *SquashFS) ReadDir(name string) ([]fs.DirEntry, error) {
	dir, err := resolveDirectory(s, name)
	if err != nil {
		return nil, err
	}

	result := make([]fs.DirEntry, len(dir.entries))
	for i := 0; i < len(result); i++ {
		result[i] = dir.entries[i]
	}
	return result, nil
}

func (s *SquashFS) readInode(inodeRef uint64, offset uint64, start uint64) (InodeHeader, any, error) {
	inodeHeader := InodeHeader{}
	superblock := s.superblock

	block, err := s.readMetadataBlock(superblock.InodeTableStart + start)
	if err != nil {
		return inodeHeader, nil, err
	}
	blockbuf := bytes.NewBuffer(block[offset:])

	if err := binary.Read(blockbuf, binary.LittleEndian, &inodeHeader); err != nil {
		return inodeHeader, nil, err
	}
	switch inodeHeader.InodeType {
	case tBasicDirectory:
		dir := BasicDirectory{}
		if err := binary.Read(blockbuf, binary.LittleEndian, &dir); err != nil {
			return inodeHeader, nil, err
		}

		de, err := readDirectoryTable(s, dir, dir.BlockStart, dir.BlockOffset, uint32(dir.FileSize))
		return inodeHeader, de, err
	case tBasicFile:
		bfile := BasicFile{}
		if err := binary.Read(blockbuf, binary.LittleEndian, &bfile.BlocksStart); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &bfile.FragmentBlockIndex); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &bfile.BlockOffset); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &bfile.FileSize); err != nil {
			return inodeHeader, nil, err
		}

		blkSzCount := bfile.FileSize / superblock.BlockSize

		if !bfile.endsInFragment() { // does this file NOT end in a fragment?
			if bfile.FileSize%superblock.BlockSize != 0 {
				blkSzCount++ // round up
			}
		}
		bfile.BlockSizes = make([]uint32, blkSzCount)
		if err := binary.Read(blockbuf, binary.LittleEndian, &bfile.BlockSizes); err != nil {
			return inodeHeader, nil, err
		}
		return inodeHeader, bfile, nil
	case tBasicSymlink:
		var hardLinkCount uint32
		var targetSize uint32
		if err := binary.Read(blockbuf, binary.LittleEndian, &hardLinkCount); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &targetSize); err != nil {
			return inodeHeader, nil, err
		}
		str := make([]byte, targetSize)
		if err := binary.Read(blockbuf, binary.LittleEndian, &str); err != nil {
			return inodeHeader, nil, err
		}
		symlink := BasicSymlink{HardLinkCount: hardLinkCount, TargetSize: targetSize, TargetPath: string(str)}
		return inodeHeader, symlink, nil
	case tExtendedDirectory:
		tmp := struct {
			HardLinkCount     uint32
			FileSize          uint32
			BlockStart        uint32
			ParentInodeNumber uint32
			IndexCount        uint16
			BlockOffset       uint16
			XattrIdx          uint32
		}{}
		dir := ExtendedDirectory{}
		if err := binary.Read(blockbuf, binary.LittleEndian, &tmp); err != nil {
			return inodeHeader, nil, err
		}
		dir.HardLinkCount = tmp.HardLinkCount
		dir.FileSize = tmp.FileSize
		dir.BlockStart = tmp.BlockStart
		dir.ParentInodeNumber = tmp.ParentInodeNumber
		dir.IndexCount = tmp.IndexCount
		dir.BlockOffset = tmp.BlockOffset
		dir.XattrIdx = tmp.XattrIdx
		dir.Index = make([]DirectoryIndex, dir.IndexCount)
		for i := range dir.Index {
			tmp := struct {
				Index    uint32
				Start    uint32
				NameSize uint32
			}{}

			if err := binary.Read(blockbuf, binary.LittleEndian, &tmp); err != nil {
				return inodeHeader, nil, err
			}
			dir.Index[i].Index = tmp.Index
			dir.Index[i].Start = tmp.Start
			dir.Index[i].NameSize = tmp.NameSize

			str := make([]byte, tmp.NameSize+1)

			if err := binary.Read(blockbuf, binary.LittleEndian, &str); err != nil {
				return inodeHeader, nil, err
			}

			dir.Index[i].Name = string(str)

		}
		de, err := readDirectoryTable(s, dir, dir.BlockStart, dir.BlockOffset, dir.FileSize)

		return inodeHeader, de, err

		// so annoying
		// i don't know how to dedupe with code with BasicFile
		// because the fields are in different order
		// but we can't just pass the full struct, because fields
		// early in the struct are required to calculate the sizes later in the struct
		// anything i can think of to dedupe this would probably end up being *more*
		// complicated than living with the super repetitive code all over this file
	case tExtendedFile:
		extfile := ExtendedFile{}
		if err := binary.Read(blockbuf, binary.LittleEndian, &extfile.BlocksStart); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &extfile.FileSize); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &extfile.Sparse); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &extfile.HardLinkCount); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &extfile.FragmentBlockIndex); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &extfile.BlockOffset); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &extfile.XattrIdx); err != nil {
			return inodeHeader, nil, err
		}

		blkSzCount := extfile.FileSize / uint64(superblock.BlockSize)

		if !extfile.endsInFragment() { // does this file NOT end in a fragment?
			if extfile.FileSize%uint64(superblock.BlockSize) != 0 {
				blkSzCount++ // round up
			}
		}
		extfile.BlockSizes = make([]uint32, blkSzCount)
		if err := binary.Read(blockbuf, binary.LittleEndian, &extfile.BlockSizes); err != nil {
			return inodeHeader, nil, err
		}
		return inodeHeader, extfile, nil
	case tExtendedSymlink:
		var hardLinkCount uint32
		var targetSize uint32
		if err := binary.Read(blockbuf, binary.LittleEndian, &hardLinkCount); err != nil {
			return inodeHeader, nil, err
		}
		if err := binary.Read(blockbuf, binary.LittleEndian, &targetSize); err != nil {
			return inodeHeader, nil, err
		}
		str := make([]byte, targetSize)
		if err := binary.Read(blockbuf, binary.LittleEndian, &str); err != nil {
			return inodeHeader, nil, err
		}
		var xattridx uint32
		if err := binary.Read(blockbuf, binary.LittleEndian, &xattridx); err != nil {
			return inodeHeader, nil, err
		}
		symlink := ExtendedSymlink{HardLinkCount: hardLinkCount, TargetSize: targetSize, TargetPath: string(str), XattrIdx: xattridx}
		return inodeHeader, symlink, nil
	default:
		return inodeHeader, nil, unimplemented(fmt.Sprintf("Unhandled inode type: %d\n", inodeHeader.InodeType))
	}
}

// Questionable use of generics?
// Seems kind of difficult to do this without having
// to copy paste the method otherwise
// unfortunately we have to pass blockstart, blockoffs, and filesize
// again now even though they'd be on dir, because it's now T
// could probably remove that with an interface?
func readDirectoryTable[T BasicDirectory | ExtendedDirectory](
	s *SquashFS, dir T, blockstart uint32, blockoffs uint16, fileSize uint32) (Directory, error) {

	superblock := s.superblock

	directories := make([]DirectoryEntry, 0)
	dirr := Directory{}

	block, err := s.readMetadataBlock(superblock.DirectoryTableStart + uint64(blockstart))
	if err != nil {
		return Directory{}, err
	}

	// The extra 3 bytes are for a virtual "." and ".." item in each directory which is
	// not written, but can be considered to be part of the logical size of the directory.
	bytesRead := 3

	blockbuf := bytes.NewBuffer(block[blockoffs:])
	for {
		len1 := blockbuf.Len()
		dirHeader := DirectoryHeader{}
		err = binary.Read(blockbuf, binary.LittleEndian, &dirHeader)
		dirr.header = dirHeader

		for i := 0; i < int(dirHeader.Count+1); i++ {
			tmp := struct {
				Offset      uint16
				InodeOffset int16
				Type        uint16
				NameSize    uint16
			}{}
			if err := binary.Read(blockbuf, binary.LittleEndian, &tmp); err != nil {
				return Directory{}, err
			}

			dirEntry := DirectoryEntry{}
			dirEntry.sqfs = s
			dirEntry.Offset = tmp.Offset
			dirEntry.Start = dirHeader.Start
			dirEntry.InodeOffset = tmp.InodeOffset
			dirEntry.InodeNumber = uint32(tmp.InodeOffset) + dirHeader.InodeNumber
			dirEntry.dtype = tmp.Type
			dirEntry.NameSize = tmp.NameSize

			str := make([]byte, dirEntry.NameSize+1)

			if err := binary.Read(blockbuf, binary.LittleEndian, &str); err != nil {
				return Directory{}, err
			}

			dirEntry.name = string(str)
			directories = append(directories, dirEntry)
		}

		len2 := blockbuf.Len()

		bytesRead = len1 - len2 + bytesRead
		if !(bytesRead < int(fileSize)) {
			break
		}
	}

	dirr.entries = directories

	return dirr, nil

}

type blockReader struct {
	curOffset uint64
	datablock []byte
	s         *SquashFS
}

func newBlockReader(s *SquashFS, start, offset uint64) (io.Reader, error) {
	reader := blockReader{
		curOffset: start,
		s:         s,
	}

	_, err := io.CopyN(ioutil.Discard, reader, int64(offset))
	if err != nil {
		return nil, err
	}

	return reader, nil
}

func (br blockReader) Read(p []byte) (int, error) {
	if len(br.datablock) == 0 {
		data, disksz, err := br.s.readMetadataBlockSingle(br.curOffset)
		if err != nil {
			return 0, err
		}
		br.curOffset += uint64(disksz)
		br.datablock = data
	}
	n := copy(p, br.datablock)
	br.datablock = br.datablock[n:]

	return n, nil
}

func (s *SquashFS) readMetadataBlock(off uint64) ([]byte, error) {

	r, err := newBlockReader(s, off, 0)
	var buf = make([]byte, 1024*100)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (s *SquashFS) readMetadataBlockSingle(off uint64) ([]byte, int, error) {
	r := s.reader

	r.Seek(int64(off), io.SeekStart)
	var header uint16
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return nil, 0, err
	}

	isUncompressed := header&(1<<15) != 0
	blockSz := header & maxBlockSz

	data := make([]byte, blockSz)
	ret := &data

	if err := binary.Read(r, binary.LittleEndian, &data); err != nil {
		return nil, 0, err
	}

	if !isUncompressed {
		inflated, err := uncompress(data)
		if err != nil {
			return nil, 0, err
		}
		ret = &inflated
	}

	return *ret, len(data) + 2, nil

}

func uncompress(b []byte) ([]byte, error) {
	buf := bytes.NewBuffer(b)
	r, err := zlib.NewReader(buf)
	if err != nil {
		return nil, err
	}
	var res bytes.Buffer
	_, err = res.ReadFrom(r)
	if err != nil {
		return nil, err
	}
	return res.Bytes(), nil
}
