package update

import (
	"ayy/appimage"
	"crypto/sha1"
	"io"
	"os"
	"strings"
)

type Progress struct {
	Percent int
	AppName string
	Text    string
	Err     error
}

func AppImage(path string, ch chan Progress) {
	defer close(ch)

	ai, err := appimage.Open(path)
	if err != nil {
		ch <- Progress{Err: err, AppName: "<unknown>"}
		return
	}
	defer ai.Close()

	appName := ai.DesktopEntry("Name")

	updInfo, err := ai.ELFSectionAsString(".upd_info")
	if err != nil {
		ch <- Progress{Err: err, AppName: appName}
		return
	}
	updater := updaterFromUpdInfo(updInfo, path)

	ch <- Progress{Percent: 0, Text: "Checking for updates", Err: nil}

	_, updavail, err := updater.hasUpdateAvailable()
	if err != nil {
		ch <- Progress{Err: err, AppName: appName}
		return
	}

	if updavail {
		ch <- Progress{Percent: 0, AppName: appName, Text: "Update Available", Err: nil}
	} else {
		if _, ok := updater.(nullUpdater); ok {
			ch <- Progress{Percent: 100, AppName: appName, Text: "No update information embedded", Err: nil}
		} else {
			ch <- Progress{Percent: 100, AppName: appName, Text: "Already up to date", Err: nil}
		}
	}

	return
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
