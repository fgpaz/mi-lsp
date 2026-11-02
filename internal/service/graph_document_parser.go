package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
)

type graphDocumentParse struct {
	Docs         []model.DocRecord
	Edges        []model.DocEdge
	Mentions     []model.DocMention
	MissingPaths []string
}

// parseWorktreeGraphDocuments uses the same single-document parser as full and
// incremental indexing. The caller supplies the validated profile; this helper
// does not discover/scan a tree, resolve lookup context, or certify old owners.
func parseWorktreeGraphDocuments(ctx context.Context, root string, paths []string, profile model.DocsReadProfile) (graphDocumentParse, error) {
	var result graphDocumentParse
	canonical, err := indexer.GraphDeltaPathsForRoot(root, root, paths, ".md")
	if err != nil || len(canonical) != len(paths) || !sameGraphDeltaPaths(canonical, paths) {
		return result, overlayGraphUnavailable("document selector is not a distinct canonical Markdown path set")
	}
	for _, path := range canonical {
		if err := ctx.Err(); err != nil {
			return graphDocumentParse{}, err
		}
		content, missing, err := readWorktreeGraphDocument(ctx, root, path)
		if err != nil {
			return graphDocumentParse{}, err
		}
		if missing {
			// Absence is not proof of prior graph ownership. The pinned base
			// join is a separate prerequisite for a document tombstone.
			result.MissingPaths = append(result.MissingPaths, path)
			continue
		}
		doc, edges, mentions, _, _, _, warning, err := docgraph.ParseSingleDoc(root, path, content, profile)
		if err != nil {
			return graphDocumentParse{}, err
		}
		if warning != "" {
			return graphDocumentParse{}, overlayGraphUnavailable("document parsing was incomplete")
		}
		result.Docs = append(result.Docs, doc)
		result.Edges = append(result.Edges, edges...)
		result.Mentions = append(result.Mentions, mentions...)
	}
	return result, nil
}

// readWorktreeGraphDocument never interprets links, permissions or non-regular
// entries as deletions. Same-file/stat checks bind the bytes to the checked
// entry; the outer overlay fingerprint must still be checked after observation.
// These checks are not an atomic filesystem snapshot.
func readWorktreeGraphDocument(ctx context.Context, root, path string) ([]byte, bool, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, false, overlayGraphUnavailable("document root is not a verified physical directory")
	}
	current := root
	var before os.FileInfo
	parts := strings.Split(filepath.FromSlash(path), string(filepath.Separator))
	for i, part := range parts {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		current = filepath.Join(current, part)
		before, err = os.Lstat(current)
		if os.IsNotExist(err) {
			return nil, true, nil
		}
		if err != nil || before.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !before.IsDir()) || (i == len(parts)-1 && !before.Mode().IsRegular()) {
			return nil, false, overlayGraphUnavailable("document path is unreadable, linked or not regular")
		}
	}
	// Keep a growing or oversized input from exhausting the query process.
	// Crossing this observation budget is incomplete, never a partial document.
	const maxDocumentBytes int64 = 64 << 20
	if before.Size() < 0 || before.Size() > maxDocumentBytes {
		return nil, false, overlayGraphUnavailable("document exceeds physical observation byte budget")
	}
	file, err := os.Open(current)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, false, overlayGraphUnavailable("document entry changed before reading")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(content)) > maxDocumentBytes {
		return nil, false, overlayGraphUnavailable("document grew beyond physical observation byte budget")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	after, err := os.Lstat(current)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(content)) != after.Size() {
		return nil, false, overlayGraphUnavailable("document changed during reading")
	}
	return content, false, nil
}
