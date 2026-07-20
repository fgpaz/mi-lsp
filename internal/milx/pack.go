package milx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MaxPackBytes       = 1 << 20
	MaxSelectionItems  = 1024
	MaxSelectionBucket = 256
)

// PackItem is a content-free, relocatable reference to a graph artifact.
type PackItem struct {
	Kind       string          `json:"kind"`
	CrossRID   string          `json:"cross_rid"`
	Digest     string          `json:"digest"`
	Path       string          `json:"path,omitempty"`
	Provenance json.RawMessage `json:"provenance"`
}

// PackSelection is the bounded, typed input to BuildPack. It deliberately
// carries references and digests rather than source, database, or audit data.
type PackSelection struct {
	GenerationID           string     `json:"generation_id"`
	GraphSchemaVersion     uint32     `json:"graph_schema_version"`
	Nodes                  []PackItem `json:"nodes,omitempty"`
	Edges                  []PackItem `json:"edges,omitempty"`
	Evidence               []PackItem `json:"evidence,omitempty"`
	Documents              []PackItem `json:"documents,omitempty"`
	AuthorityProfileDigest string     `json:"authority_profile_digest"`
	Omissions              []Omission `json:"omissions,omitempty"`
	OutputBudget           uint64     `json:"output_budget,omitempty"`
}

func packError(code, summary string) error {
	return &MILXError{Code: code, Stage: "pack", SanitizedSummary: summary}
}

func BuildPack(selection PackSelection) (Pack, error) {
	if err := validateSelection(selection); err != nil {
		return Pack{}, err
	}
	selection.Nodes = sortedItems(selection.Nodes)
	selection.Edges = sortedItems(selection.Edges)
	selection.Evidence = sortedItems(selection.Evidence)
	selection.Documents = sortedItems(selection.Documents)
	body, err := CanonicalJSON(selection)
	if err != nil {
		return Pack{}, packError("GPH_MILX_PACK_INVALID", "selection cannot be canonically encoded")
	}
	pack := Pack{Schema: "milx-pack/v1", GenerationID: selection.GenerationID, Selection: body, Provenance: Provenance{GenerationID: selection.GenerationID, ParametersDigest: selection.AuthorityProfileDigest}, Omissions: selection.Omissions}
	semantic, err := canonicalPack(pack)
	if err != nil {
		return Pack{}, err
	}
	pack.Digest = DigestHex(semantic)
	if _, err = CanonicalJSON(pack); err != nil {
		return Pack{}, packError("GPH_MILX_PACK_INVALID", "pack exceeds output budget")
	}
	return pack, nil
}

func VerifyPack(pack Pack) error {
	if pack.Schema != "milx-pack/v1" || pack.GenerationID == "" || pack.Provenance.GenerationID != pack.GenerationID || pack.Digest == "" {
		return packError("GPH_MILX_PACK_INVALID", "pack identity or provenance is invalid")
	}
	var selection PackSelection
	if err := DecodeCanonical(pack.Selection, &selection); err != nil || selection.GenerationID != pack.GenerationID || pack.Provenance.ParametersDigest != selection.AuthorityProfileDigest || validateSelection(selection) != nil {
		return packError("GPH_MILX_PACK_INVALID", "pack selection is invalid")
	}
	semantic, err := canonicalPack(pack)
	if err != nil || pack.Digest != DigestHex(semantic) {
		return packError("GPH_MILX_PACK_DIGEST_MISMATCH", "pack digest does not match its semantic payload")
	}
	encoded, err := CanonicalJSON(pack)
	if err != nil || len(encoded) > MaxPackBytes {
		return packError("GPH_MILX_PACK_INVALID", "pack exceeds size limit")
	}
	return nil
}

func canonicalPack(pack Pack) ([]byte, error) {
	pack.Digest = ""
	return CanonicalJSON(struct {
		Schema       string          `json:"schema"`
		GenerationID string          `json:"generation_id"`
		Selection    json.RawMessage `json:"selection"`
		Provenance   Provenance      `json:"provenance"`
		Omissions    []Omission      `json:"omissions"`
	}{pack.Schema, pack.GenerationID, pack.Selection, pack.Provenance, pack.Omissions})
}

// ValidateDerivedResult accepts extension output only when it remains bounded,
// provenance-bound to the prepared pack, and free of authority/write claims.
func ValidateDerivedResult(result Result, generationID, extensionID, extensionVersion, parametersDigest string) error {
	if result.Schema != "milx-result/v1" || result.ResultDigest != DigestHex(result.Result) || result.Provenance.GenerationID != generationID || result.Provenance.ExtensionID != extensionID || result.Provenance.ExtensionVersion != extensionVersion || result.Provenance.ParametersDigest != parametersDigest {
		return packError("GPH_MILX_OUTPUT_INVALID", "result provenance or digest is invalid")
	}
	if err := DecodeCanonical(result.Result, &map[string]any{}); err != nil || forbiddenResult(result.Result) {
		return packError("GPH_MILX_OUTPUT_INVALID", "result includes forbidden claims or content")
	}
	encoded, err := CanonicalJSON(result)
	if err != nil || len(encoded) > MaxPackBytes || len(result.Result) > MaxPackBytes {
		return packError("GPH_MILX_OUTPUT_INVALID", "result exceeds output limit")
	}
	for _, omission := range result.Omissions {
		if omission.Code == "" {
			return packError("GPH_MILX_OUTPUT_INVALID", "result omissions must be typed")
		}
	}
	return nil
}

func validateSelection(s PackSelection) error {
	if s.GenerationID == "" || s.GraphSchemaVersion != 1 || !strictSHA256(s.AuthorityProfileDigest) || s.OutputBudget > MaxPackBytes {
		return packError("GPH_MILX_PACK_INVALID", "selection identity, schema, authority digest, or budget is invalid")
	}
	buckets := [][]PackItem{s.Nodes, s.Edges, s.Evidence, s.Documents}
	total := 0
	for _, bucket := range buckets {
		if len(bucket) > MaxSelectionBucket {
			return packError("GPH_MILX_PACK_INVALID", "selection bucket exceeds item limit")
		}
		total += len(bucket)
		for _, item := range bucket {
			if err := validateItem(item); err != nil {
				return err
			}
		}
	}
	if total == 0 || total > MaxSelectionItems {
		return packError("GPH_MILX_PACK_INVALID", "selection item count is invalid")
	}
	for _, omission := range s.Omissions {
		if omission.Code == "" {
			return packError("GPH_MILX_PACK_INVALID", "omissions must be typed")
		}
	}
	return nil
}
func validateItem(item PackItem) error {
	if item.Kind == "" || item.CrossRID == "" || !strictSHA256(item.Digest) || len(item.Provenance) == 0 {
		return packError("GPH_MILX_PACK_INVALID", "selection item lacks required reference or provenance")
	}
	if item.Path != "" && (!filepath.IsLocal(item.Path) || filepath.IsAbs(item.Path) || strings.Contains(strings.ReplaceAll(item.Path, "\\", "/"), "..") || forbiddenPath(item.Path)) {
		return packError("GPH_MILX_CAPABILITY_DENIED", "selection item path is not repository-relative")
	}
	var v any
	if DecodeCanonical(item.Provenance, &v) != nil || forbiddenValue(v) {
		return packError("GPH_MILX_CAPABILITY_DENIED", "selection item provenance is forbidden")
	}
	return nil
}
func forbiddenPath(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	return strings.Contains(p, "/raw/") || strings.HasPrefix(p, "raw/") || strings.Contains(p, "/auditoria/") || strings.HasPrefix(p, "auditoria/")
}
func forbiddenResult(raw []byte) bool {
	var v any
	return json.Unmarshal(raw, &v) != nil || forbiddenValue(v)
}
func forbiddenValue(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			n := strings.ToLower(k)
			if strings.Contains(n, "authority") || strings.Contains(n, "write") || strings.Contains(n, "secret") || strings.Contains(n, "content") || strings.Contains(n, "source") || strings.Contains(n, "database") || strings.Contains(n, "db_handle") || n == "raw" {
				return true
			}
			if forbiddenValue(value) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if forbiddenValue(value) {
				return true
			}
		}
	}
	return false
}
func strictSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}

func sortedItems(in []PackItem) []PackItem {
	out := append([]PackItem(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.Kind+"\x00"+a.CrossRID+"\x00"+a.Digest < b.Kind+"\x00"+b.CrossRID+"\x00"+b.Digest
	})
	return out
}

var _ = bytes.Equal
var _ = fmt.Sprintf
