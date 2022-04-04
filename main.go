package main

import (
	"ayy/appimage"
	"ayy/fancy"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const AppName = "ayy"

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
			fmt.Fprintf(os.Stderr, "usage: ayy elf /foo/bar.AppImage\n")
			os.Exit(1)
		}

		fp := fancy.Print{}
		fp.Color(fancy.Cyan)

		ai := ai(os.Args[2])
		updInfo, err := ai.UpdateInfo()
		if err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"reading update info: %s\n", err)
		}
		sha256sig, err := ai.Sha256Sig()
		if err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"reading update info: %s\n", err)
		}
		sigKey, err := ai.SigKey()
		if err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"reading update info: %s\n", err)
		}
		fmt.Printf("%s: %d\n", fp.Format("Image Format Type"), ai.ImageFormatType)
		fmt.Printf("%s: %s\n", fp.Format("Update"), updInfo)

		fmt.Printf("%s:\n%s\n", fp.Format("Raw Signature"), string(sha256sig))
		fmt.Printf("%s:\n%s\n", fp.Format("Raw Signature Key"), string(sigKey))

		if ai.HasSignature() {
			sig, ok, err := ai.Signature()
			if err != nil {
				fmt.Fprintf(os.Stderr, WARNING+"Couldn't read singature: %s\n", err)
			}
			if ok {
				fmt.Printf("%s (trust on first use): [Primary Key ID: %s]\n", fp.Format("Signature"), sig.PrimaryKey.KeyIdString())
				idprint := fancy.Print{}
				idprint.Color(fancy.Yellow)
				for _, i := range sig.Identities {
					fmt.Printf("\t%s: %s\n", idprint.Format("Identity"), i.Name)
				}
			} else {
				fmt.Printf("No or invalid digital signature.\n")
			}
		}
	case "fs":

		fsHelp := "usage: ayy fs /foo/bar.AppImage command\n" +
			"\n" +
			"commands:\n" +
			"  ls <path>          List files under the specified path inside the AppImage\n" +
			"  cat <path>         Print the file at <path> inside the AppImage to stdout\n"

		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, fsHelp)
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
		default:
			fmt.Fprintf(os.Stderr, fsHelp)
			os.Exit(1)
		}

	case "lmao":
		fmt.Println("ayy lmao")
		os.Exit(0)
	case "install":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: ayy install /foo/bar.AppImage\n"+
				"\n"+
				"this is currently required to be a local path, but may also allow https urls in the future. Stay tuned.\n")
			os.Exit(1)
		}
		installAppimage(os.Args[2])
	case "list":
		listAppimages()
	case "help", "-h", "--help":
		globalHelp()
		os.Exit(0)
	default:
		globalHelp()
		os.Exit(1)
	}
}

func globalHelp() {
	fmt.Fprint(os.Stderr,
		"usage ayy <command>\n"+
			"\n"+
			"  install            Install an AppImage and integrate it into the desktop environment\n"+
			"  list               Display installed AppImages\n"+
			"  fs                 Interact with an AppImage's internal filesystem\n"+
			"  elf                Display metadata stored on the AppImage's ELF header\n"+
			"  help               Display this help\n"+
			"\n"+
			"Call this commands without any arguments for per command help.\n"+
			"")
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

func listAppimages() {
	appDir := filepath.Join(os.Getenv("HOME"), "Applications")
	filepath.Walk(appDir, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".AppImage") {
			return nil
		}
		ai := ai(path)
		desktop, err := ai.DesktopFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, WARNING+"Couldn't read internal desktop file for '%s':%w\n", path, err)
			return nil
		}
		entry, found := desktop.Group("Desktop Entry")
		if !found {
			fmt.Fprintf(os.Stderr, WARNING+"Desktop file contains no [Desktop Entry] '%s'\n", path)
			return nil
		}
		fpname := fancy.Print{}
		fpname.Color(fancy.Cyan)

		fpversion := fancy.Print{}
		fpversion.Color(fancy.Yellow)
		fmt.Printf("Name: %s\n\tVersion: %s\n\t   Path: %s\n\n", fpname.Format(entry.KV["Name"]), fpversion.Format(entry.KV["X-AppImage-Version"]), path)

		return nil
	})
}
