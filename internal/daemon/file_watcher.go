package daemon

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"log"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fsnotify/fsnotify"
)

type watcherTimer interface {
	Stop() bool
}

type watcherTimerFactory func(time.Duration, func()) watcherTimer

type FileWatcher struct {
	workspaceRoot     string
	registration      model.WorkspaceRegistration
	watcher           *fsnotify.Watcher
	debounce          map[string]*time.Timer
	debounceDur       time.Duration
	maxWatchedDirs    int
	mu                sync.Mutex
	stopCh            chan struct{}
	stopOnce          sync.Once
	wg                sync.WaitGroup
	verbose           bool
	watchedDirs       int
	batchTimer        watcherTimer
	batchTimerFactory watcherTimerFactory
	pendingBatch      map[string]struct{}
	batchRetry        map[string]int
	reindexFileFn     func(string) error
}

const (
	maxImmediateBatchRetries = 3
	maxDeferredBatchRetries  = 1
	maxBatchRetryDelay       = 30 * time.Second
)

// NewFileWatcher creates a new file watcher for a workspace.
func NewFileWatcher(registration model.WorkspaceRegistration, debounceDur time.Duration) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if debounceDur <= 0 {
		debounceDur = 500 * time.Millisecond
	}

	maxDirs := parseMaxDirsEnv("MI_LSP_WATCHER_MAX_DIRS", 10000)

	fw := &FileWatcher{
		workspaceRoot:     registration.Root,
		registration:      registration,
		watcher:           watcher,
		debounce:          make(map[string]*time.Timer),
		debounceDur:       debounceDur,
		maxWatchedDirs:    maxDirs,
		stopCh:            make(chan struct{}),
		verbose:           os.Getenv("MI_LSP_VERBOSE") != "",
		batchTimerFactory: func(delay time.Duration, fn func()) watcherTimer { return time.AfterFunc(delay, fn) },
		pendingBatch:      make(map[string]struct{}),
		batchRetry:        make(map[string]int),
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

	// Cancel all pending debounce timers and batch timer
	fw.mu.Lock()
	for filePath, timer := range fw.debounce {
		timer.Stop()
		delete(fw.debounce, filePath)
	}
	if fw.batchTimer != nil {
		fw.batchTimer.Stop()
		fw.batchTimer = nil
	}
	fw.pendingBatch = make(map[string]struct{})
	fw.batchRetry = make(map[string]int)
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
					fw.scheduleBatchReindex(event.Name)
				}
			}
			if event.Op&fsnotify.Create != 0 {
				// Watch new directories respecting max cap
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() && !shouldSkipDir(event.Name) {
					fw.mu.Lock()
					if fw.watchedDirs < fw.maxWatchedDirs {
						if err := fw.watcher.Add(event.Name); err == nil {
							fw.watchedDirs++
						}
					} else if fw.verbose {
						log.Printf("[mi-lsp:watcher] reached max watched dirs (%d), not adding %s", fw.maxWatchedDirs, event.Name)
					}
					fw.mu.Unlock()
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

// scheduleBatchReindex batches file changes into a coalesced window instead of
// per-file timers. This reduces redundant re-indexing during rapid file changes.
func (fw *FileWatcher) scheduleBatchReindex(filePath string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// A new filesystem event explicitly starts a fresh retry cycle for this file.
	// This also clears the fail-closed marker left by an exhausted deferred retry.
	delete(fw.batchRetry, filePath)
	fw.pendingBatch[filePath] = struct{}{}

	// If batch timer already running, do nothing; it will reindex all pending files
	if fw.batchTimer != nil {
		return
	}

	// Start a new batch timer window.
	fw.batchTimer = fw.newBatchTimer(fw.debounceDur)
}

func (fw *FileWatcher) newBatchTimer(delay time.Duration) watcherTimer {
	if fw.batchTimerFactory != nil {
		return fw.batchTimerFactory(delay, fw.flushBatch)
	}
	return time.AfterFunc(delay, fw.flushBatch)
}

func (fw *FileWatcher) flushBatch() {
	if fw.isStopped() {
		return
	}

	fw.mu.Lock()
	batch := fw.pendingBatch
	fw.pendingBatch = make(map[string]struct{})
	fw.batchTimer = nil
	fw.mu.Unlock()

	var deferred []string
	for filePath := range batch {
		err := fw.reindexBatchFile(filePath)
		var blocked *store.IndexLockError
		if errors.As(err, &blocked) {
			deferred = append(deferred, filePath)
			continue
		}
		fw.resetBatchRetry(filePath)
	}
	fw.deferBatchRetry(deferred)
}

// reindexBatchFile performs the initial operation and at most three immediate
// retries in the same batch. A lock contention that survives those attempts is
// returned to flushBatch for one coalesced deferred retry.
func (fw *FileWatcher) reindexBatchFile(filePath string) error {
	reindex := fw.reindexFile
	if fw.reindexFileFn != nil {
		reindex = fw.reindexFileFn
	}

	var err error
	for attempt := 0; attempt <= maxImmediateBatchRetries; attempt++ {
		err = reindex(filePath)
		var blocked *store.IndexLockError
		if !errors.As(err, &blocked) || attempt == maxImmediateBatchRetries {
			return err
		}
	}
	return err
}

func (fw *FileWatcher) deferBatchRetry(filePaths []string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.isStoppedLocked() {
		return
	}

	maxRound := 0
	deferredFiles := 0
	for _, filePath := range filePaths {
		fw.pendingBatch[filePath] = struct{}{}
		round := fw.batchRetry[filePath] + 1
		fw.batchRetry[filePath] = round
		if round > maxDeferredBatchRetries {
			// Keep the event pending, but fail closed until a new filesystem
			// event resets this marker. Never create an unbounded timer chain.
			continue
		}
		deferredFiles++
		if round > maxRound {
			maxRound = round
		}
	}
	if deferredFiles == 0 || fw.batchTimer != nil {
		return
	}

	delay := batchRetryDelay(fw.debounceDur, maxRound)
	fw.batchTimer = fw.newBatchTimer(delay)
	if fw.verbose {
		log.Printf("[mi-lsp:watcher] deferred batch retry files=%d round=%d delay=%s", deferredFiles, maxRound, delay)
	}
}

func (fw *FileWatcher) resetBatchRetry(filePath string) {
	fw.mu.Lock()
	delete(fw.batchRetry, filePath)
	fw.mu.Unlock()
}

func (fw *FileWatcher) isStopped() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.isStoppedLocked()
}

func (fw *FileWatcher) isStoppedLocked() bool {
	if fw.stopCh == nil {
		return false
	}
	select {
	case <-fw.stopCh:
		return true
	default:
		return false
	}
}

func batchRetryDelay(debounce time.Duration, round int) time.Duration {
	if debounce <= 0 {
		debounce = time.Millisecond
	}
	if round < 1 {
		round = 1
	}
	factor := time.Duration(1 << min(round-1, 4))
	delay := debounce * factor
	if delay <= 0 || delay > maxBatchRetryDelay {
		return maxBatchRetryDelay
	}
	return delay
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func (fw *FileWatcher) reindexFile(absPath string) error {
	defer func() {
		if r := recover(); r != nil {
			if fw.verbose {
				log.Printf("[mi-lsp:watcher] recovered panic reindexing %s: %v", absPath, r)
			}
		}
	}()

	// The watcher has no index-job ownership capability. Route the event through
	// the foreground incremental publisher instead of mutating files/symbols
	// directly: that path couples catalog rows with generation metadata and graph
	// state in one transaction under the same workspace lock.
	result, err := indexer.IncrementalIndex(context.Background(), fw.workspaceRoot)
	if err != nil {
		if fw.verbose {
			log.Printf("[mi-lsp:watcher] incremental update error for %s: %v", absPath, err)
		}
		return err
	}
	if fw.verbose {
		log.Printf("[mi-lsp:watcher] reindexed %s: %d symbols", absPath, result.Stats.Symbols)
	}
	fw.mu.Lock()
	delete(fw.batchRetry, absPath)
	fw.mu.Unlock()
	return nil
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
			fw.mu.Lock()
			if fw.watchedDirs >= fw.maxWatchedDirs {
				fw.mu.Unlock()
				if fw.verbose {
					log.Printf("[mi-lsp:watcher] reached max watched dirs (%d) during recursive add", fw.maxWatchedDirs)
				}
				return filepath.SkipDir
			}
			fw.mu.Unlock()

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

func parseMaxDirsEnv(envName string, defaultVal int) int {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return defaultVal
	}
	return val
}
