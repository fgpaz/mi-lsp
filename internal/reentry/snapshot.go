package reentry

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const recentWindow = 7 * 24 * time.Hour

var docIDPattern = regexp.MustCompile(`\b(?:FL|RF|TP|TECH|CT|DB)-[A-Z0-9-]+\b`)

type docChange struct {
	record    model.DocRecord
	updatedAt time.Time
}

func BuildSnapshot(root string, docs []model.DocRecord, builtAt time.Time) model.ReentryMemorySnapshot {
	changes := collectCanonicalChanges(root, docs, builtAt)
	handoff, _ := latestHandoff(root)

	snapshot := model.ReentryMemorySnapshot{
		SnapshotBuiltAt:        builtAt.UTC(),
		RecentCanonicalChanges: changes,
		Handoff:                handoff,
	}
	if len(changes) > 0 {
		snapshot.BestReentry = changes[0].Reentry
	}
	return snapshot
}

func SnapshotStale(root string, builtAt time.Time) bool {
	if builtAt.IsZero() {
		return false
	}
	return latestRelevantChange(root).After(builtAt)
}

func collectCanonicalChanges(root string, docs []model.DocRecord, builtAt time.Time) []model.ReentryMemoryChange {
	candidates := collectFilesystemCanonicalDocs(root, docs)
	cutoff := builtAt.Add(-recentWindow)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].updatedAt.Equal(candidates[j].updatedAt) {
			return candidates[i].record.Path < candidates[j].record.Path
		}
		return candidates[i].updatedAt.After(candidates[j].updatedAt)
	})

	selected := make([]docChange, 0, 3)
	for _, candidate := range candidates {
		if candidate.updatedAt.Before(cutoff) {
			continue
		}
		selected = append(selected, candidate)
		if len(selected) >= 3 {
			break
		}
	}
	if len(selected) == 0 && len(candidates) > 0 {
		selected = append(selected, candidates[0])
	}

	changes := make([]model.ReentryMemoryChange, 0, len(selected))
	for _, candidate := range selected {
		changes = append(changes, model.ReentryMemoryChange{
			Path:      candidate.record.Path,
			Title:     candidate.record.Title,
			DocID:     candidate.record.DocID,
			Why:       conciseWhy(candidate.record),
			UpdatedAt: candidate.updatedAt.UTC(),
			Reentry:   docReentryTarget(candidate.record),
		})
	}
	return changes
}

func collectFilesystemCanonicalDocs(root string, docs []model.DocRecord) []docChange {
	indexed := make(map[string]model.DocRecord, len(docs))
	for _, doc := range docs {
		indexed[filepath.ToSlash(doc.Path)] = doc
	}

	candidates := make([]docChange, 0, len(docs))
	wikiRoot := filepath.Join(root, ".docs", "wiki")
	_ = filepath.WalkDir(wikiRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "_mi-lsp") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		relative = filepath.ToSlash(relative)
		record := hydrateCanonicalDoc(root, relative, indexed[relative])
		if !isCanonicalDoc(record) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		candidates = append(candidates, docChange{record: record, updatedAt: info.ModTime()})
		return nil
	})
	return candidates
}

func latestRelevantChange(root string) time.Time {
	latest := latestTreeChange(filepath.Join(root, ".docs", "wiki"))
	rawLatest := latestTreeChange(filepath.Join(root, ".docs", "raw"))
	if rawLatest.After(latest) {
		return rawLatest
	}
	return latest
}

func latestHandoff(root string) (string, time.Time) {
	roots := []string{
		filepath.Join(root, ".docs", "raw", "plans"),
		filepath.Join(root, ".docs", "raw", "prompts"),
	}
	var latestPath string
	var latest time.Time
	for _, base := range roots {
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			info, statErr := d.Info()
			if statErr != nil {
				return nil
			}
			modTime := info.ModTime()
			if modTime.After(latest) {
				latest = modTime
				relative, relErr := filepath.Rel(filepath.Join(root, ".docs", "raw"), path)
				if relErr != nil {
					latestPath = filepath.Base(path)
				} else {
					latestPath = filepath.ToSlash(strings.TrimSuffix(relative, filepath.Ext(relative)))
				}
			}
			return nil
		})
	}
	return latestPath, latest
}

func latestTreeChange(root string) time.Time {
	var latest time.Time
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return latest
}

func isCanonicalDoc(doc model.DocRecord) bool {
	if doc.IsSnapshot || doc.Family == "generic" {
		return false
	}
	if !strings.HasPrefix(filepath.ToSlash(doc.Path), ".docs/wiki/") {
		return false
	}
	if doc.Layer == "" {
		return false
	}
	layerNumber, err := strconv.Atoi(doc.Layer)
	if err != nil {
		return false
	}
	return layerNumber >= 0 && layerNumber <= 9
}

func hydrateCanonicalDoc(root string, relativePath string, doc model.DocRecord) model.DocRecord {
	doc.Path = firstNonEmpty(doc.Path, relativePath)
	doc.Layer = firstNonEmpty(doc.Layer, detectLayer(relativePath))
	doc.Family = firstNonEmpty(doc.Family, familyForLayer(doc.Layer))
	doc.IsSnapshot = doc.IsSnapshot || isSnapshotPath(relativePath)

	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
	if err == nil {
		title := extractTitle(content)
		doc.Title = firstNonEmpty(doc.Title, title)
		doc.DocID = firstNonEmpty(doc.DocID, firstDocID(title+"\n"+string(content)))
		doc.Snippet = firstNonEmpty(doc.Snippet, extractSnippet(content))
		doc.SearchText = firstNonEmpty(doc.SearchText, normalizeText(title+"\n"+string(content)))
	}
	return doc
}

func conciseWhy(doc model.DocRecord) string {
	candidate := strings.TrimSpace(doc.Snippet)
	if candidate == "" {
		candidate = strings.TrimSpace(doc.SearchText)
	}
	if candidate == "" {
		candidate = strings.TrimSpace(doc.Title)
	}
	candidate = strings.Join(strings.Fields(candidate), " ")
	if len(candidate) <= 120 {
		return candidate
	}
	return strings.TrimSpace(candidate[:117]) + "..."
}

func docReentryTarget(doc model.DocRecord) model.ContinuationTarget {
	target := model.ContinuationTarget{Op: "nav.search"}
	switch {
	case strings.TrimSpace(doc.DocID) != "":
		target.DocID = strings.TrimSpace(doc.DocID)
		target.Query = strings.TrimSpace(doc.DocID)
	case strings.TrimSpace(doc.Title) != "":
		target.Path = doc.Path
		target.Query = strings.TrimSpace(doc.Title)
	default:
		target.Path = doc.Path
		target.Query = strings.TrimSpace(filepath.Base(doc.Path))
	}
	return target
}

func detectLayer(path string) string {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)
	if len(base) >= 2 && base[0] >= '0' && base[0] <= '9' && base[1] >= '0' && base[1] <= '9' {
		return base[:2]
	}
	switch {
	case strings.Contains(path, "/03_FL/"):
		return "03"
	case strings.Contains(path, "/04_RF/"):
		return "04"
	case strings.Contains(path, "/06_pruebas/"):
		return "06"
	case strings.Contains(path, "/07_tech/"):
		return "07"
	case strings.Contains(path, "/08_db/"):
		return "08"
	case strings.Contains(path, "/09_contratos/"):
		return "09"
	default:
		return ""
	}
}

func familyForLayer(layer string) string {
	layerNumber, err := strconv.Atoi(layer)
	if err != nil {
		return ""
	}
	switch {
	case layerNumber >= 0 && layerNumber <= 6:
		return "functional"
	case layerNumber >= 7 && layerNumber <= 9:
		return "technical"
	default:
		return ""
	}
}

func extractTitle(content []byte) string {
	for _, line := range strings.SplitN(string(content), "\n", 20) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func extractSnippet(content []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r", ""), "\n")
	parts := make([]string, 0, 4)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "```") {
			continue
		}
		parts = append(parts, line)
		if len(strings.Join(parts, " ")) >= 200 {
			break
		}
	}
	snippet := strings.Join(parts, " ")
	if len(snippet) > 220 {
		return strings.TrimSpace(snippet[:220]) + "..."
	}
	return snippet
}

func firstDocID(value string) string {
	return docIDPattern.FindString(value)
}

func normalizeText(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	return strings.Join(strings.Fields(value), " ")
}

func isSnapshotPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	snapshots := []string{"/old/", "/archive/", "/deprecated/", "/historico/", "/legacy/"}
	for _, segment := range snapshots {
		if strings.Contains(lower, segment) {
			return true
		}
		if len(segment) > 1 && strings.HasPrefix(lower, segment[1:]) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
