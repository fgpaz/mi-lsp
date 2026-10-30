package docgraph

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWikiContractAECanonGovernanceStates(t *testing.T) {
	tests := []struct {
		name             string
		governance       string
		copyRepoPolicy   bool
		wantStatus       string
		wantSource       string
		wantBlocking     bool
		wantGovernance   bool
		wantParseWarning string
	}{
		{
			name: "absent optional block",
			governance: `wiki_contract: WIKI-CONTRACT-v1
version: 1
language: en`,
			wantStatus: "not_applicable",
			wantSource: "none",
		},
		{
			name: "valid published kernel v2 canon",
			governance: `wiki_contract: WIKI-CONTRACT-v1
version: 1
language: en
ae_canon:
  mode: kernel_v2
  source: <kernel_home>/canon
  repo_policy: .docs/ae/repo-policy.yaml`,
			copyRepoPolicy: true,
			wantStatus:     "valid",
			wantSource:     "kernel_v2",
		},
		{
			name: "incomplete declared block fails closed",
			governance: `wiki_contract: WIKI-CONTRACT-v1
version: 1
language: en
ae_canon:
  source: <kernel_home>/canon
  repo_policy: .docs/ae/repo-policy.yaml`,
			wantStatus:       "mismatch",
			wantSource:       "governance",
			wantBlocking:     true,
			wantGovernance:   true,
			wantParseWarning: "ae_canon.mode is required",
		},
		{
			name:             "malformed declared block fails closed",
			governance:       "wiki_contract: WIKI-CONTRACT-v1\nversion: 1\nlanguage: en\nae_canon: [kernel_v2]",
			wantStatus:       "mismatch",
			wantSource:       "governance",
			wantBlocking:     true,
			wantGovernance:   true,
			wantParseWarning: "ae_canon must be a mapping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeGovernanceYAML(t, root, canonGovernanceV1RelPath, tt.governance)
			if tt.copyRepoPolicy {
				writeCurrentRepoPolicy(t, root)
			}

			status := InspectGovernance(root, false)
			if status.AECanon.Status != tt.wantStatus || status.AECanon.Source != tt.wantSource || status.AECanon.Blocking != tt.wantBlocking {
				t.Fatalf("AE canon status: got status=%q source=%q blocking=%v missing=%v; want status=%q source=%q blocking=%v", status.AECanon.Status, status.AECanon.Source, status.AECanon.Blocking, status.AECanon.MissingModules, tt.wantStatus, tt.wantSource, tt.wantBlocking)
			}
			if status.Blocked != tt.wantGovernance {
				t.Fatalf("governance blocked=%v, want %v; issues=%v", status.Blocked, tt.wantGovernance, status.Issues)
			}
			if tt.wantGovernance {
				if !containsIssue(status.Issues, "AE canon is blocked:") {
					t.Fatalf("missing AE canon blocking issue: %v", status.Issues)
				}
			} else if containsIssue(status.Issues, "AE canon is blocked:") {
				t.Fatalf("unexpected AE canon blocking issue: %v", status.Issues)
			}
			if tt.wantParseWarning != "" && !containsWarningFragment(status.Warnings, tt.wantParseWarning) {
				t.Fatalf("missing parse warning %q in %v", tt.wantParseWarning, status.Warnings)
			}
		})
	}
}

func writeCurrentRepoPolicy(t *testing.T, root string) {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate the test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	policy, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(".docs/ae/repo-policy.yaml")))
	if err != nil {
		t.Fatalf("read published repo policy: %v", err)
	}
	policyPath := filepath.Join(root, filepath.FromSlash(".docs/ae/repo-policy.yaml"))
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, policy, 0o644); err != nil {
		t.Fatalf("copy published repo policy: %v", err)
	}
}

func containsWarningFragment(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, fragment) {
			return true
		}
	}
	return false
}

func containsIssue(issues []string, prefix string) bool {
	for _, issue := range issues {
		if strings.HasPrefix(issue, prefix) {
			return true
		}
	}
	return false
}
