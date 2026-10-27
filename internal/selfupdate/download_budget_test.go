package selfupdate

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type distributionRoundTripper func(*http.Request) (*http.Response, error)

func (f distributionRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOfficialAssetDownloadBudgetExceedsMetadataBudget(t *testing.T) {
	for _, timeout := range []time.Duration{0, 7 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			var assetBudget, metadataBudget time.Duration
			transport := distributionRoundTripper(func(req *http.Request) (*http.Response, error) {
				deadline, ok := req.Context().Deadline()
				if !ok {
					t.Fatal("unbounded HTTP request")
				}
				body := "x"
				if req.URL.Host == "api.github.com" {
					metadataBudget = time.Until(deadline)
					body = `{"tag_name":"v0.16.0","assets":[]}`
				} else {
					assetBudget = time.Until(deadline)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			source := newGitHubSource(&http.Client{Transport: transport, Timeout: timeout})
			if _, err := source.Latest(context.Background()); err != nil {
				t.Fatal(err)
			}
			err := source.Download(context.Background(), Asset{Name: "fixture.zip", URL: "https://github.com/fgpaz/mi-lsp/releases/download/v0.16.0/fixture.zip", Size: 1}, filepath.Join(t.TempDir(), "asset.zip"))
			if err != nil {
				t.Fatal(err)
			}
			if timeout == 0 {
				if metadataBudget > 31*time.Second || assetBudget < 120*time.Second || assetBudget > 301*time.Second {
					t.Fatalf("official 288MB asset uses metadata-sized budget: metadata=%s asset=%s", metadataBudget, assetBudget)
				}
			} else if assetBudget > timeout+time.Second || assetBudget < 5*time.Second || metadataBudget > timeout+time.Second {
				t.Fatalf("explicit client timeout overridden: metadata=%s asset=%s", metadataBudget, assetBudget)
			}
		})
	}
}
