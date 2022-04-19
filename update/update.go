package update

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/lawl/ayy/appimage"
	"github.com/lawl/ayy/bytesz"
	"github.com/lawl/ayy/integrate"
)

type Progress struct {
	Percent int
	AppName string
	Text    string
	Err     error
}

func Info(aiPath string) (Updater, error) {
	ai, err := appimage.Open(aiPath)
	if err != nil {
		return nil, err
	}
	defer ai.Close()

	updInfo, err := ai.ELFSectionAsString(".upd_info")
	if err != nil {
		return nil, err
	}
	updater := updaterFromUpdInfo(updInfo, aiPath)

	return updater, nil
}

func AppImage(aiPath string, ch chan Progress) {
	defer close(ch)

	ai, err := appimage.Open(aiPath)
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
	updater := updaterFromUpdInfo(updInfo, aiPath)

	ch <- Progress{Percent: 0, Text: "Checking for updates", Err: nil}

	at, updavail, err := updater.Check()
	if err != nil {
		ch <- Progress{Err: err, AppName: appName}
		return
	}

	if updavail {
		ch <- Progress{Percent: 0, AppName: appName, Text: "Update Available", Err: nil}
		dlch := make(chan downloadProgress)
		targetPath := aiPath + ".ayydownload"
		go downloadFileWithProgress(at, dlch, targetPath)
		for dl := range dlch {
			if dl.err != nil {
				ch <- Progress{Err: err, AppName: appName}
				os.Remove(targetPath)
				return
			}
			txt := "Downloading"
			if u, err := url.Parse(at); err != nil {
				txt = "Downloading " + path.Base(u.Path)
			}
			dlstr := bytesz.Format(uint64(dl.bytesDownloaded))
			szstr := bytesz.Format(uint64(dl.size))
			if dl.size <= 0 {
				szstr = "?"
			}
			txt += fmt.Sprintf(" (%s/%s)", dlstr, szstr)
			ch <- Progress{Percent: dl.progress, AppName: appName, Text: txt, Err: nil}
		}
		if ok, err := imagesHaveSameId(targetPath, aiPath); !ok || err != nil {
			ch <- Progress{Err: fmt.Errorf("Installed image and downloaded update don't have matching IDs. Aborting for security reasons. Leaving downloaded file for inspection."), AppName: appName}
			return
		}
		ch <- Progress{Percent: 100, AppName: appName, Text: "Installing...", Err: nil}
		_, err := integrate.Install(targetPath, aiPath)
		if err != nil {
			ch <- Progress{Err: err, AppName: appName}
			return
		}
		ch <- Progress{Percent: 100, AppName: appName, Text: "Done", Err: nil}
		return
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
	Check() (url string, available bool, err error)
	InfoString() string
}

type nullUpdater struct{}

func (n nullUpdater) Check() (url string, available bool, err error) { return "", false, nil }
func (n nullUpdater) InfoString() string                             { return "no update information" }

type plingUpdater struct{}

func (n plingUpdater) Check() (url string, available bool, err error) { return "", false, nil }
func (n plingUpdater) InfoString() string                             { return "pling not supported" }

func updaterFromUpdInfo(updInfo string, localPath string) Updater {
	updInfo = strings.TrimSpace(updInfo)
	spl := strings.Split(updInfo, "|")
	for i := 0; i < len(spl); i++ {
		spl[i] = strings.TrimSpace(spl[i])
	}
	switch spl[0] {
	case "zsync":
		if len(spl) < 2 {
			return nullUpdater{}
		}
		purl, err := url.Parse(spl[1])
		if err != nil {
			return nullUpdater{}
		}
		//reject everything other than http for security reasons
		//e.g. no http, which would be allowed by spec
		//the spec only sais an URL...
		if strings.ToLower(purl.Scheme) != "https" {
			return nullUpdater{}
		}
		return httpsUpdater{
			remoteZsync: spl[1],
			localPath:   localPath,
		}
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
		//no easy to find api docs, and they should just use regular https
		//this should have never made the spec, refusing to implement
		//also cannot find a single image on pling using this, so...
		return plingUpdater{}
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

type downloadProgress struct {
	progress        int
	size            int
	bytesDownloaded int
	err             error
}

func downloadFileWithProgress(url string, progressCh chan downloadProgress, targetPath string) {
	defer close(progressCh)
	target, err := os.Create(targetPath)
	if err != nil {
		progressCh <- downloadProgress{err: err}
		return
	}

	resp, err := http.Get(url)
	if err != nil {
		progressCh <- downloadProgress{err: err}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		progressCh <- downloadProgress{err: fmt.Errorf("http status: %d %s", resp.StatusCode, resp.Status)}
		return
	}
	var bytesWrittenCounter int64
	writer := writeProgressReporter{ch: progressCh, max: resp.ContentLength, written: &bytesWrittenCounter}
	progressReader := io.TeeReader(resp.Body, writer)

	_, err = io.Copy(target, progressReader)
	if err != nil {
		progressCh <- downloadProgress{err: err}
		return
	}
	progressCh <- downloadProgress{progress: 100, err: nil, bytesDownloaded: int(bytesWrittenCounter), size: int(resp.ContentLength)}
}

type writeProgressReporter struct {
	ch      chan downloadProgress
	max     int64
	written *int64
}

func (pr writeProgressReporter) Write(p []byte) (n int, err error) {
	//doesn't actually write anything anywhere, just reports bytes "written", never errors
	*pr.written += int64(len(p))
	// max here is the reported Content-Length of the server
	// avoid division by zero, by reporting 100% on zero content length
	var percent int
	if pr.max == 0 {
		percent = 100
	} else if pr.max < 0 {
		// servers may report content length -1 if they don't know or can't be bothered to find out
		// in that case, pretend the max size is 100MiB, but wrap around to zero once we hit it
		// this hopefully makes it clear to the user that things are happening
		// and gives some kind of animation on the progress bar

		//            B   KiB    MiB
		pretendMax := 1 * 1024 * 1024 * 100
		pretendWritten := *pr.written % int64(pretendMax)

		percent = int(float32(pretendWritten) / float32(pretendMax) * 100)
	} else {
		percent = int(float32(*pr.written) / float32(pr.max) * 100)
	}

	pr.ch <- downloadProgress{progress: percent, err: nil, size: int(pr.max), bytesDownloaded: int(*pr.written)}
	return len(p), nil
}

func imagesHaveSameId(targetPath, aiPath string) (bool, error) {
	ai1, err := appimage.Open(targetPath)
	if err != nil {
		return false, err
	}
	ai2, err := appimage.Open(aiPath)
	if err != nil {
		return false, err
	}

	return ai1.ID() == ai2.ID(), nil
}
