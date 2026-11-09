package indexer

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// Bump when catalog extraction semantics change. Stamps are an optimization,
// never proof that a compiler graph is valid in another physical checkout.
const catalogStampVersion = 1
const catalogStampName = "catalog-stamps-v1.json"

type catalogStamp struct {
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
	Hash  string `json:"hash"`
}

type catalogStamps struct {
	Version  int                     `json:"version"`
	Identity string                  `json:"identity,omitempty"`
	Files    map[string]catalogStamp `json:"files"`
}

type catalogReuse struct {
	root    string
	files   map[string]model.FileRecord
	symbols map[string][]model.SymbolRecord
	stamps  catalogStamps
}

func readCatalogStamps(root string) catalogStamps {
	f, err := os.Open(filepath.Join(root, ".mi-lsp", catalogStampName))
	if err != nil {
		return catalogStamps{}
	}
	defer f.Close()
	var stamps catalogStamps
	if json.NewDecoder(io.LimitReader(f, 16<<20)).Decode(&stamps) != nil || stamps.Version != catalogStampVersion {
		return catalogStamps{}
	}
	return stamps
}

func loadCatalogReuse(ctx context.Context, root string, project model.ProjectFile) (catalogReuse, string) {
	identity, _ := workspace.ResolveRepositoryIdentity(ctx, root, project.Repos)
	// A physical folder copy can retain the provenance written by its indexer.
	// This is only a cache hint: each reused row still needs a matching stamp/hash.
	ownStamps := readCatalogStamps(root)
	if identity == "" {
		identity = ownStamps.Identity
	}
	roots := []string{root}
	if identity != "" {
		registry, err := workspace.LoadRegistryReadOnly()
		if err == nil {
			others := make([]string, 0, len(registry.Workspaces))
			for _, registration := range registry.Workspaces {
				if filepath.Clean(registration.Root) != filepath.Clean(root) {
					others = append(others, registration.Root)
				}
			}
			sort.Strings(others)
			roots = append(roots, others...)
		}
	}
	for _, source := range roots {
		if ctx.Err() != nil {
			break
		}
		if _, err := os.Stat(store.WorkspaceDBPath(source)); err != nil {
			continue
		}
		stamps := readCatalogStamps(source)
		if source != root {
			if identity == "" || stamps.Identity != identity {
				continue
			}
		}
		db, err := store.OpenReadOnlyExistingWithWAL(source, store.WorkspaceDBPath(source))
		if err != nil {
			continue
		}
		files, symbols, err := store.ReadCatalog(ctx, db)
		db.Close()
		if err == nil && len(files) != 0 {
			return catalogReuse{root: source, files: files, symbols: symbols, stamps: stamps}, identity
		}
	}
	return catalogReuse{}, identity
}

func (c catalogReuse) reuse(path string, info os.FileInfo, content []byte, repo model.WorkspaceRepo) (model.FileRecord, []model.SymbolRecord, bool) {
	file, ok := c.files[path]
	if !ok || file.Language != languageForPath(path) {
		return model.FileRecord{}, nil, false
	}
	stamp, stamped := c.stamps.Files[path]
	if content == nil {
		if !stamped || stamp.Hash != file.ContentHash || stamp.Size != info.Size() || stamp.Mtime != info.ModTime().UnixNano() {
			return model.FileRecord{}, nil, false
		}
	} else if digest(content) != file.ContentHash {
		return model.FileRecord{}, nil, false
	}
	file.RepoID, file.RepoName = repo.ID, repo.Name
	symbols := append([]model.SymbolRecord(nil), c.symbols[path]...)
	for i := range symbols {
		symbols[i].ID = 0
		symbols[i].RepoID = repo.ID
		symbols[i].RepoName = repo.Name
	}
	return file, symbols, true
}

// A failed cache write cannot invalidate a successfully extracted catalog. Each
// stamp carries its content hash, so a concurrent/failed publication only misses.
func writeCatalogStamps(root string, stamps catalogStamps) error {
	dir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "catalog-stamps-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := json.NewEncoder(f).Encode(stamps); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, catalogStampName))
}
