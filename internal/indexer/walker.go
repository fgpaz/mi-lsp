package indexer

import (
	"context"
	"io/fs"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/language"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func WalkWorkspace(ctx context.Context, root string, matcher *workspace.IgnoreMatcher) ([]string, error) {
	files := make([]string, 0, 256)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		if language.IsSupportedCodePath(path) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
