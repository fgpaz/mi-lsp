package daemon

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestBuildVersionStringUsesInjectedReleaseVersion(t *testing.T) {
	previous := model.BuildVersion
	model.BuildVersion = "v1.2.3"
	t.Cleanup(func() { model.BuildVersion = previous })

	if got := buildVersionString(); got != model.BuildVersion {
		t.Fatalf("buildVersionString() = %q, want injected %q", got, model.BuildVersion)
	}
}
