package main

import (
	"ayy/appimage"
	"ayy/squashfs"
	"ayy/xdg"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func installAppimage(path string) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		fmt.Fprintln(os.Stderr, "web fetch support coming soon(tm)")
		os.Exit(1)
	}
	ai := ai(path)

	desktop, err := ai.DesktopFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, ERROR+"Couldn't fetch internal desktop file from AppImage: %w\n", err)
		os.Exit(1)
	}

	desktopgroup, found := desktop.Group("Desktop Entry")
	if !found {
		fmt.Fprintf(os.Stderr, ERROR+"Couldn't find 'Desktop Entry' group in .desktop file\n")
		os.Exit(1)
	}

	icon, err := fs.ReadFile(ai.FS, ".DirIcon")
	if err != nil {
		fmt.Fprintf(os.Stderr, ERROR+"Couldn't read .DirIcon in AppImage: %s\n", err)
		os.Exit(1)
	}

	appDir := filepath.Join(os.Getenv("HOME"), "Applications")
	ensureExists(appDir)

	// why data home and not cache? if the user clears the cache, icons break
	// and we don't really have a mechanism to easily detect that apart from (i/fa/...)notify
	iconDir := filepath.Join(xdg.Get(xdg.DATA_HOME), AppName, "icons")
	ensureExists(iconDir)

	desktopDir := filepath.Join(xdg.Get(xdg.DATA_HOME), "applications")
	ensureExists(desktopDir)

	aiHash, err := ai.SHA256WithoutSignature()
	if err != nil {
		fmt.Fprintf(os.Stderr, ERROR+"Couldn't hash AppImage: %s\n", err)
		os.Exit(1)
	}

	iconPath := filepath.Join(iconDir, fmt.Sprintf("appimage_%x%s", aiHash, findDirIconFileExt(ai)))
	desktopPath := filepath.Join(desktopDir, fmt.Sprintf("%x.desktop", aiHash))
	appimgPath := filepath.Join(appDir, filepath.Base(path))

	err = os.Rename(path, appimgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, ERROR+"Couldn't move AppImage from '%s' => '%s': %s\n", path, appimgPath, err)
		os.Exit(1)
	}

	if err := os.Chmod(appimgPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, WARNING+"Couldn't set executable permissions on AppImage '%s': %s\n", appimgPath, err)
	}

	err = ioutil.WriteFile(iconPath, icon, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, ERROR+"Couldn't write icon to '%s': %s\n", iconPath, err)
		goto err
	}

	desktopgroup.KV["Exec"] = rewriteExecLine(desktopgroup.KV["Exec"], appimgPath)
	desktopgroup.KV["Icon"] = iconPath

	// https://specifications.freedesktop.org/desktop-entry-spec/latest/ar01s06.html
	// According to the spec, an absolute path as an icon is allowed:
	//    Icon to display in file manager, menus, etc. If the name is an absolute path, the given file will be used.

	err = ioutil.WriteFile(desktopPath, []byte(desktop.String()), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, ERROR+"Couldn't write icon to '%s': %s\n", desktopPath, err)
		goto err
	}

	return

err:
	//try to clean up failed install
	//no point in trying to handle errors in here, best effort.
	os.Remove(iconPath)
	os.Remove(desktopPath)
	os.Exit(1)
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
			fmt.Printf("sup2")
			return
		}

		sqinfo, ok := stat.Sys().(squashfs.SquashInfo)
		if !ok {
			fmt.Printf("sup3")
			return
		}
		isSymlink := stat.Mode()&fs.ModeSymlink == fs.ModeSymlink

		if !isSymlink {
			fmt.Printf("sup4")
			return
		}
		ret = filepath.Ext(sqinfo.SymlinkTarget())
		return
	}
	return
}

func ensureExists(path string) {
	if !exists(path) {
		err := os.MkdirAll(path, 0755)
		if err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"Directory '%s' does not exist, and we could not create it: %s\n", path, err)
			os.Exit(1)
		}
	}
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
