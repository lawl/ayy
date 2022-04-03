package squashfs

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
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

	for _, entry := range dir {
		if entry.name == filename {
			_, iNode, err := s.readInode(uint64(entry.InodeNumber), uint64(entry.Offset), uint64(entry.Start))
			if err != nil {
				return nil, err
			}
			switch node := iNode.(type) {
			case BasicFile:
				f := fileFromBasicFile(s, node, entry)
				return f, nil
			case BasicSymlink:
				return s.Open(node.TargetPath)
			default:
				return nil, unimplemented(fmt.Sprintf("expected file to open() to be BasicFile or BasicSymlink, is %T", iNode))
			}

		}
	}

	return nil, &retErr
}
func resolveDirectory(s *SquashFS, dirname string) ([]DirectoryEntry, error) {
	pathFragments := strings.Split(dirname, string(os.PathSeparator))
	dir, err := s.rootDir()
	if err != nil {
		return nil, err
	}
	var parent DirectoryEntry
	parent = dir[0] // "." of rootDir()

	for _, f := range pathFragments {
		for _, entry := range dir {
			if entry.name == f {
				_, dirInode, err := s.readInode(uint64(entry.InodeNumber), uint64(entry.Offset), uint64(entry.Start))
				if err != nil {
					return nil, err
				}
				this := entry
				this.name = "."
				parent.name = ".."
				var dirList []DirectoryEntry
				dirList = append(dirList, this)
				dirList = append(dirList, parent)
				parent = entry

				tmpDir, ok := dirInode.([]DirectoryEntry)

				if !ok {
					var retErr fs.PathError
					retErr.Op = "open"
					retErr.Err = errors.New(fmt.Sprintf("Inode is not a list of DirectoryEntry but %T", dirInode))
					return nil, &retErr
				}
				dirList = append(dirList, tmpDir...)
				dir = dirList
			}
		}
	}
	return dir, nil
}
func (s *SquashFS) rootDir() ([]DirectoryEntry, error) {
	h, entries, err := s.readInode(
		s.superblock.RootInodeRef,
		(s.superblock.RootInodeRef & 0xFFFF),
		(s.superblock.RootInodeRef&0xFFFF0000)>>16)
	if err != nil {
		return nil, err
	}
	de, ok := entries.([]DirectoryEntry)
	if !ok {
		return nil, errors.New("read inode, expected directory entry got something else")
	}
	var dirList []DirectoryEntry
	this := DirectoryEntry{
		dtype:       h.InodeType,
		name:        ".",
		sqfs:        s,
		InodeNumber: h.InodeNumber,
		Offset:      uint16(s.superblock.RootInodeRef & 0xFFFF),
		Start:       uint32(s.superblock.RootInodeRef&0xFFFF0000) >> 16}
	dirList = append(dirList, this)
	parent := this
	parent.name = ".."
	dirList = append(dirList, parent)
	dirList = append(dirList, de...)
	return dirList, nil
}

func (s *SquashFS) ReadDir(name string) ([]fs.DirEntry, error) {
	dir, err := resolveDirectory(s, name)
	if err != nil {
		return nil, err
	}

	result := make([]fs.DirEntry, len(dir))
	for i := 0; i < len(result); i++ {
		result[i] = dir[i]
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

		de, err := s.readDirectoryTable(dir)
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

	default:
		return inodeHeader, nil, unimplemented(fmt.Sprintf("Unhandled inode type: %d\n", inodeHeader.InodeType))
	}
}

func (s *SquashFS) readDirectoryTable(dir BasicDirectory) ([]DirectoryEntry, error) {

	superblock := s.superblock

	blockidx := dir.BlockIdx
	blockoffs := dir.BlockOffset
	size := int(dir.FileSize)

	directories := make([]DirectoryEntry, 0)

	block, err := s.readMetadataBlock(superblock.DirectoryTableStart + uint64(blockidx))
	if err != nil {
		panic(err)
	}

	// The extra 3 bytes are for a virtual "." and ".." item in each directory which is
	// not written, but can be considered to be part of the logical size of the directory.
	bytesRead := 3

	blockbuf := bytes.NewBuffer(block[blockoffs:])
	for {
		len1 := blockbuf.Len()
		dirHeader := DirectoryHeader{}
		err = binary.Read(blockbuf, binary.LittleEndian, &dirHeader)

		for i := 0; i < int(dirHeader.Count+1); i++ {
			tmp := struct {
				Offset      uint16
				InodeOffset int16
				Type        uint16
				NameSize    uint16
			}{}
			if err := binary.Read(blockbuf, binary.LittleEndian, &tmp); err != nil {
				return nil, err
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
				return nil, err
			}

			dirEntry.name = string(str)
			directories = append(directories, dirEntry)
		}

		len2 := blockbuf.Len()

		bytesRead = len1 - len2 + bytesRead
		if !(bytesRead < size) {
			break
		}
	}

	return directories, nil

}
func (s *SquashFS) readMetadataBlock(off uint64) ([]byte, error) {
	data1, compressedLen, err := s.readMetadataBlockSingle(off)
	if err != nil {
		return nil, err
	}
	data2, _, err := s.readMetadataBlockSingle(off + uint64(compressedLen))
	if err != nil {
		return nil, err
	}

	ret := make([]byte, len(data1)+len(data2))
	copy(ret, data1)
	copy(ret[len(data1):], data2)

	return ret, nil
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
	res.ReadFrom(r)

	return res.Bytes(), nil
}
