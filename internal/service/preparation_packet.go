package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/fgpaz/mi-lsp/internal/model"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var preparationNow = time.Now

func (a *App) preparationPacket(_ context.Context, request model.CommandRequest, action string) (model.Envelope, error) {
	reg, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	root, err := canonicalPreparationRoot(reg.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	payload := request.Payload
	if action == "verify" {
		return a.verifyPreparation(root, request)
	}
	if action == "refresh" {
		return a.refreshPreparation(root, request)
	}
	task := strings.TrimSpace(stringPayload(payload, "task"))
	if task == "" {
		return preparationPacketResult(request, "PREPARATION_MISSING", "automatic", "stop", nil, ""), nil
	}
	paths, err := preparationPaths(root, payload)
	if err != nil {
		return preparationPacketResult(request, "PATH_SCOPE_MISMATCH", "forbidden", "stop", nil, ""), nil
	}
	now := preparationNow()
	ttl := 15 * time.Minute
	if n, ok := payload["ttl_seconds"].(float64); ok && n > 0 {
		ttl = time.Duration(n) * time.Second
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	p := model.PreparationPacket{Schema: model.PreparationSchema, Workspace: model.PreparationWorkspace{CanonicalRoot: root, IdentityDigest: preparationDigest(root)}, Task: model.PreparationTask{Digest: preparationDigest(task), Intent: stringPayload(payload, "intent")}, Scope: model.PreparationScope{AllowedPaths: paths, DeniedClasses: []string{"authorization", "promotion", "protected_write", "broker", "database_persistence"}, ReadOnly: true}, Lineage: model.PreparationLineage{PreparationID: preparationDigest(fmt.Sprintf("%s:%d", task, now.UnixNano())), CreatedAt: now, ExpiresAt: now.Add(ttl)}, Evidence: model.PreparationEvidence{Root: root}, Status: "ready", Compatibility: "current"}
	p.Semantic.GovernanceDigest = preparationGovernanceDigest(root)
	p.Semantic.IndexDigest = preparationIndexGeneration(root)
	if v := firstPreparationPayload(payload, "plan"); v != "" {
		p.Semantic.PlanDigest = preparationDigest(v)
	}
	p.PacketDigest = packetDigest(p)
	output := stringPayload(payload, "output")
	if output == "" {
		return preparationPacketResult(request, "PREPARATION_READY", "automatic", "continue", &p, ""), nil
	}
	if err := writePreparation(root, output, p); err != nil {
		return preparationPacketResult(request, "PATH_SCOPE_MISMATCH", "forbidden", "stop", nil, ""), nil
	}
	return preparationPacketResult(request, "PREPARATION_READY", "automatic", "continue", &p, output), nil
}
func (a *App) verifyPreparation(root string, request model.CommandRequest) (model.Envelope, error) {
	path := stringPayload(request.Payload, "packet_path")
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if !validPreparationPacketPath(root, path) {
		return preparationPacketResult(request, "PATH_SCOPE_MISMATCH", "forbidden", "stop", nil, path), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return preparationPacketResult(request, "PREPARATION_MISSING", "refresh_required", "refresh", nil, path), nil
	}
	var p model.PreparationPacket
	if json.Unmarshal(b, &p) != nil || !validPreparationPacket(p) || p.PacketDigest != packetDigest(p) {
		return preparationPacketResult(request, "PACKET_TAMPERED", "refresh_required", "refresh", nil, path), nil
	}
	if filepath.Clean(p.Workspace.CanonicalRoot) != root {
		return preparationPacketResult(request, "WORKSPACE_MISMATCH", "parent_required", "request_transfer", &p, path), nil
	}
	if preparationNow().After(p.Lineage.ExpiresAt) {
		p.Status = "stale"
		return preparationPacketResult(request, "PACKET_EXPIRED", "refresh_required", "refresh", &p, path), nil
	}
	if t := strings.TrimSpace(stringPayload(request.Payload, "task")); t != "" && preparationDigest(t) != p.Task.Digest {
		return preparationPacketResult(request, "TASK_DIGEST_MISMATCH", "refresh_required", "refresh", &p, path), nil
	}
	if p.Semantic.GovernanceDigest != preparationGovernanceDigest(root) {
		return preparationPacketResult(request, "GOVERNANCE_DRIFT", "refresh_required", "refresh", &p, path), nil
	}
	if plan := firstPreparationPayload(request.Payload, "plan"); plan != "" && p.Semantic.PlanDigest != preparationDigest(plan) {
		return preparationPacketResult(request, "PLAN_DRIFT", "refresh_required", "refresh", &p, path), nil
	}
	if p.Semantic.IndexDigest != preparationIndexGeneration(root) {
		return preparationPacketResult(request, "INDEX_DRIFT", "refresh_required", "refresh", &p, path), nil
	}
	return preparationPacketResult(request, "PREPARATION_READY", "automatic", "continue", &p, path), nil
}
func preparationPacketResult(r model.CommandRequest, code, repair, action string, p *model.PreparationPacket, path string) model.Envelope {
	return model.Envelope{Ok: code == "PREPARATION_READY", Workspace: r.Context.Workspace, Backend: "preparation", Items: []model.PreparationResult{{EvidenceOnly: true, Code: code, Repairability: repair, RecommendedAction: action, Packet: p, PacketPath: path, Transferable: p != nil}}}
}
func packetDigest(p model.PreparationPacket) string {
	p.PacketDigest = ""
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
func preparationPaths(root string, p map[string]any) ([]string, error) {
	var raw []string
	if x, ok := p["affected_paths"].([]string); ok {
		raw = x
	} else if x, ok := p["affected_paths"].([]any); ok {
		for _, v := range x {
			if str, ok := v.(string); ok {
				raw = append(raw, str)
			}
		}
	}
	out := []string{}
	for _, v := range raw {
		if v == "" || strings.ContainsAny(v, "\x00\r\n") || filepath.IsAbs(v) {
			return nil, fmt.Errorf("invalid path")
		}
		clean := filepath.Clean(filepath.FromSlash(v))
		if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return nil, fmt.Errorf("escape")
		}
		full := filepath.Join(root, clean)
		if real, e := filepath.EvalSymlinks(full); e == nil {
			if !withinRoot(root, real) {
				return nil, fmt.Errorf("escape")
			}
		}
		out = append(out, filepath.ToSlash(clean))
	}
	sort.Strings(out)
	return uniquePreparationPaths(out), nil
}
func withinRoot(root, path string) bool {
	r, _ := filepath.Abs(root)
	p, _ := filepath.Abs(path)
	rel, e := filepath.Rel(r, p)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func writePreparation(root, out string, p model.PreparationPacket) error {
	full := out
	if !filepath.IsAbs(out) {
		full = filepath.Join(root, out)
	}
	full = filepath.Clean(full)
	if !withinRootPhysical(root, full) {
		return fmt.Errorf("output outside root")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(full, append(b, '\n'), 0644)
}

func validPreparationPacketPath(root, path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	return withinRootPhysical(root, path)
}
func withinRootPhysical(root, path string) bool {
	r, e := filepath.EvalSymlinks(root)
	if e != nil {
		return false
	}
	r, _ = filepath.Abs(r)
	d := filepath.Dir(path)
	if real, e := filepath.EvalSymlinks(d); e == nil {
		d = real
	} else {
		for {
			parent := filepath.Dir(d)
			if parent == d {
				break
			}
			if _, e := os.Stat(d); e == nil {
				break
			}
			d = parent
		}
	}
	return withinRoot(r, filepath.Join(d, filepath.Base(path)))
}
func validPreparationPacket(p model.PreparationPacket) bool {
	return p.Schema == model.PreparationSchema && p.Compatibility == "current" && p.Status == "ready" && p.Workspace.CanonicalRoot != "" && strings.HasPrefix(p.Workspace.IdentityDigest, "sha256:") && strings.HasPrefix(p.Task.Digest, "sha256:") && p.Task.Intent != "" && p.Scope.ReadOnly && len(p.Scope.DeniedClasses) > 0 && !p.Lineage.CreatedAt.IsZero() && !p.Lineage.ExpiresAt.IsZero() && strings.HasPrefix(p.Semantic.GovernanceDigest, "sha256:")
}
func (a *App) refreshPreparation(root string, request model.CommandRequest) (model.Envelope, error) {
	old := stringPayload(request.Payload, "packet_path")
	if !validPreparationPacketPath(root, old) {
		return preparationPacketResult(request, "PATH_SCOPE_MISMATCH", "forbidden", "stop", nil, old), nil
	}
	b, e := os.ReadFile(old)
	var p model.PreparationPacket
	if e != nil || json.Unmarshal(b, &p) != nil || !validPreparationPacket(p) || p.PacketDigest != packetDigest(p) {
		return preparationPacketResult(request, "PACKET_TAMPERED", "refresh_required", "refresh", nil, old), nil
	}
	out := stringPayload(request.Payload, "output")
	if out == "" || !validPreparationPacketPath(root, filepath.Join(root, out)) {
		return preparationPacketResult(request, "PREPARATION_OUTPUT_REQUIRED", "automatic", "stop", &p, old), nil
	}
	now := preparationNow()
	p.Lineage.ParentPreparationID = p.Lineage.PreparationID
	p.Lineage.PreparationID = preparationDigest(fmt.Sprintf("%s:%d", p.Task.Digest, now.UnixNano()))
	p.Lineage.CreatedAt = now
	p.Lineage.ExpiresAt = now.Add(15 * time.Minute)
	p.Semantic.GovernanceDigest = preparationGovernanceDigest(root)
	p.Semantic.IndexDigest = preparationIndexGeneration(root)
	p.PacketDigest = packetDigest(p)
	if e = writePreparation(root, out, p); e != nil {
		return preparationPacketResult(request, "PATH_SCOPE_MISMATCH", "forbidden", "stop", nil, out), nil
	}
	return preparationPacketResult(request, "PREPARATION_READY", "automatic", "continue", &p, out), nil
}
