package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type Asset struct {
	Name, URL string
	Size      int64
}
type Release struct {
	Version string
	Assets  []Asset
}
type Source interface {
	Latest(context.Context) (Release, error)
	Download(context.Context, Asset, string) error
}

var errNoRelease = errors.New("release unavailable")
var errNetwork = errors.New("network unavailable")

const maxMetadata = 2 << 20
const maxAsset = 512 << 20
const latestURL = "https://api.github.com/repos/fgpaz/mi-lsp/releases/latest"

type githubSource struct{ client *http.Client }

func allowedURL(raw string, initial bool) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Host != u.Hostname() || u.Fragment != "" {
		return false
	}
	switch u.Hostname() {
	case "api.github.com":
		return u.Path == "/repos/fgpaz/mi-lsp/releases/latest" && u.RawQuery == ""
	case "github.com":
		return strings.HasPrefix(u.EscapedPath(), "/fgpaz/mi-lsp/releases/download/") && !strings.Contains(u.Path, "..") && u.RawQuery == ""
	case "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return !initial
	}
	return false
}
func newGitHubSource(client *http.Client) Source {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	if copyClient.Timeout == 0 {
		copyClient.Timeout = 30 * time.Second
	}
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 || !allowedURL(req.URL.String(), false) {
			return errors.New("redirect rejected")
		}
		return nil
	}
	return &githubSource{client: &copyClient}
}
func (s *githubSource) get(ctx context.Context, raw string) (*http.Response, error) {
	if !allowedURL(raw, true) {
		return nil, errors.New("invalid release URL")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return nil, errors.New("invalid release URL")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, e := s.client.Do(req)
	if e != nil {
		return nil, errNetwork
	}
	if res.StatusCode == http.StatusNotFound {
		res.Body.Close()
		return nil, errNoRelease
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, errNetwork
	}
	return res, nil
}
func (s *githubSource) Latest(ctx context.Context) (Release, error) {
	r, e := s.get(ctx, latestURL)
	if e != nil {
		return Release{}, e
	}
	defer r.Body.Close()
	b, e := io.ReadAll(io.LimitReader(r.Body, maxMetadata+1))
	if e != nil || len(b) > maxMetadata {
		return Release{}, errNetwork
	}
	var v struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if json.Unmarshal(b, &v) != nil {
		return Release{}, errNoRelease
	}
	version := strings.TrimPrefix(v.Tag, "v")
	if _, e = compareVersions(version, version); e != nil {
		return Release{}, errNoRelease
	}
	out := Release{Version: version}
	for _, a := range v.Assets {
		if a.Name == "" || path.Base(a.Name) != a.Name || !allowedURL(a.URL, true) {
			continue
		}
		out.Assets = append(out.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
	}
	return out, nil
}
func (s *githubSource) Download(ctx context.Context, a Asset, dst string) error {
	if a.Size < 0 || a.Size > maxAsset || !allowedURL(a.URL, true) || !strings.HasPrefix(a.URL, "https://github.com/fgpaz/mi-lsp/releases/download/") {
		return errors.New("invalid asset")
	}
	r, e := s.get(ctx, a.URL)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	limit := int64(maxAsset)
	if a.Size > 0 {
		limit = a.Size
	}
	f, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return errors.New("cannot stage asset")
	}
	n, e := io.Copy(f, io.LimitReader(r.Body, limit+1))
	closeErr := f.Close()
	if e != nil || closeErr != nil || n > limit || (a.Size > 0 && n != a.Size) {
		_ = os.Remove(dst)
		return errors.New("invalid asset size or download")
	}
	return nil
}
