package indexer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	goObservationDiagnosticMaxBytes  = 512
	goObservationDiagnosticMaxGroups = 8
)

var diagnosticCapabilities = map[string]struct{}{
	"declarations": {}, "contains": {}, "imports": {}, "references": {}, "calls": {},
	"implements": {}, "extends": {}, "tests": {}, "route_to_handler": {},
	"publishes": {}, "consumes": {}, "reads": {}, "writes": {}, "doc_mentions": {},
	"doc_wikilink": {}, "doc_embed": {}, "doc_markdown_link": {}, "doc_id": {},
	"doc_hierarchy": {},
}

var diagnosticOmissionReasons = map[string]struct{}{
	"read_error": {}, "parse_error": {}, "cgo_list_error": {}, "listing_error": {},
	"no_parseable_sources": {}, "type_check_no_sources": {}, "cancelled": {},
	"type_check_error": {}, "embedded_field_unsupported": {}, "import_path_invalid": {},
	"external_target": {}, "unsupported_symbol_kind": {}, "semantic_identity_owner_collision": {},
	"additional_owner_evidence": {}, "target_endpoint_missing": {}, "local_target_missing_ref": {},
	"owner_endpoint_missing": {}, "partial": {},
}

// wrapGoObservationStageError retains the strict gate error identity while
// attaching a private, bounded observation summary for repair diagnostics.
func wrapGoObservationStageError(batch model.GraphObservationBatch, gateErr error) error {
	return fmt.Errorf("go graph observation is not stageable: %w; diagnostics=%s", gateErr, goObservationDiagnosticSummary(batch))
}

func goObservationDiagnosticSummary(batch model.GraphObservationBatch) string {
	backendKnown := batch.Backend == "go" || batch.Backend == "roslyn" || batch.Backend == "tsserver" || batch.Backend == "pyright"
	backend := batch.Backend
	if !backendKnown {
		backend = "unknown"
	}
	completeness := batch.Completeness
	if completeness != model.GraphCompletenessComplete && completeness != model.GraphCompletenessPartial {
		completeness = "unknown"
	}

	declarationsObserved, declarationsExpected := 0, 0
	for _, coverage := range batch.Coverage {
		if coverage.Capability != "declarations" {
			continue
		}
		declarationsObserved += maxDiagnosticCount(coverage.Observed)
		declarationsExpected += maxDiagnosticCount(coverage.Eligible)
	}

	capabilityGroups := diagnosticCapabilityGroups(batch.Capabilities)
	omissionGroups := diagnosticOmissionGroups(batch.Omissions)
	_, capabilityOmitted := boundedDiagnosticGroups(capabilityGroups, goObservationDiagnosticMaxGroups)
	_, omissionOmitted := boundedDiagnosticGroups(omissionGroups, goObservationDiagnosticMaxGroups)

	// Fixed fields intentionally precede bounded arrays so truncation cannot
	// hide completeness, backend, declaration counts, or node count.
	parts := []string{
		"completeness=" + completeness,
		"backend=" + backend,
		fmt.Sprintf("backend_known=%t", backendKnown),
		fmt.Sprintf("declarations_observed=%d", declarationsObserved),
		fmt.Sprintf("declarations_expected=%d", declarationsExpected),
		fmt.Sprintf("nodes=%d", len(batch.Nodes)),
		fmt.Sprintf("capabilities_omitted=%d", capabilityOmitted),
		fmt.Sprintf("omissions_omitted=%d", omissionOmitted),
	}
	capabilities, _ := boundedDiagnosticGroups(capabilityGroups, goObservationDiagnosticMaxGroups)
	omissions, _ := boundedDiagnosticGroups(omissionGroups, goObservationDiagnosticMaxGroups)
	parts = append(parts, "capabilities="+capabilities, "omissions="+omissions)
	return boundDiagnosticSummary(strings.Join(parts, ";"))
}

func diagnosticCapabilityGroups(capabilities []model.GraphObservationCapability) []string {
	counts := make(map[string]int)
	for _, capability := range capabilities {
		name := capability.Capability
		if _, ok := diagnosticCapabilities[name]; !ok {
			name = "unknown"
		}
		state := capability.State
		if state != model.GraphObservationStatusStable && state != model.GraphObservationStatusExperimental && state != model.GraphObservationStatusGated && state != model.GraphObservationStatusUnsupported && state != model.GraphObservationStatusUnavailable {
			state = "unknown"
		}
		counts[name+"="+state]++
	}
	return sortedDiagnosticGroups(counts)
}

func diagnosticOmissionGroups(omissions []model.GraphObservationOmission) []string {
	counts := make(map[string]int)
	for _, omission := range omissions {
		code := omission.ReasonCode
		if _, ok := diagnosticOmissionReasons[code]; !ok {
			code = "unknown"
		}
		counts[code]++
	}
	return sortedDiagnosticGroups(counts)
}

func sortedDiagnosticGroups(counts map[string]int) []string {
	groups := make([]string, 0, len(counts))
	for value, count := range counts {
		groups = append(groups, fmt.Sprintf("%s=%d", value, count))
	}
	sort.Strings(groups)
	return groups
}

func boundedDiagnosticGroups(groups []string, maxGroups int) (string, int) {
	if len(groups) == 0 {
		return "none", 0
	}
	if len(groups) <= maxGroups {
		return strings.Join(groups, ","), 0
	}
	return strings.Join(groups[:maxGroups], ","), len(groups) - maxGroups
}

func boundDiagnosticSummary(value string) string {
	if len(value) <= goObservationDiagnosticMaxBytes {
		return value
	}
	const suffix = ";truncated=true"
	return value[:goObservationDiagnosticMaxBytes-len(suffix)] + suffix
}

func maxDiagnosticCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
