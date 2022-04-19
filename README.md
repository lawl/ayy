# ayy: decentralized linux package management

Another package manager *yawn*...? *No wait*, this one's *different*, I swear.

[✓] does not invent a new package format  
[✓] works with existing packages  
[✓] you don't need anyone's permission to make your application available to users. Any web server will do. Independent of any stores.  
[✓] distribution agnostic  
[✓] pretty fast  
[✓] single, static, dependency free binary  
[✓] no extra daemons  
[✓] get your software from the source. no bug-introducing middlemen  
[✓] TOFU signature scheme (like e.g. Android)


ayy is a package manager for [AppImage](https://appimage.org/). You might have seen those and wondered why there is no icon? Why is it not in the menu of my GNOME/KDE? What about updates?  

AppImages actually also have support for embedding this information, in a standardized way and `ayy` makes use of that.

`ayy` is a project to make use of those things. It takes AppImages, adds menu entries and icons to your desktop environment, allows you to set up aliases for use in $PATH, and lets you update all applications with a single command (provided they embed update information).

AppImages are already available in lots of places, because they solve a problem for developers with or without `ayy`. However, as a user, I've found them annoying to use. This project fixes that.

It should be simple to use, and fast:
```shell
$ time ayy install VSCodium-x86_64.AppImage 

real    0m0.057s
user    0m0.046s
sys     0m0.017s
```

That's it.

## How do I get it?

`ayy` is currently in alpha and you will need to download the source code and build it. You will need Go 1.18 or newer. Building is as simple as

    go build

in the `ayy` directory. This will produce the `ayy` binary, i suggest you drop it somewhere into your $PATH.

Usage:

```
$ ayy -h
usage ayy <command>

  install            Install an AppImage and integrate it into the desktop environment
  remove             Locate installed AppImage by name, uninstall and unintegrate it
  upgrade            Update all images in Applications folder
  list               Display installed AppImages
  alias              Manage aliases for AppImage in PATH
  show               Show details of an AppImage
  fs                 Interact with an AppImage's internal filesystem
  inspect            Inspect an AppImage file. Development command. Dumps assorted information.
  help               Display this help

Call these commands without any arguments for per command help.
```

Pre-built binaries and a proper installation-guide will be provided once the project is ready for a wider user base.

## Contributing

`ayy` does currently not accept Pull Requests, since I'm not sure if I want to keep the current license. Please do open issues though.