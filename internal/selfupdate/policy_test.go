package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type releaseFixture struct {
	release Release
	files   map[string][]byte
	calls   int
}

func (s *releaseFixture) Latest(context.Context) (Release, error) { s.calls++; return s.release, nil }
func (s *releaseFixture) Download(_ context.Context, a Asset, dst string) error {
	b, ok := s.files[a.Name]
	if !ok {
		return fmt.Errorf("fixture missing asset")
	}
	return os.WriteFile(dst, b, 0600)
}
func archiveFixture(t *testing.T, content []byte) (string, []byte) {
	t.Helper()
	osname := runtime.GOOS
	arch := runtime.GOARCH
	if osname == "windows" {
		osname = "win"
	}
	if arch == "amd64" {
		arch = "x64"
	}
	base := "mi-lsp_0.17.0_" + osname + "-" + arch
	var b bytes.Buffer
	if runtime.GOOS == "windows" {
		w := zip.NewWriter(&b)
		f, e := w.Create("mi-lsp.exe")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(content); e != nil {
			t.Fatal(e)
		}
		if e = w.Close(); e != nil {
			t.Fatal(e)
		}
		return base + ".zip", b.Bytes()
	}
	gz := gzip.NewWriter(&b)
	w := tar.NewWriter(gz)
	if e := w.WriteHeader(&tar.Header{Name: "mi-lsp", Mode: 0755, Size: int64(len(content))}); e != nil {
		t.Fatal(e)
	}
	if _, e := w.Write(content); e != nil {
		t.Fatal(e)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	if e := gz.Close(); e != nil {
		t.Fatal(e)
	}
	return base + ".tar.gz", b.Bytes()
}
func newReleaseFixture(t *testing.T, bad bool) *releaseFixture {
	t.Helper()
	name, archive := archiveFixture(t, []byte("new"))
	sum := sha256.Sum256(archive)
	if bad {
		sum[0] ^= 0xff
	}
	checks := "mi-lsp_0.17.0_checksums.txt"
	url := "https://github.com/fgpaz/mi-lsp/releases/download/v0.17.0/"
	return &releaseFixture{release: Release{Version: "0.17.0", Assets: []Asset{{Name: name, URL: url + name, Size: int64(len(archive))}, {Name: checks, URL: url + checks}}}, files: map[string][]byte{name: archive, checks: []byte(fmt.Sprintf("%x  %s\n", sum, name))}}
}
func TestApplyChecksumAndReadback(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		bad, failReadback       bool
		result, code, installed string
	}{{"bad checksum", true, false, "failed", "UPDATE_CHECKSUM_MISMATCH", "0.16.0"}, {"updated", false, false, "updated", "", "0.17.0"}, {"rollback", false, true, "failed", "UPDATE_READBACK_FAILED_ROLLED_BACK", "0.16.0"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MI_LSP_CHANNEL", "")
			t.Setenv("MI_LSP_AUTO_UPDATE", "")
			c := fixtureConfig(t, &fixtureSource{})
			s := newReleaseFixture(t, tc.bad)
			c.Source = s
			c.Readback = func(_ context.Context, path string) (string, error) {
				b, e := os.ReadFile(path)
				if e != nil {
					return "", e
				}
				switch string(b) {
				case "old":
					return "0.16.0", nil
				case "new":
					if tc.failReadback {
						return "", fmt.Errorf("bad readback")
					}
					return "0.17.0", nil
				}
				return "", fmt.Errorf("unknown bytes")
			}
			st, _ := New(c).Apply(context.Background(), false)
			if st.LastResult != tc.result || st.Installed != tc.installed {
				t.Fatalf("status=%+v", st)
			}
			if tc.code != "" && (st.Code == nil || *st.Code != tc.code) {
				t.Fatalf("code=%v", st.Code)
			}
			if tc.result == "updated" && (st.Previous == nil || *st.Previous != "0.16.0" || st.Latest != st.Installed || st.Code != nil) {
				t.Fatalf("updated=%+v", st)
			}
			if tc.result != "updated" && st.Previous != nil {
				t.Fatalf("false previous=%v", *st.Previous)
			}
			b, e := os.ReadFile(c.ExecutablePath)
			if e != nil || string(b) != map[bool]string{true: "new", false: "old"}[tc.result == "updated"] {
				t.Fatalf("disk=%q %v", b, e)
			}
			if _, e = os.Stat(filepath.Join(c.HomeDir, "workers")); !os.IsNotExist(e) {
				t.Fatal("unexpected worker installation")
			}
		})
	}
}
func TestMissingChecksumsNeverUpdates(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	c := fixtureConfig(t, &fixtureSource{})
	s := newReleaseFixture(t, false)
	s.release.Assets = s.release.Assets[:1]
	c.Source = s
	st, _ := New(c).Apply(context.Background(), false)
	b, err := os.ReadFile(c.ExecutablePath)
	if err != nil || string(b) != "old" || st.LastResult == "updated" {
		t.Fatalf("missing checksums: status=%+v disk=%q err=%v", st, b, err)
	}
}
func TestExplicitPinMarker(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	s := &fixtureSource{}
	c := fixtureConfig(t, s)
	p := filepath.Join(c.HomeDir, ".mi-lsp", "update", "pin.json")
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(`{"schema":"mi-lsp-update-pin/v1","pinned":true}`), 0600); e != nil {
		t.Fatal(e)
	}
	st, _ := New(c).Apply(context.Background(), false)
	if st.LastResult != "pinned" || s.calls != 0 || !strings.EqualFold(st.Channel, "pinned") {
		t.Fatalf("status=%+v calls=%d", st, s.calls)
	}
}
