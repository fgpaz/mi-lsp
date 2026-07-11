package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const semanticPreparationSchema = "semantic-preparation-evidence/v1"

// prepare is deliberately evidence-only. In particular, it never calls edit-plan
// with apply enabled and never derives paths from an implicit git diff.
func (a *App) prepare(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	evidence := model.SemanticPreparationEvidence{
		Schema:       semanticPreparationSchema,
		AllowedPaths: []string{},
		Timings:      map[string]int64{},
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return preparationFailureEnvelope(request, evidence, "workspace", "workspace_resolution_failed", "selector_validation", false, started), nil
	}
	root, err := canonicalPreparationRoot(registration.Root)
	if err != nil {
		return preparationFailureEnvelope(request, evidence, "workspace", "workspace_root_unavailable", "selector_validation", false, started), nil
	}
	evidence.WorkspaceRoot = root
	task := strings.TrimSpace(stringPayload(request.Payload, "task"))
	if task == "" {
		return preparationFailureEnvelope(request, evidence, "validation", "task_required", "selector_validation", false, started), nil
	}
	evidence.TaskDigest = preparationDigest(task)
	planText := strings.TrimSpace(firstPreparationPayload(request.Payload, "plan", "packet", "edit_plan"))
	if planText != "" {
		evidence.PlanDigest = preparationDigest(planText)
	}

	timing := func(name string, begin time.Time) { evidence.Timings[name] = time.Since(begin).Milliseconds() }

	begin := time.Now()
	status := docgraph.InspectGovernance(root, false)
	evidence.GovernanceDigest = preparationGovernanceDigest(root)
	timing("governance", begin)
	if evidence.GovernanceDigest == "" {
		return preparationFailureEnvelope(request, evidence, "governance", "governance_unavailable", "governance", false, started), nil
	}
	if status.Blocked || status.AECanon.Blocking {
		evidence.Warnings = append(evidence.Warnings, "governance is blocked; preparation remains evidence-only")
		evidence.Failure = &model.PreparationFailure{Kind: "governance", Code: "governance_blocked", Stage: "governance"}
	}

	begin = time.Now()
	evidence.IndexGeneration = preparationIndexGeneration(root)
	timing("index_generation", begin)

	// Route and pack are invoked as service methods, not through the CLI or daemon.
	child := request
	child.Context.Workspace = root
	child.Context.CallerCWD = root
	child.Context.WorkspaceSource = "explicit"
	child.Payload = map[string]any{"task": task}
	begin = time.Now()
	routeEnv, routeErr := a.route(ctx, child)
	timing("route", begin)
	if routeErr != nil {
		evidence.Warnings = append(evidence.Warnings, "route unavailable: route_failed")
	} else if !routeEnv.Ok {
		evidence.Warnings = append(evidence.Warnings, "route returned a non-success envelope")
	}
	begin = time.Now()
	packEnv, packErr := a.pack(ctx, child)
	timing("pack", begin)
	if packErr != nil {
		evidence.Warnings = append(evidence.Warnings, "pack unavailable: pack_failed")
	} else if !packEnv.Ok {
		evidence.Warnings = append(evidence.Warnings, "pack returned a non-success envelope")
	}

	begin = time.Now()
	if planText != "" {
		packet, parseErr := parseEditPlanPacket(planText, false)
		if parseErr != nil {
			return preparationFailureEnvelope(request, evidence, "validation", "plan_invalid", "plan", false, started), nil
		}
		if _, _, _, validateErr := validateEditPlanPacket(root, &packet, false, false, false); validateErr != nil {
			return preparationFailureEnvelope(request, evidence, "validation", "plan_invalid", "plan", false, started), nil
		}
		for _, target := range packet.Targets {
			rel, relErr := makeRelative(root, target.Path)
			if relErr != nil {
				return preparationFailureEnvelope(request, evidence, "validation", "plan_path_invalid", "plan", false, started), nil
			}
			evidence.AllowedPaths = append(evidence.AllowedPaths, rel)
		}
	} else {
		paths := affectedPathsFromPayload(request.Payload["affected_paths"])
		if len(paths) == 0 {
			paths = affectedPathsFromPayload(request.Payload["paths"])
		}
		if len(paths) == 0 {
			evidence.Warnings = append(evidence.Warnings, "no task-specific affected paths or plan supplied; allowed_paths is empty")
		} else {
			for _, path := range paths {
				_, rel, relErr := resolveEditPlanPath(root, path, nil)
				if relErr != nil {
					return preparationFailureEnvelope(request, evidence, "validation", "affected_path_invalid", "paths", false, started), nil
				}
				evidence.AllowedPaths = append(evidence.AllowedPaths, rel)
			}
		}
	}
	sort.Strings(evidence.AllowedPaths)
	evidence.AllowedPaths = uniquePreparationPaths(evidence.AllowedPaths)
	timing("allowed_paths", begin)
	evidence.TotalMS = time.Since(started).Milliseconds()
	warnings := append([]string{}, evidence.Warnings...)
	if evidence.Failure != nil {
		warnings = append(warnings, evidence.Failure.Kind+"/"+evidence.Failure.Code)
	}
	return model.Envelope{Ok: evidence.Failure == nil, Workspace: registration.Name, Backend: "semantic-preparation", Items: []model.SemanticPreparationEvidence{evidence}, Warnings: warnings, Stats: model.Stats{Ms: evidence.TotalMS}}, nil
}

func preparationFailureEnvelope(request model.CommandRequest, evidence model.SemanticPreparationEvidence, kind, code, stage string, retryable bool, started time.Time) model.Envelope {
	evidence.Failure = &model.PreparationFailure{Kind: kind, Code: code, Stage: stage, Retryable: retryable}
	evidence.TotalMS = time.Since(started).Milliseconds()
	return model.Envelope{Ok: false, Workspace: request.Context.Workspace, Backend: "semantic-preparation", Items: []model.SemanticPreparationEvidence{evidence}, Warnings: []string{kind + "/" + code}, Stats: model.Stats{Ms: evidence.TotalMS}, Error: &model.EnvelopeError{Kind: kind, Code: code, Message: code, Stage: stage, Retryable: retryable}}
}

func canonicalPreparationRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("workspace root unavailable")
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = filepath.Clean(resolved)
	}
	return abs, nil
}

func PreparationDigest(value string) string { return preparationDigest(value) }

func preparationDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func PreparationGovernanceDigest(root string) string { return preparationGovernanceDigest(root) }

func preparationGovernanceDigest(root string) string {
	paths := []string{
		filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md"),
		filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"),
	}
	h := sha256.New()
	readFiles := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		readFiles++
		h.Write([]byte(filepath.ToSlash(path)))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	if readFiles == 0 {
		return ""
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func preparationIndexGeneration(root string) string {
	generation, err := store.ReadWorkspaceGenerationSnapshot(context.Background(), root)
	if err != nil {
		return "unavailable"
	}
	return generation
}

// PreparationCacheIdentity is shared by direct preparation and daemon caching.
func PreparationCacheIdentity(root string) (string, string, error) {
	canonical, err := canonicalPreparationRoot(root)
	if err != nil {
		return "", "", err
	}
	generation, err := store.ReadWorkspaceGenerationSnapshot(context.Background(), canonical)
	if err != nil {
		return canonical, "", err
	}
	return canonical, generation, nil
}

func firstPreparationPayload(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniquePreparationPaths(paths []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "" && !seen[path] {
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}
