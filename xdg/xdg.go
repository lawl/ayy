package xdg

import (
	"os"
	"path/filepath"
	"strconv"
)

const (
	DATA_HOME = iota
	CONFIG_HOME
	STATE_HOME
	CACHE_HOME
	RUNTIME_DIR
)

func Get(dir int) string {
	switch dir {
	case DATA_HOME:
		return withfallback("XDG_DATA_HOME", filepath.Join(os.Getenv("HOME"), ".local/share"))
	case CONFIG_HOME:
		return withfallback("XDG_CONFIG_HOME", filepath.Join(os.Getenv("HOME"), ".config"))
	case STATE_HOME:
		return withfallback("XDG_STATE_HOME", filepath.Join(os.Getenv("HOME"), ".local/state"))
	case CACHE_HOME:
		return withfallback("XDG_CACHE_HOME", filepath.Join(os.Getenv("HOME"), ".cache"))
	case RUNTIME_DIR:
		return withfallback("XDG_RUNTIME_DIR", filepath.Join("/run/user/"+strconv.Itoa(os.Getuid())))
	default:
		panic("invalid or unhandled XDG constant supplied")
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func withfallback(xdg, fallback string) string {
	xdgdir := os.Getenv(xdg)
	if xdgdir == "" || !exists(xdgdir) {
		if !exists(fallback) {
			if xdg == "XDG_RUNTIME_DIR" {
				os.MkdirAll(fallback, 0700)
			} else {
				os.MkdirAll(fallback, 0755) // we don't care if these fail, that's just a best effort attempt anyways.
			}
		}
		return fallback
	}
	return xdgdir
}
