package main

import (
	"fmt"
	"os"
	"strings"
)

func installAppimage(path string) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		fmt.Fprintln(os.Stderr, "web fetch support coming soon(tm)")
		os.Exit(1)
	}
	ai := ai(path)
	_ = ai
}
