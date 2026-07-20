package milx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
)

var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var allowedOperations = map[string]bool{"analysis": true, "describe": true, "prepare": true, "execute": true, "cancel": true, "health": true, "shutdown": true}
var allowedCapabilities = map[string]bool{"graph.read.nodes": true, "graph.read.edges": true, "graph.read.evidence": true, "documents.read.pack": true, "analysis.emit": true, "visual.emit": true, "import.emit-advisory": true}
var allowedSchemas = map[string]bool{"milx-envelope/v1": true, "milx-manifest/v1": true, "milx-pack/v1": true, "milx-result/v1": true, "milx-provenance/v1": true, "milx-omissions/v1": true}

func (m Manifest) Validate() error {
	if m.Schema != ManifestSchema || !idPattern.MatchString(m.ExtensionID) || !idPattern.MatchString(m.ExtensionVersion) {
		return &MILXError{Code: "GPH_MILX_MANIFEST_INVALID", Stage: "manifest", SanitizedSummary: "manifest identity or schema is invalid"}
	}
	if !digestPattern.MatchString(m.ExecutableSHA256) || m.ProtocolMin > m.ProtocolMax || m.ProtocolMin > 1 || m.ProtocolMax < 1 || !m.Deterministic {
		return &MILXError{Code: "GPH_MILX_MANIFEST_INVALID", Stage: "manifest", SanitizedSummary: "manifest protocol, digest, or determinism is invalid"}
	}
	if err := closedSorted(m.Operations, allowedOperations, "operation"); err != nil {
		return err
	}
	if err := closedSorted(m.InputSchemas, allowedSchemas, "schema"); err != nil {
		return err
	}
	if err := closedSorted(m.OutputSchemas, allowedSchemas, "schema"); err != nil {
		return err
	}
	if err := closedSorted(m.Capabilities, allowedCapabilities, "capability"); err != nil {
		return err
	}
	for k := range m.ResourceHints {
		if !idPattern.MatchString(k) {
			return &MILXError{Code: "GPH_MILX_MANIFEST_INVALID", Stage: "manifest", SanitizedSummary: "resource hint key is invalid"}
		}
	}
	return nil
}
func closedSorted(values []string, allowed map[string]bool, kind string) error {
	seen := map[string]bool{}
	for _, v := range values {
		if seen[v] || !allowed[v] {
			return &MILXError{Code: "GPH_MILX_MANIFEST_INVALID", Stage: "manifest", SanitizedSummary: fmt.Sprintf("unknown or duplicate %s", kind)}
		}
		seen[v] = true
	}
	if !sort.StringsAreSorted(values) {
		return &MILXError{Code: "GPH_MILX_MANIFEST_INVALID", Stage: "manifest", SanitizedSummary: kind + " list is not canonical"}
	}
	return nil
}
func (m Manifest) Digest() ([32]byte, error) {
	var z [32]byte
	if err := m.Validate(); err != nil {
		return z, err
	}
	b, err := CanonicalJSON(m)
	if err != nil {
		return z, err
	}
	return sha256.Sum256(b), nil
}
func DigestHex(b []byte) string { d := sha256.Sum256(b); return hex.EncodeToString(d[:]) }

// VerifyExecutableDigest verifies the manifest's strict lowercase SHA-256 digest.
func VerifyExecutableDigest(executable []byte, expected string) error {
	if !digestPattern.MatchString(expected) {
		return &MILXError{Code: "GPH_MILX_MANIFEST_INVALID", Stage: "manifest", SanitizedSummary: "executable digest is invalid"}
	}
	if DigestHex(executable) != expected {
		return &MILXError{Code: "GPH_MILX_EXECUTABLE_DIGEST_MISMATCH", Stage: "manifest", SanitizedSummary: "executable digest does not match"}
	}
	return nil
}
