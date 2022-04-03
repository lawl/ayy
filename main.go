package main

import (
	"ayy/appimage"
	"ayy/fancy"
	"flag"
	"fmt"
	"os"
	"strings"
)

var WARNING string
var ERROR string
var INFO string

func init() {
	fp := fancy.Print{}
	WARNING = fp.Bold().Color(fancy.Yellow).Format("WARNING: ")
	fp = fancy.Print{}
	ERROR = fp.Bold().Color(fancy.Red).Format("ERROR: ")
	fp = fancy.Print{}
	INFO = fp.Bold().Color(fancy.Cyan).Format("INFO: ")
}

func main() {
	if len(os.Args) < 2 {
		globalHelp()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "elf":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage '%s elf /foo/bar.AppImage'\n", os.Args[0])
			os.Exit(1)
		}
		ai := ai(os.Args[2])
		updInfo, err := ai.UpdateInfo()
		if err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"reading update info: %s\n", err)
		}
		sha256sig, err := ai.Sha256Sig()
		if err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"reading update info: %s\n", err)
		}
		fmt.Printf("Update: %s\n", updInfo)

		//I have found a total of ZERO appimages using this so far, very likely just won't implement
		//there may also be better signature schemes one could implement
		fmt.Printf("SHA256 sig: %s\n", string(sha256sig))
	case "fs":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "TODO Usage '%s fs /foo/bar.AppImage subcommand'\n", os.Args[0])
			os.Exit(1)
		}

		switch os.Args[3] {
		case "ls":
			ls := flag.NewFlagSet("ls", flag.ExitOnError)
			usebytes := ls.Bool("b", false, "Display sizes in bytes instead of human readable string")
			if err := ls.Parse(os.Args[4:]); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Unable to parse flags: %s\n", err)
				os.Exit(1)
			}
			listFiles(os.Args[2], ls.Arg(0), *usebytes)
			os.Exit(0)
		case "cat":
			cat := flag.NewFlagSet("cat", flag.ExitOnError)
			if err := cat.Parse(os.Args[4:]); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Unable to parse flags: %s\n", err)
				os.Exit(1)
			}
			catFile(os.Args[2], cat.Arg(0))
			os.Exit(0)
		}

		fmt.Fprintln(os.Stderr, "No flags passed, don't know what to do")
		os.Exit(1)
	case "lmao":
		fmt.Println("ayy lmao")
		os.Exit(0)
	case "install":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "TODO Usage '%s install /foo/bar.AppImage; can be URL!!1!'\n", os.Args[0])
			os.Exit(1)
		}
		installAppimage(os.Args[2])
	default:
		globalHelp()
		os.Exit(1)
	}
}

func globalHelp() {
	fmt.Fprint(os.Stderr, "TODO: write global command useage\n")
}

func ai(path string) *appimage.AppImage {
	app, err := appimage.NewAppImage(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, ERROR+"Couldn't open AppImage: %s\n", err)
		os.Exit(1)
	}
	return app
}

func unrootPath(s string) string {
	return strings.TrimLeft(s, string(os.PathSeparator))
}
