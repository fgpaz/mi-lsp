package service

import (
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func applyWikiRepoCompatHint(env model.Envelope, request model.CommandRequest, operation string, workspaceName string, task string) model.Envelope {
	repo := strings.TrimSpace(stringPayload(request.Payload, "repo"))
	if repo == "" {
		return env
	}
	warning := fmt.Sprintf("--repo %q is ignored by %s because wiki navigation is workspace-scoped; use nav wiki for documentation exploration", repo, operation)
	env.Warnings = appendStringIfMissing(env.Warnings, warning)
	compatHint := wikiRepoCompatCommand(operation, workspaceName, task)
	if strings.TrimSpace(env.Hint) == "" {
		env.Hint = compatHint
	} else if !strings.Contains(env.Hint, compatHint) {
		env.Hint = strings.TrimSpace(env.Hint) + "; " + compatHint
	}
	return env
}

func wikiRepoCompatCommand(operation string, workspaceName string, task string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		task = "understand this task"
	}
	workspaceFlag := ""
	if strings.TrimSpace(workspaceName) != "" {
		workspaceFlag = " --workspace " + workspaceName
	}
	switch operation {
	case "nav.route":
		return fmt.Sprintf("--repo is ignored here; use mi-lsp nav wiki route %q%s --format toon", task, workspaceFlag)
	case "nav.pack":
		return fmt.Sprintf("--repo is ignored here; use mi-lsp nav wiki pack %q%s --format toon", task, workspaceFlag)
	default:
		return fmt.Sprintf("--repo is ignored here; use mi-lsp nav wiki search %q%s --format toon", task, workspaceFlag)
	}
}
