package indexer

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/workspace"
)

var supportedExtensions = map[string]struct{}{
	".cs":  {},
	".js":  {},
	".jsx": {},
	".ts":  {},
	".tsx": {},
	".py":  {},
	".pyi": {},
}

func WalkWorkspace(root string, matcher *workspace.IgnoreMatcher) ([]string, error) {
	files := make([]string, 0, 256)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if matcher.ShouldIgnore(root, path) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if _, ok := supportedExtensions[strings.ToLower(filepath.Ext(path))]; ok {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
