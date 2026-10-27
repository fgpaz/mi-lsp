package selfupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func httpResponse(r *http.Request, status int, body string, headers http.Header) *http.Response {
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func TestVersionCompareWithCases(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
		bad  bool
	}{{"0.16.0", "0.16.0", 0, false}, {"0.16.10", "0.16.2", 1, false}, {"0.9.0", "0.10.0", -1, false}, {"1.0.0-rc.1", "1.0.0", -1, false}, {"1.0.0+build.1", "1.0.0", 0, false}, {"nonsense", "0.16.0", 0, true}, {"1.02.0", "1.2.0", 0, true}} {
		t.Run(tc.a+"/"+tc.b, func(t *testing.T) {
			got, err := compareVersions(tc.a, tc.b)
			if tc.bad {
				if err == nil {
					t.Fatal("invalid version accepted")
				}
				return
			}
			if err != nil || (got < 0) != (tc.want < 0) || (got > 0) != (tc.want > 0) {
				t.Fatalf("compare=%d, err=%v", got, err)
			}
		})
	}
}
func TestGitHubLatestFixedEndpoint(t *testing.T) {
	calls := 0
	c := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.github.com/repos/fgpaz/mi-lsp/releases/latest" {
			t.Fatalf("unexpected URL %s", r.URL.Redacted())
		}
		return httpResponse(r, 200, `{"tag_name":"v0.17.0","assets":[{"name":"mi-lsp_0.17.0_checksums.txt","browser_download_url":"https://github.com/fgpaz/mi-lsp/releases/download/v0.17.0/mi-lsp_0.17.0_checksums.txt","size":12}]}`, nil), nil
	})}
	rel, err := newGitHubSource(c).Latest(context.Background())
	if err != nil || rel.Version != "0.17.0" || len(rel.Assets) != 1 || calls != 1 {
		t.Fatalf("release=%+v calls=%d err=%v", rel, calls, err)
	}
}
func TestGitHubDownloadScopeAndRedirects(t *testing.T) {
	official := "https://github.com/fgpaz/mi-lsp/releases/download/v0.17.0/a.zip"
	for _, tc := range []struct {
		name, url, redirect string
		allow               bool
	}{{"official", official, "", true}, {"official redirect", official, "https://release-assets.githubusercontent.com/file", true}, {"objects redirect", official, "https://objects.githubusercontent.com/file", true}, {"unrelated host", "https://evil.invalid/a.zip", "", false}, {"http", "http://github.com/fgpaz/mi-lsp/releases/download/v0.17.0/a.zip", "", false}, {"other repo", "https://github.com/other/mi-lsp/releases/download/v0.17.0/a.zip", "", false}, {"credential", "https://user:pass@github.com/fgpaz/mi-lsp/releases/download/v0.17.0/a.zip", "", false}, {"redirect evil", official, "https://evil.invalid/file", false}, {"redirect downgrade", official, "http://release-assets.githubusercontent.com/file", false}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Hostname() == "evil.invalid" || r.URL.Scheme != "https" {
					t.Fatal("unsafe request reached transport")
				}
				if tc.redirect != "" && calls == 1 {
					return httpResponse(r, 302, "", http.Header{"Location": []string{tc.redirect}}), nil
				}
				return httpResponse(r, 200, "archive", nil), nil
			})}
			dst := filepath.Join(t.TempDir(), "asset")
			err := newGitHubSource(client).Download(context.Background(), Asset{Name: "a.zip", URL: tc.url, Size: 7}, dst)
			if tc.allow && err != nil {
				t.Fatal(err)
			}
			if !tc.allow && err == nil {
				t.Fatal("unsafe URL accepted")
			}
			if !tc.allow {
				if b, e := os.ReadFile(dst); e == nil && len(b) > 0 {
					t.Fatal("unsafe payload persisted")
				}
			}
		})
	}
}
func TestGitHubDownloadRejectsOversize(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return httpResponse(r, 200, strings.Repeat("x", 1024), nil), nil
	})}
	dst := filepath.Join(t.TempDir(), "asset")
	err := newGitHubSource(client).Download(context.Background(), Asset{Name: "a.zip", URL: "https://github.com/fgpaz/mi-lsp/releases/download/v0.17.0/a.zip", Size: 4}, dst)
	if err == nil {
		t.Fatal("oversize payload accepted")
	}
}
func TestGitHubFailuresAndLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
	}{{"network", 0, "", errors.New("offline")}, {"notfound", 404, "", nil}, {"large metadata", 200, strings.Repeat(" ", 8<<20) + `{"tag_name":"v0.17.0","assets":[]}`, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			c := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if tc.err != nil {
					return nil, tc.err
				}
				return httpResponse(r, tc.status, tc.body, nil), nil
			})}
			if _, err := newGitHubSource(c).Latest(context.Background()); err == nil {
				t.Fatal("expected failure")
			}
		})
	}
}
