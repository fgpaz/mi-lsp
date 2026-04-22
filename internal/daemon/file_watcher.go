package daemon

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
	"github.com/fsnotify/fsnotify"
)

type FileWatcher struct {
	workspaceRoot string
	registration  model.WorkspaceRegistration
	watcher       *fsnotify.Watcher
	debounce      map[string]*time.Timer
	debounceDur   time.Duration
	mu            sync.Mutex
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
	verbose       bool
	watchedDirs   int
}

// NewFileWatcher creates a new file watcher for a workspace.
func NewFileWatcher(registration model.WorkspaceRegistration, debounceDur time.Duration) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if debounceDur <= 0 {
		debounceDur = 500 * time.Millisecond
	}

	fw := &FileWatcher{
		workspaceRoot: registration.Root,
		registration:  registration,
		watcher:       watcher,
		debounce:      make(map[string]*time.Timer),
		debounceDur:   debounceDur,
		stopCh:        make(chan struct{}),
		verbose:       os.Getenv("MI_LSP_VERBOSE") != "",
	}

	return fw, nil
}

// Start begins watching the workspace for file changes.
func (fw *FileWatcher) Start(ctx context.Context) error {
	// Add workspace root for watching
	err := fw.addWatchRecursive(fw.workspaceRoot)
	if err != nil {
		return err
	}

	fw.wg.Add(1)
	go fw.watchLoop(ctx)
	return nil
}

// Stop closes the watcher and stops the watch loop.
func (fw *FileWatcher) Stop() {
	fw.stopOnce.Do(func() {
		close(fw.stopCh)
	})

	// Cancel all pending debounce timers
	fw.mu.Lock()
	for filePath, timer := range fw.debounce {
		timer.Stop()
		delete(fw.debounce, filePath)
	}
	fw.mu.Unlock()

	// Wait for watchLoop to exit
	fw.wg.Wait()

	// Close watcher last
	_ = fw.watcher.Close()
}

func (fw *FileWatcher) watchLoop(ctx context.Context) {
	defer fw.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-fw.stopCh:
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if isWatchableFile(event.Name) {
					fw.scheduleReindex(event.Name)
				}
			}
			if event.Op&fsnotify.Create != 0 {
				// Watch new directories
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() && !shouldSkipDir(event.Name) {
					if err := fw.watcher.Add(event.Name); err == nil {
						fw.mu.Lock()
						fw.watchedDirs++
						fw.mu.Unlock()
					}
				}
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			if fw.verbose {
				log.Printf("[mi-lsp:watcher] error: %v", err)
			}
		}
	}
}

func (fw *FileWatcher) PendingEvents() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return len(fw.debounce)
}

func (fw *FileWatcher) WatchedDirCount() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.watchedDirs
}

func (fw *FileWatcher) scheduleReindex(filePath string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if timer, exists := fw.debounce[filePath]; exists {
		timer.Stop()
		delete(fw.debounce, filePath)
	}

	fw.debounce[filePath] = time.AfterFunc(fw.debounceDur, func() {
		fw.mu.Lock()
		delete(fw.debounce, filePath)
		fw.mu.Unlock()
		fw.reindexFile(filePath)
	})
}

func (fw *FileWatcher) reindexFile(absPath string) {
	defer func() {
		if r := recover(); r != nil {
			if fw.verbose {
				log.Printf("[mi-lsp:watcher] recovered panic reindexing %s: %v", absPath, r)
			}
		}
	}()

	relPath, err := filepath.Rel(fw.workspaceRoot, absPath)
	if err != nil {
		return
	}
	relPath = filepath.ToSlash(relPath)

	// Load project topology to determine repo ownership
	registration, err := workspace.DetectWorkspace(fw.workspaceRoot)
	if err != nil {
		if fw.verbose {
			log.Printf("[mi-lsp:watcher] detect workspace error for %s: %v", relPath, err)
		}
		return
	}

	projectFile, err := workspace.LoadProjectTopology(fw.workspaceRoot, registration)
	if err != nil {
		if fw.verbose {
			log.Printf("[mi-lsp:watcher] load topology error for %s: %v", relPath, err)
		}
		return
	}

	// Determine repo ownership
	repoID, repoName := indexer.ResolveRepoFromProjectFile(fw.workspaceRoot, projectFile, absPath)

	// Extract symbols
	symbols, language, err := indexer.ExtractFileSymbols(fw.workspaceRoot, relPath, repoID, repoName)
	if err != nil {
		if fw.verbose {
			log.Printf("[mi-lsp:watcher] extract error for %s: %v", relPath, err)
		}
		return
	}

	if err := store.WithWorkspaceWriteLock(fw.workspaceRoot, func() error {
		db, err := store.Open(fw.workspaceRoot)
		if err != nil {
			return err
		}
		defer db.Close()

		content, err := os.ReadFile(absPath)
		if err != nil {
			return err
		}
		contentHash := computeHash(content)
		return store.ReplaceFileSymbols(context.Background(), db, relPath, repoID, repoName, language, contentHash, symbols)
	}); err != nil {
		if fw.verbose {
			log.Printf("[mi-lsp:watcher] db update error for %s: %v", relPath, err)
		}
	} else if fw.verbose {
		log.Printf("[mi-lsp:watcher] reindexed %s: %d symbols", relPath, len(symbols))
	}
}

func (fw *FileWatcher) addWatchRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			if shouldSkipDir(path) {
				return filepath.SkipDir
			}
			if watchErr := fw.watcher.Add(path); watchErr != nil {
				// Log but don't fail — some dirs may not be watchable
				if fw.verbose {
					log.Printf("[mi-lsp:watcher] skip dir %s: %v", path, watchErr)
				}
			} else {
				fw.mu.Lock()
				fw.watchedDirs++
				fw.mu.Unlock()
			}
		}
		return nil
	})
}

var watchableExtensions = map[string]struct{}{
	".cs": {}, ".ts": {}, ".tsx": {}, ".js": {}, ".jsx": {}, ".py": {},
}

func isWatchableFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := watchableExtensions[ext]
	return ok
}

var skipDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, "bin": {}, "obj": {}, "dist": {},
	".mi-lsp": {}, ".vs": {}, ".idea": {}, "__pycache__": {},
	".worktrees": {}, "vendor": {}, ".next": {}, "out": {},
}

func shouldSkipDir(dirPath string) bool {
	base := pathpkg.Base(strings.ReplaceAll(dirPath, "\\", "/"))
	_, ok := skipDirs[base]
	return ok
}

func computeHash(content []byte) string {
	// Compute SHA1 hash of content
	sum := sha1.Sum(content)
	return hex.EncodeToString(sum[:])
}
