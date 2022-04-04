package fakezsync

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/url"
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

	Also unadressed is the issue of servers not supporting
	Range HTTP requests. (Possibly some CDNs?)
	And how that is detected and handled without corrupting
	the file.

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

func ResolveZsyncURL(urlstring string) (*url.URL, error) {
	zsync, err := url.Parse(urlstring)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(urlstring)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	fatFile, err := findFatFileLocation(resp.Body)
	if err != nil {
		return nil, err
	}
	faturl, err := url.Parse(fatFile)
	if err != nil {
		return nil, err
	}
	if faturl.IsAbs() {
		return faturl, nil
	}
	return zsync.ResolveReference(faturl), nil
}

func findFatFileLocation(zsyncFile io.Reader) (string, error) {
	buf := bufio.NewReader(zsyncFile)

	header, err := buf.ReadString('\n')
	if err != nil {
		return "", err
	}
	key, _, found := strings.Cut(header, ":")
	if !found || key != "zsync" {
		return "", errors.New("not a zsync file")
	}

	for {
		header, err := buf.ReadString('\n')
		if err != nil {
			return "", err
		}
		key, value, found := strings.Cut(header, ":")
		if !found {
			// must already be in the binary garbage
			// every line above should have a ":"
			return "", errors.New("no URL header found in zsync file")
		}
		if key == "URL" {
			return strings.TrimSpace(value), nil
		}
	}
}
