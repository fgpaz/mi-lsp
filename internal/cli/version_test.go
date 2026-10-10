package cli

import (
	"strings"
	"testing"
)

func TestReleaseLdflagsMetadataOverridesGoBuildInfo(t *testing.T) {
	previousVersion, previousRevision := buildVersion, buildRevision
	buildVersion = "v1.2.3"
	buildRevision = "0123456789abcdef0123456789abcdef01234567"
	t.Cleanup(func() {
		buildVersion, buildRevision = previousVersion, previousRevision
	})

	rootInfo := buildRootVersionInfo("")
	if rootInfo.Version != buildVersion || rootInfo.VCSRevision != buildRevision {
		t.Fatalf("root build metadata = version %q revision %q; want %q and %q", rootInfo.Version, rootInfo.VCSRevision, buildVersion, buildRevision)
	}
	if got := rootVersionString(rootInfo); !strings.HasPrefix(got, "v1.2.3 revision=0123456789ab ") {
		t.Fatalf("root --version output = %q; want release version and revision", got)
	}

	commandInfo := buildVersionInfo("")
	if commandInfo.Version != buildVersion || commandInfo.VCSRevision != buildRevision {
		t.Fatalf("version command metadata = version %q revision %q; want %q and %q", commandInfo.Version, commandInfo.VCSRevision, buildVersion, buildRevision)
	}
}
