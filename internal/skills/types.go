package skills

import "time"

const (
	SchemaCatalog = "mi-lsp-skill-catalog/v1"
	SchemaPlan    = "mi-lsp-skill-plan/v1"

	// Tiers
	TierParentRouter = "parent_router"
	TierToolInfra    = "tool_infra"
	TierCore         = "core"
	TierOptional     = "optional"
	TierBundle       = "bundle"

	// Audiences
	AudienceParent = "parent"
	AudienceLeaf   = "leaf"
	AudienceBoth   = "both"

	// Roles (plan/search)
	RoleParent = "parent"
	RoleLeaf   = "leaf"

	// Token cost classes
	TokenCostLow  = "low"
	TokenCostMed  = "med"
	TokenCostHigh = "high"

	// Plan entry modes
	ModeManifest = "manifest"
	ModeSelected = "selected"
	ModeRouter   = "router"

	// Known families
	FamilyAE             = "ae"
	FamilyCEO            = "ceo"
	FamilyComms          = "comms"
	FamilyData           = "data"
	FamilyFrontendDesign = "frontend_design"
	FamilyInfra          = "infra"
	FamilyMI             = "mi"
	FamilyOther          = "other"
	FamilyPlanningDocs   = "planning_docs"
	FamilyPS             = "ps"
	FamilyQA             = "qa"
	FamilyResearch       = "research"
	FamilySCM            = "scm"
	FamilyTesting        = "testing"
	FamilyToolMisc       = "tool_misc"
	FamilyToolInfra      = "tool_infra"
	FamilyBuho           = "buho"

	// Env / defaults
	EnvCatalogPath = "MI_LSP_SKILLS_CATALOG"

	// Scan limits
	MaxIndexedBodyLines = 80
	MaxIndexedBytes     = 4096
)

// SkillRecord is one classified skill in the catalog.
type SkillRecord struct {
	ID             string    `json:"id"`
	Family         string    `json:"family"`
	Tier           string    `json:"tier"`
	Audience       string    `json:"audience"`
	ParentSkill    *string   `json:"parent_skill,omitempty"`
	Bundle         *string   `json:"bundle,omitempty"`
	Critical       bool      `json:"critical"`
	Aliases        []string  `json:"aliases,omitempty"`
	TokenCostClass string    `json:"token_cost_class"`
	Description    string    `json:"description,omitempty"`
	WhenToUse      string    `json:"when_to_use,omitempty"`
	WhenNotToUse   *string   `json:"when_not_to_use,omitempty"`
	SourcePath     string    `json:"source_path,omitempty"`
	ContentSHA256  string    `json:"content_sha256,omitempty"`
	IndexedText    string    `json:"indexed_text,omitempty"`
	Embedding      []float32 `json:"embedding,omitempty"`
}

// Catalog is the persisted skill catalog wrapper.
type Catalog struct {
	Schema      string        `json:"schema"`
	GeneratedAt time.Time     `json:"generated_at"`
	SkillsRoot  string        `json:"skills_root"`
	Skills      []SkillRecord `json:"skills"`
	Warnings    []string      `json:"warnings,omitempty"`
}

// PlanBudget bounds a skill plan.
type PlanBudget struct {
	MaxSkills   int `json:"max_skills"`
	TokenBudget int `json:"token_budget"`
}

// PlanEntry is one selected or always-on skill in a plan.
type PlanEntry struct {
	ID      string   `json:"id"`
	Aliases []string `json:"aliases,omitempty"`
	Family  string   `json:"family,omitempty"`
	Tier    string   `json:"tier,omitempty"`
	Score   float64  `json:"score,omitempty"`
	Mode    string   `json:"mode"`
	Why     string   `json:"why,omitempty"`
}

// SkillPlan is the output of skills plan (schema mi-lsp-skill-plan/v1).
type SkillPlan struct {
	Schema          string      `json:"schema"`
	Role            string      `json:"role"`
	Task            string      `json:"task"`
	Budget          PlanBudget  `json:"budget"`
	Always          []PlanEntry `json:"always"`
	Routers         []PlanEntry `json:"routers"`
	Selected        []PlanEntry `json:"selected"`
	BundlesOptional []string    `json:"bundles_optional"`
	DenyFamilies    []string    `json:"deny_families"`
	Warnings        []string    `json:"warnings,omitempty"`
	WhyNotCheaper   string      `json:"why_not_cheaper"`
}

// SeedRow is one row from the embedded seed CSV.
type SeedRow struct {
	ID                string
	Family            string
	SuggestedTier     string
	SuggestedAudience string
	SuggestedAliases  []string
	CriticalCandidate bool
}

// ScanResult is a parsed SKILL.md head (frontmatter + short body).
type ScanResult struct {
	ID           string
	SourcePath   string
	Description  string
	WhenToUse    string
	WhenNotToUse string
	IndexedText  string
	ContentSHA   string
	Name         string
	Frontmatter  map[string]any
}

// IndexOptions controls catalog indexing.
type IndexOptions struct {
	SkillsRoot     string
	CatalogPath    string
	SeedPath       string // optional external seed; empty uses embedded
	WithEmbeddings bool
}

// SearchOptions filters and ranks skills.
type SearchOptions struct {
	Query    string
	Audience string // parent|leaf|"" (any)
	Family   string
	TopK     int
	// QueryEmbedding is optional precomputed query vector.
	QueryEmbedding []float32
}

// SearchHit is a ranked search result.
type SearchHit struct {
	Skill SkillRecord `json:"skill"`
	Score float64     `json:"score"`
	Why   string      `json:"why,omitempty"`
}

// PlanOptions builds a skill_plan.
type PlanOptions struct {
	Role        string
	Task        string
	MaxSkills   int
	TokenBudget int
}
