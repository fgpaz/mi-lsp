package indexer

import (
	"errors"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestGoObservationDiagnosticSummaryIsDeterministicAndBounded(t *testing.T) {
	batch := model.GraphObservationBatch{
		Backend:      "go",
		Completeness: model.GraphCompletenessPartial,
		Capabilities: []model.GraphObservationCapability{
			{Backend: "go", Capability: "calls", State: model.GraphObservationStatusUnavailable},
			{Backend: "go", Capability: "declarations", State: model.GraphObservationStatusStable},
			{Backend: "go", Capability: "calls", State: model.GraphObservationStatusUnavailable},
			{Backend: "go", Capability: "secret", State: "stable"},
		},
		Coverage: []model.GraphObservationCoverage{{Backend: "go", Capability: "declarations", Eligible: 7, Observed: 3}},
		Nodes:    make([]model.GraphObservationNode, 3),
		Omissions: []model.GraphObservationOmission{
			{ReasonCode: "type_check_error"},
			{ReasonCode: "unsupported_symbol_kind"},
			{ReasonCode: "type_check_error"},
		},
	}
	first := goObservationDiagnosticSummary(batch)
	second := goObservationDiagnosticSummary(batch)
	if first != second {
		t.Fatalf("summary is not deterministic:\n%s\n%s", first, second)
	}
	if len(first) > goObservationDiagnosticMaxBytes {
		t.Fatalf("summary length=%d, max=%d", len(first), goObservationDiagnosticMaxBytes)
	}
	for _, expected := range []string{"completeness=partial", "backend_known=true", "declarations_observed=3", "declarations_expected=7", "nodes=3", "type_check_error=2", "unknown=stable=1"} {
		if !strings.Contains(first, expected) {
			t.Fatalf("summary missing %q: %s", expected, first)
		}
	}
}

func TestGoObservationDiagnosticSummaryRedactsUnboundedFields(t *testing.T) {
	batch := model.GraphObservationBatch{
		Backend:      strings.Repeat("/secret/path", 100),
		Completeness: "partial-secret",
		Capabilities: []model.GraphObservationCapability{{Capability: "secret", State: "stable"}},
		Omissions:    []model.GraphObservationOmission{{ReasonCode: "secret_code"}},
	}
	summary := goObservationDiagnosticSummary(batch)
	if len(summary) > goObservationDiagnosticMaxBytes {
		t.Fatalf("summary length=%d, max=%d", len(summary), goObservationDiagnosticMaxBytes)
	}
	for _, forbidden := range []string{"/secret/path", "partial-secret", "secret=stable", "secret_code"} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("summary leaked %q: %s", forbidden, summary)
		}
	}
	if !strings.Contains(summary, "backend=unknown") || !strings.Contains(summary, "completeness=unknown") || !strings.Contains(summary, "unknown=1") {
		t.Fatalf("summary did not redact unknown values: %s", summary)
	}
}

func TestGoObservationDiagnosticSummaryCapsArraysAndIgnoresOrder(t *testing.T) {
	capabilityNames := []string{"declarations", "contains", "imports", "references", "calls", "implements", "extends", "tests", "route_to_handler", "publishes", "consumes", "reads", "writes", "doc_mentions", "doc_wikilink", "doc_embed", "doc_markdown_link", "doc_id", "doc_hierarchy"}
	reasons := []string{"read_error", "parse_error", "cgo_list_error", "listing_error", "no_parseable_sources", "type_check_no_sources", "cancelled", "type_check_error", "embedded_field_unsupported", "import_path_invalid", "external_target", "unsupported_symbol_kind", "semantic_identity_owner_collision", "additional_owner_evidence", "target_endpoint_missing", "local_target_missing_ref", "owner_endpoint_missing", "partial"}
	batch := model.GraphObservationBatch{Backend: "go", Completeness: model.GraphCompletenessPartial, Coverage: []model.GraphObservationCoverage{{Capability: "declarations", Eligible: 19, Observed: 3}}, Nodes: make([]model.GraphObservationNode, 3)}
	for _, name := range capabilityNames {
		batch.Capabilities = append(batch.Capabilities, model.GraphObservationCapability{Capability: name, State: model.GraphObservationStatusStable})
	}
	for _, reason := range reasons {
		batch.Omissions = append(batch.Omissions, model.GraphObservationOmission{ReasonCode: reason})
	}
	first := goObservationDiagnosticSummary(batch)
	for i, j := 0, len(batch.Capabilities)-1; i < j; i, j = i+1, j-1 {
		batch.Capabilities[i], batch.Capabilities[j] = batch.Capabilities[j], batch.Capabilities[i]
	}
	for i, j := 0, len(batch.Omissions)-1; i < j; i, j = i+1, j-1 {
		batch.Omissions[i], batch.Omissions[j] = batch.Omissions[j], batch.Omissions[i]
	}
	second := goObservationDiagnosticSummary(batch)
	if first != second {
		t.Fatalf("reordered observations changed summary:\n%s\n%s", first, second)
	}
	if len(first) > goObservationDiagnosticMaxBytes || !strings.Contains(first, "capabilities_omitted=11") || !strings.Contains(first, "omissions_omitted=10") {
		t.Fatalf("bounded summary missing omission counts: %s", first)
	}
	if !strings.Contains(first, "declarations_observed=3") || !strings.Contains(first, "declarations_expected=19") || !strings.Contains(first, "nodes=3") {
		t.Fatalf("fixed fields were hidden: %s", first)
	}
}

func TestGoObservationStageDiagnosticPreservesStrictGate(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	batch.Completeness = model.GraphCompletenessPartial
	batch.Omissions = append(batch.Omissions, model.GraphObservationOmission{
		Ref: "omission:partial", OwnerPath: "src/go/main.go", SubjectKind: "project",
		Backend: "go", Capability: "declarations", ReasonCode: "parse_error",
		RecoveryHintCode: "repair_source",
	})
	batch.Coverage[0].Eligible++
	batch.Coverage[0].Omitted++
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatalf("seal partial observation fixture: %v", err)
	}
	gateErr := batch.ReadyForStaging()
	if gateErr == nil {
		t.Fatal("partial batch unexpectedly passed ReadyForStaging")
	}
	wrapped := wrapGoObservationStageError(batch, gateErr)
	if !errors.Is(wrapped, model.ErrGraphObservationNotStageable) {
		t.Fatal("diagnostic wrapper changed the strict gate error identity")
	}
	if !strings.Contains(wrapped.Error(), "incomplete or unsupported backend") {
		t.Fatalf("gate message was not preserved: %v", wrapped)
	}
}
