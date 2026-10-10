package model

// BuildVersion and BuildRevision are populated by release linker flags. They
// are shared by the CLI and daemon so both report the same release metadata.
var (
	BuildVersion  string
	BuildRevision string
)

// ReleaseMetadata overlays injected release metadata on Go build metadata.
func ReleaseMetadata(version, revision string) (string, string) {
	if BuildVersion != "" {
		version = BuildVersion
	}
	if version == "" {
		version = "unknown"
	}
	if BuildRevision != "" {
		revision = BuildRevision
	}
	return version, revision
}
