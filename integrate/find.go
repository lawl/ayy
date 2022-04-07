package integrate

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lawl/ayy/appimage"
)

func FindImageById(id appimage.AppImageID) (path string, found bool, err error) {
	files, _ := List()

	for _, file := range files {
		ai, err := appimage.Open(file)
		if err != nil {
			continue
		}
		defer ai.Close()

		curid := ai.ID()
		if curid == id {
			return file, true, nil
		}
	}

	return "", false, nil
}

//FindImageByName scans images for the name the user deals with
//case insensitive
func FindImageByName(name string) (path string, found bool, err error) {
	files, _ := List()

	for _, file := range files {
		ai, err := appimage.Open(file)
		if err != nil {
			continue
		}
		defer ai.Close()

		curName := ai.DesktopEntry("Name")
		if strings.ToLower(curName) == strings.ToLower(name) {
			return file, true, nil
		}
	}
	return "", false, nil
}

func AppDir() string {
	return filepath.Join(os.Getenv("HOME"), "Applications")
}

func List() (list []string, nNotAppImage int) {
	var appList []string
	filepath.Walk(AppDir(), func(path string, info fs.FileInfo, err error) error {
		ai, err := appimage.Open(path)
		if err != nil {
			nNotAppImage++
			return nil
		}
		defer ai.Close()
		appList = append(appList, path)
		return nil
	})

	return appList, nNotAppImage
}
