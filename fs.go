package main

import (
	"ayy/bytesz"
	"ayy/fancy"
	"ayy/squashfs"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func listFiles(aiPath, internalPath string, usebytes bool) {
	ai := ai(aiPath)

	entries, err := fs.ReadDir(ai.FS, unrootPath(internalPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't list directory: %s\n", err)
		os.Exit(1)
	}

	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to fetch file info for '%s': %s", e.Name(), err)
		}
		sqinfo, ok := info.Sys().(squashfs.SquashInfo)
		if !ok {
			fmt.Fprintln(os.Stderr, "FileInfo must implement SquashInfo. This is a bug in the code.")
			os.Exit(1)
		}
		linkTarget := ""
		isSymlink := info.Mode()&fs.ModeSymlink == fs.ModeSymlink
		if isSymlink {
			linkTarget = " -> "
			targetname := sqinfo.SymlinkTarget()
			if !filepath.IsAbs(targetname) {
				targetname = filepath.Join(unrootPath(internalPath), targetname)
			}
			target, err := ai.FS.Open(targetname)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Couldn't read symlink pointing to %s: %s", targetname, err)
			}
			targetstat, err := target.Stat()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Couldn't stat symlink target %s: %s", sqinfo.SymlinkTarget(), err)
			}
			linkTarget += colorizeFilename(sqinfo.SymlinkTarget(), targetstat)
		}
		name := colorizeFilename(e.Name(), info)
		var size string
		if !usebytes {
			size = bytesz.Format(uint64(info.Size()))
		} else {
			size = fmt.Sprintf("%10d", info.Size())
		}
		fmt.Printf("%s  %4d %4d  %s  %s  %s%s\n", info.Mode(), sqinfo.Uid(), sqinfo.Gid(), size, info.ModTime().Format("Jan 02 2006 15:04"), name, linkTarget)
	}
}

func colorizeFilename(name string, info fs.FileInfo) string {
	isSymlink := info.Mode()&fs.ModeSymlink == fs.ModeSymlink
	fp := fancy.Print{}
	if info.IsDir() {
		fp.Bold().Color(fancy.Blue)
	} else if isSymlink {
		fp.Bold().Color(fancy.Cyan)
	} else if isExecutableBySomeone(info.Mode()) {
		fp.Bold().Color(fancy.Green)
	}

	return fp.Format(name)
}

func isExecutableBySomeone(mode os.FileMode) bool {
	return mode&0111 != 0
}

func catFile(aiPath, internalPath string) {
	ai := ai(aiPath)

	file, err := ai.FS.Open(unrootPath(internalPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't open file: %s\n", err)
		os.Exit(1)
	}
	_, err = io.Copy(os.Stdout, file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error copying file to stdout: %s", err)
	}
}
