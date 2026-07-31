package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// probeStat is a narrow test seam for host filesystems that cannot expose
// unreadable paths consistently (notably Windows ACLs).
var probeStat = os.Stat

// ProbeWorkspace is deliberately independent from App execution, daemon
// routing, indexing, telemetry, and registry writes.
func ProbeWorkspace(ctx context.Context, options model.ProbeOptions) (model.ProbeReport, error) {
	selector := strings.TrimSpace(options.Selector)
	callerCWD := strings.TrimSpace(options.CallerCWD)
	if callerCWD == "" {
		callerCWD, _ = os.Getwd()
	}

	report := model.ProbeReport{Status: model.ProbeStatusUnknown, SideEffects: false}
	report.Workspace.Selector = selector
	report.Workspace.SelectorKind = selectorKind(selector)

	resolution, err := workspace.ResolveWorkspaceSelectionReadOnly(selector, callerCWD)
	if err != nil {
		var selectorErr *workspace.WorkspaceSelectorError
		if selector != "" || errors.As(err, &selectorErr) {
			return report, err
		}
		report.Status = model.ProbeStatusAbsent
		report.Warnings = append(report.Warnings, err.Error())
		return report, nil
	}

	identity, identityErr := workspace.InspectWorkspaceIdentity(resolution.Registration.Root)
	if identityErr != nil {
		report.Status = model.ProbeStatusUnknown
		report.Warnings = append(report.Warnings, identityErr.Error())
		return report, nil
	}
	report.Workspace.ResolutionSource = string(resolution.Source)
	report.Workspace.DisplayRoot = identity.DisplayRoot
	report.Workspace.CanonicalRoot = identity.CanonicalRoot
	report.Warnings = append(report.Warnings, resolution.Warnings...)

	portablePath := workspace.ProjectConfigPath(identity.DisplayRoot)
	legacyDBPath := store.WorkspaceDBPath(identity.DisplayRoot)
	operationalDBPath := store.OperationalWorkspaceDBPath(identity.CanonicalRoot)
	report.State.PortablePath = portablePath
	report.State.LegacyPath = legacyDBPath
	report.State.OperationalPath = operationalDBPath
	report.State.Operational = "missing"
	report.State.MigrationStatus = "not_needed"

	stateDirInfo, stateDirErr := probeStat(workspace.WorkspaceStateDir(identity.DisplayRoot))
	switch {
	case stateDirErr == nil && !stateDirInfo.IsDir():
		report.State.Config = "unknown"
		report.State.Operational = "unknown"
		report.State.Database = "not_checked"
		appendProbeStateWarning(&report, "workspace state directory", errors.New("not a directory"))
		report.Status = model.ProbeStatusUnknown
		return report, nil
	case stateDirErr != nil && !errors.Is(stateDirErr, os.ErrNotExist):
		report.State.Config = "unknown"
		report.State.Operational = "unknown"
		report.State.Database = "not_checked"
		appendProbeStateWarning(&report, "workspace state directory", stateDirErr)
		report.Status = model.ProbeStatusUnknown
		return report, nil
	}

	configInfo, configErr := probeStat(portablePath)
	switch {
	case errors.Is(configErr, os.ErrNotExist):
		report.State.Config = "missing"
	case configErr != nil:
		report.State.Config = "unknown"
		appendProbeStateWarning(&report, "portable configuration", configErr)
	case configInfo.IsDir():
		report.State.Config = "unknown"
		report.Warnings = append(report.Warnings, "portable project configuration is a directory")
	default:
		if _, loadErr := workspace.LoadProjectFile(identity.DisplayRoot); loadErr != nil {
			report.State.Config = "unknown"
			appendProbeStateWarning(&report, "portable configuration", loadErr)
		} else {
			report.State.Config = "portable"
			report.Evidence.Files = append(report.Evidence.Files, portablePath)
			report.Evidence.Mtimes = map[string]time.Time{portablePath: configInfo.ModTime()}
		}
	}

	legacyInfo, legacyErr := probeStat(legacyDBPath)
	if legacyErr != nil && !errors.Is(legacyErr, os.ErrNotExist) {
		report.State.Operational = "unknown"
		report.State.Database = "not_checked"
		appendProbeStateWarning(&report, "legacy state", legacyErr)
		report.Status = model.ProbeStatusUnknown
		return report, nil
	}
	if legacyErr == nil {
		report.State.MigrationStatus = "legacy_present"
		report.Evidence.Files = append(report.Evidence.Files, legacyDBPath)
		if report.Evidence.Mtimes == nil {
			report.Evidence.Mtimes = map[string]time.Time{}
		}
		report.Evidence.Mtimes[legacyDBPath] = legacyInfo.ModTime()
	}

	operationalInfo, operationalErr := probeStat(operationalDBPath)
	if operationalErr != nil && !errors.Is(operationalErr, os.ErrNotExist) {
		report.State.Operational = "unknown"
		report.State.Database = "not_checked"
		appendProbeStateWarning(&report, "operational state", operationalErr)
		report.Status = model.ProbeStatusUnknown
		return report, nil
	}
	if operationalErr == nil {
		report.Evidence.Files = append(report.Evidence.Files, operationalDBPath)
		if report.Evidence.Mtimes == nil {
			report.Evidence.Mtimes = map[string]time.Time{}
		}
		report.Evidence.Mtimes[operationalDBPath] = operationalInfo.ModTime()
	}

	if operationalInfo != nil || legacyInfo != nil {
		report.State.Operational = "local"
	}

	dbPath := operationalDBPath
	dbInfo := operationalInfo
	if operationalErr != nil {
		dbPath = legacyDBPath
		dbInfo = legacyInfo
	}
	if dbInfo == nil {
		report.State.Database = "absent"
		report.Evidence.DBMode = "not_opened"
		report.Status = statusWithoutDatabase(report.State.Config)
		return report, nil
	}

	db, openErr := store.OpenReadOnlyExisting(identity.DisplayRoot, dbPath)
	if openErr != nil {
		report.State.Database = "unreadable"
		report.Evidence.DBMode = "not_opened"
		report.Status = model.ProbeStatusUnknown
		appendProbeStateWarning(&report, "database", openErr)
		return report, nil
	}
	defer db.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		report.State.Database = "unreadable"
		report.Evidence.DBMode = "ro"
		report.Status = model.ProbeStatusUnknown
		appendProbeStateWarning(&report, "database", err)
		return report, nil
	}
	report.State.Database = "present_ro"
	report.Evidence.DBMode = "ro"
	switch report.State.Config {
	case "portable":
		if dbInfo.ModTime().Before(configInfo.ModTime()) {
			report.Status = model.ProbeStatusStale
		} else {
			report.Status = model.ProbeStatusCurrent
		}
	case "missing":
		report.Status = model.ProbeStatusPartial
	default:
		report.Status = model.ProbeStatusUnknown
	}
	return report, nil
}

func selectorKind(selector string) string {
	if strings.TrimSpace(selector) == "" {
		return "omitted"
	}
	if filepath.IsAbs(selector) || strings.ContainsAny(selector, `/\\`) || selector == "." || selector == ".." {
		return "path"
	}
	return "alias"
}

func statusWithoutDatabase(config string) model.ProbeStatus {
	if config == "portable" {
		return model.ProbeStatusPartial
	}
	if config == "unknown" {
		return model.ProbeStatusUnknown
	}
	return model.ProbeStatusAbsent
}

func appendProbeStateWarning(report *model.ProbeReport, label string, err error) {
	if report == nil {
		return
	}
	reason := sanitizeProbeError(err)
	warning := strings.TrimSpace(label) + ": evidence_unavailable_or_unreadable"
	if reason != "" {
		warning += ": " + reason
	}
	report.Warnings = append(report.Warnings, warning)
	report.Evidence.Warnings = append(report.Evidence.Warnings, warning)
}

func sanitizeProbeError(err error) string {
	if err == nil {
		return ""
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil {
		err = pathErr.Err
	}
	message := strings.Join(strings.Fields(strings.TrimSpace(err.Error())), " ")
	if len(message) > 200 {
		message = strings.TrimSpace(message[:200])
	}
	return message
}
