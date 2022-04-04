package update

import (
	"ayy/appimage"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"strings"
)

func AppImage(path string) error {
	ai, err := appimage.Open(path)
	if err != nil {
		return err
	}
	defer ai.Close()
	updInfo, err := ai.ELFSectionAsString(".upd_info")
	if err != nil {
		return err
	}
	updater := updaterFromUpdInfo(updInfo, path)

	at, updavail, err := updater.hasUpdateAvailable()
	if err != nil {
		return err
	}
	fmt.Printf("%s has an update available %t, at: %s\n", path, updavail, at)

	return nil
}

type Updater interface {
	hasUpdateAvailable() (string, bool, error)
	update() error
}

type nullUpdater struct{}

func (n nullUpdater) hasUpdateAvailable() (string, bool, error) { return "", false, nil }
func (n nullUpdater) update() error                             { return nil }

func updaterFromUpdInfo(updInfo string, localPath string) Updater {
	updInfo = strings.TrimSpace(updInfo)
	spl := strings.Split(updInfo, "|")
	switch spl[0] {
	case "zsync":
		panic("zsync updater not implemented yet")
	case "gh-releases-zsync":
		if len(spl) < 5 {
			return nullUpdater{}
		}
		return ghUpdater{
			ghUsername:  spl[1],
			ghRepo:      spl[2],
			releaseName: spl[3],
			filename:    spl[4],
			localPath:   localPath,
		}
	case "pling-v1-zsync":
		panic("pling updater not implemented yet")
	default:
		return nullUpdater{}
	}
}

func sha1file(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	sha := sha1.New()
	io.Copy(sha, file)

	return sha.Sum(nil), nil
}
