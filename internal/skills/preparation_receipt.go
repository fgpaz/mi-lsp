package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const PreparationReceiptSchema = "mi-lsp-preparation/v1"

type PreparationReceipt struct {
	Schema            string   `json:"schema"`
	PreparationID     string   `json:"preparation_id"`
	PacketPath        string   `json:"packet_path"`
	Digest            string   `json:"digest"`
	Transferable      bool     `json:"transferable"`
	Workspace         string   `json:"workspace,omitempty"`
	EvidenceRoot      string   `json:"evidence_root,omitempty"`
	Scope             []string `json:"scope,omitempty"`
	EvidenceOnly      bool     `json:"evidence_only,omitempty"`
	Repairability     string   `json:"repairability,omitempty"`
	RecommendedAction string   `json:"recommended_action,omitempty"`
}

func WritePreparationReceipt(path, workspace, evidenceRoot string, scope []string, packetPath string, failure error) (*PreparationReceipt, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	if strings.TrimSpace(workspace) == "" && strings.TrimSpace(evidenceRoot) == "" {
		return nil, fmt.Errorf("workspace or evidence_root required")
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	target = filepath.Clean(target)
	allowed := false
	for _, root := range []string{workspace, evidenceRoot} {
		if root == "" {
			continue
		}
		r, e := filepath.Abs(root)
		if e != nil {
			continue
		}
		r = filepath.Clean(r)
		if physicalContained(r, target) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("receipt path outside declared root")
	}
	if info, e := os.Lstat(target); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("receipt path is symlink")
	}
	now := time.Now().UTC()
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", workspace, evidenceRoot, strings.Join(scope, "\x00"), now.Format(time.RFC3339Nano))))
	r := &PreparationReceipt{Schema: PreparationReceiptSchema, PreparationID: hex.EncodeToString(sum[:16]), PacketPath: target, Transferable: failure == nil, Workspace: workspace, EvidenceRoot: evidenceRoot, Scope: append([]string(nil), scope...), EvidenceOnly: true}
	if failure != nil {
		r.Repairability = "refresh_required"
		r.RecommendedAction = "refresh"
	}
	meta := *r
	meta.Digest = ""
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	r.Digest = "sha256:" + hex.EncodeToString(digest[:])
	raw, err = json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return nil, err
	}
	if err = os.WriteFile(target, append(raw, '\n'), 0644); err != nil {
		return nil, err
	}
	return r, nil
}

func physicalContained(root, path string) bool {
	rr, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	d := filepath.Dir(path)
	for {
		if _, e := os.Lstat(d); e == nil {
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
	real, err := filepath.EvalSymlinks(d)
	if err != nil {
		return false
	}
	candidate := filepath.Join(real, filepath.Base(path))
	if info, e := os.Lstat(path); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	rel, err := filepath.Rel(rr, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
