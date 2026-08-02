package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/skills"
)

func newSkillsCommand(state *rootState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Index, list, search, and plan local agent skills",
		Long: `Manage the local skill catalog used by agent harnesses.

Commands operate directly against the skills root and catalog JSON
(preferDaemon=false). Default catalog: $HOME/.mi-lsp/skills/catalog.json
(or MI_LSP_SKILLS_CATALOG). Default skills root: $HOME/.agents/skills.`,
	}
	cmd.AddCommand(
		newSkillsIndexCommand(state),
		newSkillsListCommand(state),
		newSkillsSearchCommand(state),
		newSkillsPlanCommand(state),
	)
	return cmd
}

func newSkillsIndexCommand(state *rootState) *cobra.Command {
	var (
		skillsRoot                                 string
		catalogPath                                string
		seedPath                                   string
		withEmbeddings                             bool
		preparationOutput, workspace, evidenceRoot string
		scope                                      []string
	)
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Scan skills root and write catalog.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			started := time.Now()
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()

			cat, path, err := skills.IndexAndSave(ctx, skills.IndexOptions{
				SkillsRoot:     skillsRoot,
				CatalogPath:    catalogPath,
				SeedPath:       seedPath,
				WithEmbeddings: withEmbeddings,
			})
			opts := state.queryOptions(cmd, "skills.index", nil)
			if err != nil {
				receiptItems := []any{}
				if receipt, receiptErr := skills.WritePreparationReceipt(preparationOutput, workspace, evidenceRoot, scope, preparationOutput, err); receiptErr != nil {
					return receiptErr
				} else if receipt != nil {
					receiptItems = append(receiptItems, map[string]any{"preparation_receipt": receipt})
				}
				if printErr := state.printEnvelope(model.Envelope{
					Ok:       false,
					Backend:  "skills",
					Items:    receiptItems,
					Warnings: []string{err.Error()},
					Error: &model.EnvelopeError{
						Code:    "skills_index_failed",
						Message: err.Error(),
						Stage:   "skills.index",
					},
					Stats: model.Stats{Ms: time.Since(started).Milliseconds()},
				}, opts); printErr != nil {
					return printErr
				}
				return envelopePrintedError{err: err}
			}

			items := make([]any, 0, 1)
			if receipt, receiptErr := skills.WritePreparationReceipt(preparationOutput, workspace, evidenceRoot, scope, preparationOutput, nil); receiptErr != nil {
				return receiptErr
			} else if receipt != nil {
				items = append(items, map[string]any{"preparation_receipt": receipt})
			}
			items = append(items, map[string]any{
				"catalog_path":    path,
				"skills_root":     cat.SkillsRoot,
				"skill_count":     len(cat.Skills),
				"generated_at":    cat.GeneratedAt.Format(time.RFC3339),
				"schema":          cat.Schema,
				"with_embeddings": withEmbeddings,
			})
			// Include a compact skill id list under items for inspection.
			for _, s := range cat.Skills {
				items = append(items, map[string]any{
					"id":       s.ID,
					"family":   s.Family,
					"tier":     s.Tier,
					"audience": s.Audience,
					"critical": s.Critical,
					"aliases":  s.Aliases,
				})
			}
			return state.printEnvelope(model.Envelope{
				Ok:       true,
				Backend:  "skills",
				Items:    items,
				Warnings: cat.Warnings,
				Stats: model.Stats{
					Ms:    time.Since(started).Milliseconds(),
					Files: len(cat.Skills),
				},
				Hint: fmt.Sprintf("catalog written to %s", path),
			}, opts)
		},
	}
	cmd.Flags().StringVar(&skillsRoot, "skills-root", "", "Skills root directory (default: $HOME/.agents/skills)")
	cmd.Flags().StringVar(&catalogPath, "catalog", "", "Catalog JSON path (default: $HOME/.mi-lsp/skills/catalog.json or MI_LSP_SKILLS_CATALOG)")
	cmd.Flags().StringVar(&seedPath, "seed", "", "Optional seed CSV path (default: embedded seed)")
	cmd.Flags().BoolVar(&withEmbeddings, "with-embeddings", false, "Best-effort embedding of indexed skill text")
	cmd.Flags().StringVar(&preparationOutput, "preparation-output", "", "Optional portable preparation receipt path")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace metadata for preparation receipt")
	cmd.Flags().StringVar(&evidenceRoot, "evidence-root", "", "Evidence root metadata for preparation receipt")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "Scope metadata for preparation receipt")
	return cmd
}

func newSkillsListCommand(state *rootState) *cobra.Command {
	var (
		catalogPath string
		family      string
		tier        string
		audience    string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List skills from the catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			started := time.Now()
			path, err := skills.ResolveCatalogPath(catalogPath)
			opts := state.queryOptions(cmd, "skills.list", nil)
			if err != nil {
				return err
			}
			cat, err := skills.LoadCatalog(path)
			if err != nil {
				return state.printEnvelope(model.Envelope{
					Ok:      false,
					Backend: "skills",
					Items:   []any{},
					Error: &model.EnvelopeError{
						Code:     "skills_catalog_missing",
						Message:  err.Error(),
						Stage:    "skills.list",
						HintCode: "run_skills_index",
					},
					Hint:  "run: mi-lsp skills index",
					Stats: model.Stats{Ms: time.Since(started).Milliseconds()},
				}, opts)
			}
			filtered := skills.FilterSkills(cat, family, tier, audience)
			items := make([]any, 0, len(filtered))
			for _, s := range filtered {
				items = append(items, map[string]any{
					"id":               s.ID,
					"family":           s.Family,
					"tier":             s.Tier,
					"audience":         s.Audience,
					"critical":         s.Critical,
					"aliases":          s.Aliases,
					"token_cost_class": s.TokenCostClass,
					"description":      s.Description,
					"when_to_use":      s.WhenToUse,
					"source_path":      s.SourcePath,
				})
			}
			return state.printEnvelope(model.Envelope{
				Ok:      true,
				Backend: "skills",
				Items:   items,
				Stats: model.Stats{
					Ms:    time.Since(started).Milliseconds(),
					Files: len(filtered),
				},
			}, opts)
		},
	}
	cmd.Flags().StringVar(&catalogPath, "catalog", "", "Catalog JSON path")
	cmd.Flags().StringVar(&family, "family", "", "Filter by family")
	cmd.Flags().StringVar(&tier, "tier", "", "Filter by tier")
	cmd.Flags().StringVar(&audience, "audience", "", "Filter by audience (parent|leaf|both)")
	return cmd
}

func newSkillsSearchCommand(state *rootState) *cobra.Command {
	var (
		catalogPath string
		audience    string
		family      string
		topK        int
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Hybrid lexical search over the skill catalog",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			started := time.Now()
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			query := strings.Join(args, " ")
			opts := state.queryOptions(cmd, "skills.search", nil)

			path, err := skills.ResolveCatalogPath(catalogPath)
			if err != nil {
				return err
			}
			cat, err := skills.LoadCatalog(path)
			if err != nil {
				return state.printEnvelope(model.Envelope{
					Ok:      false,
					Backend: "skills",
					Items:   []any{},
					Error: &model.EnvelopeError{
						Code:     "skills_catalog_missing",
						Message:  err.Error(),
						Stage:    "skills.search",
						HintCode: "run_skills_index",
					},
					Hint:  "run: mi-lsp skills index",
					Stats: model.Stats{Ms: time.Since(started).Milliseconds()},
				}, opts)
			}

			hits, warnings := skills.Search(ctx, cat, skills.SearchOptions{
				Query:    query,
				Audience: audience,
				Family:   family,
				TopK:     topK,
			})
			items := make([]any, 0, len(hits))
			for _, h := range hits {
				items = append(items, map[string]any{
					"id":               h.Skill.ID,
					"score":            h.Score,
					"why":              h.Why,
					"family":           h.Skill.Family,
					"tier":             h.Skill.Tier,
					"audience":         h.Skill.Audience,
					"critical":         h.Skill.Critical,
					"aliases":          h.Skill.Aliases,
					"token_cost_class": h.Skill.TokenCostClass,
					"description":      h.Skill.Description,
					"when_to_use":      h.Skill.WhenToUse,
				})
			}
			return state.printEnvelope(model.Envelope{
				Ok:       true,
				Backend:  "skills",
				Items:    items,
				Warnings: warnings,
				Stats: model.Stats{
					Ms:    time.Since(started).Milliseconds(),
					Files: len(hits),
				},
			}, opts)
		},
	}
	cmd.Flags().StringVar(&catalogPath, "catalog", "", "Catalog JSON path")
	cmd.Flags().StringVar(&audience, "audience", "", "Audience filter: parent|leaf")
	cmd.Flags().StringVar(&family, "family", "", "Family filter")
	cmd.Flags().IntVar(&topK, "top-k", 8, "Maximum results")
	return cmd
}

func newSkillsPlanCommand(state *rootState) *cobra.Command {
	var (
		catalogPath string
		skillsRoot  string
		role        string
		task        string
		tokenBudget int
		maxSkills   int
	)
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Build a skill_plan for a role and task",
		RunE: func(cmd *cobra.Command, args []string) error {
			started := time.Now()
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			opts := state.queryOptions(cmd, "skills.plan", nil)

			if strings.TrimSpace(role) == "" {
				return fmt.Errorf("--role is required (parent|leaf)")
			}
			if strings.TrimSpace(task) == "" {
				return fmt.Errorf("--task is required")
			}

			path, err := skills.ResolveCatalogPath(catalogPath)
			if err != nil {
				return err
			}
			cat, err := skills.LoadCatalog(path)
			if err != nil {
				// Auto-build from skills-root when catalog missing.
				root := skillsRoot
				built, buildErr := skills.BuildCatalog(ctx, skills.IndexOptions{
					SkillsRoot: root,
				})
				if buildErr != nil {
					return state.printEnvelope(model.Envelope{
						Ok:      false,
						Backend: "skills",
						Items:   []any{},
						Error: &model.EnvelopeError{
							Code:     "skills_catalog_missing",
							Message:  err.Error(),
							Stage:    "skills.plan",
							HintCode: "run_skills_index",
						},
						Hint:  "run: mi-lsp skills index",
						Stats: model.Stats{Ms: time.Since(started).Milliseconds()},
					}, opts)
				}
				cat = built
			}

			plan, err := skills.BuildPlan(ctx, cat, skills.PlanOptions{
				Role:        role,
				Task:        task,
				MaxSkills:   maxSkills,
				TokenBudget: tokenBudget,
			})
			if err != nil {
				return state.printEnvelope(model.Envelope{
					Ok:      false,
					Backend: "skills",
					Items:   []any{},
					Error: &model.EnvelopeError{
						Code:    "skills_plan_failed",
						Message: err.Error(),
						Stage:   "skills.plan",
					},
					Stats: model.Stats{Ms: time.Since(started).Milliseconds()},
				}, opts)
			}

			// Prefer Items as list with full plan as single map item so
			// json/toon/yaml/compact renderers share the same payload shape.
			planItem, convErr := skillPlanToItem(plan)
			if convErr != nil {
				return convErr
			}
			return state.printEnvelope(model.Envelope{
				Ok:       true,
				Backend:  "skills",
				Items:    []any{planItem},
				Warnings: plan.Warnings,
				Stats: model.Stats{
					Ms: time.Since(started).Milliseconds(),
				},
			}, opts)
		},
	}
	cmd.Flags().StringVar(&catalogPath, "catalog", "", "Catalog JSON path")
	cmd.Flags().StringVar(&skillsRoot, "skills-root", "", "Skills root used if catalog is missing")
	cmd.Flags().StringVar(&role, "role", "", "Plan role: parent|leaf")
	cmd.Flags().StringVar(&task, "task", "", "Natural-language task description")
	cmd.Flags().IntVar(&tokenBudget, "token-budget", 4000, "Approximate skill token budget")
	cmd.Flags().IntVar(&maxSkills, "max-skills", 8, "Maximum selected skills")
	return cmd
}

// skillPlanToItem converts a SkillPlan to a generic map for Envelope.Items.
func skillPlanToItem(plan *skills.SkillPlan) (map[string]any, error) {
	if plan == nil {
		return nil, fmt.Errorf("nil skill plan")
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	var item map[string]any
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	return item, nil
}

// newSeedCommand is the top-level compatibility surface for portable skill seeding.
func newSeedCommand(state *rootState) *cobra.Command {
	cmd := newSkillsIndexCommand(state)
	cmd.Use = "seed"
	cmd.Short = "Seed and index a local skill catalog"
	return cmd
}
