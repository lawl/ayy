package fakezsync

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

/*
	The integration of zsync is a bit unfortunate on many levels.

	There is no standard format that's specified anywhere, meaning
	you would have to resort to the original C source code.

	I found 2 Go implementations of zsync. Neither was maintained.
	Both had too many dependencies for my taste, both had some
	things i disagreed with that would have required forking.

	Some had open issues that patching files broke them.

	Other implementations, e.g. in C++ report that they're
	also still debugging their libraries.

	To be able to support zsync, I would have to implement it
	myself from scratch.

	But that's not where the trouble ends. Zsync sais it achieves
	small deltas by looking inside the compressed data blocks.

	I'm unsure if or how AppImages make proper use of that.
	Zsync only supports DEFLATE (gzip) but the compression
	in squashfs is configurable.

	ayy *also* only supports DEFLATE AppImages, so maybe that
	would be ok.

	There is zero description if or how AppImageKit makes sure
	that the compressed squashfs blocks align with whatever
	blocks zsync uses. Nor does it take care of the issue
	of enforcing DEFLATE.

	In a quick test where I patched the SquasFS implementation
	to print the offset of compressed blocks of existing
	AppImages I had laying around, none was aligned with the
	blocksize specified in zsync.

	Also unadressed is the issue of servers not supporting
	Range HTTP requests. (Possibly some CDNs?)
	And how that is detected and handled without corrupting
	the file.

	We also cannot patch the file in-place on the file system
	since the zsync file itself it not signed. If we patched
	it in place and then check the signature and figure out
	it doesn't match, we now have a (porentially) malicious
	executable on the system, and really the only thing we can
	do at that point is... immediately delete it. And the user
	having lost their program.

	Then this would also require hacks such as stripping the +x
	during patching, to make sure it cannot be executed before
	the signature has been checked.

	So the only option is to make a copy on the filesystem and
	patch that, with removes further benefits of zsync, as that
	takes time too.

	Last but not least, the question is, if it even matters.
	When using apt, i feel like I'm usually waiting for

	    Processing triggers for man-db...

	or similar, and not downloading.

	So, *our* zsync implementation downloads the zsync-file,
	figures out where the original full file is from there.

	Now we've turned this into a boring HTTPS download problem.
	Way less exciting, hopefully a lot more stable.
*/

/*
	Example zsync file:

	zsync: 0.6.2
	Filename: foobar.AppImage
	MTime: Sat, 01 Dec 2069 04:20:00 +0000
	Blocksize: 2048
	Length: 4206969
	Hash-Lengths: 2,2,4
	URL: foobar.AppImage
	SHA-1: 3e83b13d99bf0de6c6bde5ac5ca4ae687a3d46db

	<binary>

*/

type Zsync struct {
	Version     string
	Filename    string
	Mtime       string
	Blocksize   int
	HashLengths []int
	URL         string
	SHA1        []byte
}

func Parse(urlstring string) (Zsync, error) {
	zsync := Zsync{}

	resp, err := http.Get(urlstring)
	if err != nil {
		return zsync, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return zsync, fmt.Errorf("http status: %d %s", resp.StatusCode, resp.Status)
	}

	buf := bufio.NewReader(resp.Body)

	header, err := buf.ReadString('\n')
	if err != nil {
		return zsync, err
	}
	key, value, found := strings.Cut(header, ":")
	if !found || key != "zsync" {
		return zsync, errors.New("not a zsync file")
	}
	zsync.Version = value

	for {
		header, err := buf.ReadString('\n')
		if err != nil {
			return zsync, err
		}
		header = strings.TrimSpace(header)
		// header seems to be delimited by \n\n
		// so an empty line should mean we're done here
		if header == "" {
			return zsync, nil
		}
		key, value, found := strings.Cut(header, ":")
		if !found {
			// must already be in the binary garbage
			// every line above should have a ":"
			// or something else is weird
			return zsync, errors.New("error parsing zsync file header, delimiter ':' not found")
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "filename":
			zsync.Filename = value
		case "mtime":
			zsync.Mtime = value
		case "blocksize":
			zsync.Blocksize, err = strconv.Atoi(value)
			if err != nil {
				return zsync, err
			}
		case "hashlengths":
			spl := strings.Split(value, ",")
			var lst []int
			for _, v := range spl {
				v = strings.TrimSpace(v)
				in, err := strconv.Atoi(v)
				if err != nil {
					return zsync, err
				}
				lst = append(lst, in)
			}
			zsync.HashLengths = lst
		case "url":
			zsyncUrl, err := url.Parse(urlstring)
			if err != nil {
				return zsync, err
			}
			faturl, err := url.Parse(value)
			if err != nil {
				return zsync, err
			}
			if faturl.IsAbs() {
				zsync.URL = faturl.String()
			} else {
				zsync.URL = zsyncUrl.ResolveReference(faturl).String()
			}
		case "sha-1":
			b, err := hex.DecodeString(value)
			if err != nil {
				return zsync, err
			}
			zsync.SHA1 = b
		default:
			continue
		}
	}
}
