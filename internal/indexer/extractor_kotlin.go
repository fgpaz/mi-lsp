package indexer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

// This is the fork's raw compiler protocol, NOT a new Graph schema or identity
// registry. Names and offsets remain display/evidence; identity is descriptor-sourced.
type KotlinCompilerDescriptor struct {
	Identity          string `json:"identity"`
	Signature         string `json:"signature"`
	DescriptorClass   string `json:"descriptor_class"`
	SymbolKind        string `json:"symbol_kind"`
	DisplayName       string `json:"display_name"`
	CompilerModule    string `json:"compiler_module"`
	ObservationModule string `json:"observation_module"`
	DispatchReceiver  string `json:"dispatch_receiver"`
	ExtensionReceiver string `json:"extension_receiver"`
}
type KotlinCompilerDeclaration struct {
	KotlinCompilerDescriptor
	OwnerPath        string `json:"owner_path"`
	SourceSHA256     string `json:"source_sha256"`
	BindingSlice     string `json:"binding_slice"`
	StartOffsetUTF16 int    `json:"start_offset_utf16"`
	EndOffsetUTF16   int    `json:"end_offset_utf16"`
}
type KotlinCompilerReference struct {
	FromIdentity       string                   `json:"from_identity"`
	FromCompilerModule string                   `json:"from_compiler_module"`
	FromOwnerPath      string                   `json:"from_owner_path"`
	TargetOwnerPath    string                   `json:"target_owner_path"`
	Target             KotlinCompilerDescriptor `json:"target"`
	OwnerPath          string                   `json:"owner_path"`
	SourceSHA256       string                   `json:"source_sha256"`
	BindingSlice       string                   `json:"binding_slice"`
	StartOffsetUTF16   int                      `json:"start_offset_utf16"`
	EndOffsetUTF16     int                      `json:"end_offset_utf16"`
}
type KotlinCompilerOmission struct {
	OwnerPath  string `json:"owner_path"`
	Capability string `json:"capability"`
	ReasonCode string `json:"reason_code"`
}
type KotlinCompilerObservation struct {
	SchemaVersion     int                               `json:"schema_version"`
	ExtensionVersion  string                            `json:"extension_version"`
	ServerVersion     string                            `json:"server_version"`
	CompilerVersion   string                            `json:"compiler_version"`
	UpstreamCommit    string                            `json:"upstream_commit"`
	ModuleRoot        string                            `json:"module_root"`
	ModuleDescriptor  string                            `json:"module_descriptor"`
	Configuration     worker.KotlinMainConfiguration    `json:"configuration"`
	ConfigFingerprint model.GraphDigest                 `json:"config_fingerprint"`
	SourceFingerprint model.GraphDigest                 `json:"source_fingerprint"`
	Resolution        string                            `json:"resolution"`
	Completeness      string                            `json:"completeness"`
	DiagnosticErrors  int                               `json:"diagnostic_errors"`
	Capabilities      map[string]string                 `json:"capabilities"`
	Coverage          map[string]KotlinCompilerCoverage `json:"coverage"`
	Declarations      []KotlinCompilerDeclaration       `json:"declarations"`
	References        []KotlinCompilerReference         `json:"references"`
	Omissions         []KotlinCompilerOmission          `json:"omissions"`
	Limitations       []string                          `json:"limitations"`
}

type KotlinCompilerCoverage struct {
	Eligible   int `json:"eligible"`
	Observed   int `json:"observed"`
	Unresolved int `json:"unresolved"`
	Omitted    int `json:"omitted"`
}

func kotlinObservationError(code string) error {
	return &model.GraphObservationError{Code: code, Field: "kotlin_compiler_observation", Message: "Kotlin compiler observation rejected; no batch staged"}
}

// AdaptKotlinCompilerObservation maps original compiler descriptors to the
// registered Kotlin subset. It never promotes raw partial/experimental claims.
func AdaptKotlinCompilerObservation(ctx context.Context, request GraphObservationRequest, raw json.RawMessage) (model.GraphObservationBatch, error) {
	if err := ctx.Err(); err != nil {
		return model.GraphObservationBatch{}, err
	}
	if len(raw) == 0 || len(raw) > 8<<20 {
		return model.GraphObservationBatch{}, kotlinObservationError("GPH_BACKEND_PROVENANCE_MISSING")
	}
	var observation KotlinCompilerObservation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&observation) != nil || decoder.Decode(new(any)) != io.EOF {
		return model.GraphObservationBatch{}, kotlinObservationError("GPH_BACKEND_PROVENANCE_MISSING")
	}
	if err := validateKotlinCompilerObservation(request, observation); err != nil {
		return model.GraphObservationBatch{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.GraphObservationBatch{}, err
	}
	return buildKotlinObservationBatch(ctx, request, observation)
}

func validateKotlinCompilerObservation(request GraphObservationRequest, o KotlinCompilerObservation) error {
	bad := func() error { return kotlinObservationError("GPH_BACKEND_PROVENANCE_MISSING") }
	if o.SchemaVersion != 2 || o.ExtensionVersion != worker.KotlinObservationVersion || o.ServerVersion != worker.KotlinForkServerVersion || o.CompilerVersion != worker.KotlinCompilerVersion || o.UpstreamCommit != worker.KotlinUpstreamCommit || o.Resolution != "BindingContext+ModuleDescriptor" || o.Completeness != "partial" || o.DiagnosticErrors < 0 || len(o.Limitations) == 0 {
		return bad()
	}
	if request.Backend != worker.KotlinBackend || !filepath.IsAbs(o.ModuleRoot) {
		return bad()
	}
	entry := request.EntrypointPath
	if !filepath.IsAbs(entry) {
		entry = filepath.Join(request.WorkspaceRoot, filepath.FromSlash(entry))
	}
	realEntry, err := filepath.EvalSymlinks(entry)
	if err != nil || filepath.Dir(realEntry) != o.ModuleRoot {
		return bad()
	}
	config, err := worker.ReadKotlinObservationConfiguration(o.ModuleRoot)
	if err != nil || config.SHA256 != o.ConfigFingerprint.String() || !reflect.DeepEqual(config.Main, o.Configuration) || o.SourceFingerprint == (model.GraphDigest{}) || o.ModuleDescriptor != "<"+config.Main.ModuleName+">" {
		return bad()
	}
	if len(o.Capabilities) != 3 || o.Capabilities["declarations"] != "experimental" || o.Capabilities["references"] != "experimental" || o.Capabilities["calls"] != "unsupported" {
		return bad()
	}
	if len(o.Coverage) != 2 {
		return bad()
	}
	for _, cap := range []string{"declarations", "references"} {
		c, ok := o.Coverage[cap]
		observed := len(o.Declarations)
		if cap == "references" {
			observed = len(o.References)
		}
		omitted, unresolved := 0, 0
		for _, x := range o.Omissions {
			if x.Capability == cap {
				if x.ReasonCode == "compiler_reference_unresolved" || x.ReasonCode == "compiler_declaration_unresolved" {
					unresolved++
				} else {
					omitted++
				}
			}
		}
		if !ok || c.Eligible < 0 || c.Observed != observed || c.Omitted != omitted || c.Unresolved != unresolved || c.Eligible != observed+omitted+unresolved {
			return bad()
		}
	}
	ordered := append([]worker.KotlinCompilerInput(nil), config.Main.Sources...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	hash := sha256.New()
	for i, source := range ordered {
		if i > 0 {
			fmt.Fprint(hash, "\n")
		}
		fmt.Fprintf(hash, "%s\x00%s", source.Path, source.SHA256)
	}
	var expected model.GraphDigest
	copy(expected[:], hash.Sum(nil))
	if o.SourceFingerprint != expected {
		return bad()
	}
	sources := map[string]string{}
	for _, source := range config.Main.Sources {
		rel, err := filepath.Rel(o.ModuleRoot, source.Path)
		if err != nil {
			return bad()
		}
		sources[filepath.ToSlash(rel)] = source.SHA256
	}
	evidence := func(owner, digest string, start, end int) bool {
		return sources[owner] == digest && digest != "" && start >= 0 && end >= start
	}
	descriptor := func(d KotlinCompilerDescriptor) bool {
		return strings.HasPrefix(d.Identity, "descriptor-renderer-fq/v1|") && len(d.Identity) <= 8192 && d.Signature != "" && (d.SymbolKind == "type" || d.SymbolKind == "function" || d.SymbolKind == "method" || d.SymbolKind == "property") && d.DescriptorClass != "" && d.CompilerModule != "" && d.ObservationModule == o.ModuleDescriptor
	}
	declarations := map[string]bool{}
	for _, d := range o.Declarations {
		if !descriptor(d.KotlinCompilerDescriptor) || d.BindingSlice != "DECLARATION_TO_DESCRIPTOR" || !evidence(d.OwnerPath, d.SourceSHA256, d.StartOffsetUTF16, d.EndOffsetUTF16) {
			return bad()
		}
		// Reject collisions rather than coalescing differently-owned descriptors.
		key := d.OwnerPath + "\x00" + d.Identity
		if declarations[key] {
			return bad()
		}
		declarations[key] = true
	}
	identities := map[string]bool{}
	for _, d := range o.Declarations {
		identities[d.Identity] = true
	}
	for _, r := range o.References {
		if !descriptor(r.Target) || !identities[r.FromIdentity] || r.BindingSlice != "REFERENCE_TARGET" || !evidence(r.OwnerPath, r.SourceSHA256, r.StartOffsetUTF16, r.EndOffsetUTF16) {
			return bad()
		}
	}
	for _, omission := range o.Omissions {
		if sources[omission.OwnerPath] == "" || (omission.Capability != "declarations" && omission.Capability != "references") || omission.ReasonCode == "" {
			return bad()
		}
	}
	return nil
}

// Compiler identity joins include module and declaration owner, never FQN guesses.
func kotlinDescriptorJoin(identity, module, owner string) string {
	b, _ := json.Marshal([]string{identity, module, owner})
	return string(b)
}

func buildKotlinObservationBatch(ctx context.Context, request GraphObservationRequest, o KotlinCompilerObservation) (model.GraphObservationBatch, error) {
	bad := func() (model.GraphObservationBatch, error) {
		return model.GraphObservationBatch{}, kotlinObservationError("GPH_BACKEND_PROVENANCE_MISSING")
	}
	repo, err := model.NormalizeRepositoryIdentity(request.RepositoryIdentity)
	if err != nil {
		return bad()
	}
	prefix, err := filepath.Rel(request.RepoRoot, o.ModuleRoot)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return bad()
	}
	prefix = filepath.ToSlash(prefix)
	owner := func(p string) string { return filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(p))) }
	project := owner("kotlin-main-jvm")
	b := model.GraphObservationBatch{SchemaVersion: 1, WorkspaceIdentity: repo, RepositoryIdentity: repo,
		Backend: worker.KotlinBackend, BackendVersion: o.ServerVersion, ExtractorVersion: o.ExtensionVersion,
		ProjectOrModule: project, SourceFingerprint: o.SourceFingerprint, ConfigFingerprint: o.ConfigFingerprint, Completeness: o.Completeness,
		Nodes: []model.GraphObservationNode{}, Edges: []model.GraphObservationEdge{}, Evidence: []model.GraphObservationEvidence{}, Unresolved: []model.GraphObservationUnresolved{}, Omissions: []model.GraphObservationOmission{}}
	for _, cap := range []string{"declarations", "references"} {
		b.Capabilities = append(b.Capabilities, model.GraphObservationCapability{Backend: b.Backend, Capability: cap, State: o.Capabilities[cap]})
	}
	data := map[string][]byte{}
	for _, input := range o.Configuration.Sources {
		if err := ctx.Err(); err != nil {
			return model.GraphObservationBatch{}, err
		}
		info, err := os.Stat(input.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 2<<20 {
			return bad()
		}
		bytes, err := os.ReadFile(input.Path)
		if err != nil || !utf8.Valid(bytes) || fmt.Sprintf("%x", sha256.Sum256(bytes)) != input.SHA256 {
			return bad()
		}
		rel, err := filepath.Rel(o.ModuleRoot, input.Path)
		if err != nil {
			return bad()
		}
		data[filepath.ToSlash(rel)] = bytes
	}
	addEvidence := func(node, edge, path string, start, end int, digest model.GraphDigest, claim string) error {
		rng, observed, err := kotlinCompilerRange(data[path], start, end)
		if err != nil {
			return err
		}
		ref := fmt.Sprintf("evidence:%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%d", node, edge, path, start, end))))
		b.Evidence = append(b.Evidence, model.GraphObservationEvidence{Ref: ref, NodeRef: node, EdgeRef: edge, SourceURI: owner(path), Range: &rng, Backend: b.Backend, ExtractorVersion: b.ExtractorVersion, SourceDigest: digest, ObservedDigest: observed, ClaimKind: claim, Status: model.GraphRecordExact})
		return nil
	}
	joins := map[string]string{}
	for _, d := range o.Declarations {
		semantic, _ := json.Marshal([]string{"kotlin-descriptor/v1", d.CompilerModule, d.Identity, d.DispatchReceiver, d.ExtensionReceiver})
		key, err := model.NewNodeKey(model.NodeKeyFields{RepositoryIdentity: repo, BackendType: b.Backend, Language: "kotlin", ProjectOrModule: project, OwnerPath: owner(d.OwnerPath), SymbolKind: d.SymbolKind, SemanticIdentity: string(semantic)})
		if err != nil {
			return bad()
		}
		hash, err := key.Hash()
		if err != nil {
			return bad()
		}
		ref := model.NodeRID(hash)
		join := kotlinDescriptorJoin(d.Identity, d.CompilerModule, d.OwnerPath)
		if joins[join] != "" {
			return bad()
		}
		joins[join] = ref
		digest, err := model.ParseGraphDigest(d.SourceSHA256)
		if err != nil {
			return bad()
		}
		b.Nodes = append(b.Nodes, model.GraphObservationNode{Ref: ref, Key: key, DisplayName: d.DisplayName, SourceDigest: digest, ClaimStatus: model.GraphRecordExact, Resolution: "kotlin/binding-context"})
		if addEvidence(ref, "", d.OwnerPath, d.StartOffsetUTF16, d.EndOffsetUTF16, digest, "declaration") != nil {
			return bad()
		}
	}
	omit := func(path, cap, reason string) {
		b.Omissions = append(b.Omissions, model.GraphObservationOmission{Ref: fmt.Sprintf("omission:%d", len(b.Omissions)), OwnerPath: owner(path), SubjectKind: "file", Backend: b.Backend, Capability: cap, ReasonCode: reason, RecoveryHintCode: "review_kotlin_domain"})
	}
	seenEdges := map[string]bool{}
	seenEvidence := map[string]bool{}
	for _, r := range o.References {
		from := joins[kotlinDescriptorJoin(r.FromIdentity, r.FromCompilerModule, r.FromOwnerPath)]
		to := joins[kotlinDescriptorJoin(r.Target.Identity, r.Target.CompilerModule, r.TargetOwnerPath)]
		if from == "" || to == "" {
			return bad()
		} // raw v2 promises local representable endpoints
		digest, err := model.ParseGraphDigest(r.SourceSHA256)
		if err != nil {
			return bad()
		}
		ref := fmt.Sprintf("reference:%x", sha256.Sum256([]byte(from+"|"+to+"|"+owner(r.OwnerPath))))
		if seenEdges[ref] {
			omit(r.OwnerPath, "references", "repeated_reference_occurrence")
		} else {
			b.Edges = append(b.Edges, model.GraphObservationEdge{Ref: ref, FromRef: from, ToRef: to, Relation: "references", Scope: "symbol", Status: model.GraphRecordExact, OwnerPath: owner(r.OwnerPath), Backend: b.Backend, Resolution: "kotlin/binding-context", SourceDigest: digest})
			seenEdges[ref] = true
		}
		evidenceKey := fmt.Sprintf("%s|%d|%d", ref, r.StartOffsetUTF16, r.EndOffsetUTF16)
		if !seenEvidence[evidenceKey] {
			if addEvidence("", ref, r.OwnerPath, r.StartOffsetUTF16, r.EndOffsetUTF16, digest, "references") != nil {
				return bad()
			}
			seenEvidence[evidenceKey] = true
		}
	}
	for i, x := range o.Omissions {
		if x.ReasonCode == "compiler_reference_unresolved" || x.ReasonCode == "compiler_declaration_unresolved" {
			sel := model.GraphDigest(sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", x.OwnerPath, i, x.ReasonCode))))
			b.Unresolved = append(b.Unresolved, model.GraphObservationUnresolved{Ref: model.UnresolvedRID(sel), OwnerPath: owner(x.OwnerPath), SubjectKind: "file", Capability: x.Capability, SelectorDigest: sel, ReasonCode: x.ReasonCode, Backend: b.Backend, RecoveryHintCode: "resolve_compiler_target"})
		} else {
			omit(x.OwnerPath, x.Capability, x.ReasonCode)
		}
	}
	// Domain coverage is the compiler scan. Authority is the receipt already
	// required by ReadKotlinObservationConfiguration. Promotion stays closed:
	// RF-GPH-008 oracles are absent and this revision cannot self-grant stable.
	config, err := worker.ReadKotlinObservationConfiguration(o.ModuleRoot)
	if err != nil || config.SHA256 != o.ConfigFingerprint.String() {
		return bad()
	}
	allowed, _ := worker.KotlinPromotionAllowed(worker.KotlinPromotionEvidence{AuthorityCertified: config.Receipt.Certified != nil && *config.Receipt.Certified})
	if allowed || o.Completeness != "partial" {
		return bad()
	}
	for _, cap := range []string{"declarations", "references"} {
		if config.Receipt.Certified == nil || !*config.Receipt.Certified {
			omit("src/main", cap, "configuration_authority_unverified")
		}
		omit("src/main", cap, "promotion_oracle_missing")
		if o.DiagnosticErrors != 0 {
			omit("src/main", cap, "compiler_analysis_incomplete")
		}
		c := model.GraphObservationCoverage{Backend: b.Backend, Capability: cap}
		if cap == "declarations" {
			c.Observed = len(b.Nodes)
		} else {
			c.Observed = len(b.Edges)
		}
		for _, x := range b.Omissions {
			if x.Capability == cap {
				c.Omitted++
			}
		}
		for _, x := range b.Unresolved {
			if x.Capability == cap {
				c.Unresolved++
			}
		}
		c.Eligible = c.Observed + c.Omitted + c.Unresolved
		b.Coverage = append(b.Coverage, c)
	}
	if err := ctx.Err(); err != nil {
		return model.GraphObservationBatch{}, err
	}
	if err := model.PrepareGraphObservationBatch(&b); err != nil {
		return model.GraphObservationBatch{}, err
	}
	return b, nil
}

// UTF-16 offsets are evidence only. Columns here are 1-based UTF-8 bytes,
// matching GraphEvidence. Surrogate interiors and out-of-bounds offsets fail.
func kotlinCompilerRange(data []byte, start, end int) (model.GraphObservationRange, model.GraphDigest, error) {
	position := func(target int) (int, int, int, bool) {
		units, line, column := 0, 1, 1
		for offset, r := range string(data) {
			if units == target {
				return offset, line, column, true
			}
			n := 1
			if r > 0xffff {
				n = 2
			}
			units += n
			if units > target {
				return 0, 0, 0, false
			}
			if r == '\n' {
				line++
				column = 1
			} else {
				column += utf8.RuneLen(r)
			}
		}
		return len(data), line, column, units == target
	}
	a, sl, sc, ok := position(start)
	z, el, ec, okEnd := position(end)
	if !ok || !okEnd || start < 0 || end < start || a > z || !utf8.Valid(data) {
		return model.GraphObservationRange{}, model.GraphDigest{}, kotlinObservationError("GPH_OBS_RANGE_INVALID")
	}
	return model.GraphObservationRange{StartLine: sl, StartColumn: sc, EndLine: el, EndColumn: ec}, model.GraphDigest(sha256.Sum256(data[a:z])), nil
}
