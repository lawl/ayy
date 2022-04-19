package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lawl/ayy/appimage"
	"github.com/lawl/ayy/bytesz"
	"github.com/lawl/ayy/fancy"
	"github.com/lawl/ayy/integrate"
	"github.com/lawl/ayy/squashfs"
	"github.com/lawl/ayy/update"
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

	flag.Usage = func() {
		fmt.Fprint(os.Stderr,
			"usage ayy <command>\n"+
				"\n"+
				"  install            Install an AppImage and integrate it into the desktop environment\n"+
				"  remove             Locate installed AppImage by name, uninstall and unintegrate it\n"+
				"  upgrade            Update all images in Applications folder\n"+
				"  list               Display installed AppImages\n"+
				"  alias              Manage aliases for AppImage in PATH\n"+
				"  show               Show details of an AppImage\n"+
				"  fs                 Interact with an AppImage's internal filesystem\n"+
				"  inspect            Inspect an AppImage file. Development command. Dumps assorted information.\n"+
				"  help               Display this help\n"+
				"\n"+
				"Call these commands without any arguments for per command help.\n"+
				"\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	switch flag.Arg(0) {
	case "inspect":
		inspect := flag.NewFlagSet("inspect", flag.ExitOnError)
		inspect.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: ayy inspect /foo/bar.AppImage\n"+
				"\n")
			inspect.PrintDefaults()
		}
		inspect.Parse(flag.Args()[1:])

		if inspect.NArg() < 1 {
			inspect.Usage()
			os.Exit(1)
		}

		fp := fancy.Print{}
		fp.Color(fancy.Yellow)
		for _, arg := range inspect.Args() {
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
			printAppImageDetails(arg)
		}

	case "fs":

		fs := flag.NewFlagSet("fs", flag.ExitOnError)
		fs.Usage = func() {
			fmt.Fprintf(os.Stderr,
				"usage: ayy fs /foo/bar.AppImage command\n"+
					"\n"+
					"commands:\n"+
					"  ls <path>          List files under the specified path inside the AppImage\n"+
					"  cat <path>         Print the file at <path> inside the AppImage to stdout\n"+
					"\n")
			fs.PrintDefaults()
		}
		fs.Parse(flag.Args()[1:])

		if fs.NArg() < 1 {
			fs.Usage()
			os.Exit(1)
		}
		file := fs.Arg(0)
		switch fs.Arg(1) {
		case "ls":
			ls := flag.NewFlagSet("ls", flag.ExitOnError)
			ls.Usage = func() {
				fmt.Fprintf(os.Stderr,
					"usage: ayy fs /foo/bar.AppImage ls <path inside appimage>\n"+
						"\n")
				ls.PrintDefaults()
			}
			usebytes := ls.Bool("b", false, "Display sizes in bytes instead of human readable string")
			if err := ls.Parse(fs.Args()[2:]); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Unable to parse flags: %s\n", err)
				os.Exit(1)
			}
			if ls.NArg() < 1 {
				ls.Usage()
				os.Exit(1)
			}

			for _, arg := range ls.Args() {
				fmt.Printf("%s:\n", arg)
				listFiles(file, ls.Arg(0), *usebytes)
			}
			os.Exit(0)
		case "cat":
			cat := flag.NewFlagSet("cat", flag.ExitOnError)
			cat.Usage = func() {
				fmt.Fprintf(os.Stderr,
					"usage: ayy fs /foo/bar.AppImage cat <path to file inside appimage>\n"+
						"\n")
				cat.PrintDefaults()
			}
			if err := cat.Parse(fs.Args()[2:]); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Unable to parse flags: %s\n", err)
				os.Exit(1)
			}
			if cat.NArg() < 1 {
				cat.Usage()
				os.Exit(1)
			}
			for _, arg := range cat.Args() {
				catFile(file, arg)
			}
			os.Exit(0)
		default:
			fs.Usage()
			os.Exit(1)
		}

	case "lmao":
		fmt.Println("ayy lmao")
		os.Exit(0)
	case "install":
		install := flag.NewFlagSet("install", flag.ExitOnError)
		install.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: ayy install /foo/bar.AppImage\n"+
				"\n"+
				"this is currently required to be a local path, but may also allow https urls in the future. Stay tuned.\n"+
				"\n")
			install.PrintDefaults()
		}
		install.Parse(flag.Args()[1:])

		if install.NArg() < 1 {
			install.Usage()
			os.Exit(1)
		}

		for _, arg := range install.Args() {
			_, err := integrate.Install(arg, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"Cannot install AppImage: %s\n", err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	case "remove":
		remove := flag.NewFlagSet("remove", flag.ExitOnError)
		id := remove.Bool("id", false, "use id instead of name")
		remove.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: ayy remove <name>\n"+
				"\n"+
				"Hint: Find names with 'ayy list'\n"+
				"\n")
			remove.PrintDefaults()
		}
		remove.Parse(flag.Args()[1:])

		if remove.NArg() < 1 {
			remove.Usage()
			os.Exit(1)
		}

		for _, arg := range remove.Args() {
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
	case "upgrade":
		// TODO: support arguments, so that e.g. "ayy upgrade foo bar" only updates foo and bar
		appList, _ := integrate.List()
		parallelUpgrade(appList)
		os.Exit(0)
	case "show":
		show := flag.NewFlagSet("show", flag.ExitOnError)
		id := show.Bool("id", false, "use id instead of name")
		show.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: ayy show <name>\n"+
				"\n"+
				"Hint: Find names with 'ayy list'\n"+
				"\n")
			show.PrintDefaults()
		}
		show.Parse(flag.Args()[1:])

		if show.NArg() < 1 {
			show.Usage()
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
	case "alias":
		alias := flag.NewFlagSet("alias", flag.ExitOnError)
		id := alias.Bool("id", false, "use id instead of name")
		add := alias.String("add", "", "Add an alias")
		remove := alias.String("remove", "", "Remove an alias")
		list := alias.Bool("list", false, "List all aliases")
		alias.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: ayy alias <name>\n"+
				"\n"+
				"Hint: Find names with 'ayy list'\n"+
				"\n")
			alias.PrintDefaults()
		}
		alias.Parse(flag.Args()[1:])

		if *list {
			l := integrate.ListPathWrappers()
			yellow := fancy.Print{}
			yellow.Color(fancy.Yellow)
			cyan := fancy.Print{}
			cyan.Color(fancy.Cyan)
			for _, pwe := range l {
				wrapperName := pwe.WrapperName
				appName := pwe.AppImagePath
				ai := ai(pwe.AppImagePath)
				entryName := ai.DesktopEntry("Name")
				if entryName != "" {
					appName = cyan.Format(entryName)
				}
				fmt.Printf("%s %s %s\n", wrapperName, yellow.Format(" => "), appName)
			}
			os.Exit(0)
		}

		if *remove != "" {
			if err := integrate.RemovePathWrapper(*remove); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"not removing wrapper: %s\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}

		if alias.NArg() < 1 {
			alias.Usage()
			os.Exit(1)
		}

		path := findAppImagefromCLIArgs(alias.Arg(0), *id)
		if *add != "" {
			if err := integrate.CreatePathWrapper(*add, path); err != nil {
				fmt.Fprintf(os.Stderr, ERROR+"cannot create wrapper: %s\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}

		//printaliases()
		os.Exit(0)
	case "help", "-h", "--help":
		flag.Usage()
		os.Exit(0)
	default:
		flag.Usage()
		os.Exit(1)
	}
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
	lst, nNotAI := integrate.List()

	cyan := fancy.Print{}
	cyan.Color(fancy.Cyan)

	normal := fancy.Print{}

	tbl := newTable(40, -1).withFormatters(cyan, normal)
	tbl.printHead("Name", "ID")
	for _, v := range lst {
		ai := ai(v)
		defer ai.Close()

		name := ai.DesktopEntry("Name")
		id := ai.ID()

		tbl.printRow(name, string(id))

	}
	if nNotAI > 0 {
		fmt.Println()
		fmt.Fprintf(os.Stdout, INFO+"%d files in Application folder are not AppImages.\n", nNotAI)
	}
}

func printAppImageDetails(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
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

	wrappers := integrate.PathWrappersForAppImage(path)
	wrapperNames := make([]string, len(wrappers))
	for i, pwe := range wrappers {
		wrapperNames[i] = pwe.WrapperName
	}
	wrapperstr := strings.Join(wrapperNames, ", ")

	fmt.Printf("Name: %s\n"+
		"\t  Version: %s\n"+
		"\tInstalled: %s\n"+
		"\t  Aliases: %s\n"+
		"\t     Path: %s\n"+
		"\t       ID: %s\n"+
		"",
		cyan.Format(name), yellow.Format(version), installedStr, wrapperstr, path, appstreamid)

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

func findAppImagefromCLIArgs(name string, id bool) string {
	if id {
		path, found, err := integrate.FindImageById(appimage.AppImageID(name))
		if err != nil {
			fmt.Fprintf(os.Stderr, ERROR+"searching images: %s\n", err)
			os.Exit(1)
		}
		fp := fancy.Print{}
		fp.Color(fancy.Cyan)
		if !found {
			fmt.Fprintf(os.Stderr, ERROR+"No image matching ID '%s' found\n", fp.Format(name))
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
