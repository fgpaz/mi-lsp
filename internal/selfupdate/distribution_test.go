package selfupdate

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type distributionZeroReader struct{}

func (distributionZeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

// Public 0.15.6 win-arm64 has 708896784 expanded bytes (1973 entries).
// Its ignored worker payload is legitimate, not a zip bomb.
func TestTransactionAcceptsOfficialScaleWrappedDistribution(t *testing.T) {
	target := transactionTarget(t)
	archive := filepath.Join(t.TempDir(), officialWrapper()+".zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	cli, err := zw.Create(officialWrapper() + "/" + cliName())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(cli, "new-image"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 9; i++ {
		w, err := zw.Create(officialWrapper() + "/workers/fixture/" + string(rune('a'+i)) + ".bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.CopyN(w, distributionZeroReader{}, 64<<20); err != nil {
			t.Fatal(err)
		}
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
	if err != nil || !result.Changed || result.Installed != "0.16.0" {
		t.Fatalf("valid official-scale archive rejected: result=%+v err=%v", result, err)
	}
	assertImage(t, target, "new-image")
	if _, err = os.Stat(filepath.Join(filepath.Dir(target), "workers")); !os.IsNotExist(err) {
		t.Fatal("ignored workers were promoted")
	}
}
