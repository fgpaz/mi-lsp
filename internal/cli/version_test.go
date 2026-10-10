package cli

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestReleaseLdflagsMetadataOverridesGoBuildInfo(t *testing.T) {
	previousVersion, previousRevision := model.BuildVersion, model.BuildRevision
	model.BuildVersion = "v1.2.3"
	model.BuildRevision = "0123456789abcdef0123456789abcdef01234567"
	t.Cleanup(func() {
		model.BuildVersion, model.BuildRevision = previousVersion, previousRevision
	})

	rootInfo := buildRootVersionInfo("")
	if rootInfo.Version != model.BuildVersion || rootInfo.VCSRevision != model.BuildRevision {
		t.Fatalf("root build metadata = version %q revision %q; want %q and %q", rootInfo.Version, rootInfo.VCSRevision, model.BuildVersion, model.BuildRevision)
	}
	if got := rootVersionString(rootInfo); !strings.HasPrefix(got, "v1.2.3 revision=0123456789ab ") {
		t.Fatalf("root --version output = %q; want release version and revision", got)
	}

	commandInfo := buildVersionInfo("")
	if commandInfo.Version != model.BuildVersion || commandInfo.VCSRevision != model.BuildRevision {
		t.Fatalf("version command metadata = version %q revision %q; want %q and %q", commandInfo.Version, commandInfo.VCSRevision, model.BuildVersion, model.BuildRevision)
	}
}
