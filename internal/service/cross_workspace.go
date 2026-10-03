package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// crossWorkspaceWriteOperations keep the harness cross-workspace gate: they
// mutate state of the selected workspace (index, preparation packets, links).
// Every other workspace-aware operation is a read and proceeds with a warning.
var crossWorkspaceWriteOperations = map[string]bool{
	"index.run":       true,
	"index.start":     true,
	"index.cancel":    true,
	"index.run-job":   true,
	"workspace.link":  true,
	"nav.prepare":     true,
	"prepare.create":  true,
	"prepare.refresh": true,
}

func crossWorkspaceOperationIsWrite(operation string) bool {
	return crossWorkspaceWriteOperations[strings.TrimSpace(operation)]
}

const crossWorkspaceOverrideLog = "overrides.log"

// recordCrossWorkspaceOverride appends one JSON line to ~/.mi-lsp/overrides.log
// each time --allow-cross-workspace unlocks a gated write. It is best effort:
// a failure to record never blocks the operator's override.
func recordCrossWorkspaceOverride(operation, clientName, selector, selectedRoot, callerCWD string) {
	dir, err := workspace.GlobalDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	line, err := json.Marshal(map[string]string{
		"ts":            time.Now().UTC().Format(time.RFC3339),
		"override":      "allow-cross-workspace",
		"operation":     operation,
		"client":        clientName,
		"selector":      selector,
		"selected_root": selectedRoot,
		"caller_cwd":    callerCWD,
	})
	if err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(dir, crossWorkspaceOverrideLog), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(line, '\n'))
}
