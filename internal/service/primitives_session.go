package service

import (
	"sort"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const sessionLimit = 200
const sessionIdle = 30 * time.Minute

type sessionEntry struct {
	mark       int64
	touched    time.Time
	delivered  map[string]string
	history    []sessionRevision
	snapshots  map[int64]map[string]string
	dedupe     map[string]map[string]string
	dedupeKeys []string
}
type sessionRevision struct {
	mark int64
	id   string
	rev  string
}

// SessionState is process-local q-v1 memory. It stores identities and revisions,
// never source content or raw queries.
type SessionState struct {
	mu      sync.Mutex
	entries map[string]*sessionEntry
}

func NewSessionState() *SessionState { return &SessionState{entries: map[string]*sessionEntry{}} }
func (s *SessionState) entry(session string, now time.Time) *sessionEntry {
	if s.entries == nil {
		s.entries = map[string]*sessionEntry{}
	}
	for id, entry := range s.entries {
		if now.Sub(entry.touched) > sessionIdle {
			delete(s.entries, id)
		}
	}
	entry := s.entries[session]
	if entry == nil {
		entry = &sessionEntry{delivered: map[string]string{}, snapshots: map[int64]map[string]string{}}
		s.entries[session] = entry
	}
	entry.touched = now
	if len(s.entries) > sessionLimit {
		ids := make([]string, 0, len(s.entries))
		for id := range s.entries {
			if id != session {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return s.entries[ids[i]].touched.Before(s.entries[ids[j]].touched) })
		for len(s.entries) > sessionLimit && len(ids) > 0 {
			delete(s.entries, ids[0])
			ids = ids[1:]
		}
	}
	return entry
}
func (s *SessionState) NextMark(session string) int64 {
	if s == nil || session == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entry(session, time.Now())
	entry.mark++
	snapshot := make(map[string]string, len(entry.delivered))
	for id, rev := range entry.delivered {
		snapshot[id] = rev
	}
	entry.snapshots[entry.mark] = snapshot
	for mark := range entry.snapshots {
		if mark < entry.mark-64 {
			delete(entry.snapshots, mark)
		}
	}
	return entry.mark
}
func (s *SessionState) Seen(session, id, rev string) bool {
	if s == nil || session == "" || id == "" || rev == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entry(session, time.Now())
	return entry.delivered[id] == rev
}
func (s *SessionState) Remember(session string, item model.QItem) {
	if s == nil || session == "" || item.ID == "" || item.Rev() == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entry(session, time.Now())
	rev := item.Rev()
	if previous, ok := entry.delivered[item.ID]; !ok || previous != rev {
		entry.mark++
		entry.delivered[item.ID] = rev
		entry.history = append(entry.history, sessionRevision{mark: entry.mark, id: item.ID, rev: rev})
		if len(entry.history) > 4096 {
			entry.history = append([]sessionRevision(nil), entry.history[len(entry.history)-2048:]...)
		}
	}
}
func (s *SessionState) ChangedCandidates(session string, since int64) []model.QItem {
	if s == nil || session == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entry(session, time.Now())
	snapshot := entry.snapshots[since]
	out := make([]model.QItem, 0, len(snapshot))
	for id, rev := range snapshot {
		out = append(out, model.QItem{ID: id, Revision: rev, Origin: model.ItemOriginCatalog})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (s *SessionState) FilterDedupe(session, key string, items []model.QItem) []model.QItem {
	if s == nil || session == "" || key == "" {
		return items
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entry(session, time.Now())
	known := entry.dedupe[key]
	out := items[:0]
	for _, item := range items {
		if known == nil || known[item.ID] != item.Rev() {
			out = append(out, item)
		}
	}
	return out
}
func (s *SessionState) RememberDedupe(session, key string, items []model.QItem) {
	if s == nil || session == "" || key == "" || len(items) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entry(session, time.Now())
	if entry.dedupe == nil {
		entry.dedupe = map[string]map[string]string{}
	}
	known := entry.dedupe[key]
	if known == nil {
		if len(entry.dedupeKeys) >= 32 {
			delete(entry.dedupe, entry.dedupeKeys[0])
			entry.dedupeKeys = entry.dedupeKeys[1:]
		}
		entry.dedupeKeys = append(entry.dedupeKeys, key)
		known = map[string]string{}
		entry.dedupe[key] = known
	}
	for _, item := range items {
		if item.ID != "" {
			if _, exists := known[item.ID]; !exists && len(known) >= 4096 {
				for id := range known {
					delete(known, id)
					break
				}
			}
			known[item.ID] = item.Rev()
		}
	}
}

func (s *SessionState) Changed(session string, since int64) []model.QItem {
	if s == nil || session == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entry(session, time.Now())
	latest := map[string]model.QItem{}
	for _, item := range entry.history {
		if item.mark > since {
			latest[item.id] = model.QItem{ID: item.id, Revision: item.rev, Origin: model.ItemOriginCatalog}
		}
	}
	out := make([]model.QItem, 0, len(latest))
	for _, item := range latest {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
