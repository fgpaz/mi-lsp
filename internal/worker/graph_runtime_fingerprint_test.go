package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoEnvConfigDigestHashesContentsWithoutExposure(t *testing.T) {
	config := filepath.Join(t.TempDir(), "goenv")
	t.Setenv("GOENV", config)
	if err := os.WriteFile(config, []byte("CGO_ENABLED=0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := goEnvConfigDigest()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(first, "CGO_ENABLED=0") {
		t.Fatal("GOENV digest exposed raw settings")
	}
	if err := os.WriteFile(config, []byte("CGO_ENABLED=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := goEnvConfigDigest()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("GOENV content change did not change config digest")
	}
}

func TestGoEnvConfigDigestResolvesDefaultPath(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	config := filepath.Join(xdg, "go", "env")
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("CGO_ENABLED=0\n")
	if err := os.WriteFile(config, contents, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("GOENV", "")

	got, err := goEnvConfigDigest()
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := sha256.Sum256(contents)
	if got != hex.EncodeToString(wantBytes[:]) {
		t.Fatalf("default GOENV digest = %q, want content digest", got)
	}
}

func TestGoEnvConfigDigestHonorsOff(t *testing.T) {
	t.Setenv("GOENV", "off")
	first, err := goEnvConfigDigest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := goEnvConfigDigest()
	if err != nil {
		t.Fatal(err)
	}
	if first != "off" || second != first {
		t.Fatalf("GOENV=off digest = %q then %q, want stable off sentinel", first, second)
	}
}

func TestGraphObserverRuntimeFingerprintDoesNotCacheUnreadableGoEnv(t *testing.T) {
	// A directory is resolved by `go env GOENV`, but cannot be read as a
	// configuration file. Failures need a fresh key rather than a stable one.
	t.Setenv("GOENV", t.TempDir())
	root := t.TempDir()
	before := goEnvFingerprintFailureNonce.Load()
	_ = GraphObserverRuntimeFingerprint(root)
	middle := goEnvFingerprintFailureNonce.Load()
	_ = GraphObserverRuntimeFingerprint(root)
	after := goEnvFingerprintFailureNonce.Load()
	if middle != before+1 || after != middle+1 {
		t.Fatalf("unreadable GOENV did not force a per-call uncacheable nonce: %d -> %d -> %d", before, middle, after)
	}
}
