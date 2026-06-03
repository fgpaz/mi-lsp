package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

func newVersionCommand(state *rootState) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show mi-lsp build and runtime provenance",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildVersionInfo(state.repoRoot)
			opts := state.queryOptions(cmd, "version", nil)
			if !flagChanged(cmd, "format") {
				opts.Format = "text"
			}
			return state.printEnvelope(model.Envelope{
				Ok:      true,
				Backend: "version",
				Items:   []model.VersionInfo{info},
				Stats:   model.Stats{Files: 1},
			}, opts)
		},
	}
}

func buildVersionInfo(toolRoot string) model.VersionInfo {
	cliPath, executableHash := executableSnapshot()
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return model.VersionInfo{
			Command:          "mi-lsp",
			Version:          "unknown",
			GoVersion:        runtime.Version(),
			GOOS:             runtime.GOOS,
			GOARCH:           runtime.GOARCH,
			ProtocolVersion:  model.ProtocolVersion,
			WorkerRID:        worker.ResolveRID(),
			ToolRoot:         toolRoot,
			CLIPath:          cliPath,
			ExecutableSHA256: executableHash,
		}
	}

	version := info.Main.Version
	if version == "" {
		version = "unknown"
	}

	return model.VersionInfo{
		Command:          "mi-lsp",
		Version:          version,
		ModulePath:       info.Main.Path,
		GoVersion:        info.GoVersion,
		GOOS:             runtime.GOOS,
		GOARCH:           runtime.GOARCH,
		ProtocolVersion:  model.ProtocolVersion,
		WorkerRID:        worker.ResolveRID(),
		ToolRoot:         toolRoot,
		CLIPath:          cliPath,
		ExecutableSHA256: executableHash,
		VCSRevision:      buildSetting(info, "vcs.revision"),
		VCSTime:          buildSetting(info, "vcs.time"),
		VCSModified:      buildSetting(info, "vcs.modified"),
	}
}

func buildRootVersionInfo(toolRoot string) model.VersionInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return model.VersionInfo{
			Command:         "mi-lsp",
			Version:         "unknown",
			GoVersion:       runtime.Version(),
			GOOS:            runtime.GOOS,
			GOARCH:          runtime.GOARCH,
			ProtocolVersion: model.ProtocolVersion,
			WorkerRID:       worker.ResolveRID(),
			ToolRoot:        toolRoot,
		}
	}
	version := info.Main.Version
	if version == "" {
		version = "unknown"
	}
	return model.VersionInfo{
		Command:         "mi-lsp",
		Version:         version,
		ModulePath:      info.Main.Path,
		GoVersion:       info.GoVersion,
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		ProtocolVersion: model.ProtocolVersion,
		WorkerRID:       worker.ResolveRID(),
		ToolRoot:        toolRoot,
		VCSRevision:     buildSetting(info, "vcs.revision"),
		VCSTime:         buildSetting(info, "vcs.time"),
		VCSModified:     buildSetting(info, "vcs.modified"),
	}
}

func rootVersionString(info model.VersionInfo) string {
	parts := []string{strings.TrimSpace(info.Version)}
	if strings.TrimSpace(info.VCSRevision) != "" {
		revision := info.VCSRevision
		if len(revision) > 12 {
			revision = revision[:12]
		}
		parts = append(parts, "revision="+revision)
	}
	parts = append(parts, "protocol="+info.ProtocolVersion)
	parts = append(parts, "rid="+info.WorkerRID)
	return strings.Join(nonEmptyStrings(parts), " ")
}

func nonEmptyStrings(items []string) []string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func executableSnapshot() (string, string) {
	path, err := os.Executable()
	if err != nil {
		return "", ""
	}
	hash, err := fileSHA256(path)
	if err != nil {
		return path, ""
	}
	return path, hash
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func buildSetting(info *debug.BuildInfo, key string) string {
	if info == nil {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}
