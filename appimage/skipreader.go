package appimage

import (
	"errors"
	"io"
	"sort"
)

// wraps another reader and can be configured to "skip"
// any number of bytes at any offset
// we require this to exclude certains sections in the ELF
// from being hashed. Namely the ELF sections containing
// the signature and key itself.

type Reader struct {
	skips             []skip
	currentOffset     int
	r                 io.Reader
	nullBytesToReturn int
}
type skip struct {
	offset int
	size   int
}

func (s *Reader) AddSkip(offset, size int) {
	s.skips = append(s.skips, skip{offset, size})
}

func NewSkipReader(r io.Reader) *Reader {
	sr := Reader{}
	sr.r = r

	return &sr
}

func (s *Reader) Read(p []byte) (n int, err error) {

	// sort so we only need to check the first element and after that can throw it away
	sort.SliceStable(s.skips, func(i, j int) bool {
		return s.skips[i].offset < s.skips[j].offset
	})

	// apparently, to calculate the hash, the section isn't *skipped*
	// but considered to be entirely null bytes
	if s.nullBytesToReturn > 0 {
		defer func() { s.skips = s.skips[1:] }()
		if len(p) > s.nullBytesToReturn {
			p = p[:s.nullBytesToReturn]
			//completely null out the return buffer
			for i := 0; i < len(p); i++ {
				p[i] = 0x00
			}

			junk := make([]byte, len(p))

			//drain the original reader by that many bytes
			n, err := io.ReadFull(s.r, junk)
			s.currentOffset += n
			s.nullBytesToReturn -= n
			return n, err
		}

	}

	if len(s.skips) > 0 {
		skip := s.skips[0]
		//would the read intersect any section we must skip?
		requestedN := len(p)
		if s.currentOffset+requestedN > skip.offset {
			// read up exactly to the offset by resizing the slice
			p = p[:skip.offset-s.currentOffset]
			n, err := s.r.Read(p)
			s.currentOffset += n
			if err == nil {
				//if that read didn't error, silently throw away the skip area
				if s.currentOffset == skip.offset { // sanity check, should always be true
					s.nullBytesToReturn = skip.size
				} else {
					return 0, errors.New("BUG: skip reader hit invalid code path")
				}
			}
			return n, err
		}
	}

	n, err = s.r.Read(p)
	s.currentOffset += n
	return n, err
}
