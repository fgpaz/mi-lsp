package telemetry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type WorkspaceIdentity struct {
	Input   string
	Root    string
	Alias   string
	Display string
}

type ErrorInfo struct {
	Kind string
	Code string
}

type Window struct {
	Name     string
	Since    time.Time
	Duration time.Duration
	Label    string
}

func NormalizeAccessEvent(event model.AccessEvent) model.AccessEvent {
	identity := ResolveWorkspaceIdentity(firstNonEmpty(event.WorkspaceInput, event.Workspace))
	if strings.TrimSpace(event.WorkspaceInput) == "" {
		event.WorkspaceInput = identity.Input
	}
	if strings.TrimSpace(event.WorkspaceRoot) == "" {
		event.WorkspaceRoot = firstNonEmpty(identity.Root, event.Workspace, event.WorkspaceInput)
	}
	if strings.TrimSpace(event.WorkspaceAlias) == "" {
		event.WorkspaceAlias = identity.Alias
	}
	event.Workspace = firstNonEmpty(event.WorkspaceAlias, identity.Display, event.Workspace, event.WorkspaceInput, event.WorkspaceRoot)
	if strings.TrimSpace(event.WorkspaceRoot) == "" {
		event.WorkspaceRoot = firstNonEmpty(event.Workspace, event.WorkspaceInput)
	}
	if strings.TrimSpace(event.ErrorKind) == "" || strings.TrimSpace(event.ErrorCode) == "" {
		info := ClassifyErrorInfo(event.Backend, event.Error, event.Warnings)
		if strings.TrimSpace(event.ErrorKind) == "" {
			event.ErrorKind = info.Kind
		}
		if strings.TrimSpace(event.ErrorCode) == "" {
			event.ErrorCode = info.Code
		}
	}
	if event.WarningCount == 0 && len(event.Warnings) > 0 {
		event.WarningCount = len(event.Warnings)
	}
	if strings.TrimSpace(event.PatternMode) == "" {
		event.PatternMode = "none"
	}
	if strings.TrimSpace(event.RoutingOutcome) == "" {
		switch {
		case strings.EqualFold(strings.TrimSpace(event.Route), "direct_fallback"):
			event.RoutingOutcome = "direct_fallback"
		case strings.EqualFold(strings.TrimSpace(event.Backend), "router"):
			event.RoutingOutcome = "router_error"
		case strings.TrimSpace(event.Repo) != "":
			event.RoutingOutcome = "narrowed_repo"
		default:
			event.RoutingOutcome = "direct"
		}
	}
	if strings.TrimSpace(event.FailureStage) == "" {
		event.FailureStage = "none"
	}
	if strings.TrimSpace(event.TruncationReason) == "" {
		event.TruncationReason = "none"
	}
	return event
}

func ResolveWorkspaceIdentity(raw string) WorkspaceIdentity {
	identity := WorkspaceIdentity{Input: strings.TrimSpace(raw)}
	trimmed := identity.Input
	if trimmed == "" {
		identity.Display = "unscoped"
		return identity
	}

	registry, _ := workspace.LoadRegistry()
	if ws, ok := registry.Workspaces[trimmed]; ok {
		root := cleanPath(ws.Root)
		identity.Root = root
		identity.Alias = trimmed
		identity.Display = trimmed
		return identity
	}

	if path, ok := resolveExistingPath(trimmed); ok {
		identity.Root = path
		if alias := findAliasForRoot(registry, path); alias != "" {
			identity.Alias = alias
			identity.Display = alias
			return identity
		}
		identity.Display = filepath.Base(path)
		return identity
	}

	identity.Root = trimmed
	identity.Display = trimmed
	return identity
}

func WorkspaceAnalyticsKey(event model.AccessEvent) string {
	return firstNonEmpty(strings.TrimSpace(event.WorkspaceRoot), strings.TrimSpace(event.Workspace), "unscoped")
}

func WorkspaceDisplay(event model.AccessEvent) string {
	rootBase := ""
	if strings.TrimSpace(event.WorkspaceRoot) != "" {
		rootBase = filepath.Base(strings.TrimSpace(event.WorkspaceRoot))
	}
	return firstNonEmpty(strings.TrimSpace(event.WorkspaceAlias), strings.TrimSpace(event.Workspace), rootBase, strings.TrimSpace(event.WorkspaceInput), "unscoped")
}

func WorkspaceMatches(event model.AccessEvent, filter string) bool {
	needle := strings.TrimSpace(filter)
	if needle == "" {
		return true
	}
	for _, candidate := range []string{event.WorkspaceRoot, event.WorkspaceAlias, event.Workspace, event.WorkspaceInput} {
		if strings.EqualFold(strings.TrimSpace(candidate), needle) {
			return true
		}
	}
	return false
}

func ClassifyErrorInfo(backend string, errorText string, warnings []string) ErrorInfo {
	message := strings.ToLower(strings.TrimSpace(strings.Join(append([]string{errorText}, warnings...), "\n")))
	backend = strings.ToLower(strings.TrimSpace(backend))
	if message == "" {
		return ErrorInfo{}
	}
	if backend == "roslyn" {
		switch {
		case strings.Contains(message, "global.json") || strings.Contains(message, "requested sdk version"):
			return ErrorInfo{Kind: "sdk", Code: "dotnet_global_json_mismatch"}
		case strings.Contains(message, "no installed .net sdks") || strings.Contains(message, "no installed dotnet sdks") || strings.Contains(message, "compatible .net sdk was not found") || strings.Contains(message, "find any installed .net sdks"):
			return ErrorInfo{Kind: "sdk", Code: "dotnet_sdk_missing"}
		case IsRoslynWorkerBootstrapText(message):
			return ErrorInfo{Kind: "worker_bootstrap", Code: "roslyn_worker_bootstrap"}
		case strings.TrimSpace(errorText) != "":
			return ErrorInfo{Kind: "backend_runtime", Code: "roslyn_generic"}
		default:
			return ErrorInfo{}
		}
	}
	if strings.TrimSpace(errorText) != "" {
		return ErrorInfo{Kind: "backend_runtime", Code: backend + "_generic"}
	}
	return ErrorInfo{}
}

func IsRoslynWorkerBootstrapText(text string) bool {
	message := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(message, "dll was not found"):
		return true
	case strings.Contains(message, "no compatible roslyn worker available"):
		return true
	case strings.Contains(message, "bundled/global worker"):
		return true
	case strings.Contains(message, "protocol version mismatch"):
		return true
	case strings.Contains(message, "worker binary"):
		return true
	default:
		return false
	}
}

func ResolveWindow(name string, now time.Time) (Window, error) {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		trimmed = "7d"
	}
	if now.IsZero() {
		now = time.Now()
	}
	switch trimmed {
	case "recent":
		return Window{Name: "recent", Since: now.Add(-24 * time.Hour), Duration: 24 * time.Hour, Label: "recent (24h)"}, nil
	case "7d", "30d", "90d":
		days := map[string]int{"7d": 7, "30d": 30, "90d": 90}[trimmed]
		duration := time.Duration(days) * 24 * time.Hour
		return Window{Name: trimmed, Since: now.Add(-duration), Duration: duration, Label: trimmed}, nil
	default:
		if duration, err := time.ParseDuration(trimmed); err == nil {
			return Window{Name: trimmed, Since: now.Add(-duration), Duration: duration, Label: trimmed}, nil
		}
		if strings.HasSuffix(trimmed, "d") {
			var days int
			if _, err := fmt.Sscanf(strings.TrimSuffix(trimmed, "d"), "%d", &days); err == nil && days > 0 {
				duration := time.Duration(days) * 24 * time.Hour
				return Window{Name: trimmed, Since: now.Add(-duration), Duration: duration, Label: trimmed}, nil
			}
		}
		return Window{}, fmt.Errorf("unsupported window %q", name)
	}
}

func resolveExistingPath(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	if _, err := os.Stat(abs); err != nil {
		return "", false
	}
	return cleanPath(abs), true
}

func findAliasForRoot(registry model.RegistryFile, root string) string {
	aliases := make([]string, 0)
	for alias, ws := range registry.Workspaces {
		if cleanPath(ws.Root) == root {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) == 0 {
		return ""
	}
	sort.Strings(aliases)
	return aliases[0]
}

func cleanPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
