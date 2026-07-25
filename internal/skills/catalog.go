package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/embed"
)

// DefaultCatalogPath returns $HOME/.mi-lsp/skills/catalog.json.
func DefaultCatalogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mi-lsp", "skills", "catalog.json"), nil
}

// DefaultSkillsRoot returns $HOME/.agents/skills.
func DefaultSkillsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

// ResolveCatalogPath prefers explicit override, then MI_LSP_SKILLS_CATALOG, then default.
func ResolveCatalogPath(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return override, nil
	}
	if env := strings.TrimSpace(os.Getenv(EnvCatalogPath)); env != "" {
		return env, nil
	}
	return DefaultCatalogPath()
}

// ResolveSkillsRoot prefers explicit override, then default.
func ResolveSkillsRoot(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return override, nil
	}
	return DefaultSkillsRoot()
}

// LoadCatalog reads a catalog JSON file.
func LoadCatalog(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("catalog json: %w", err)
	}
	if c.Schema == "" {
		c.Schema = SchemaCatalog
	}
	return &c, nil
}

// SaveCatalog writes catalog JSON, creating parent directories as needed.
func SaveCatalog(path string, c *Catalog) error {
	if c == nil {
		return fmt.Errorf("nil catalog")
	}
	if c.Schema == "" {
		c.Schema = SchemaCatalog
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

// BuildCatalog scans skillsRoot, merges seed classification, and optionally embeds.
func BuildCatalog(ctx context.Context, opts IndexOptions) (*Catalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	root, err := ResolveSkillsRoot(opts.SkillsRoot)
	if err != nil {
		return nil, err
	}

	var seedRows []SeedRow
	if strings.TrimSpace(opts.SeedPath) != "" {
		seedRows, err = LoadSeedFile(opts.SeedPath)
	} else {
		seedRows, err = LoadEmbeddedSeed()
	}
	if err != nil {
		return nil, fmt.Errorf("load seed: %w", err)
	}
	seedByID := SeedMap(seedRows)

	scans, scanWarnings, err := ScanSkillsRoot(root)
	if err != nil {
		return nil, err
	}

	// Index scanned skills; also keep seed-only entries without source when helpful?
	// Spec: index from skills root. Seed provides classification. Only scanned skills
	// land in the catalog, but tests often need classify without scan — BuildCatalog
	// is scan-driven. For empty scan with seed present, include classified seed rows
	// so plan/search still work offline.
	byID := map[string]SkillRecord{}
	var warnings []string
	warnings = append(warnings, scanWarnings...)

	for _, sc := range scans {
		seed := seedByID[sc.ID]
		if seed.ID == "" {
			// try directory basename match already applied in scan
			seed = seedByID[filepath.Base(filepath.Dir(sc.SourcePath))]
		}
		if seed.ID == "" {
			seed = SeedRow{ID: sc.ID}
		}
		rec := Classify(sc.ID, seed, &sc)
		byID[rec.ID] = rec
	}

	// If the root is empty (or fixture partial), still surface seed rows that were
	// not scanned so catalog is useful. Mark them without source_path.
	if len(byID) == 0 {
		for _, row := range seedRows {
			byID[row.ID] = Classify(row.ID, row, nil)
		}
		warnings = append(warnings, "no SKILL.md found under skills root; catalog built from seed only")
	}

	recs := make([]SkillRecord, 0, len(byID))
	for _, r := range byID {
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })

	if opts.WithEmbeddings {
		embWarnings := attachEmbeddings(ctx, recs)
		warnings = append(warnings, embWarnings...)
		// write back embeddings into slice (attachEmbeddings mutates)
	}

	return &Catalog{
		Schema:      SchemaCatalog,
		GeneratedAt: time.Now().UTC(),
		SkillsRoot:  root,
		Skills:      recs,
		Warnings:    uniqueStrings(warnings),
	}, nil
}

// IndexAndSave builds a catalog and writes it to catalog path.
func IndexAndSave(ctx context.Context, opts IndexOptions) (*Catalog, string, error) {
	cat, err := BuildCatalog(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	path, err := ResolveCatalogPath(opts.CatalogPath)
	if err != nil {
		return nil, "", err
	}
	if err := SaveCatalog(path, cat); err != nil {
		return nil, "", err
	}
	return cat, path, nil
}

// FilterSkills returns skills matching optional family/tier/audience filters.
func FilterSkills(cat *Catalog, family, tier, audience string) []SkillRecord {
	if cat == nil {
		return nil
	}
	family = strings.TrimSpace(family)
	tier = strings.TrimSpace(tier)
	audience = strings.TrimSpace(audience)
	out := make([]SkillRecord, 0, len(cat.Skills))
	for _, s := range cat.Skills {
		if family != "" && !strings.EqualFold(s.Family, family) {
			continue
		}
		if tier != "" && !strings.EqualFold(s.Tier, tier) {
			continue
		}
		if audience != "" && !audienceMatches(s.Audience, audience) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func audienceMatches(skillAudience, filter string) bool {
	skillAudience = strings.ToLower(strings.TrimSpace(skillAudience))
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" || skillAudience == "" || skillAudience == AudienceBoth {
		return true
	}
	return skillAudience == filter
}

func attachEmbeddings(ctx context.Context, recs []SkillRecord) []string {
	client, warning := bestEffortEmbedClient()
	if client == nil {
		if warning != "" {
			return []string{warning}
		}
		return []string{"embeddings_unavailable"}
	}

	texts := make([]string, len(recs))
	for i, r := range recs {
		texts[i] = embeddingText(r)
	}
	// Batch via client; on failure warn and continue without vectors.
	vectors, err := client.Embed(ctx, texts)
	if err != nil {
		return []string{fmt.Sprintf("embeddings_unavailable: %v", err)}
	}
	if len(vectors) != len(recs) {
		return []string{"embeddings_unavailable: vector count mismatch"}
	}
	for i := range recs {
		recs[i].Embedding = vectors[i]
	}
	return nil
}

func embeddingText(r SkillRecord) string {
	parts := []string{r.ID}
	parts = append(parts, r.Aliases...)
	if r.Description != "" {
		parts = append(parts, r.Description)
	}
	if r.WhenToUse != "" {
		parts = append(parts, r.WhenToUse)
	}
	if r.IndexedText != "" {
		// Cap for embed cost.
		t := r.IndexedText
		if len(t) > 1500 {
			t = t[:1500]
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, "\n")
}

func bestEffortEmbedClient() (*embed.Client, string) {
	apiKeyEnv := ""
	for _, env := range []string{"MI_LSP_EMBEDDINGS_API_KEY", "OPENAI_API_KEY"} {
		if strings.TrimSpace(os.Getenv(env)) != "" {
			apiKeyEnv = env
			break
		}
	}
	if apiKeyEnv == "" {
		return nil, "embeddings_unavailable: no API key in MI_LSP_EMBEDDINGS_API_KEY or OPENAI_API_KEY"
	}
	baseURL := strings.TrimSpace(os.Getenv("MI_LSP_EMBEDDINGS_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(os.Getenv("MI_LSP_EMBEDDINGS_MODEL"))
	if model == "" {
		model = "text-embedding-3-small"
	}
	return embed.New(embed.Config{
		Provider:  "openai-compatible",
		BaseURL:   baseURL,
		Model:     model,
		APIKeyEnv: apiKeyEnv,
		TimeoutMS: 30000,
		BatchSize: 32,
	}), ""
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// FindByID returns a skill by id or alias.
func FindByID(cat *Catalog, id string) (SkillRecord, bool) {
	if cat == nil {
		return SkillRecord{}, false
	}
	id = strings.TrimSpace(id)
	for _, s := range cat.Skills {
		if s.ID == id {
			return s, true
		}
		for _, a := range s.Aliases {
			if a == id {
				return s, true
			}
		}
	}
	return SkillRecord{}, false
}
