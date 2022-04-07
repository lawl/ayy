package update

import (
	"ayy/fakezsync"
	"ayy/parallel"
	"bytes"
)

type httpsUpdater struct {
	remoteZsync string
	localPath   string
}

func (upd httpsUpdater) check() (url string, available bool, err error) {
	var zsync fakezsync.Zsync
	var sha []byte
	err = parallel.BailFast(func() error {
		var err error
		zsync, err = fakezsync.Parse(upd.remoteZsync)
		if err != nil {
			return err
		}
		return nil
	}, func() error {
		var err error
		sha, err = sha1file(upd.localPath)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}

	if bytes.Compare(zsync.SHA1, sha) != 0 {
		return zsync.URL, true, nil
	}

	return "", false, nil
}
