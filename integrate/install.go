package integrate

import (
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lawl/ayy/appimage"
	"github.com/lawl/ayy/squashfs"
	"github.com/lawl/ayy/xdg"
)

func Unintegrate(appimgPath string) error {
	ai, err := appimage.Open(appimgPath)
	if err != nil {
		return err
	}
	defer ai.Close()

	iconPath := IconPath(ai)
	desktopPath := DesktopFilePath(ai)

	if exists(iconPath) {
		if err := os.Remove(iconPath); err != nil {
			return err
		}
	}

	if exists(desktopPath) {
		if err := os.Remove(desktopPath); err != nil {
			return err
		}
	}

	list := PathWrappersForAppImage(appimgPath)
	for _, pwe := range list {
		if err := os.Remove(pwe.WrapperPath); err != nil {
			return err
		}
	}

	return nil
}

func Integrate(appimgPath string) (err error) {

	ai, err := appimage.Open(appimgPath)
	if err != nil {
		return err
	}
	defer ai.Close()

	desktop, err := ai.DesktopFile()
	if err != nil {
		return errors.New("Couldn't read .desktop file. Currently unsupported. .desktop-less installs currently unsupported.")
	}

	desktopgroup, found := desktop.Group("Desktop Entry")
	if !found {
		return errors.New("Couldn't find 'Desktop Entry' group in .desktop file. .desktop-less installs currently unsupported.")
	}

	icon, err := findIcon(ai)
	if err != nil {
		return fmt.Errorf("Unable to find icon: ", err)
	}

	appDir := AppDir()
	if err := ensureExists(appDir); err != nil {
		return err
	}

	iconPath := IconPath(ai)
	desktopPath := DesktopFilePath(ai)

	err = ioutil.WriteFile(iconPath, icon, 0644)
	if err != nil {
		goto err
	}

	desktopgroup.KV["Exec"] = rewriteExecLine(desktopgroup.KV["Exec"], appimgPath)
	desktopgroup.KV["Icon"] = iconPath

	// https://specifications.freedesktop.org/desktop-entry-spec/latest/ar01s06.html
	// According to the spec, an absolute path as an icon is allowed:
	//    Icon to display in file manager, menus, etc. If the name is an absolute path, the given file will be used.

	err = ioutil.WriteFile(desktopPath, []byte(desktop.String()), 0644)
	if err != nil {
		goto err
	}

	// don't care if it errors, best effort
	exec.Command("update-desktop-database", desktopPath).Run()
	exec.Command("xdg-desktop-menu", "forceupdate", "--mode", "user").Run()

	return nil

err:
	//try to clean up failed install
	//no point in trying to handle errors in here, best effort.
	os.Remove(iconPath)
	os.Remove(desktopPath)
	return err
}

func IconPath(ai *appimage.AppImage) string {
	// why data home and not cache? if the user clears the cache, icons break
	// and we don't really have a mechanism to easily detect that apart from (i/fa/...)notify
	iconDir := filepath.Join(xdg.Get(xdg.DATA_HOME), "ayy", "icons")
	if err := ensureExists(iconDir); err != nil {
		//best effort, we'll explode later it's fine
	}

	return filepath.Join(iconDir, fmt.Sprintf("appimage_%s%s", ai.ID(), findDirIconFileExt(ai)))
}

func DesktopFilePath(ai *appimage.AppImage) string {
	desktopDir := filepath.Join(xdg.Get(xdg.DATA_HOME), "applications")
	if err := ensureExists(desktopDir); err != nil {
		//best effort, we'll explode later it's fine
	}
	return filepath.Join(desktopDir, fmt.Sprintf("appimage_%s.desktop", ai.ID()))
}

func findDirIconFileExt(ai *appimage.AppImage) (ret string) {
	// Spec: https://docs.appimage.org/reference/appdir.html
	/*
		PNG icon located in the root directory. Can be used by e.g., thumbnailers,
		to display application icons rather than a generic filetype symbol.
		Should be in one of the standard image sizes, e.g., 128x128 or 256x256 pixels.
		[...snip...]
		In most cases, .DirIcon is a symlink to this file.
	*/

	// So we can not rely on this being a symlink.
	// check if it's a symlink and read the file ending of the target
	// if not, assume PNG

	//uh, we want to check if .DirIcon is a symlink
	//but our Open() always follows symlinks and
	//we don't have lstat().
	//however, our directory listing code doesnt follow
	//so this is why we do the weird directory listing stuff as a workaround
	//go devs in a github issue said dunno, symlinks are implementation specific we guess :D
	ret = ".png"

	entries, err := fs.ReadDir(ai.FS, ".")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() != ".DirIcon" {
			continue
		}

		stat, err := e.Info()
		if err != nil {
			return
		}

		sqinfo, ok := stat.Sys().(squashfs.SquashInfo)
		if !ok {
			return
		}
		isSymlink := stat.Mode()&fs.ModeSymlink == fs.ModeSymlink

		if !isSymlink {
			return
		}
		ret = filepath.Ext(sqinfo.SymlinkTarget())
		return
	}
	return
}

func ensureExists(path string) error {
	if !exists(path) {
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return fmt.Errorf("Directory '%s' does not exist, and we could not create it: %s\n", path, err)
		}
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func rewriteExecLine(exec, newbin string) string {
	toks := strings.Split(exec, " ")
	foundApprun := false
	for i, tok := range toks {
		if tok == "AppRun" {
			toks[i] = newbin
			foundApprun = true
			break
		}
	}

	if !foundApprun && len(toks) >= 1 {
		toks[0] = newbin
	}

	return strings.Join(toks, " ")
}

//newPath may be an empty string, in that case MoveToApplications will decide this itself
func MoveToApplications(appImagePath string, newPath string, replace bool) (retNewPath string, err error) {
	appDir := AppDir()
	if err := ensureExists(appDir); err != nil {
		return "", err
	}

	ai, err := appimage.Open(appImagePath)
	if err != nil {
		return "", err
	}
	defer ai.Close()

	//check if this is upgrading an existing image
	path, foundExisting, err := FindImageById(ai.ID())

	if newPath == "" {
		if foundExisting {
			newPath = path
		} else {
			newPath = filepath.Join(appDir, filepath.Base(appImagePath))
		}
	}

	if foundExisting && err == nil {
		newPath = path
		oldai, err := appimage.Open(path)
		if err != nil {
			return "", fmt.Errorf("Found existing AppImage '%s', with same ID '%s', but couldn't open it, refusing installation for security reasons: %s", path, ai.ID(), err)
		}
		if oldai.HasSignature() {
			oldkey, err := oldai.ELFSectionAsString(".sig_key")
			if err != nil {
				return "", fmt.Errorf("Found existing AppImage '%s', with same ID '%s', read old signature, refusing installation for security reasons: %s", path, ai.ID(), err)
			}
			newkey, err := ai.ELFSectionAsString(".sig_key")
			if err != nil {
				return "", fmt.Errorf("Couldn't read signature of '%s', ID '%s', refusing installation for security reasons: %s", path, ai.ID(), err)
			}
			if oldkey != newkey {
				return "", fmt.Errorf("Found existing AppImage '%s', with same ID '%s', but different signature, WILL NOT PROCEED WITH INSTALLATION: %s", path, ai.ID(), err)
			}

			_, ok, err := ai.Signature()
			if err != nil || !ok {
				return "", fmt.Errorf("AppImage '%s', with ID '%s', has a signature that does not verify, WILL NOT PROCEED WITH INSTALLATION: %s", path, ai.ID(), err)
			}
		}
	} else if foundExisting && err != nil {
		return "", fmt.Errorf("Found existing AppImage '%s', with same ID '%s', but an error occured, refusing installation for security reasons: %s", path, ai.ID(), err)
	}
	if replace {
		err = os.Rename(appImagePath, newPath)
	} else {
		err = os.Link(appImagePath, newPath)
	}

	if err != nil {
		return "", err
	}
	if err := os.Chmod(newPath, 0755); err != nil {
		return "", fmt.Errorf("Couldn't set executable permissions on AppImage '%s': %s\n", appImagePath, err)
	}

	return newPath, nil
}

func IsIntegrated(ai *appimage.AppImage) bool {
	icon := IconPath(ai)
	desktop := DesktopFilePath(ai)

	return exists(icon) && exists(desktop)
}

// Install installs the AppImage specified at appImagePath.
// the original file is left untouched.
// Optionally optionalNewPath may specify where. If an empty
// string is supplied, the new path will be figure out automatically
func Install(appImagePath, optionalNewPath string) (newPath string, err error) {
	path, err := MoveToApplications(appImagePath, optionalNewPath, false)
	if err != nil {
		return "", err
	}
	err = Integrate(path)
	if err != nil {
		return "", err
	}
	return path, err
}

// Upgrade installs the AppImage specified at appImagePath, replacing
// any existing file. and moving the specified appimage to the appropriate
// location
// Optionally optionalNewPath may specify where. If an empty
// string is supplied, the new path will be figure out automatically
func Upgrade(appImagePath, optionalNewPath string) (newPath string, err error) {
	path, err := MoveToApplications(appImagePath, optionalNewPath, true)
	if err != nil {
		return "", err
	}
	err = Integrate(path)
	if err != nil {
		return "", err
	}
	return path, err
}

func findIcon(ai *appimage.AppImage) ([]byte, error) {
	icon, err := fs.ReadFile(ai.FS, ".DirIcon")
	if err == nil {
		return icon, nil
	}

	desktopIconName := ai.DesktopEntry("Icon")
	endings := []string{"png", "svg", "svgz"}
	for _, end := range endings {
		icon, err := fs.ReadFile(ai.FS, desktopIconName+"."+end)
		if err == nil {
			return icon, nil
		}
	}
	return nil, errors.New("Unable to find .DirIcon, or $APPICON.{png, svg, svgz}")
}
