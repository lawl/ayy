package main

import (
	"ayy/appimage"
	"ayy/fancy"
	"ayy/integrate"
	"ayy/update"
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
		for _, arg := range os.Args[2:] {
			ai := ai(arg)
			updInfo, err := ai.ELFSectionAsString(".upd_info")
			if err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"reading update info: %s\n", err)
			}
			sha256sig, err := ai.ELFSectionAsString(".sha256_sig")
			if err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"reading update info: %s\n", err)
			}
			sigKey, err := ai.ELFSectionAsString(".sig_key")
			if err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"reading update info: %s\n", err)
			}
			fmt.Printf("%s: %d\n", fp.Format("Image Format Type"), ai.ImageFormatType)
			fmt.Printf("%s: %s\n", fp.Format("Update"), updInfo)

			fmt.Printf("%s:\n%s\n", fp.Format("Raw Signature"), string(sha256sig))
			fmt.Printf("%s:\n%s\n", fp.Format("Raw Signature Key"), string(sigKey))
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
			for _, arg := range ls.Args() {
				fmt.Printf("%s:\n", arg)
				listFiles(os.Args[2], ls.Arg(0), *usebytes)
			}
			os.Exit(0)
		case "cat":
			cat := flag.NewFlagSet("cat", flag.ExitOnError)
			if err := cat.Parse(os.Args[4:]); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Unable to parse flags: %s\n", err)
				os.Exit(1)
			}
			for _, arg := range cat.Args() {
				catFile(os.Args[2], arg)
			}
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
		for _, arg := range os.Args[2:] {
			id, err := integrate.MoveToApplications(arg, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Cannot move AppImage to Application directory: %s\n", err)
				os.Exit(1)
			}
			err = integrate.Integrate(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Cannot integrate app image: %s\n", err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	case "uninstall":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: ayy uninstall <name>\n"+
				"\n"+
				"Hint: Find names with 'ayy list'\n")
			os.Exit(1)
		}
		uninstall := flag.NewFlagSet("uninstall", flag.ExitOnError)
		id := uninstall.String("id", "", "use id instead of name")
		if err := uninstall.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"Unable to parse flags: %s\n", err)
			os.Exit(1)
		}
		for _, arg := range uninstall.Args() {
			path := findAppImagefromCLIArgs(arg, *id)

			if err := integrate.Unintegrate(path); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Unable to unintegrate AppImage: %s\n", err)
				os.Exit(1)
			}
			if err := os.Remove(path); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Unable delete AppImage file '%s': %s\n", path, err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	case "update":
		// TODO: support arguments, so that e.g. "ayy update foo bar" only updates foo and bar
		appDir := filepath.Join(os.Getenv("HOME"), "Applications")
		var appList []string
		err := filepath.Walk(appDir, func(path string, info fs.FileInfo, err error) error {
			if !strings.HasSuffix(info.Name(), ".AppImage") {
				return nil
			}
			appList = append(appList, path)
			return nil
		})
		parallelUpgrade(appList)
		if err != nil {
			panic(err)
		}
		os.Exit(0)
	case "show":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: ayy show <name>\n"+
				"\n"+
				"Hint: Find names with 'ayy list'\n")
			os.Exit(1)
		}
		show := flag.NewFlagSet("show", flag.ExitOnError)
		id := show.String("id", "", "use id instead of name")
		if err := show.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"Unable to parse flags: %s\n", err)
			os.Exit(1)
		}
		for _, arg := range show.Args() {
			path := findAppImagefromCLIArgs(arg, *id)
			printAppImageDetails(path)
		}

		os.Exit(0)
	case "list":
		listAppimages()
		os.Exit(0)
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
			"  uninstall          Locate installed AppImage by name, uninstall and unintegrate it\n"+
			"  update             Update all images in Applications folder\n"+
			"  list               Display installed AppImages\n"+
			"  show               Show details of an AppImage\n"+
			"  fs                 Interact with an AppImage's internal filesystem\n"+
			"  elf                Display metadata stored on the AppImage's ELF header\n"+
			"  help               Display this help\n"+
			"\n"+
			"Call this commands without any arguments for per command help.\n"+
			"")
}

func ai(path string) *appimage.AppImage {
	app, err := appimage.Open(path)
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
		return printAppImageDetails(path)
	})
}

func printAppImageDetails(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	if !strings.HasSuffix(info.Name(), ".AppImage") {
		return nil
	}
	ai := ai(path)

	name := ai.DesktopEntry("Name")
	version := ai.DesktopEntry("X-AppImage-Version")
	appstreamid := ai.ID()

	cyan := fancy.Print{}
	cyan.Color(fancy.Cyan)

	yellow := fancy.Print{}
	yellow.Color(fancy.Yellow)

	nocolor := fancy.Print{}
	nocolor.Color(fancy.Red).Dim().Bold()

	yescolor := fancy.Print{}
	yescolor.Color(fancy.Green).Dim().Bold()

	no := nocolor.Format("no")
	yes := yescolor.Format("yes")

	installedStr := no
	if integrate.IsIntegrated(ai) {
		installedStr = yes
	}

	hasOkSig := false

	fmt.Printf("Name: %s\n"+
		"\t  Version: %s\n"+
		"\tInstalled: %s\n"+
		"\t     Path: %s\n"+
		"\t       ID: %s\n"+
		"",
		cyan.Format(name), yellow.Format(version), installedStr, path, appstreamid)

	fmt.Print("\tSignature: ")
	if ai.HasSignature() {
		sig, ok, err := ai.Signature()
		if err == nil {
			if ok {
				hasOkSig = true
				fmt.Printf("%s ", yes)
				fmt.Printf("[Primary Key ID: %s] (trust on first use)\n", sig.PrimaryKey.KeyIdString())
				idprint := fancy.Print{}
				idprint.Color(fancy.Yellow)
				for _, i := range sig.Identities {
					fmt.Printf("\t           %s: %s\n", idprint.Format("Identity"), i.Name)
				}
			}
		}
	}
	if !hasOkSig {
		fmt.Printf("%s\n", no)
	}

	return nil
}

func findAppImagefromCLIArgs(name, id string) string {
	if id != "" {
		path, found, err := integrate.FindImageById(appimage.AppImageID(id))
		if err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"searching images: %s\n", err)
			os.Exit(1)
		}
		fp := fancy.Print{}
		fp.Color(fancy.Cyan)
		if !found {
			fmt.Fprintf(os.Stderr, ERROR+"No image matching ID '%s' found\n", fp.Format(id))
			os.Exit(1)
		}

		return path
	}

	path, found, err := integrate.FindImageByName(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, ERROR+"searching images: %s\n", err)
		os.Exit(1)
	}
	fp := fancy.Print{}
	fp.Color(fancy.Cyan)
	if !found {
		fmt.Fprintf(os.Stderr, ERROR+"No image matching name '%s' found\n", fp.Format(name))
		os.Exit(1)
	}

	return path
}

type progressReport struct {
	id         int
	percent    int
	terminated bool
	text       string
	err        error
	jobindex   int
	appname    string
}

type upgradeJob struct {
	appImagePath string
	jobindex     int
}

func parallelUpgrade(filesToProcess []string) {
	const maxConcurrency = 10

	percentDone := make(chan progressReport)
	jobs := make(chan upgradeJob)
	defer close(percentDone)

	var progressBars [maxConcurrency]fancy.ProgressBar

	spawned := 0

	concurrencyCount := len(filesToProcess)
	if len(filesToProcess) > maxConcurrency {
		concurrencyCount = maxConcurrency
	}

	//spawn workers
	for i := 0; i < concurrencyCount; i++ {
		progressBars[i] = *fancy.NewProgressBar(20, 100)
		fmt.Println("") // reserve a line to print status
		go upgradeWorker(percentDone, i, jobs)
		spawned++
	}
	fancy.CursorSave()

	//go routine to feed workers
	go func() {
		for i := range filesToProcess {
			jobject := upgradeJob{appImagePath: filesToProcess[i], jobindex: i}
			jobs <- jobject
		}
		close(jobs)
	}()

	fp := fancy.Print{}
	fp.Color(fancy.Cyan)

	errors := make([]error, len(filesToProcess))

	// status printer
	for {
		if spawned == 0 {
			break
		}
		status := <-percentDone
		if status.terminated {
			spawned--
			continue
		}
		if status.err != nil {
			errors[status.jobindex] = status.err
			continue
		}
		fancy.CursorRestore()
		fancy.CursorUp(status.id + 1)
		fancy.CursorColumn(0)
		fancy.EraseRemainingLine()
		progressBars[status.id].Print(status.percent)
		fmt.Print(" " + fp.Format(status.appname) + " " + status.text)
	}

	fancy.CursorRestore()

	for i, err := range errors {
		if err != nil {
			fmt.Printf(ERROR+"Processing '%s': %s", filesToProcess[i], err)
		}
	}
}

func upgradeWorker(status chan progressReport, workerid int, jobs chan upgradeJob) {
newjob:
	for job := range jobs {

		progress := make(chan update.Progress)
		go update.AppImage(job.appImagePath, progress)
		for p := range progress {
			if p.Err != nil {
				status <- progressReport{id: workerid, percent: 100, appname: p.AppName, text: "Error", err: p.Err}
				break newjob
			}
			status <- progressReport{id: workerid, percent: p.Percent, appname: p.AppName, text: p.Text}
		}

	}

	status <- progressReport{terminated: true}
}
