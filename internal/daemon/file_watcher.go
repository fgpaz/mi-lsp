package daemon

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/language"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fsnotify/fsnotify"
)

type watcherTimer interface {
	Stop() bool
}

type watcherTimerFactory func(time.Duration, func()) watcherTimer

type watchDomain string

const (
	watchDomainCode             watchDomain = "code"
	watchDomainDocs             watchDomain = "docs"
	watchDomainAuthorityConfig watchDomain = "authority_config"
)

type watcherEvent struct {
	path string
	op   fsnotify.Op
}

type watcherDiagnostic struct {
	error     string
	attempts  int
	exhausted bool
}

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
	pendingOps        map[string]fsnotify.Op
	batchRetry        map[string]int
	batchDiagnostics  map[string]watcherDiagnostic
	lostEvents        bool
	reindexFileFn     func(string) error
	reindexBatchFn    func([]watcherEvent) error
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
	workspaceRoot := registration.Root
	if strings.TrimSpace(workspaceRoot) != "" {
		if absRoot, absErr := filepath.Abs(workspaceRoot); absErr == nil {
			workspaceRoot = filepath.Clean(absRoot)
			registration.Root = workspaceRoot
		}
	}

	fw := &FileWatcher{
		workspaceRoot:     workspaceRoot,
		registration:      registration,
		watcher:           watcher,
		debounce:          make(map[string]*time.Timer),
		debounceDur:       debounceDur,
		maxWatchedDirs:    maxDirs,
		stopCh:            make(chan struct{}),
		verbose:           os.Getenv("MI_LSP_VERBOSE") != "",
		batchTimerFactory: func(delay time.Duration, fn func()) watcherTimer { return time.AfterFunc(delay, fn) },
		pendingBatch:      make(map[string]struct{}),
		pendingOps:        make(map[string]fsnotify.Op),
		batchRetry:        make(map[string]int),
		batchDiagnostics:  make(map[string]watcherDiagnostic),
	}
	fw.reindexBatchFn = fw.reindexWatchBatch

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
	fw.pendingOps = make(map[string]fsnotify.Op)
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
				fw.recordLostEvent("event stream closed")
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				if isWatchableFileForRoot(fw.workspaceRoot, event.Name) {
					fw.scheduleBatchEvent(event)
				}
			}
			if event.Op&fsnotify.Create != 0 {
				// Watch new directories respecting max cap and canonical exclusions.
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() && !shouldSkipWatchDir(fw.workspaceRoot, event.Name) {
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
				fw.recordLostEvent("error stream closed")
				return
			}
			fw.recordLostEvent(errString(err))
			if fw.verbose {
				log.Printf("[mi-lsp:watcher] error: %v", err)
			}
		}
	}
}

func (fw *FileWatcher) PendingEvents() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return len(fw.debounce) + len(fw.pendingBatch)
}

func (fw *FileWatcher) WatchedDirCount() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.watchedDirs
}

func (fw *FileWatcher) scheduleReindex(filePath string) {
	if !isWatchableFileForRoot(fw.workspaceRoot, filePath) {
		return
	}
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
	fw.scheduleBatchEvent(fsnotify.Event{Name: filePath, Op: fsnotify.Write})
}

func (fw *FileWatcher) scheduleBatchEvent(event fsnotify.Event) {
	filePath := strings.TrimSpace(strings.ReplaceAll(event.Name, "\\", "/"))
	filePath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(filePath)))
	if filePath == "" || filePath == "." || !isWatchableFileForRoot(fw.workspaceRoot, filePath) {
		return
	}
	op := event.Op & (fsnotify.Write | fsnotify.Create | fsnotify.Remove | fsnotify.Rename)
	if op == 0 {
		op = fsnotify.Write
	}

	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.pendingBatch == nil {
		fw.pendingBatch = make(map[string]struct{})
	}
	if fw.pendingOps == nil {
		fw.pendingOps = make(map[string]fsnotify.Op)
	}
	if fw.batchRetry == nil {
		fw.batchRetry = make(map[string]int)
	}
	if fw.batchDiagnostics == nil {
		fw.batchDiagnostics = make(map[string]watcherDiagnostic)
	}

	// A new filesystem event explicitly starts a fresh retry cycle for this file.
	// This also clears the fail-closed marker left by an exhausted deferred retry.
	delete(fw.batchRetry, filePath)
	delete(fw.batchDiagnostics, filePath)
	fw.pendingBatch[filePath] = struct{}{}
	fw.pendingOps[filePath] |= op

	// If batch timer already running, do nothing; it will reindex all pending files.
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
	pendingOps := fw.pendingOps
	fw.pendingBatch = make(map[string]struct{})
	fw.pendingOps = make(map[string]fsnotify.Op)
	fw.batchTimer = nil
	fw.mu.Unlock()

	paths := make([]string, 0, len(batch))
	events := make([]watcherEvent, 0, len(batch))
	for filePath := range batch {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	for _, filePath := range paths {
		op := pendingOps[filePath]
		if op == 0 {
			op = fsnotify.Write
		}
		events = append(events, watcherEvent{path: filePath, op: op})
	}

	// Production watchers route a coalesced batch once per domain. Test and
	// compatibility callers that provide reindexFileFn retain per-path retry
	// behavior and diagnostics.
	if fw.reindexBatchFn != nil && fw.reindexFileFn == nil {
		err := fw.reindexBatchWithRetries(events)
		if err == nil {
			for _, event := range events {
				fw.resetBatchRetry(event.path)
			}
			return
		}
		var blocked *store.IndexLockError
		deferred := make([]watcherEvent, 0, len(events))
		for _, event := range events {
			fw.recordBatchFailure(event, err)
			if errors.As(err, &blocked) {
				deferred = append(deferred, event)
			} else {
				// Non-lock failures remain pending with a diagnostic. A fresh
				// filesystem event re-arms the cycle; failed work is never
				// treated as current.
				fw.retainBatchFailure(event)
			}
		}
		fw.deferBatchRetryEvents(deferred)
		return
	}

	deferred := make([]watcherEvent, 0, len(events))
	for _, event := range events {
		err := fw.reindexBatchEvent(event)
		if err == nil {
			fw.resetBatchRetry(event.path)
			continue
		}

		var blocked *store.IndexLockError
		fw.recordBatchFailure(event, err)
		if errors.As(err, &blocked) {
			deferred = append(deferred, event)
			continue
		}
		// Non-lock failures remain pending with a diagnostic. A fresh filesystem
		// event re-arms the cycle; failed work is never treated as current.
		fw.retainBatchFailure(event)
	}
	fw.deferBatchRetryEvents(deferred)
}

func (fw *FileWatcher) reindexBatchWithRetries(events []watcherEvent) error {
	if fw.reindexBatchFn == nil {
		return fw.reindexWatchBatch(events)
	}
	var err error
	for attempt := 0; attempt <= maxImmediateBatchRetries; attempt++ {
		err = fw.reindexBatchFn(events)
		var blocked *store.IndexLockError
		if !errors.As(err, &blocked) || attempt == maxImmediateBatchRetries {
			return err
		}
	}
	return err
}

func (fw *FileWatcher) reindexWatchBatch(events []watcherEvent) error {
	if len(events) == 0 {
		return nil
	}
	codeChanged := make([]string, 0)
	codeDeleted := make([]string, 0)
	docsChanged := make([]string, 0)
	docsDeleted := make([]string, 0)
	authorityChanged := make([]string, 0)
	authorityDeleted := make([]string, 0)
	hasAuthority := false
	for _, event := range events {
		switch classifyWatchPath(fw.workspaceRoot, event.path) {
		case watchDomainAuthorityConfig:
			hasAuthority = true
			changed, deleted := watcherEventPaths(event)
			authorityChanged = append(authorityChanged, changed...)
			authorityDeleted = append(authorityDeleted, deleted...)
		case watchDomainDocs:
			changed, deleted := watcherEventPaths(event)
			docsChanged = append(docsChanged, changed...)
			docsDeleted = append(docsDeleted, deleted...)
		case watchDomainCode:
			changed, deleted := watcherEventPaths(event)
			codeChanged = append(codeChanged, changed...)
			codeDeleted = append(codeDeleted, deleted...)
		}
	}

	ctx := context.Background()
	if hasAuthority {
		// T4 exposes unbounded authority fanout as an explicit invalidation
		// error. Keep the whole mixed batch fail-closed; no domain is stamped
		// current until the owner performs a bounded full rebuild.
		allChanged := append(append(append([]string(nil), authorityChanged...), docsChanged...), codeChanged...)
		allDeleted := append(append(append([]string(nil), authorityDeleted...), docsDeleted...), codeDeleted...)
		_, err := indexer.IncrementalIndexWithPaths(ctx, fw.workspaceRoot, allChanged, allDeleted)
		return err
	}

	// Keep mixed code+document mutations in one T4 publication. Splitting the
	// calls would let the later code pass repair/publish a graph from a state
	// that T4 intentionally leaves stale after a document mutation.
	allChanged := append(append([]string(nil), docsChanged...), codeChanged...)
	allDeleted := append(append([]string(nil), docsDeleted...), codeDeleted...)
	if len(allChanged) == 0 && len(allDeleted) == 0 {
		return nil
	}
	_, err := indexer.IncrementalIndexWithPaths(ctx, fw.workspaceRoot, allChanged, allDeleted)
	return err
}

// reindexBatchFile performs the initial operation and at most three immediate
// retries in the same batch. A lock contention that survives those attempts is
// returned to flushBatch for one coalesced deferred retry.
func (fw *FileWatcher) reindexBatchFile(filePath string) error {
	return fw.reindexBatchEvent(watcherEvent{path: filePath, op: fsnotify.Write})
}

func (fw *FileWatcher) reindexBatchEvent(event watcherEvent) error {
	reindex := func(string) error {
		return fw.reindexWatchEvent(event)
	}
	if fw.reindexFileFn != nil {
		reindex = fw.reindexFileFn
	}

	var err error
	for attempt := 0; attempt <= maxImmediateBatchRetries; attempt++ {
		err = reindex(event.path)
		var blocked *store.IndexLockError
		if !errors.As(err, &blocked) || attempt == maxImmediateBatchRetries {
			return err
		}
	}
	return err
}

func (fw *FileWatcher) deferBatchRetry(filePaths []string) {
	events := make([]watcherEvent, 0, len(filePaths))
	for _, filePath := range filePaths {
		events = append(events, watcherEvent{path: filePath, op: fsnotify.Write})
	}
	fw.deferBatchRetryEvents(events)
}

func (fw *FileWatcher) deferBatchRetryEvents(events []watcherEvent) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.isStoppedLocked() {
		return
	}
	if fw.pendingBatch == nil {
		fw.pendingBatch = make(map[string]struct{})
	}
	if fw.pendingOps == nil {
		fw.pendingOps = make(map[string]fsnotify.Op)
	}
	if fw.batchRetry == nil {
		fw.batchRetry = make(map[string]int)
	}

	maxRound := 0
	deferredFiles := 0
	for _, event := range events {
		if event.path == "" {
			continue
		}
		fw.pendingBatch[event.path] = struct{}{}
		op := event.op
		if op == 0 {
			op = fsnotify.Write
		}
		fw.pendingOps[event.path] |= op
		round := fw.batchRetry[event.path] + 1
		fw.batchRetry[event.path] = round
		fw.updateDiagnosticLocked(event.path, watcherDiagnostic{attempts: round, exhausted: round > maxDeferredBatchRetries})
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
	delete(fw.batchDiagnostics, filePath)
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

func (fw *FileWatcher) recordLostEvent(message string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.isStoppedLocked() {
		return
	}
	fw.lostEvents = true
	fw.updateDiagnosticLocked("<watcher>", watcherDiagnostic{error: "lost_event: " + message, exhausted: true})
}

func (fw *FileWatcher) LostEvents() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.lostEvents
}

func (fw *FileWatcher) PendingDiagnostics() map[string]string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	result := make(map[string]string, len(fw.batchDiagnostics))
	for path, diagnostic := range fw.batchDiagnostics {
		message := diagnostic.error
		if message == "" {
			message = "pending"
		}
		if diagnostic.exhausted {
			message += " (exhausted; remains pending)"
		}
		result[path] = message
	}
	return result
}

func errString(err error) string {
	if err == nil {
		return "watcher event loss"
	}
	return err.Error()
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

func (fw *FileWatcher) reindexFile(absPath string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("watcher reindex panic for %s: %v", absPath, r)
			if fw.verbose {
				log.Printf("[mi-lsp:watcher] recovered panic reindexing %s: %v", absPath, r)
			}
		}
	}()
	return fw.reindexWatchEvent(watcherEvent{path: absPath, op: fsnotify.Write})
}

func (fw *FileWatcher) reindexWatchEvent(event watcherEvent) error {
	if event.path == "" {
		return fmt.Errorf("watcher event has no path")
	}
	domain := classifyWatchPath(fw.workspaceRoot, event.path)
	if domain == "" {
		return nil
	}

	ctx := context.Background()
	switch domain {
	case watchDomainAuthorityConfig:
		// T4 intentionally exposes config fanout as a fail-closed invalidation
		// path. Do not call the broad docs-only publisher here: a partial parse
		// must not advance a docs generation or make the watcher a freshness
		// oracle. The foreground/full-index owner can repair this safely.
		changed, deleted := watcherEventPaths(event)
		_, err := indexer.IncrementalIndexWithPaths(ctx, fw.workspaceRoot, changed, deleted)
		if err != nil {
			if fw.verbose {
				log.Printf("[mi-lsp:watcher] authority invalidation for %s: %v", event.path, err)
			}
			return err
		}
		return nil
	case watchDomainDocs:
		changed, deleted := watcherEventPaths(event)
		result, err := indexer.IncrementalIndexWithPaths(ctx, fw.workspaceRoot, changed, deleted)
		if err != nil {
			if fw.verbose {
				log.Printf("[mi-lsp:watcher] docs incremental update error for %s: %v", event.path, err)
			}
			return err
		}
		if fw.verbose {
			log.Printf("[mi-lsp:watcher] docs refresh %s: %d docs", event.path, result.Docs)
		}
		return nil
	case watchDomainCode:
		changed, deleted := watcherEventPaths(event)
		result, err := indexer.IncrementalIndexWithPaths(ctx, fw.workspaceRoot, changed, deleted)
		if err != nil {
			if fw.verbose {
				log.Printf("[mi-lsp:watcher] code incremental update error for %s: %v", event.path, err)
			}
			return err
		}
		if fw.verbose {
			log.Printf("[mi-lsp:watcher] code refresh %s: %d symbols", event.path, result.Stats.Symbols)
		}
		return nil
	default:
		return nil
	}
}

func watcherEventPaths(event watcherEvent) (changed, deleted []string) {
	op := event.op
	if op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		deleted = append(deleted, event.path)
	}
	if op&(fsnotify.Write|fsnotify.Create) != 0 || op&(fsnotify.Remove|fsnotify.Rename) == 0 {
		changed = append(changed, event.path)
	}
	return changed, deleted
}

func (fw *FileWatcher) recordBatchFailure(event watcherEvent, err error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	attempts := fw.batchRetry[event.path]
	fw.updateDiagnosticLocked(event.path, watcherDiagnostic{error: errString(err), attempts: attempts})
}

func (fw *FileWatcher) retainBatchFailure(event watcherEvent) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.pendingBatch == nil {
		fw.pendingBatch = make(map[string]struct{})
	}
	if fw.pendingOps == nil {
		fw.pendingOps = make(map[string]fsnotify.Op)
	}
	fw.pendingBatch[event.path] = struct{}{}
	fw.pendingOps[event.path] |= event.op
}

func (fw *FileWatcher) updateDiagnosticLocked(path string, diagnostic watcherDiagnostic) {
	if fw.batchDiagnostics == nil {
		fw.batchDiagnostics = make(map[string]watcherDiagnostic)
	}
	current := fw.batchDiagnostics[path]
	if diagnostic.error == "" {
		diagnostic.error = current.error
	}
	if diagnostic.attempts < current.attempts {
		diagnostic.attempts = current.attempts
	}
	diagnostic.exhausted = diagnostic.exhausted || current.exhausted
	fw.batchDiagnostics[path] = diagnostic
}

func (fw *FileWatcher) addWatchRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			if shouldSkipWatchDir(fw.workspaceRoot, path) {
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

var watchableExtensions = supportedWatchableExtensions()

func supportedWatchableExtensions() map[string]struct{} {
	extensions := language.Extensions()
	result := make(map[string]struct{}, len(extensions))
	for _, extension := range extensions {
		result[extension] = struct{}{}
	}
	return result
}

func isWatchableFile(path string) bool {
	return isWatchableFileForRoot("", path)
}

func isWatchableFileForRoot(root, path string) bool {
	return classifyWatchPath(root, path) != ""
}

func classifyWatchPath(root, path string) watchDomain {
	path = normalizeWatchPath(root, path)
	if path == "" || isExcludedWatchPath(path) {
		return ""
	}
	if indexer.IsAuthorityConfigPath(path) {
		return watchDomainAuthorityConfig
	}
	if language.IsSupportedCodePath(path) {
		return watchDomainCode
	}
	if isCanonicalMarkdownWatchPath(path) {
		return watchDomainDocs
	}
	return ""
}

func normalizeWatchPath(root, path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || strings.ContainsRune(path, 0) {
		return ""
	}
	wasAbsolute := filepath.IsAbs(filepath.FromSlash(path))
	if root != "" {
		absRoot, rootErr := filepath.Abs(root)
		absPath, pathErr := filepath.Abs(filepath.FromSlash(path))
		if rootErr == nil && pathErr == nil {
			if rel, relErr := filepath.Rel(filepath.Clean(absRoot), filepath.Clean(absPath)); relErr == nil {
				outside := filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
				if outside && wasAbsolute {
					return ""
				}
				if !outside {
					path = rel
				}
			}
		}
	}
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return strings.ToLower(path)
}

func isExcludedWatchPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for index, part := range parts {
		if _, skip := skipDirs[part]; skip {
			return true
		}
		if part == ".docs" && index+1 < len(parts) {
			switch parts[index+1] {
			case "raw", "auditoria", "temp":
				return true
			}
		}
	}
	return false
}

func isCanonicalMarkdownWatchPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	if !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".mdown") && !strings.HasSuffix(lower, ".markdown") {
		return false
	}
	for _, prefix := range []string{".docs/", "wiki/", "docs/", "bibliotecas/"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	base := strings.ToLower(filepath.Base(lower))
	return strings.HasPrefix(base, "readme")
}

func shouldSkipWatchDir(root, dirPath string) bool {
	return shouldSkipDir(dirPath) || isExcludedWatchPath(normalizeWatchPath(root, dirPath))
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
