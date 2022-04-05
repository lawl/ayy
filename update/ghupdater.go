package update

import (
	"ayy/appimage"
	"ayy/fakezsync"
	"ayy/parallel"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
)

type release struct {
	Assets []struct {
		BrowserDownloadURL string      `json:"browser_download_url"`
		ContentType        string      `json:"content_type"`
		CreatedAt          string      `json:"created_at"`
		DownloadCount      int64       `json:"download_count"`
		ID                 int64       `json:"id"`
		Label              interface{} `json:"label"`
		Name               string      `json:"name"`
		NodeID             string      `json:"node_id"`
		Size               int64       `json:"size"`
		State              string      `json:"state"`
		UpdatedAt          string      `json:"updated_at"`
		Uploader           struct {
			AvatarURL         string `json:"avatar_url"`
			EventsURL         string `json:"events_url"`
			FollowersURL      string `json:"followers_url"`
			FollowingURL      string `json:"following_url"`
			GistsURL          string `json:"gists_url"`
			GravatarID        string `json:"gravatar_id"`
			HTMLURL           string `json:"html_url"`
			ID                int64  `json:"id"`
			Login             string `json:"login"`
			NodeID            string `json:"node_id"`
			OrganizationsURL  string `json:"organizations_url"`
			ReceivedEventsURL string `json:"received_events_url"`
			ReposURL          string `json:"repos_url"`
			SiteAdmin         bool   `json:"site_admin"`
			StarredURL        string `json:"starred_url"`
			SubscriptionsURL  string `json:"subscriptions_url"`
			Type              string `json:"type"`
			URL               string `json:"url"`
		} `json:"uploader"`
		URL string `json:"url"`
	} `json:"assets"`
	AssetsURL string `json:"assets_url"`
	Author    struct {
		AvatarURL         string `json:"avatar_url"`
		EventsURL         string `json:"events_url"`
		FollowersURL      string `json:"followers_url"`
		FollowingURL      string `json:"following_url"`
		GistsURL          string `json:"gists_url"`
		GravatarID        string `json:"gravatar_id"`
		HTMLURL           string `json:"html_url"`
		ID                int64  `json:"id"`
		Login             string `json:"login"`
		NodeID            string `json:"node_id"`
		OrganizationsURL  string `json:"organizations_url"`
		ReceivedEventsURL string `json:"received_events_url"`
		ReposURL          string `json:"repos_url"`
		SiteAdmin         bool   `json:"site_admin"`
		StarredURL        string `json:"starred_url"`
		SubscriptionsURL  string `json:"subscriptions_url"`
		Type              string `json:"type"`
		URL               string `json:"url"`
	} `json:"author"`
	Body        string `json:"body"`
	CreatedAt   string `json:"created_at"`
	Draft       bool   `json:"draft"`
	HTMLURL     string `json:"html_url"`
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	NodeID      string `json:"node_id"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	Reactions   struct {
		PlusOne    int64  `json:"+1"`
		MinusOne   int64  `json:"-1"`
		Confused   int64  `json:"confused"`
		Eyes       int64  `json:"eyes"`
		Heart      int64  `json:"heart"`
		Hooray     int64  `json:"hooray"`
		Laugh      int64  `json:"laugh"`
		Rocket     int64  `json:"rocket"`
		TotalCount int64  `json:"total_count"`
		URL        string `json:"url"`
	} `json:"reactions"`
	TagName         string `json:"tag_name"`
	TarballURL      string `json:"tarball_url"`
	TargetCommitish string `json:"target_commitish"`
	UploadURL       string `json:"upload_url"`
	URL             string `json:"url"`
	ZipballURL      string `json:"zipball_url"`
}

type ghUpdater struct {
	ghUsername  string
	ghRepo      string
	releaseName string
	filename    string
	localPath   string
}

func (ghu ghUpdater) hasUpdateAvailable() (url string, available bool, err error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/%s", ghu.ghUsername, ghu.ghRepo, ghu.releaseName)
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	jayson := release{}
	if err = json.NewDecoder(resp.Body).Decode(&jayson); err != nil {
		return "", false, err
	}

	for _, asset := range jayson.Assets {
		match, err := filepath.Match(ghu.filename, asset.Name)
		if err != nil {
			return "", false, err
		}
		if match {
			var zsync fakezsync.Zsync
			var sha []byte
			err := parallel.BailFast(func() error {
				var err error
				zsync, err = fakezsync.Parse(asset.BrowserDownloadURL)
				if err != nil {
					return err
				}
				return nil
			}, func() error {
				var err error
				ai, err := appimage.Open(ghu.localPath)
				if err != nil {
					return err
				}
				sha, err = ai.SHA1WithoutSignature()
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
	}

	return "", false, nil
}
func (n ghUpdater) update() error { return nil }
