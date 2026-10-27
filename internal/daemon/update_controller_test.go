package daemon

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/selfupdate"
)

func TestUpdateControllerRequiresVerifiedUpdatedDiskVersion(t *testing.T) {
	running := model.DaemonState{Version: "0.15.6+a15bootstrap"}
	for _, tc := range []struct {
		name   string
		status selfupdate.Status
		disk   string
		want   bool
	}{
		{"verified upgrade", selfupdate.Status{LastResult: "updated", Installed: "0.16.0", Latest: "0.16.0"}, "0.16.0", true},
		{"readback mismatch", selfupdate.Status{LastResult: "updated", Installed: "0.16.0", Latest: "0.16.0"}, "0.15.6", false},
		{"latest mismatch", selfupdate.Status{LastResult: "updated", Installed: "0.16.0", Latest: "0.17.0"}, "0.16.0", false},
		{"failed", selfupdate.Status{LastResult: "failed", Installed: "0.16.0", Latest: "0.16.0"}, "0.16.0", false},
		{"downgrade", selfupdate.Status{LastResult: "updated", Installed: "0.15.4", Latest: "0.15.4"}, "0.15.4", false},
		{"same", selfupdate.Status{LastResult: "updated", Installed: "0.15.6+a15bootstrap", Latest: "0.15.6+a15bootstrap"}, "0.15.6+a15bootstrap", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := updateCandidateVerified(running, tc.status, tc.disk); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
