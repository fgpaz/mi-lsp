package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	canonModeReadOnly      = "read-only"
	defaultCanonEscapeMax  = 1
	canonRoleProducto      = "producto"
	canonRoleEcosistema    = "ecosistema"
	canonRoleGobiernoLocal = "gobierno_local"
)

var (
	ErrCanonRootAbsolute   = errors.New("canon_root_absolute")
	ErrCanonRootEscape     = errors.New("canon_root_escape")
	ErrCanonRootSymlink    = errors.New("canon_root_symlink")
	ErrCanonRootMissing    = errors.New("canon_root_missing")
	ErrCanonRootInspection = errors.New("canon_root_inspection_failed")
	ErrCanonRoleInvalid    = errors.New("canon_role_invalid")
	ErrCanonModeInvalid    = errors.New("canon_mode_invalid")
	ErrCanonIDDuplicate    = errors.New("canon_id_duplicate")
	ErrCanonIDMissing      = errors.New("canon_id_missing")
	ErrCanonRootReadOnly   = errors.New("canon_root_read_only")
)

var validCanonRoles = map[string]struct{}{
	canonRoleProducto:      {},
	canonRoleEcosistema:    {},
	canonRoleGobiernoLocal: {},
}

// ResolvedCanon is a [[canon]] table after workspace-root-relative resolution.
type ResolvedCanon struct {
	ID           string
	Role         string
	Mode         string
	DeclaredRoot string
	AbsRoot      string
}

func CanonEscapeMax(project model.ProjectFile) int {
	if project.CanonPolicy == nil || project.CanonPolicy.EscapeMax == nil {
		return defaultCanonEscapeMax
	}
	return *project.CanonPolicy.EscapeMax
}

func ValidateCanonRole(role string) error {
	role = strings.TrimSpace(role)
	if _, ok := validCanonRoles[role]; !ok {
		return fmt.Errorf("%w: role %q is invalid; use producto, ecosistema, or gobierno_local", ErrCanonRoleInvalid, role)
	}
	return nil
}

func ValidateCanonDeclarations(project model.ProjectFile) error {
	escapeMax := CanonEscapeMax(project)
	seenIDs := map[string]string{}
	for _, canon := range project.Canons {
		id := strings.TrimSpace(canon.ID)
		if id == "" {
			return fmt.Errorf("%w: [[canon]] is missing id; set a unique id", ErrCanonIDMissing)
		}
		idKey := strings.ToLower(id)
		if previous, ok := seenIDs[idKey]; ok {
			return fmt.Errorf("%w: canon id %q duplicates %q (ids are case-insensitive); give each [[canon]] a unique id", ErrCanonIDDuplicate, id, previous)
		}
		seenIDs[idKey] = id

		role := strings.TrimSpace(canon.Role)
		if _, ok := validCanonRoles[role]; !ok {
			return fmt.Errorf("%w: canon %q role %q is invalid; use producto, ecosistema, or gobierno_local", ErrCanonRoleInvalid, id, role)
		}

		mode := strings.TrimSpace(canon.Mode)
		if mode != "" && !strings.EqualFold(mode, canonModeReadOnly) {
			return fmt.Errorf("%w: canon %q mode %q is invalid; omit mode or set mode = \"read-only\" (only read-only is implemented)", ErrCanonModeInvalid, id, mode)
		}

		declared := declaredCanonRoot(canon.Root)
		if declared == "" {
			return fmt.Errorf("%w: canon %q root is empty; declare a relative path from the workspace root (use ../sibling, not C:\\...)", ErrCanonRootAbsolute, id)
		}
		if canonDeclaredRootIsAbsolute(declared) {
			return fmt.Errorf("%w: canon %q root must be relative to the workspace; use ../sibling, not an absolute, drive, UNC, or home path", ErrCanonRootAbsolute, id)
		}
		escapes := countCanonEscapes(declared)
		if escapes > escapeMax {
			return fmt.Errorf("%w: canon %q root has %d parent escapes, max is %d; raise [canon_policy].escape_max or use a relative directory within the allowed limit", ErrCanonRootEscape, id, escapes, escapeMax)
		}
	}
	return nil
}

func ResolveCanons(workspaceRoot string, project model.ProjectFile) ([]ResolvedCanon, error) {
	if err := ValidateCanonDeclarations(project); err != nil {
		return nil, err
	}
	if len(project.Canons) == 0 {
		return nil, nil
	}
	wsRoot, err := absWorkspaceRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	resolved := make([]ResolvedCanon, 0, len(project.Canons))
	for _, canon := range project.Canons {
		item, err := resolveCanon(wsRoot, canon)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, item)
	}
	return resolved, nil
}

func ResolveCanonsByRole(workspaceRoot string, project model.ProjectFile, role string) ([]ResolvedCanon, error) {
	resolved, err := ResolveCanons(workspaceRoot, project)
	if err != nil {
		return nil, err
	}
	role = strings.TrimSpace(role)
	if role == "" {
		return resolved, nil
	}
	filtered := make([]ResolvedCanon, 0, len(resolved))
	for _, canon := range resolved {
		if strings.EqualFold(canon.Role, role) {
			filtered = append(filtered, canon)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no [[canon]] matched role %q; available: %s; pass a declared role or omit --role", role, formatCanonAvailability(project.Canons))
	}
	return filtered, nil
}

func PathIsReadOnlyCanon(workspaceRoot string, project model.ProjectFile, absTarget string) (bool, error) {
	if strings.TrimSpace(absTarget) == "" {
		return false, errors.New("canon read-only check requires an absolute target path")
	}
	target := filepath.Clean(absTarget)
	if !filepath.IsAbs(target) {
		return false, fmt.Errorf("canon read-only check requires an absolute target path, got %q", absTarget)
	}
	resolved, err := ResolveCanons(workspaceRoot, project)
	if err != nil {
		return false, err
	}
	for _, canon := range resolved {
		if pathIsInside(canon.AbsRoot, target) {
			return true, nil
		}
	}
	return false, nil
}

func resolveCanon(workspaceRoot string, canon model.WorkspaceCanon) (ResolvedCanon, error) {
	id := strings.TrimSpace(canon.ID)
	declared := declaredCanonRoot(canon.Root)
	// Root is relative to the workspace root (the directory that contains
	// .mi-lsp/project.toml), never to .mi-lsp/ and never to cwd.
	absRoot := filepath.Clean(filepath.Join(workspaceRoot, filepath.FromSlash(declared)))
	if err := rejectCanonRootSymlinks(absRoot, id); err != nil {
		return ResolvedCanon{}, err
	}
	info, err := os.Lstat(absRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ResolvedCanon{}, fmt.Errorf("%w: canon %q root does not exist; create the directory or fix the relative path from the workspace root", ErrCanonRootMissing, id)
		}
		return ResolvedCanon{}, fmt.Errorf("%w: canon %q root could not be inspected; verify filesystem access and use a valid relative path", ErrCanonRootInspection, id)
	}
	if !info.IsDir() {
		return ResolvedCanon{}, fmt.Errorf("%w: canon %q root is not a directory; point [[canon]].root at a real directory", ErrCanonRootMissing, id)
	}
	return ResolvedCanon{
		ID:           id,
		Role:         strings.TrimSpace(canon.Role),
		Mode:         canonModeReadOnly,
		DeclaredRoot: declared,
		AbsRoot:      absRoot,
	}, nil
}

func rejectCanonRootSymlinks(absRoot, id string) error {
	current := filepath.Clean(absRoot)
	for {
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%w: canon %q root does not exist; create the directory or fix the relative path from the workspace root", ErrCanonRootMissing, id)
			}
			return fmt.Errorf("%w: canon %q root could not be inspected; verify filesystem access and use a valid relative path", ErrCanonRootInspection, id)
		}
		link, linkErr := canonComponentIsLink(info, current)
		if linkErr != nil {
			return fmt.Errorf("%w: canon %q root could not be inspected for a symlink or junction; verify filesystem access and use a valid relative path", ErrCanonRootInspection, id)
		}
		if link {
			return fmt.Errorf("%w: canon %q root is a symlink or junction; point [[canon]].root at a real directory", ErrCanonRootSymlink, id)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func absWorkspaceRoot(workspaceRoot string) (string, error) {
	trimmed := strings.TrimSpace(workspaceRoot)
	if trimmed == "" {
		return "", errors.New("workspace root is required to resolve [[canon]]")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: verify workspace root is accessible", ErrCanonRootInspection)
	}
	return filepath.Clean(abs), nil
}

func declaredCanonRoot(root string) string {
	return filepath.ToSlash(strings.TrimSpace(root))
}

func countCanonEscapes(declared string) int {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(declared)))
	count := 0
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			count++
		}
	}
	return count
}

func canonDeclaredRootIsAbsolute(root string) bool {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return false
	}
	if filepath.IsAbs(trimmed) {
		return true
	}
	if strings.HasPrefix(trimmed, "/") {
		return true
	}
	if strings.HasPrefix(trimmed, `\\`) || strings.HasPrefix(trimmed, "//") {
		return true
	}
	if isHomeRelativeCanonRoot(trimmed) {
		return true
	}
	return isWindowsVolumePath(trimmed)
}

func isHomeRelativeCanonRoot(path string) bool {
	if path == "~" {
		return true
	}
	return strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`)
}

func isWindowsVolumePath(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	drive := path[0]
	return (drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')
}

func pathIsInside(absRoot, absTarget string) bool {
	root := filepath.Clean(absRoot)
	target := filepath.Clean(absTarget)
	if caseInsensitivePlatform() {
		root = strings.ToLower(root)
		target = strings.ToLower(target)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}

func canonComponentIsLink(info os.FileInfo, path string) (bool, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	return canonPathIsReparse(path)
}

func formatCanonAvailability(canons []model.WorkspaceCanon) string {
	if len(canons) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(canons))
	for _, canon := range canons {
		parts = append(parts, fmt.Sprintf("id=%q role=%q", strings.TrimSpace(canon.ID), strings.TrimSpace(canon.Role)))
	}
	return strings.Join(parts, ", ")
}
