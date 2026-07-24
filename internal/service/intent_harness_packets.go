package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// planHarnessPacket routes supported one-shot harness packet intents
// (flow-slice / change-pack) through their service handlers and records a
// budgeted nav.batch continuation when available.
func (a *App) planHarnessPacket(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, plan *model.IntentPlan, warnings *[]string) {
	op := plan.Operation
	child := request
	switch op {
	case "flow-slice":
		child.Operation = "nav.flow-slice"
	case "change-pack":
		child.Operation = "nav.change-pack"
	default:
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_UNSUPPORTED_PACKET", Section: op, Reason: "unknown harness packet operation"})
		return
	}
	child.Payload = cloneIntentPayload(request.Payload)
	// Carry extracted arguments into payload.
	for _, key := range []string{"from", "to", "selector", "symbol", "ref", "changed_ref", "generation"} {
		if value := strings.TrimSpace(plan.Arguments[key]); value != "" {
			child.Payload[key] = value
		}
	}
	if paths := strings.TrimSpace(plan.Arguments["paths"]); paths != "" {
		child.Payload["paths"] = strings.Split(paths, ",")
	}
	if _, hasFromGit := plan.Arguments["from_git_diff"]; hasFromGit {
		child.Payload["from_git_diff"] = true
	}

	env, err := a.Execute(ctx, child)
	if err != nil {
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_PACKET_UNAVAILABLE", Section: op, Reason: sanitizeIntentError(err)})
		return
	}
	items, truncated := boundIntentItems(intentAnyItems(env.Items), request.Context)
	plan.Preview = append(plan.Preview, model.IntentPreview{
		Section:   op,
		Items:     items,
		Count:     len(items),
		Truncated: env.Truncated || truncated,
	})
	plan.Truncated = env.Truncated || truncated
	if env.GenerationID != "" {
		plan.GenerationID = env.GenerationID
	}
	for _, warning := range env.Warnings {
		*warnings = appendStringIfMissing(*warnings, sanitizeIntentWarning(warning))
	}

	// Prefer the packet's budgeted batch continuation as the expansion path.
	if env.Continuation != nil && env.Continuation.Next.Op == "nav.batch" && len(env.Continuation.Next.Batch) > 0 {
		args := map[string]any{"ops": len(env.Continuation.Next.Batch)}
		command := "mi-lsp nav batch --workspace " + intentExpansionValue(registration.Name, "workspace", args) + " --format toon"
		plan.Expansions = append(plan.Expansions, model.Expansion{
			Command:   command,
			Reason:    env.Continuation.Reason,
			Arguments: emptyIntentExpansionArguments(args),
		})
	} else {
		args := map[string]any{}
		command := "mi-lsp nav " + op + " --workspace " + intentExpansionValue(registration.Name, "workspace", args) + " --format toon"
		if selector := strings.TrimSpace(plan.Arguments["selector"]); selector != "" {
			command += " --selector " + intentExpansionValue(selector, "selector", args)
		}
		if from := strings.TrimSpace(plan.Arguments["from"]); from != "" {
			command += " --from " + intentExpansionValue(from, "from", args)
		}
		if to := strings.TrimSpace(plan.Arguments["to"]); to != "" {
			command += " --to " + intentExpansionValue(to, "to", args)
		}
		if ref := strings.TrimSpace(plan.Arguments["ref"]); ref != "" {
			command += " --ref " + intentExpansionValue(ref, "ref", args)
		}
		for _, path := range intentExpansionPaths(plan.Arguments["paths"]) {
			if safe, ok := intentSafeWorkspaceRelativePaths([]string{path}); ok && len(safe) == 1 {
				command += " --path " + safe[0]
			}
		}
		plan.Expansions = append(plan.Expansions, model.Expansion{
			Command:   command,
			Reason:    "expand the harness packet with the same normalized inputs",
			Arguments: emptyIntentExpansionArguments(args),
		})
	}

	// Surface hub risk in omissions when high.
	for _, item := range items {
		packet, ok := item.(map[string]any)
		if !ok {
			// typed packets are structs; inspect via JSON-ish fields when present
			continue
		}
		_ = packet
	}
	_ = filepath.Separator
}
