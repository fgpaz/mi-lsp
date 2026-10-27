package repair

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyRejectsStateWithMissingFieldsWithoutOverwriting(t *testing.T) {
	cases := []string{`{}`, `null`, `{"attempts":[]}`, `{"schema":"mi-lsp-repair-state/v1"}`}
	for _, content := range cases {
		t.Run(content, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("LOCALAPPDATA", home)
			t.Setenv("XDG_DATA_HOME", home)
			path, err := statePath()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			report := Run(Options{Apply: true, Worker: true})
			if report.OK {
				t.Fatalf("expected blocked report: %+v", report)
			}
			found := false
			for _, code := range report.ReasonCodes {
				if code == "REPAIR_STATE_INVALID" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing reason: %+v", report)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != content {
				t.Fatalf("state changed: %q: %v", after, err)
			}
		})
	}
}
