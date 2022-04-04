package appimage

import (
	"ayy/desktop"
	"ayy/elf"
	"ayy/squashfs"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

type AppImage struct {
	ImageFormatType uint
	FS              fs.FS
	elf             *elf.File
	file            *os.File
}

func NewAppImage(file string) (*AppImage, error) {

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	el, err := elf.Open(f)
	if err != nil {
		return nil, err
	}
	ai := AppImage{}
	ai.file = f
	ai.elf = el

	if el.ABIVersion != 0x41 || el.Pad[0] != 0x49 {
		return nil, fmt.Errorf("file is not an AppImage. Expected AppImage magic at offset 0x08")
	}

	if el.Pad[1] != 1 && el.Pad[1] != 2 {
		return nil, fmt.Errorf("file looks like an AppImage, but invalid version number '%d'", el.Pad[1])
	}
	ai.ImageFormatType = uint(el.Pad[1])

	if ai.ImageFormatType == 1 {
		return nil, errors.New("AppImage v1 is not supported currently. Use AppImage v2")
	}

	elfSz := el.Shoff + (int64(el.Shentsize) * int64(el.Shnum))

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	squashReader := io.NewSectionReader(f, elfSz, stat.Size()-elfSz)

	sqfs, err := squashfs.New(squashReader)
	if err != nil {
		return nil, err
	}

	ai.FS = sqfs

	return &ai, nil
}
func (ai *AppImage) ELFSectionAsString(section string) (string, error) {
	b, err := ai.elf.Section(section).Data()
	if err != nil {
		return "", err
	}
	ret := strings.Trim(string(b), "\x00")
	return ret, nil
}

func (ai *AppImage) DesktopFile() (*desktop.File, error) {
	matches, err := fs.Glob(ai.FS, "*.desktop")
	if err != nil {
		return nil, fmt.Errorf("Cannot glob for desktop file: %w", err)
	}

	if len(matches) == 0 {
		return nil, errors.New("AppImage does not contain a desktop file. Integration for desktop-file less images is not supported yet.")
	}
	internalDesktopFilePath := matches[0]

	buf, err := fs.ReadFile(ai.FS, internalDesktopFilePath)
	if err != nil {
		return nil, fmt.Errorf("Couldn't open file: %w\n", err)
	}

	desktop, err := desktop.ParseEntry(string(buf))
	if err != nil {
		return nil, fmt.Errorf("Couldn't parse file: %w\n", err)
	}

	return desktop, nil
}
