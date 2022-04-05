package integrate

import (
	"ayy/appimage"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func FindImageById(id appimage.AppImageID) (path string, found bool, err error) {
	appDir := filepath.Join(os.Getenv("HOME"), "Applications")
	if err := ensureExists(appDir); err != nil {
		return "", false, err
	}

	files, err := ioutil.ReadDir(appDir)
	if err != nil {
		return "", false, err
	}

	for _, info := range files {
		curpath := filepath.Join(appDir, info.Name())
		if info.IsDir() {
			continue
		}

		ai, err := appimage.Open(curpath)
		if err != nil {
			continue
		}
		defer ai.Close()

		curid := ai.ID()
		if curid == id {
			return curpath, true, nil
		}
	}

	return "", false, nil
}

//FindImageByName scans images for the name the user deals with
//case insensitive
func FindImageByName(name string) (path string, found bool, err error) {
	appDir := filepath.Join(os.Getenv("HOME"), "Applications")
	if err := ensureExists(appDir); err != nil {
		return "", false, err
	}

	files, err := ioutil.ReadDir(appDir)
	if err != nil {
		return "", false, err
	}

	for _, info := range files {
		curpath := filepath.Join(appDir, info.Name())
		if info.IsDir() {
			continue
		}

		ai, err := appimage.Open(curpath)
		if err != nil {
			continue
		}
		defer ai.Close()

		curName := ai.DesktopEntry("Name")
		if strings.ToLower(curName) == strings.ToLower(name) {
			return curpath, true, nil
		}
	}
	return "", false, nil
}
