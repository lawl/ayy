package main

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
)

func installAppimage(path string) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		fmt.Fprintln(os.Stderr, "web fetch support coming soon(tm)")
		os.Exit(1)
	}
	ai := ai(path)

	matches, err := fs.Glob(ai.FS, "*.desktop")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot glob for desktop file: %s\n", err)
		os.Exit(1)
	}

	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "Error: AppImage does not contain a desktop file. Integration for desktop-file less images is not supported yet.")
		os.Exit(1)
	}
	internalDesktopFilePath := matches[0]
	if len(matches) > 1 {
		fmt.Fprintf(os.Stderr, "Warning: Multiple .desktop files found in AppImage root, using '%s'\n", internalDesktopFilePath)
	}

}
