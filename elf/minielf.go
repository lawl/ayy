package elf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

//just enough ELF parsing to deal with appimages

type File struct {
	osFile *os.File
	ELF64Header
	endianness binary.ByteOrder
	sections   []Section
}

func Open(file *os.File) (*File, error) {
	//EI_MAG0..EI_MAG3 <- 4 bytes
	//EI_CLASS         <- 1 byte
	//EI_DATA         <- 1 byte
	//we need the CLASS to know if it's
	//32 or 64 bit, as the remaining sizes in the header
	//depend on that
	//and we need DATA to know the endianness

	f := File{}
	f.osFile = file

	buf := make([]byte, 6)

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	_, err := io.ReadFull(file, buf)
	if err != nil {
		return nil, err
	}
	if buf[0] != 0x7F || buf[1] != 'E' || buf[2] != 'L' || buf[3] != 'F' {
		return nil, errors.New("Not an ELF file, invalid magic bytes")
	}
	// peeked 5 bytes, stitch back together a full stream
	breader := bytes.NewReader(buf)
	fullfile := io.MultiReader(breader, file)

	f.endianness = binary.LittleEndian
	if buf[5] == 2 {
		f.endianness = binary.BigEndian
	} else if buf[5] != 1 {
		return nil, errors.New("ELF reports to be neither big, nor little endian. We don't support medium endian.")
	}

	if buf[4] != 1 && buf[4] != 2 {
		return nil, errors.New("ELF file reports to be neither 32 nor 64 bit. Cannot handle.")
	}

	//what we're doing here is read the 32bit header, and then transfer
	//things over to the 64 bit header, casting 32bit values to 64bit.
	//this might be... uh... dubious, but for what we need, it's fine
	//and simplifies the code.
	if buf[4] == 1 { //32 bit
		smolheader := ELF32Header{}
		if err := binary.Read(fullfile, f.endianness, &smolheader); err != nil {
			return nil, err
		}
		f.Mag0 = smolheader.Mag0
		f.Mag1 = smolheader.Mag1
		f.Mag2 = smolheader.Mag2
		f.Mag3 = smolheader.Mag3
		f.Class = smolheader.Class
		f.Data = smolheader.Data
		f.EIVersion = smolheader.EIVersion
		f.OsABI = smolheader.OsABI
		f.ABIVersion = smolheader.ABIVersion
		f.Pad = smolheader.Pad
		f.Type = smolheader.Type
		f.Machine = smolheader.Machine
		f.EVersion = smolheader.EVersion
		f.Entry = uint64(smolheader.Entry)
		f.PhOff = uint64(smolheader.PhOff)
		f.Shoff = int64(smolheader.Shoff)
		f.Flags = smolheader.Flags
		f.EhSize = smolheader.EhSize
		f.PhEntSize = smolheader.PhEntSize
		f.PhNum = smolheader.PhNum
		f.Shentsize = smolheader.Shentsize
		f.Shnum = smolheader.Shnum
		f.ShStrNdx = smolheader.ShStrNdx
	} else if buf[4] == 2 { //64 bit
		bigheader := ELF64Header{}
		if err := binary.Read(fullfile, f.endianness, &bigheader); err != nil {
			return nil, err
		}
		f.Mag0 = bigheader.Mag0
		f.Mag1 = bigheader.Mag1
		f.Mag2 = bigheader.Mag2
		f.Mag3 = bigheader.Mag3
		f.Class = bigheader.Class
		f.Data = bigheader.Data
		f.EIVersion = bigheader.EIVersion
		f.OsABI = bigheader.OsABI
		f.ABIVersion = bigheader.ABIVersion
		f.Pad = bigheader.Pad
		f.Type = bigheader.Type
		f.Machine = bigheader.Machine
		f.EVersion = bigheader.EVersion
		f.Entry = bigheader.Entry
		f.PhOff = bigheader.PhOff
		f.Shoff = bigheader.Shoff
		f.Flags = bigheader.Flags
		f.EhSize = bigheader.EhSize
		f.PhEntSize = bigheader.PhEntSize
		f.PhNum = bigheader.PhNum
		f.Shentsize = bigheader.Shentsize
		f.Shnum = bigheader.Shnum
		f.ShStrNdx = bigheader.ShStrNdx
	}

	f.osFile.Seek(f.Shoff, io.SeekStart) //TODO err handle
	for i := 0; i < int(f.Shnum); i++ {
		section := readSectionHeader(&f)
		f.sections = append(f.sections, Section{header: section, osFile: f.osFile})
	}

	//pre-resolve all section names
	strSection := f.sections[f.ShStrNdx]
	for i := range f.sections {
		header := f.sections[i].header
		if _, err := f.osFile.Seek(int64(strSection.header.Shoffset)+int64(header.Shname), io.SeekStart); err != nil {
			return nil, err
		}
		str, err := readNullTerminatedString(f.osFile)
		if err != nil {
			return nil, err
		}
		f.sections[i].name = str

	}

	return &f, nil
}

//this is slow, but who cares?
func readNullTerminatedString(f io.Reader) (string, error) {
	var str []byte
	for {
		b := make([]byte, 1)
		if _, err := io.ReadFull(f, b); err != nil {
			return "", err
		}
		if b[0] == 0x00 {
			break
		}
		str = append(str, b[0])
	}
	return string(str), nil
}

type Section struct {
	name   string
	header SectionHeader64
	osFile *os.File
}

func readSectionHeader(f *File) SectionHeader64 {
	section := SectionHeader64{}
	if f.Class == 1 {
		smolsection := SectionHeader32{}
		if err := binary.Read(f.osFile, f.endianness, &smolsection); err != nil {
			panic(err)
		}
		section.Shname = smolsection.Shname
		section.Shtype = smolsection.Shtype
		// <32bit sizes>
		section.Shflags = uint64(smolsection.Shflags)
		section.Shaddr = uint64(smolsection.Shaddr)
		section.Shoffset = uint64(smolsection.Shoffset)
		section.Shsize = uint64(smolsection.Shsize)
		// </32bit sizes>
		section.Shlink = smolsection.Shlink
		section.Shinfo = smolsection.Shinfo
		// <32bit sizes>
		section.Shaddralign = uint64(smolsection.Shaddralign)
		section.Shentsize = uint64(smolsection.Shentsize)
	} else {

		if err := binary.Read(f.osFile, f.endianness, &section); err != nil {
			panic(err)
		}
	}
	return section
}

type ELF32Header struct {
	Mag0       byte
	Mag1       byte
	Mag2       byte
	Mag3       byte
	Class      byte
	Data       byte
	EIVersion  byte
	OsABI      byte
	ABIVersion byte
	Pad        [7]byte
	Type       uint16
	Machine    uint16
	EVersion   uint32
	// <32bit sizes>
	Entry uint32
	PhOff uint32
	Shoff int32
	// </32bit sizes>
	Flags     uint32
	EhSize    uint16
	PhEntSize uint16
	PhNum     uint16
	Shentsize uint16
	Shnum     uint16
	ShStrNdx  uint16
}

type ELF64Header struct {
	Mag0       byte
	Mag1       byte
	Mag2       byte
	Mag3       byte
	Class      byte
	Data       byte
	EIVersion  byte
	OsABI      byte
	ABIVersion byte
	Pad        [7]byte
	Type       uint16
	Machine    uint16
	EVersion   uint32
	// <64bit sizes>
	Entry uint64
	PhOff uint64
	Shoff int64
	// </64bit sizes>
	Flags     uint32
	EhSize    uint16
	PhEntSize uint16
	PhNum     uint16
	Shentsize uint16
	Shnum     uint16
	ShStrNdx  uint16
}

type SectionHeader32 struct {
	Shname uint32
	Shtype uint32
	// <32bit sizes>
	Shflags  uint32
	Shaddr   uint32
	Shoffset uint32
	Shsize   uint32
	// </32bit sizes>
	Shlink uint32
	Shinfo uint32
	// <32bit sizes>
	Shaddralign uint32
	Shentsize   uint32
	// </32bit sizes>
}

type SectionHeader64 struct {
	Shname uint32
	Shtype uint32
	// <64bit sizes>
	Shflags  uint64
	Shaddr   uint64
	Shoffset uint64
	Shsize   uint64
	// </64bit sizes>
	Shlink uint32
	Shinfo uint32
	// <64bit sizes>
	Shaddralign uint64
	Shentsize   uint64
	// </64bit sizes>

}

func (f *File) Section(name string) *Section {
	for _, sec := range f.sections {
		if sec.name == name {
			return &sec
		}
	}
	return nil
}

func (s *Section) Data() ([]byte, error) {
	buf := make([]byte, s.header.Shsize)
	if _, err := s.osFile.Seek(int64(s.header.Shoffset), io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(s.osFile, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (s *Section) Offset() int {
	return int(s.header.Shoffset)
}

func (s *Section) Length() int {
	return int(s.header.Shsize)
}
