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
	now := time.Now().UTC()
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", workspace, evidenceRoot, strings.Join(scope, "\x00"), now.Format(time.RFC3339Nano))))
	r := &PreparationReceipt{Schema: PreparationReceiptSchema, PreparationID: hex.EncodeToString(sum[:16]), PacketPath: packetPath, Transferable: true, Workspace: workspace, EvidenceRoot: evidenceRoot, Scope: append([]string(nil), scope...)}
	if failure != nil {
		r.EvidenceOnly = true
		r.Repairability = "reparable"
		r.RecommendedAction = "repair the seed/catalog input and rerun the command"
	}
	meta := *r
	meta.Digest = ""
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	r.Digest = hex.EncodeToString(digest[:])
	raw, err = json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err = os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
		return nil, err
	}
	return r, nil
}
