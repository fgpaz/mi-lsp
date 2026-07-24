package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	// Default hub threshold: nodes in the top percentile by degree-like centrality.
	defaultHubCentralityFloor = 0.75
	defaultHubImpactFloor     = 0.70
	maxHubWarnings            = 8
)

// HubRisk summarizes god-node / community-overlap risk for a change or flow.
type HubRisk struct {
	HubsTouched          []HubRiskNode `json:"hubs_touched,omitempty"`
	CommunitiesTouched   []string      `json:"communities_touched,omitempty"`
	CommunityOverlapRisk string        `json:"community_overlap_risk,omitempty"` // none|low|medium|high
	Warning              string        `json:"warning,omitempty"`
}

type HubRiskNode struct {
	NodeKey     string  `json:"node_key,omitempty"`
	Display     string  `json:"display,omitempty"`
	OwnerPath   string  `json:"owner_path,omitempty"`
	CommunityID string  `json:"community_id,omitempty"`
	Centrality  float64 `json:"centrality,omitempty"`
	Impact      float64 `json:"impact,omitempty"`
	Score       float64 `json:"score,omitempty"`
	Reason      string  `json:"reason,omitempty"`
}

func assessHubRisk(ctx context.Context, db *sql.DB, focusPaths []string, focusSymbols []string) (HubRisk, map[string]bool, []string) {
	risk := HubRisk{CommunityOverlapRisk: "none"}
	hubPaths := map[string]bool{}
	warnings := []string{}
	if db == nil {
		return risk, hubPaths, warnings
	}
	rankEnv, err := GraphRank(ctx, db, GraphRankRequest{Limit: 64, Intent: "harness"})
	if err != nil || !rankEnv.Ok || len(rankEnv.Items) == 0 {
		if err != nil {
			warnings = append(warnings, "hub risk unavailable: "+sanitizeIntentError(err))
		}
		return risk, hubPaths, warnings
	}

	focusPathSet := map[string]bool{}
	for _, path := range focusPaths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path != "" {
			focusPathSet[strings.ToLower(path)] = true
		}
	}
	focusSymbolSet := map[string]bool{}
	for _, symbol := range focusSymbols {
		symbol = strings.TrimSpace(symbol)
		if symbol != "" {
			focusSymbolSet[strings.ToLower(symbol)] = true
		}
	}

	communities := map[string]struct{}{}
	hubs := make([]HubRiskNode, 0)
	for _, rank := range rankEnv.Items {
		isHub := rank.Centrality >= defaultHubCentralityFloor || rank.Impact >= defaultHubImpactFloor
		if !isHub {
			continue
		}
		owner := filepath.ToSlash(rank.OwnerPath)
		if owner != "" {
			hubPaths[owner] = true
		}
		touchesFocus := false
		if owner != "" && focusPathSet[strings.ToLower(owner)] {
			touchesFocus = true
		}
		if !touchesFocus {
			for focusPath := range focusPathSet {
				if owner != "" && (strings.HasPrefix(focusPath, strings.ToLower(owner)) || strings.HasPrefix(strings.ToLower(owner), focusPath)) {
					touchesFocus = true
					break
				}
			}
		}
		if !touchesFocus {
			displayLower := strings.ToLower(rank.Display)
			for symbol := range focusSymbolSet {
				if symbol != "" && (displayLower == symbol || strings.Contains(displayLower, symbol)) {
					touchesFocus = true
					break
				}
			}
		}
		if !touchesFocus {
			// Still record hub path map for demotion, but only warn on focus touches.
			continue
		}
		if rank.CommunityID != "" {
			communities[rank.CommunityID] = struct{}{}
		}
		hubs = append(hubs, HubRiskNode{
			NodeKey:     rank.NodeKey,
			Display:     rank.Display,
			OwnerPath:   owner,
			CommunityID: rank.CommunityID,
			Centrality:  rank.Centrality,
			Impact:      rank.Impact,
			Score:       rank.Score,
			Reason:      "high centrality/impact graph hub touched by focus",
		})
	}

	sort.SliceStable(hubs, func(i, j int) bool {
		if hubs[i].Score != hubs[j].Score {
			return hubs[i].Score > hubs[j].Score
		}
		return hubs[i].NodeKey < hubs[j].NodeKey
	})
	if len(hubs) > maxHubWarnings {
		hubs = hubs[:maxHubWarnings]
	}
	risk.HubsTouched = hubs

	communityIDs := make([]string, 0, len(communities))
	for id := range communities {
		communityIDs = append(communityIDs, id)
	}
	sort.Strings(communityIDs)
	risk.CommunitiesTouched = communityIDs

	switch {
	case len(communityIDs) >= 4 || len(hubs) >= 4:
		risk.CommunityOverlapRisk = "high"
		risk.Warning = "change/flow touches multiple high-centrality hubs across communities; prefer narrow multi-read of leaf nodes first"
	case len(communityIDs) >= 2 || len(hubs) >= 2:
		risk.CommunityOverlapRisk = "medium"
		risk.Warning = "change/flow spans more than one graph community or hub; verify cross-boundary contracts"
	case len(hubs) == 1:
		risk.CommunityOverlapRisk = "low"
		risk.Warning = "one graph hub is in scope; treat hub edits as high blast-radius"
	default:
		risk.CommunityOverlapRisk = "none"
	}

	// Also mark global top hubs for demotion even if not focused.
	for _, rank := range rankEnv.Items {
		if rank.Centrality >= defaultHubCentralityFloor || rank.Impact >= defaultHubImpactFloor {
			if owner := filepath.ToSlash(rank.OwnerPath); owner != "" {
				hubPaths[owner] = true
			}
		}
	}
	return risk, hubPaths, warnings
}

func hubPathSetFromRanks(ranks []model.GraphRank) map[string]bool {
	out := map[string]bool{}
	for _, rank := range ranks {
		if rank.Centrality >= defaultHubCentralityFloor || rank.Impact >= defaultHubImpactFloor {
			if owner := filepath.ToSlash(rank.OwnerPath); owner != "" {
				out[owner] = true
			}
		}
	}
	return out
}
