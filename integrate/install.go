package integrate

import (
	"ayy/appimage"
	"ayy/squashfs"
	"ayy/xdg"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func AppImage(path string) (warnings []string, err error) {

	ai, err := appimage.NewAppImage(path)
	if err != nil {
		return warnings, err
	}

	desktop, err := ai.DesktopFile()
	if err != nil {
		return warnings, errors.New("Couldn't read .desktop file. Currently unsupported. .desktop-less installs currently unsupported.")
	}

	desktopgroup, found := desktop.Group("Desktop Entry")
	if !found {
		return warnings, errors.New("Couldn't find 'Desktop Entry' group in .desktop file. .desktop-less installs currently unsupported.")
	}

	icon, err := fs.ReadFile(ai.FS, ".DirIcon")
	if err != nil {
		return warnings, err
	}

	appDir := filepath.Join(os.Getenv("HOME"), "Applications")
	if err := ensureExists(appDir); err != nil {
		return warnings, err
	}

	// why data home and not cache? if the user clears the cache, icons break
	// and we don't really have a mechanism to easily detect that apart from (i/fa/...)notify
	iconDir := filepath.Join(xdg.Get(xdg.DATA_HOME), "ayy", "icons")
	if err := ensureExists(iconDir); err != nil {
		return warnings, err
	}

	desktopDir := filepath.Join(xdg.Get(xdg.DATA_HOME), "applications")
	if err := ensureExists(desktopDir); err != nil {
		return warnings, err
	}

	aiHash, err := ai.SHA256WithoutSignature()
	if err != nil {
		return warnings, err
	}

	iconPath := filepath.Join(iconDir, fmt.Sprintf("appimage_%x%s", aiHash, findDirIconFileExt(ai)))
	desktopPath := filepath.Join(desktopDir, fmt.Sprintf("%x.desktop", aiHash))
	appimgPath := filepath.Join(appDir, filepath.Base(path))

	err = os.Rename(path, appimgPath)
	if err != nil {
		return warnings, err
	}

	if err := os.Chmod(appimgPath, 0755); err != nil {
		warnings = append(warnings, fmt.Sprintf("Couldn't set executable permissions on AppImage '%s': %s\n", appimgPath, err))
	}

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

	return

err:
	//try to clean up failed install
	//no point in trying to handle errors in here, best effort.
	os.Remove(iconPath)
	os.Remove(desktopPath)
	return warnings, err
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
