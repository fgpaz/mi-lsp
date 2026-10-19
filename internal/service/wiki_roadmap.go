package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// loadWikiRoadmapMetadata treats the external backup as optional: absence is an empty projection,
// while other read failures are surfaced as typed non-blocking roadmap warnings.
func loadWikiRoadmapMetadata(root string) model.WikiRoadmapParseResult {
	result := model.WikiRoadmapParseResult{
		Source: model.WikiRoadmapSource, Canonical: false,
		ByRS: make(map[string]model.WikiRoadmapMetadata),
	}
	path := filepath.Join(root, filepath.FromSlash(model.WikiRoadmapSource))
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return result
	}
	if err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = fmt.Errorf("roadmap source must be a regular file")
		}
		result.Warnings = append(result.Warnings, model.WikiRoadmapWarning{
			Code: "roadmap_unreadable", Message: fmt.Sprintf("roadmap_unreadable:%s:%v", model.WikiRoadmapSource, err), Blocking: false,
		})
		return result
	}
	content, err := os.ReadFile(path)
	if err != nil {
		result.Warnings = append(result.Warnings, model.WikiRoadmapWarning{Code: "roadmap_unreadable", Message: fmt.Sprintf("roadmap_unreadable:%s:%v", model.WikiRoadmapSource, err), Blocking: false})
		return result
	}
	return model.ParseWikiRoadmapMarkdown(string(content))
}
