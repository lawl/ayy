package appimage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/lawl/ayy/appstream"
	"github.com/lawl/ayy/desktop"
	"github.com/lawl/ayy/elf"
	"github.com/lawl/ayy/squashfs"
)

type AppImage struct {
	ImageFormatType uint
	FS              *squashfs.SquashFS
	elf             *elf.File
	file            *os.File
}

func Open(file string) (*AppImage, error) {

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
func (ai *AppImage) Close() {
	ai.file.Close()
}
func (ai *AppImage) ELFSectionAsString(section string) (string, error) {
	sect := ai.elf.Section(section)
	if sect == nil {
		return "", fmt.Errorf("ELF section '%s' not found", section)
	}
	b, err := sect.Data()
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

func (ai *AppImage) DesktopEntry(s string) (name string) {
	desktop, err := ai.DesktopFile()
	if err != nil {
		return ""
	}
	entry, found := desktop.Group("Desktop Entry")
	if !found {
		return ""
	}
	return entry.KV[s]
}

func (ai *AppImage) AppStreamFile() (*appstream.Component, error) {
	matches, err := fs.Glob(ai.FS, "usr/share/appdata/*.appdata.xml")
	if err != nil {
		return nil, fmt.Errorf("Cannot glob for appstream file: %w", err)
	}

	if len(matches) == 0 {
		return nil, errors.New("AppImage does not contain an appstream file.")
	}
	internalFilePath := matches[0]

	file, err := ai.FS.Open(internalFilePath)
	if err != nil {
		return nil, fmt.Errorf("Couldn't open file: %w\n", err)
	}

	component, err := appstream.Parse(file)
	if err != nil {
		return nil, err
	}

	return component, nil
}

func (ai *AppImage) AppStreamID() string {
	component, err := ai.AppStreamFile()
	if err != nil {
		return ""
	}
	return component.ID
}

type AppImageID string

// ID tries to generate a stable identifier for the application that remains the same
// across updates. AppImage does not specify anything like that.
// However, the docs note that optionally an AppStream can be placed at a known location.
// an AppStream does specify such an ID. If available, use that.
// If not, we try to make a synthetic ID. If the application has update information
// (.upd_info ELF section) we build a synthetic ID from that, as the AppImage spec say
//
//     URL to the .zsync file (URL MUST NOT change from version to version)
//
// for Zsync URLs. For github updates <username>-<repo> should hopefully reasonably stable
// and for Pling, we have a product ID that we can use.
//
// If a file has no update information, we take the (slightly processed) "Name" entry from
// the .desktop file.
//
// If there's no desktop file, we're out of ideas and just use the (slightly processed) file
// name of the AppImage on disk. Using some simple heuristics trying to cut out version numbers.
func (ai *AppImage) ID() AppImageID {
	asid := ai.AppStreamID()
	if asid != "" {
		return AppImageID(asid)
	}
	var sanitizer = regexp.MustCompile(`[^A-Za-z\-]`)
	updInfo, err := ai.ELFSectionAsString(".upd_info")
	if err == nil {
		spl := strings.Split(updInfo, "|")
		if len(spl) == 0 {
			goto desktop
		}
		switch spl[0] {
		case "zsync":
			if len(spl) < 2 {
				goto desktop
			}
			u, err := url.Parse(spl[1])
			if err != nil {
				goto desktop
			}
			filename := strings.ToLower(path.Base(u.Path))
			filename = strings.TrimSuffix(filename, ".zsync")
			filename = strings.TrimSuffix(filename, ".appimage")
			return AppImageID("ayy_" + strings.ToLower(sanitizer.ReplaceAllString(u.Hostname()+filename, "")))
		case "gh-releases-zsync":
			if len(spl) < 3 {
				goto desktop
			}
			return AppImageID("ayy_gh-" + strings.ToLower(sanitizer.ReplaceAllString(spl[1]+"-"+spl[2]+"-"+spl[3], "")))
		case "pling-v1-zsync":
			if len(spl) < 2 {
				goto desktop
			}
			return AppImageID("ayy_pling1z-" + strings.ToLower(sanitizer.ReplaceAllString(spl[1], "")))
		default:
			goto desktop
		}
	}
desktop:
	did := ai.DesktopEntry("Name")
	did = strings.Split(did, "-")[0]
	did = strings.Split(did, "_")[0]

	if did != "" {
		return AppImageID("ayy_dsk-" + strings.ToLower(sanitizer.ReplaceAllString(did, "")))
	}

	lastResort := ai.file.Name()
	filename := strings.ToLower(lastResort)
	filename = strings.TrimSuffix(filename, ".appimage")
	filename = sanitizer.ReplaceAllString(filename, "")
	return AppImageID(filename)
}
