package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var skipDirNames = map[string]struct{}{
	"bin":          {},
	"node_modules": {},
	".git":         {},
	".hg":          {},
	".svn":         {},
}

// credential-looking basenames we never open during skill scans.
var credentialBasenames = map[string]struct{}{
	".env":                 {},
	".env.local":           {},
	"credentials":          {},
	"credentials.json":     {},
	"secrets":              {},
	"secrets.json":         {},
	"secret.yaml":          {},
	"secret.yml":           {},
	"id_rsa":               {},
	"id_ed25519":           {},
	"private.key":          {},
	"service-account.json": {},
}

// ScanSkillsRoot walks skillsRoot and parses each SKILL.md (frontmatter + short body head).
func ScanSkillsRoot(skillsRoot string) ([]ScanResult, []string, error) {
	root := filepath.Clean(skillsRoot)
	info, err := os.Stat(root)
	if err != nil {
		return nil, nil, fmt.Errorf("skills root: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("skills root is not a directory: %s", root)
	}

	var (
		results  []ScanResult
		warnings []string
	)

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			warnings = append(warnings, fmt.Sprintf("walk error %s: %v", path, walkErr))
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			// Case-insensitive: Windows and mixed-case trees use Bin/, Node_Modules/, etc.
			if _, skip := skipDirNames[strings.ToLower(name)]; skip {
				return filepath.SkipDir
			}
			// Never descend into hidden dirs except the root itself.
			if strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.EqualFold(name, "SKILL.md") {
			return nil
		}
		if looksCredentialPath(path) {
			warnings = append(warnings, fmt.Sprintf("skipped credential-looking path: %s", path))
			return nil
		}

		scan, err := ParseSkillFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("parse %s: %v", path, err))
			return nil
		}
		// Default id from parent directory name when frontmatter lacks name.
		if scan.ID == "" {
			scan.ID = filepath.Base(filepath.Dir(path))
		}
		results = append(results, scan)
		return nil
	})
	if err != nil {
		return results, warnings, err
	}
	return results, warnings, nil
}

// ParseSkillFile reads a single SKILL.md and returns frontmatter + short indexed head.
func ParseSkillFile(path string) (ScanResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ScanResult{}, err
	}
	// Hard cap raw read for safety (already limited to SKILL.md).
	if len(raw) > 512*1024 {
		raw = raw[:512*1024]
	}

	fm, body, err := splitFrontmatter(string(raw))
	if err != nil {
		return ScanResult{}, err
	}

	meta := map[string]any{}
	if strings.TrimSpace(fm) != "" {
		if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
			// Soft-fail: keep body, empty meta.
			meta = map[string]any{}
		}
	}

	name := stringField(meta, "name")
	desc := stringField(meta, "description")
	when := stringField(meta, "when_to_use")
	if when == "" {
		when = stringField(meta, "when-to-use")
	}
	whenNot := stringField(meta, "when_not_to_use")
	if whenNot == "" {
		whenNot = stringField(meta, "when-not-to-use")
	}

	head := takeHead(body, MaxIndexedBodyLines, MaxIndexedBytes)
	// Indexed text: id-ish name + description + when + body head.
	parts := []string{}
	if name != "" {
		parts = append(parts, name)
	}
	if desc != "" {
		parts = append(parts, desc)
	}
	if when != "" {
		parts = append(parts, when)
	}
	if head != "" {
		parts = append(parts, head)
	}
	indexed := strings.TrimSpace(strings.Join(parts, "\n"))
	if len(indexed) > MaxIndexedBytes {
		indexed = indexed[:MaxIndexedBytes]
	}
	sum := sha256.Sum256([]byte(indexed))

	id := name
	if id == "" {
		id = filepath.Base(filepath.Dir(path))
	}

	return ScanResult{
		ID:           id,
		SourcePath:   path,
		Description:  desc,
		WhenToUse:    when,
		WhenNotToUse: whenNot,
		IndexedText:  indexed,
		ContentSHA:   hex.EncodeToString(sum[:]),
		Name:         name,
		Frontmatter:  meta,
	}, nil
}

func splitFrontmatter(content string) (frontmatter, body string, err error) {
	content = strings.TrimPrefix(content, "\uFEFF")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return "", "", nil
	}
	if strings.TrimSpace(lines[0]) != "---" {
		return "", content, nil
	}
	var fmLines []string
	closed := false
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			body = strings.Join(lines[i+1:], "\n")
			closed = true
			break
		}
		fmLines = append(fmLines, lines[i])
	}
	if !closed {
		// Unterminated frontmatter: treat whole file as body.
		return "", content, nil
	}
	return strings.Join(fmLines, "\n"), body, nil
}

func takeHead(body string, maxLines, maxBytes int) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	out := strings.Join(lines, "\n")
	if len(out) > maxBytes {
		out = out[:maxBytes]
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func looksCredentialPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if _, ok := credentialBasenames[base]; ok {
		return true
	}
	lower := strings.ToLower(path)
	for _, needle := range []string{
		string(filepath.Separator) + "secrets" + string(filepath.Separator),
		string(filepath.Separator) + ".ssh" + string(filepath.Separator),
		string(filepath.Separator) + "credentials" + string(filepath.Separator),
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
