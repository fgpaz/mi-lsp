package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

func TestDoctorRuntimeFailureMakesReportFail(t *testing.T) {
	for _, kind := range []string{"removed", "corrupt"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("LOCALAPPDATA", home)
			t.Setenv("XDG_DATA_HOME", home)
			if kind == "removed" {
				path := filepath.Join(home, "runtime")
				if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := runtimeinfo.Record("runtime", path); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				p := runtimeinfo.Locate().ReceiptPath
				if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			report, err := RunDoctor(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if report.ExitCode != 1 {
				t.Fatalf("exit = %d", report.ExitCode)
			}
			for _, check := range report.Checks {
				if check.ID == "runtime-location" {
					if !check.Required || check.Status != "fail" || check.OK {
						t.Fatalf("check: %+v", check)
					}
					if kind == "removed" && check.ReasonCode != "RUNTIME_REMOVED" {
						t.Fatalf("reason: %+v", check)
					}
					if kind == "corrupt" && check.ReasonCode != "REPAIR_FAILED" {
						t.Fatalf("reason: %+v", check)
					}
					return
				}
			}
			t.Fatal("missing runtime check")
		})
	}
}
