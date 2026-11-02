package model

import (
	"fmt"
	"path"
	"strings"
)

// GraphWorktreeDomain binds a native Go observation to the pinned graph's
// legacy request-root key domain while retaining its physical workspace paths.
// It is an in-memory proof object, not a wire or persistence contract.
type GraphWorktreeDomain struct {
	PinGenerationID       GraphDigest
	TopologyDigest        GraphDigest
	WorkspaceIdentity     string
	RepositoryIdentity    string
	Backend               string
	ProjectOrModule       string // Native NodeKey domain, relative to RequestRoot.
	PhysicalPrefix        string // RequestRoot relative to the workspace root.
	PhysicalModule        string // ProjectOrModule projected into workspace coordinates.
	ModulePath            string // go.mod semantic namespace from the native receipt.
	SourceFingerprint     GraphDigest
	ConfigFingerprint     GraphDigest
	BatchDigest           GraphDigest
	ScopeFingerprint      GraphDigest
	ListInvocationDigest  GraphDigest
	ListResultDigest      GraphDigest
	PinnedPackageNodeKeys []GraphDigest
	OwnerMasks            []GraphWorktreeOwnerMask
}

// GraphWorktreeOwnerMask distinguishes a physical workspace owner from a
// semantic package owner in one exact backend/module key domain.
type GraphWorktreeOwnerMask struct {
	Kind          string // "source" or "package".
	OwnerPath     string // Native Go owner coordinate.
	WorkspacePath string // Physical workspace coordinate; empty for package masks.
}

func (d GraphWorktreeDomain) Validate() error {
	if d.Backend != "go" || d.WorkspaceIdentity == "" || d.RepositoryIdentity == "" || d.WorkspaceIdentity != d.RepositoryIdentity || d.ProjectOrModule == "" || d.ModulePath == "" || d.PinGenerationID == (GraphDigest{}) || d.TopologyDigest == (GraphDigest{}) || d.SourceFingerprint == (GraphDigest{}) || d.ConfigFingerprint == (GraphDigest{}) || d.BatchDigest == (GraphDigest{}) || d.ScopeFingerprint == (GraphDigest{}) || d.ListInvocationDigest == (GraphDigest{}) || d.ListResultDigest == (GraphDigest{}) {
		return fmt.Errorf("explicit_incomplete: Go worktree domain lacks a pinned receipt identity")
	}
	if !validWorktreeDomainPath(d.PhysicalPrefix, true) || !validWorktreeDomainPath(d.ProjectOrModule, false) || !validWorktreeDomainPath(d.PhysicalModule, false) || path.Join(d.PhysicalPrefix, d.ProjectOrModule) != d.PhysicalModule {
		return fmt.Errorf("explicit_incomplete: Go worktree domain paths are not a canonical physical projection")
	}
	if len(d.PinnedPackageNodeKeys) == 0 {
		return fmt.Errorf("explicit_incomplete: Go worktree domain has no pinned package-key witness")
	}
	seenKeys := map[GraphDigest]bool{}
	for _, key := range d.PinnedPackageNodeKeys {
		if key == (GraphDigest{}) || seenKeys[key] {
			return fmt.Errorf("explicit_incomplete: Go worktree domain has an invalid pinned package-key witness")
		}
		seenKeys[key] = true
	}
	seenMasks := map[string]bool{}
	for _, mask := range d.OwnerMasks {
		if !validWorktreeDomainPath(mask.OwnerPath, false) {
			return fmt.Errorf("explicit_incomplete: Go worktree owner mask is not a safe relative path")
		}
		key := mask.Kind + "\x00" + mask.OwnerPath + "\x00" + mask.WorkspacePath
		if seenMasks[key] {
			return fmt.Errorf("explicit_incomplete: duplicate Go worktree owner mask")
		}
		seenMasks[key] = true
		switch mask.Kind {
		case "source":
			if strings.HasPrefix(mask.OwnerPath, "@package/") || !validWorktreeDomainPath(mask.WorkspacePath, false) || path.Join(d.PhysicalPrefix, mask.OwnerPath) != mask.WorkspacePath {
				return fmt.Errorf("explicit_incomplete: Go source owner mask does not match its physical workspace path")
			}
		case "package":
			if !strings.HasPrefix(mask.OwnerPath, "@package/") || mask.WorkspacePath != "" {
				return fmt.Errorf("explicit_incomplete: Go package mask must remain a semantic owner")
			}
		default:
			return fmt.Errorf("explicit_incomplete: unknown Go worktree owner-mask kind")
		}
	}
	return nil
}

func (d GraphWorktreeDomain) ProjectOwner(owner string) (string, bool) {
	if !validWorktreeDomainPath(owner, false) || strings.HasPrefix(owner, "@package/") {
		return owner, false
	}
	projected := path.Join(d.PhysicalPrefix, owner)
	if !validWorktreeDomainPath(projected, false) {
		return owner, false
	}
	return projected, true
}

func (d GraphWorktreeDomain) MasksOwner(owner string) bool {
	for _, mask := range d.OwnerMasks {
		if mask.OwnerPath == owner {
			return true
		}
	}
	return false
}

func validWorktreeDomainPath(value string, allowDot bool) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00") || strings.HasPrefix(value, "/") || (len(value) > 1 && value[1] == ':') {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != ".." && !strings.HasPrefix(clean, "../") && (allowDot || clean != ".")
}
