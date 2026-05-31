package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	evidenceInventoryPreviewScanLimit = 5000
	evidenceInventoryFullScanLimit    = 50000
)

var errEvidenceInventoryScanLimit = errors.New("evidence inventory scan limit reached")

type evidenceInventoryResult struct {
	Query                  string                   `json:"query"`
	Mode                   string                   `json:"mode"`
	RecommendedReadPath    string                   `json:"recommended_read_path"`
	ContextLoadingProfile  string                   `json:"context_loading_profile"`
	EvidenceLoadingProfile string                   `json:"evidence_loading_profile"`
	Canonical              model.RouteCanonicalLane `json:"canonical"`
	LookupStatus           *model.WikiLookupStatus  `json:"lookup_status,omitempty"`
	EvidenceRoots          []evidenceInventoryRoot  `json:"evidence_roots,omitempty"`
	NextQueries            []string                 `json:"next_queries,omitempty"`
}

type evidenceInventoryRoot struct {
	Root                   string                           `json:"root"`
	ArtifactType           string                           `json:"artifact_type"`
	Verdict                string                           `json:"verdict,omitempty"`
	SummaryFirst           []string                         `json:"summary_first,omitempty"`
	Artifacts              map[string]evidenceArtifactStats `json:"artifacts,omitempty"`
	HeavyArtifacts         map[string]evidenceArtifactStats `json:"heavy_artifacts,omitempty"`
	Authority              string                           `json:"authority"`
	RecommendedReadPath    string                           `json:"recommended_read_path,omitempty"`
	ContextLoadingProfile  string                           `json:"context_loading_profile,omitempty"`
	EvidenceLoadingProfile string                           `json:"evidence_loading_profile,omitempty"`
	NextQueries            []string                         `json:"next_queries,omitempty"`
	score                  int
	summaryPaths           []string
}

type evidenceArtifactStats struct {
	Files              int    `json:"files"`
	Bytes              int64  `json:"bytes"`
	EstimatedRawTokens int64  `json:"estimated_raw_tokens,omitempty"`
	LatestMTime        string `json:"latest_mtime,omitempty"`
	ContentEmbedded    bool   `json:"content_embedded"`
	OmittedRaw         bool   `json:"omitted_raw,omitempty"`
}

type evidenceInventoryScan struct {
	roots     map[string]*evidenceInventoryRoot
	files     int
	truncated bool
	warnings  []string
}

// evidenceInventory handles nav.evidence.inventory. It returns metadata only:
// no prompt bodies, transcript turns, log excerpts, screenshot OCR, secrets or PHI.
func (a *App) evidenceInventory(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.evidence.inventory"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	queryText, _ := request.Payload["query"].(string)
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return model.Envelope{}, fmt.Errorf("query is required")
	}

	query := loadDocQueryContext(ctx, registration, queryText)
	defer query.Close()
	route := query.canonicalRoute(request.Context, false)
	route.LookupStatus = routeLookupStatus(ctx, query, registration.Name, queryText, route, "nav.evidence.inventory")

	scan := scanEvidenceInventory(ctx, registration.Root, registration.Name, queryText, request.Context)
	roots := materializeEvidenceInventoryRoots(scan.roots, registration.Name, queryText, request.Context)
	if len(roots) == 0 {
		scan.warnings = appendStringIfMissing(scan.warnings, "no evidence roots found under .docs/auditoria or .docs/raw")
	}

	recommendedReadPath, contextProfile, evidenceProfile := chooseEvidenceInventoryProfiles(roots, route)
	result := evidenceInventoryResult{
		Query:                  queryText,
		Mode:                   previewFullMode(request.Context.Full),
		RecommendedReadPath:    recommendedReadPath,
		ContextLoadingProfile:  contextProfile,
		EvidenceLoadingProfile: evidenceProfile,
		Canonical:              route.Canonical,
		LookupStatus:           route.LookupStatus,
		EvidenceRoots:          roots,
		NextQueries:            buildEvidenceInventoryNextQueries(registration.Name, queryText, roots, route),
	}

	memory, _ := loadReentryMemory(ctx, registration.Root)
	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "evidence.inventory",
		Mode:      result.Mode,
		Items:     []evidenceInventoryResult{result},
		Stats: model.Stats{
			Files:             scan.files,
			WorkspacesQueried: 1,
		},
		Warnings: scan.warnings,
	}
	if scan.truncated {
		env.Truncated = true
		env.Continuation = &model.Continuation{
			Reason: "evidence_inventory_truncated",
			Next: model.ContinuationTarget{
				Op:    "nav.evidence.inventory",
				Query: queryText,
				Full:  true,
			},
		}
	}
	if !request.Context.Full && env.NextHint == nil {
		next := "preview mode: rerun with --full for a wider evidence inventory"
		env.NextHint = &next
	}
	env = attachMemoryPointer(env, memory)
	return applyCoachPolicy(env, request.Context), nil
}

func scanEvidenceInventory(ctx context.Context, workspaceRoot string, workspaceName string, queryText string, opts model.QueryOptions) evidenceInventoryScan {
	scan := evidenceInventoryScan{roots: map[string]*evidenceInventoryRoot{}}
	limit := evidenceInventoryPreviewScanLimit
	if opts.Full {
		limit = evidenceInventoryFullScanLimit
	}

	scan.scanAuditRoots(ctx, workspaceRoot, workspaceName, queryText, limit)
	scan.scanRawRoot(ctx, workspaceRoot, workspaceName, queryText, ".docs/raw/prompts", "raw_prompts", limit)
	scan.scanRawRoot(ctx, workspaceRoot, workspaceName, queryText, ".docs/raw/plans", "raw_plans", limit)
	return scan
}

func (s *evidenceInventoryScan) scanAuditRoots(ctx context.Context, workspaceRoot string, workspaceName string, queryText string, limit int) {
	auditRoot := filepath.Join(workspaceRoot, ".docs", "auditoria")
	if !isPathInsideWorkspace(workspaceRoot, auditRoot) {
		s.warnings = appendStringIfMissing(s.warnings, ".docs/auditoria skipped: outside workspace")
		return
	}
	if _, err := os.Stat(auditRoot); err != nil {
		return
	}

	err := filepath.WalkDir(auditRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			s.warnings = appendStringIfMissing(s.warnings, "skipped evidence path: "+safeWorkspaceRel(workspaceRoot, path))
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		s.files++
		if s.files > limit {
			s.truncated = true
			return errEvidenceInventoryScanLimit
		}
		rel := safeWorkspaceRel(workspaceRoot, path)
		if kind, ok := summaryArtifactKind(d.Name()); ok {
			rootRel := filepath.ToSlash(filepath.Dir(rel))
			root := s.ensureRoot(rootRel, "evidence_bundle", "evidence_not_canon", workspaceName, queryText)
			root.addArtifact(kind, info, false)
			root.addSummaryFile(d.Name(), rel)
			if kind == "verdict" {
				root.Verdict = readEvidenceVerdict(path)
			}
		}
		if kind, rootRel, ok := heavyArtifactRoot(rel); ok {
			root := s.ensureRoot(rootRel, inferAuditArtifactType(rootRel), "evidence_not_canon", workspaceName, queryText)
			root.addArtifact(kind, info, true)
			root.addHeavyArtifact(kind, info)
		}
		return nil
	})
	if errors.Is(err, errEvidenceInventoryScanLimit) {
		s.warnings = appendStringIfMissing(s.warnings, fmt.Sprintf("evidence inventory scan truncated after %d files", limit))
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		s.warnings = appendStringIfMissing(s.warnings, "evidence inventory scan warning: "+err.Error())
	}
}

func (s *evidenceInventoryScan) scanRawRoot(ctx context.Context, workspaceRoot string, workspaceName string, queryText string, relRoot string, kind string, limit int) {
	if s.truncated {
		return
	}
	absRoot := filepath.Join(workspaceRoot, filepath.FromSlash(relRoot))
	if !isPathInsideWorkspace(workspaceRoot, absRoot) {
		s.warnings = appendStringIfMissing(s.warnings, relRoot+" skipped: outside workspace")
		return
	}
	if _, err := os.Stat(absRoot); err != nil {
		return
	}
	root := s.ensureRoot(relRoot, kind, "evidence_not_canon", workspaceName, queryText)
	err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		s.files++
		if s.files > limit {
			s.truncated = true
			return errEvidenceInventoryScanLimit
		}
		root.addArtifact(kind, info, true)
		root.addHeavyArtifact(kind, info)
		return nil
	})
	if errors.Is(err, errEvidenceInventoryScanLimit) {
		s.warnings = appendStringIfMissing(s.warnings, fmt.Sprintf("evidence inventory scan truncated after %d files", limit))
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		s.warnings = appendStringIfMissing(s.warnings, relRoot+" scan warning: "+err.Error())
	}
}

func (s *evidenceInventoryScan) ensureRoot(rootRel string, artifactType string, authority string, workspaceName string, queryText string) *evidenceInventoryRoot {
	rootRel = filepath.ToSlash(strings.TrimSpace(rootRel))
	if rootRel == "." || rootRel == "" {
		rootRel = "."
	}
	root, ok := s.roots[rootRel]
	if !ok {
		root = &evidenceInventoryRoot{
			Root:           rootRel,
			ArtifactType:   artifactType,
			Authority:      authority,
			Artifacts:      map[string]evidenceArtifactStats{},
			HeavyArtifacts: map[string]evidenceArtifactStats{},
		}
		s.roots[rootRel] = root
	}
	if root.ArtifactType == "evidence_bundle" && artifactType != "" {
		root.ArtifactType = artifactType
	}
	root.score += evidenceInventoryScore(rootRel, queryText)
	return root
}

func materializeEvidenceInventoryRoots(rootsByPath map[string]*evidenceInventoryRoot, workspaceName string, queryText string, opts model.QueryOptions) []evidenceInventoryRoot {
	roots := make([]evidenceInventoryRoot, 0, len(rootsByPath))
	for _, root := range rootsByPath {
		if len(root.Artifacts) == 0 && len(root.HeavyArtifacts) == 0 && len(root.SummaryFirst) == 0 {
			continue
		}
		sortSummaryFirst(root)
		root.RecommendedReadPath, root.ContextLoadingProfile, root.EvidenceLoadingProfile = chooseEvidenceInventoryRootProfiles(*root)
		root.NextQueries = buildEvidenceRootNextQueries(workspaceName, root.Root, root.summaryPaths)
		roots = append(roots, *root)
	}
	sort.SliceStable(roots, func(i, j int) bool {
		leftSummary := len(roots[i].SummaryFirst) > 0
		rightSummary := len(roots[j].SummaryFirst) > 0
		if leftSummary != rightSummary {
			return leftSummary
		}
		if roots[i].score != roots[j].score {
			return roots[i].score > roots[j].score
		}
		return roots[i].Root < roots[j].Root
	})
	limit := opts.MaxItems
	if limit <= 0 {
		limit = 20
	}
	if opts.Full && limit < 100 {
		limit = 100
	}
	if len(roots) > limit {
		roots = roots[:limit]
	}
	return roots
}

func (r *evidenceInventoryRoot) addArtifact(kind string, info os.FileInfo, raw bool) {
	if r.Artifacts == nil {
		r.Artifacts = map[string]evidenceArtifactStats{}
	}
	stats := updateEvidenceArtifactStats(r.Artifacts[kind], info, raw)
	r.Artifacts[kind] = stats
}

func (r *evidenceInventoryRoot) addHeavyArtifact(kind string, info os.FileInfo) {
	if r.HeavyArtifacts == nil {
		r.HeavyArtifacts = map[string]evidenceArtifactStats{}
	}
	stats := updateEvidenceArtifactStats(r.HeavyArtifacts[kind], info, true)
	r.HeavyArtifacts[kind] = stats
}

func (r *evidenceInventoryRoot) addSummaryFile(name string, relPath string) {
	name = filepath.ToSlash(name)
	for _, existing := range r.SummaryFirst {
		if existing == name {
			return
		}
	}
	r.SummaryFirst = append(r.SummaryFirst, name)
	r.summaryPaths = append(r.summaryPaths, filepath.ToSlash(relPath))
}

func updateEvidenceArtifactStats(stats evidenceArtifactStats, info os.FileInfo, raw bool) evidenceArtifactStats {
	stats.Files++
	stats.Bytes += info.Size()
	stats.EstimatedRawTokens = (stats.Bytes + 3) / 4
	stats.ContentEmbedded = false
	if raw {
		stats.OmittedRaw = true
	}
	if info.ModTime().After(parseEvidenceMTime(stats.LatestMTime)) {
		stats.LatestMTime = info.ModTime().UTC().Format(time.RFC3339)
	}
	return stats
}

func parseEvidenceMTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func summaryArtifactKind(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "manifest.yaml", "manifest.yml":
		return "manifest", true
	case "verdict.md", "verdict.yaml", "verdict.yml":
		return "verdict", true
	case "issues.yaml", "issues.yml":
		return "issues", true
	case "assertions.yaml", "assertions.yml", "assertions.json", "assertions.md":
		return "assertions", true
	case "summary.md", "summary.yaml", "summary.yml":
		return "summary", true
	case "hashes.yaml", "hashes.yml", "hashes.json":
		return "hashes", true
	default:
		return "", false
	}
}

func heavyArtifactRoot(rel string) (string, string, bool) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		switch strings.ToLower(part) {
		case "turns":
			if i > 0 {
				return "turns", strings.Join(parts[:i], "/"), true
			}
		case "logs":
			if i > 0 {
				return "logs", strings.Join(parts[:i], "/"), true
			}
		case "screenshots":
			if i > 0 {
				return "screenshots", strings.Join(parts[:i], "/"), true
			}
		}
	}
	return "", "", false
}

func inferAuditArtifactType(rootRel string) string {
	lower := strings.ToLower(filepath.ToSlash(rootRel))
	if strings.Contains(lower, "qa-conversacional") || strings.Contains(lower, "cqa") {
		return "cqa_bundle"
	}
	if strings.Contains(lower, "audit") || strings.Contains(lower, "auditoria") {
		return "audit_bundle"
	}
	return "evidence_bundle"
}

func readEvidenceVerdict(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4096))
	if err != nil {
		return ""
	}
	upper := strings.ToUpper(string(data))
	for _, status := range []string{"BLOCKED", "FAIL", "PASS", "APPROVED"} {
		if strings.Contains(upper, status) {
			return status
		}
	}
	return ""
}

func sortSummaryFirst(root *evidenceInventoryRoot) {
	if len(root.SummaryFirst) <= 1 {
		return
	}
	priority := map[string]int{
		"manifest.yaml":   10,
		"manifest.yml":    10,
		"verdict.md":      20,
		"verdict.yaml":    20,
		"verdict.yml":     20,
		"issues.yaml":     30,
		"issues.yml":      30,
		"assertions.yaml": 40,
		"assertions.yml":  40,
		"assertions.json": 40,
		"assertions.md":   40,
		"summary.md":      50,
		"summary.yaml":    50,
		"summary.yml":     50,
		"hashes.yaml":     60,
		"hashes.yml":      60,
		"hashes.json":     60,
	}
	type pair struct {
		name string
		path string
	}
	pairs := make([]pair, 0, len(root.SummaryFirst))
	for i, name := range root.SummaryFirst {
		path := ""
		if i < len(root.summaryPaths) {
			path = root.summaryPaths[i]
		}
		pairs = append(pairs, pair{name: name, path: path})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		pi := priority[strings.ToLower(pairs[i].name)]
		pj := priority[strings.ToLower(pairs[j].name)]
		if pi != pj {
			return pi < pj
		}
		return pairs[i].name < pairs[j].name
	})
	root.SummaryFirst = root.SummaryFirst[:0]
	root.summaryPaths = root.summaryPaths[:0]
	for _, item := range pairs {
		root.SummaryFirst = append(root.SummaryFirst, item.name)
		root.summaryPaths = append(root.summaryPaths, item.path)
	}
}

func chooseEvidenceInventoryProfiles(roots []evidenceInventoryRoot, route model.RouteResult) (string, string, string) {
	hasSummary := false
	hasHeavy := false
	hasOnlyRawPromptsOrPlans := len(roots) > 0
	for _, root := range roots {
		if len(root.SummaryFirst) > 0 {
			hasSummary = true
		}
		if len(root.HeavyArtifacts) > 0 {
			hasHeavy = true
		}
		if root.ArtifactType != "raw_prompts" && root.ArtifactType != "raw_plans" {
			hasOnlyRawPromptsOrPlans = false
		}
	}
	switch {
	case hasSummary:
		return "manifest_verdict", "CL1_EXACT", "EL1_MANIFEST_VERDICT"
	case hasHeavy && !hasOnlyRawPromptsOrPlans:
		return "targeted_raw", "CL1_EXACT", "EL3_TARGETED_RAW"
	case route.Canonical.AnchorDoc.Path != "":
		return "route", "CL1_EXACT", "EL0_NONE"
	default:
		return "wiki_search", "CL1_EXACT", "EL0_NONE"
	}
}

func chooseEvidenceInventoryRootProfiles(root evidenceInventoryRoot) (string, string, string) {
	if len(root.SummaryFirst) > 0 {
		return "manifest_verdict", "CL1_EXACT", "EL1_MANIFEST_VERDICT"
	}
	if root.ArtifactType == "raw_prompts" || root.ArtifactType == "raw_plans" {
		return "route", "CL1_EXACT", "EL0_NONE"
	}
	if len(root.HeavyArtifacts) > 0 {
		return "targeted_raw", "CL1_EXACT", "EL3_TARGETED_RAW"
	}
	return "route", "CL1_EXACT", "EL0_NONE"
}

func previewFullMode(full bool) string {
	if full {
		return "full"
	}
	return "preview"
}

func buildEvidenceInventoryNextQueries(workspaceName string, queryText string, roots []evidenceInventoryRoot, route model.RouteResult) []string {
	queries := []string{}
	if route.Canonical.AnchorDoc.Path != "" {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki pack %q --workspace %s --format toon", queryText, workspaceName))
	}
	for _, root := range roots {
		for _, query := range root.NextQueries {
			queries = appendUniqueQuery(queries, query)
		}
		if len(queries) >= 4 {
			break
		}
	}
	return queries
}

func buildEvidenceRootNextQueries(workspaceName string, rootRel string, summaryPaths []string) []string {
	if len(summaryPaths) == 0 {
		return nil
	}
	items := make([]string, 0, len(summaryPaths))
	for _, path := range summaryPaths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		items = append(items, filepath.ToSlash(path)+":1-120")
		if len(items) == 3 {
			break
		}
	}
	if len(items) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("mi-lsp nav multi-read %s --workspace %s --format toon", strings.Join(items, " "), workspaceName)}
}

func evidenceInventoryScore(path string, queryText string) int {
	score := 0
	lowerPath := strings.ToLower(filepath.ToSlash(path))
	for _, token := range strings.Fields(strings.ToLower(queryText)) {
		token = strings.Trim(token, `"'.,;:()[]{}<>`)
		if len(token) < 3 {
			continue
		}
		if strings.Contains(lowerPath, token) {
			score += 10
		}
	}
	return score
}

func safeWorkspaceRel(workspaceRoot string, path string) string {
	rel, err := filepath.Rel(workspaceRoot, path)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(rel)
}

func isPathInsideWorkspace(workspaceRoot string, path string) bool {
	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
