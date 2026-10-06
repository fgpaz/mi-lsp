package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/query"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const qDefaultTimeout = 10 * time.Second

type qExecution struct {
	items      []model.QItem
	reason     string
	fallback   string
	generation string
	truncated  bool
}

// ExecuteQ runs the q-v1 pipeline over the existing read-only service APIs.
func (a *App) ExecuteQ(ctx context.Context, request model.CommandRequest, sessions *SessionState) (model.Envelope, error) {
	started := time.Now()
	if request.Payload == nil {
		request.Payload = map[string]any{}
	}
	pipelineText := stringPayload(request.Payload, "q")
	pipeline, parseErr := query.Parse(pipelineText)
	if parseErr != nil {
		return qFailureFor("stage_failed", "parse", parseErr, request.Context.Workspace), nil
	}
	if pipeline.Workspace != "" {
		request.Context.Workspace = pipeline.Workspace
	}
	if pipeline.Fresh {
		request.Payload["fresh"] = true
	}
	if session := stringPayload(request.Payload, "session_id"); session != "" {
		request.Context.SessionID = session
	}
	if request.Context.SessionID == "" {
		request.Context.SessionID = "q-" + strconv.Itoa(os.Getpid())
	}
	maxBytes := pipeline.MaxBytes
	if raw, ok := request.Payload["max_bytes"]; ok {
		maxBytes = intFromAny(raw, 0)
	}
	if maxBytes < 1024 {
		return qFailureFor("stage_failed", "options", errors.New("max_bytes must be at least 1024"), request.Context.Workspace), nil
	}
	budget := pipeline.Budget
	if raw, ok := request.Payload["budget"]; ok {
		budget = intFromAny(raw, 0)
	}
	if budget < 1 || budget > 12000 {
		return qFailureFor("stage_failed", "options", errors.New("budget must be between 1 and 12000"), request.Context.Workspace), nil
	}
	timeout := qDefaultTimeout
	if raw, ok := request.Payload["timeout_ms"]; ok {
		value := intFromAny(raw, 0)
		if value < 1 {
			return qFailureFor("stage_failed", "options", errors.New("timeout_ms must be between 1 and 30000"), request.Context.Workspace), nil
		}
		if value > 30000 {
			value = 30000
		}
		timeout = time.Duration(value) * time.Millisecond
	}
	qctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	registration, _, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return qFailureFor("stage_failed", "workspace", err, request.Context.Workspace), nil
	}
	wid := model.WorkspaceID(registration.Root)
	request.Context.Workspace = registration.Name
	request.Context.NoAutoRegister = true
	request.Context.MaxItems = max(request.Context.MaxItems, 200)
	generation, _ := store.ReadWorkspaceGenerationSnapshot(qctx, registration.Root)
	pageToken := stringPayload(request.Payload, "page")
	if pageToken == "" {
		pageToken = pipeline.Page
	}
	pageOffset := 0
	if pageToken != "" {
		claims, code := verifyQCursor(pageToken, pipelineText, wid)
		if code != "" {
			return qFailureFor(code, "cursor", errors.New(code), registration.Name), nil
		}
		if claims.Generation != generation {
			return qFailureFor("snapshot_changed", "cursor", errors.New("snapshot_changed"), registration.Name), nil
		}
		pageOffset = claims.Offset
	}
	items := []model.QItem{}
	stages := []model.QStage{}
	firstFailure := ""
	fallbackUsed := ""
	explicitSort := false
	for _, stage := range pipeline.Stages {
		if qctx.Err() != nil {
			firstFailure = qContextCode(qctx.Err())
			break
		}
		before := len(items)
		markStart := time.Now()
		var result qExecution
		switch stage.Verb {
		case "sym":
			result = a.qSym(qctx, request, stage, wid, registration.Root)
		case "text":
			result = a.qText(qctx, request, stage, wid, registration.Root)
		case "docs":
			if len(stage.Args) > 0 {
				result = a.qDocs(qctx, request, stage, wid, registration.Root)
			} else {
				result = a.qDocsForItems(qctx, request, items, wid, registration.Root)
			}
		case "id":
			result = a.qIDs(qctx, request, stage, wid, registration.Root)
		case "diff":
			result = a.qDiff(qctx, request, stage, wid, registration.Root)
		case "changed":
			result = a.qChanged(qctx, request, stage, sessions, wid, registration.Root)
		case "edges":
			result = a.qEdges(qctx, request, stage, items, wid)
		case "read":
			result = a.qRead(qctx, request, stage, items, sessions, wid, registration.Root)
		case "where":
			items = qWhere(items, stage)
			result.items = items
		case "limit":
			items = qLimit(items, stage)
			result.items = items
		case "uniq":
			items = qUniq(items)
			result.items = items
		case "sort":
			items = qSort(items, stage)
			result.items = items
			explicitSort = true
		case "fields":
			if len(stage.Args) > 0 {
				pipeline.Fields = splitQFields(stage.Args[0])
			}
			result.items = items
		case "count":
			result.items = []model.QItem{{Kind: "count", Name: "count", Score: len(items), Origin: "q"}}
		case "describe":
			text := query.Describe(firstArg(stage))
			result.items = []model.QItem{{Kind: "description", Name: "q-v1", Text: text, Origin: "q"}}
		default:
			result.reason = "stage_failed"
		}
		if result.reason == "" {
			items = result.items
		}
		if result.fallback != "" {
			fallbackUsed = result.fallback
		}
		if result.reason != "" && firstFailure == "" {
			firstFailure = qPublicReason(result.reason)
		}
		stageReason := qPublicReason(result.reason)
		stages = append(stages, model.QStage{Verb: stage.Verb, In: before, Out: len(items), Ms: time.Since(markStart).Milliseconds(), Truncated: result.truncated, Reason: stageReason})
	}
	if qctx.Err() != nil {
		code := qContextCode(qctx.Err())
		if code == "cancelled" || firstFailure != "cancelled" {
			firstFailure = code
		}
	}
	if !explicitSort {
		items = qSort(items, query.Stage{Verb: "sort", Args: []string{"file"}})
	}
	dedupeEnabled, _ := request.Payload["dedupe"].(bool)
	queryDigest := sha256.Sum256([]byte(pipelineText))
	dedupeKey := wid + "|" + generation + "|" + hex.EncodeToString(queryDigest[:])
	if pageOffset > len(items) {
		return qFailureFor("cursor_invalid", "cursor", errors.New("cursor offset outside result"), registration.Name), nil
	}
	items = items[pageOffset:]
	if dedupeEnabled && sessions != nil {
		items = sessions.FilterDedupe(request.Context.SessionID, dedupeKey, items)
	}
	mark := int64(0)
	if sessions != nil {
		mark = sessions.NextMark(request.Context.SessionID)
	}
	rows := make(model.QItems, 0, len(items))
	used := 0
	truncated := false
	for _, item := range items {
		row := item.Project(pipeline.Fields).With("sensibilidad", frontmatterSensibilidad(registration.Root, item.File))
		itemBytes := row.JSONSize()
		if used+itemBytes > budget*4 {
			truncated = true
			break
		}
		used += itemBytes
		rows = append(rows, row)
	}
	env := model.Envelope{ContractVersion: "q-v1", Operation: "q", Ok: true, Workspace: registration.Name, Backend: "q", Items: rows, Stages: stages, Budget: &model.QBudget{Requested: budget, Used: (used + 3) / 4}, SessionMark: mark, Truncated: truncated, GenerationID: generation}
	env.FallbackUsed = fallbackUsed
	if firstFailure != "" {
		env.Partial = len(rows) > 0
		env.Ok = env.Partial
		env.Reason = firstFailure
		env.Error = &model.EnvelopeError{Kind: "q", Code: firstFailure, ReasonCode: firstFailure, Message: firstFailure, Stage: "execute", Detail: "q pipeline ended with a typed failure"}
	}
	nextOffset := pageOffset + len(rows)
	if truncated {
		env.Continuation = qContinuation(pipelineText, registration.Name, wid, generation, nextOffset)
	}
	if elapsed := time.Since(started); elapsed > timeout && env.Reason != "cancelled" {
		env.Partial = len(rows) > 0
		env.Reason = "timeout"
		env.Ok = env.Partial
		env.Error = &model.EnvelopeError{Kind: "q", Code: "timeout", ReasonCode: "timeout", Message: "timeout", Stage: "execute"}
	}
	if maxBytes > 0 {
		env = qBoundEnvelope(env, items, pipeline.Fields, maxBytes, pipelineText, wid, registration.Name, generation, pageOffset)
	}
	if env.Budget != nil {
		if output, ok := env.Items.(model.QItems); ok {
			total := 0
			for _, row := range output {
				total += row.JSONSize()
			}
			env.Budget.Used = (total + 3) / 4
		}
	}
	if dedupeEnabled && sessions != nil && env.Ok && !env.Partial {
		byID := map[string]model.QItem{}
		for _, item := range items {
			byID[item.ID] = item
		}
		if output, ok := env.Items.(model.QItems); ok {
			delivered := make([]model.QItem, 0, len(output))
			for _, row := range output {
				if id, ok := row.Get("id"); ok {
					if item, found := byID[fmt.Sprint(id)]; found {
						delivered = append(delivered, item)
					}
				}
			}
			sessions.RememberDedupe(request.Context.SessionID, dedupeKey, delivered)
		}
	}
	return env, nil
}

func qPublicReason(reason string) string {
	switch reason {
	case "timeout", "cancelled", "cursor_invalid", "cursor_expired", "snapshot_changed", "stage_failed", "max_bytes_item_exceeded":
		return reason
	case "":
		return ""
	default:
		return "stage_failed"
	}
}
func qContinuation(queryText, workspace, wid, generation string, offset int) *model.Continuation {
	return &model.Continuation{Reason: "q_page", Next: model.ContinuationTarget{Op: "q", Query: queryText, Workspace: workspace}, Cursor: signQCursor(queryText, wid, generation, offset)}
}
func qFailureFor(code, stage string, err error, workspace string) model.Envelope {
	env := qFailure(code, stage, err)
	env.Workspace = workspace
	return env
}
func qFailure(code, stage string, err error) model.Envelope {
	detail := "q operation failed"
	if err != nil {
		detail = err.Error()
		if len(detail) > 240 {
			detail = detail[:240]
		}
	}
	return model.Envelope{ContractVersion: "q-v1", Operation: "q", Ok: false, Items: model.QItems{}, Stages: []model.QStage{}, Budget: &model.QBudget{Requested: 2000}, Reason: code, Error: &model.EnvelopeError{Kind: "q", Code: code, ReasonCode: code, Message: code, Stage: stage, Detail: detail}}
}
func qEnvReason(env model.Envelope) string {
	if env.Error != nil || !env.Ok {
		return "stage_failed"
	}
	return ""
}
func qContextCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "timeout"
}
func firstArg(s query.Stage) string {
	if len(s.Args) > 0 {
		return s.Args[0]
	}
	return ""
}
func parseSince(s query.Stage) int64 {
	for _, arg := range s.Args {
		if strings.HasPrefix(arg, "since=") {
			n, _ := strconv.ParseInt(strings.TrimPrefix(arg, "since="), 10, 64)
			return n
		}
	}
	if v := s.Options["since"]; v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}
func splitQFields(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (a *App) qCall(ctx context.Context, request model.CommandRequest, operation string, payload map[string]any) (model.Envelope, error) {
	request.Operation = operation
	request.Payload = payload
	return a.Execute(ctx, request)
}
func (a *App) qSym(ctx context.Context, r model.CommandRequest, s query.Stage, wid, root string) qExecution {
	pattern := firstArg(s)
	if pattern == "" {
		return qExecution{reason: "stage_failed"}
	}
	pattern = strings.ReplaceAll(pattern, "*", "%")
	lookup := pattern
	if s.Flags["exact"] && strings.Contains(lookup, ".") {
		lookup = lookup[strings.LastIndex(lookup, ".")+1:]
	}
	payload := map[string]any{"pattern": lookup, "kind": s.Options["kind"], "exact": s.Flags["exact"], "repo": s.Options["repo"]}
	env, err := a.qCall(ctx, r, "nav.find", payload)
	if err != nil {
		return qExecution{reason: "stage_failed"}
	}
	items := qItemsFromEnvelope(env, wid, root)
	if s.Flags["exact"] && lookup != pattern {
		filtered := items[:0]
		for _, item := range items {
			if item.Name == pattern {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return qExecution{items: items, generation: env.GenerationID, reason: qEnvReason(env), fallback: env.FallbackUsed, truncated: env.Truncated}
}
func (a *App) qText(ctx context.Context, r model.CommandRequest, s query.Stage, wid, root string) qExecution {
	pattern := firstArg(s)
	if pattern == "" {
		return qExecution{reason: "stage_failed"}
	}
	payload := map[string]any{"pattern": pattern, "regex": s.Flags["regex"], "include_content": true, "context_mode": "lines"}
	if v := s.Options["path"]; v != "" {
		payload["path"] = v
	}
	if v := s.Options["type"]; v != "" {
		payload["type"] = v
	}
	env, err := a.qCall(ctx, r, "nav.search", payload)
	if err != nil {
		return qExecution{reason: "stage_failed"}
	}
	items := qItemsFromEnvelope(env, wid, root)
	for i := range items {
		if items[i].Line > 0 {
			rev := fileRevision(root, items[i].File, items[i].Line, max(items[i].Line, items[i].EndLine))
			items[i].ID = model.StableRangeID(wid, items[i].File, items[i].Line, max(items[i].Line, items[i].EndLine), rev).String()
			items[i].Revision = rev
		}
		items[i].Kind = "text"
		items[i].Origin = "text"
	}
	if glob := s.Options["path"]; glob != "" {
		filtered := items[:0]
		for _, item := range items {
			matched, _ := filepath.Match(glob, filepath.ToSlash(item.File))
			if matched || strings.Contains(filepath.ToSlash(item.File), strings.TrimSuffix(glob, "*")) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if typ := strings.ToLower(s.Options["type"]); typ != "" {
		filtered := items[:0]
		exts := map[string][]string{"go": {".go"}, "ts": {".ts", ".tsx"}, "js": {".js", ".jsx"}, "cs": {".cs"}, "py": {".py"}}
		for _, item := range items {
			ok := false
			for _, ext := range exts[typ] {
				if strings.EqualFold(filepath.Ext(item.File), ext) {
					ok = true
				}
			}
			if ok {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return qExecution{items: items, reason: qEnvReason(env), fallback: env.FallbackUsed, generation: env.GenerationID, truncated: env.Truncated}
}
func (a *App) qDocsForItems(ctx context.Context, r model.CommandRequest, items []model.QItem, wid, root string) qExecution {
	var out []model.QItem
	for _, source := range items {
		result := a.qDocs(ctx, r, query.Stage{Verb: "docs", Args: []string{source.Name}}, wid, root)
		if result.reason != "" {
			return qExecution{items: out, reason: result.reason}
		}
		for _, doc := range result.items {
			doc.In = source.ID
			out = append(out, doc)
		}
	}
	return qExecution{items: out}
}
func (a *App) qDocs(ctx context.Context, r model.CommandRequest, s query.Stage, wid, root string) qExecution {
	payload := map[string]any{"query": firstArg(s), "top": 200, "include_content": true}
	if v := s.Options["layer"]; v != "" {
		payload["layer"] = v
	}
	env, err := a.qCall(ctx, r, "nav.wiki.search", payload)
	if err != nil {
		return qExecution{reason: "stage_failed"}
	}
	return qExecution{items: qItemsFromEnvelope(env, wid, root), reason: qEnvReason(env), fallback: env.FallbackUsed, generation: env.GenerationID, truncated: env.Truncated}
}
func (a *App) qIDs(ctx context.Context, r model.CommandRequest, s query.Stage, wid, root string) qExecution {
	var items []model.QItem
	for _, raw := range strings.Split(strings.Join(s.Args, ","), ",") {
		parsed, err := model.ParseID(strings.TrimSpace(raw))
		if err != nil {
			return qExecution{reason: "stage_failed"}
		}
		if !parsed.Legacy && parsed.WorkspaceID != wid {
			return qExecution{reason: "stage_failed"}
		}
		switch parsed.Scheme {
		case model.IDSchemeSymbol:
			lookup := parsed.Name
			if strings.Contains(lookup, ".") {
				lookup = lookup[strings.LastIndex(lookup, ".")+1:]
			}
			env, e := a.qCall(ctx, r, "nav.find", map[string]any{"pattern": lookup, "exact": true})
			if e != nil || qEnvReason(env) != "" {
				return qExecution{reason: "stage_failed"}
			}
			found := qItemsFromEnvelope(env, wid, root)
			var matches []model.QItem
			for _, it := range found {
				candidate, parseErr := model.ParseID(it.ID)
				if parseErr == nil && candidate.File == parsed.File && candidate.Name == parsed.Name && (parsed.Sig == "" || strings.HasPrefix(candidate.Sig, parsed.Sig)) {
					matches = append(matches, it)
				}
			}
			if parsed.Rev != "" && len(matches) > 1 {
				for _, it := range matches {
					if strings.HasPrefix(it.Rev(), parsed.Rev) {
						matches = []model.QItem{it}
						break
					}
				}
			}
			if len(matches) > 1 {
				return qExecution{reason: "stage_failed"}
			}
			if len(matches) == 1 {
				it := matches[0]
				it.Stale = parsed.Legacy || parsed.Rev != "" && parsed.Rev != it.Rev()
				items = append(items, it)
			} else {
				identity := model.StableSymbolID(wid, model.SymbolRecord{FilePath: parsed.File, Name: parsed.Name, QualifiedName: parsed.Name, SignatureHash: parsed.Sig}, "", parsed.Sig != "").String()
				items = append(items, model.QItem{ID: identity, Kind: "symbol", Missing: true, Origin: "catalog"})
			}
		case model.IDSchemeDoc:
			env, e := a.qCall(ctx, r, "nav.wiki.search", map[string]any{"query": parsed.Name, "top": 100, "include_content": true})
			if e != nil || qEnvReason(env) != "" {
				return qExecution{reason: "stage_failed"}
			}
			found := qItemsFromEnvelope(env, wid, root)
			matched := false
			for _, it := range found {
				candidate, _ := model.ParseID(it.ID)
				if candidate.Name == parsed.Name && (parsed.File == "" || candidate.File == parsed.File) {
					it.Stale = parsed.Legacy || parsed.Rev != "" && parsed.Rev != it.Rev()
					items = append(items, it)
					matched = true
					break
				}
			}
			if !matched {
				identity := model.StableDocumentID(wid, parsed.File, parsed.Name, "").String()
				items = append(items, model.QItem{ID: identity, Kind: "doc", Name: parsed.Name, File: parsed.File, Missing: true, Origin: "wiki"})
			}
		case model.IDSchemeRange:
			rev := fileRevision(root, parsed.File, parsed.Start, parsed.End)
			identity := model.StableRangeID(wid, parsed.File, parsed.Start, parsed.End, rev).String()
			if rev == "" {
				items = append(items, model.QItem{ID: identity, Kind: "text", File: parsed.File, Line: parsed.Start, EndLine: parsed.End, Missing: true, Origin: "text"})
				continue
			}
			items = append(items, model.QItem{ID: identity, Revision: rev, Kind: "text", File: parsed.File, Line: parsed.Start, EndLine: parsed.End, Stale: parsed.Legacy || parsed.Rev != "" && parsed.Rev != rev, Origin: "text"})
		}
	}
	return qExecution{items: items}
}
func (a *App) qChanged(ctx context.Context, r model.CommandRequest, s query.Stage, sessions *SessionState, wid, root string) qExecution {
	if sessions == nil {
		return qExecution{reason: "stage_failed"}
	}
	var out []model.QItem
	for _, old := range sessions.ChangedCandidates(r.Context.SessionID, parseSince(s)) {
		if ctx.Err() != nil {
			return qExecution{items: out, reason: qContextCode(ctx.Err())}
		}
		found := a.qIDs(ctx, r, query.Stage{Verb: "id", Args: []string{old.ID}}, wid, root)
		if found.reason != "" {
			return qExecution{items: out, reason: found.reason}
		}
		if len(found.items) == 0 {
			continue
		}
		current := found.items[0]
		if current.Missing {
			out = append(out, current)
			continue
		}
		if current.Rev() != old.Rev() {
			current.Stale = true
			out = append(out, current)
		}
	}
	return qExecution{items: out}
}

func (a *App) qDiff(ctx context.Context, r model.CommandRequest, s query.Stage, wid, root string) qExecution {
	registration, _, err := a.resolveWorkspaceWithProjectForNavigation(r)
	if err != nil {
		return qExecution{reason: "stage_failed"}
	}
	ref := s.Options["ref"]
	if ref == "" {
		ref = "HEAD"
	}
	changes, err := getGitFileChangeTypes(ctx, registration.Root, ref)
	if err != nil {
		return qExecution{reason: qContextCodeOrStage(ctx, err)}
	}
	db, err := openWorkspaceDB(registration, "q.diff", true)
	if err != nil {
		return qExecution{reason: "stage_failed"}
	}
	defer db.Close()
	paths := make([]string, 0, len(changes))
	for path := range changes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var out []model.QItem
	for _, path := range paths {
		if ctx.Err() != nil {
			return qExecution{items: out, reason: qContextCode(ctx.Err())}
		}
		symbols, findErr := store.SymbolsByFile(ctx, db, path, 200, 0)
		if findErr != nil {
			return qExecution{items: out, reason: "stage_failed"}
		}
		counts := map[string]int{}
		for _, record := range symbols {
			counts[record.QualifiedName]++
		}
		for _, record := range symbols {
			if len(out) >= 200 {
				break
			}
			name := model.SymbolQName(record)
			signature := strings.ToLower(record.SignatureHash)
			if counts[record.QualifiedName] > 1 && !validQRevision(signature) {
				continue
			}
			revision := fileRevision(root, record.FilePath, record.StartLine, record.EndLine)
			id := model.StableSymbolID(wid, record, revision, counts[record.QualifiedName] > 1).String()
			out = append(out, model.QItem{ID: id, Revision: revision, Kind: record.Kind, Name: name, File: record.FilePath, Line: record.StartLine, EndLine: record.EndLine, Origin: "diff", Lang: record.Language, Parent: record.Parent, Signature: record.Signature})
		}
	}
	return qExecution{items: out}
}
func (a *App) qEdgesSequential(ctx context.Context, r model.CommandRequest, s query.Stage, items []model.QItem, wid string) qExecution {
	dir := firstArg(s)
	if dir == "" {
		return qExecution{reason: "stage_failed"}
	}
	depth, _ := strconv.Atoi(s.Options["depth"])
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	var out []model.QItem
	if dir != "refs" && dir != "callers" && dir != "callees" && dir != "impl" {
		return qExecution{reason: "stage_failed"}
	}
	for _, source := range items {
		if len(out) >= 200 {
			break
		}
		if ctx.Err() != nil {
			return qExecution{items: out, reason: qContextCode(ctx.Err())}
		}
		payload := map[string]any{"symbol": source.Name, "selector": source.Name, "file": source.File, "line": source.Line, "depth": depth, "limit": 50}
		op := "nav.refs"
		if dir == "callers" {
			op = "nav.callers"
		}
		if dir == "callees" {
			op = "nav.callees"
		}
		if dir == "impl" {
			op = "nav.related"
		}
		env, err := a.qCall(ctx, r, op, payload)
		if err != nil {
			return qExecution{items: out, reason: "stage_failed"}
		}
		sourceItems := qItemsFromEnvelope(env, wid, "")
		if len(sourceItems) > 50 {
			sourceItems = sourceItems[:50]
		}
		for _, item := range sourceItems {
			if len(out) >= 200 {
				break
			}
			item.Edge = &model.QEdge{Dir: dir, From: source.ID, Depth: depth}
			out = append(out, item)
		}
	}
	return qExecution{items: out}
}
func (a *App) qRead(ctx context.Context, r model.CommandRequest, s query.Stage, items []model.QItem, sessions *SessionState, wid, root string) qExecution {
	if len(items) == 0 {
		for _, arg := range s.Args {
			if file, start, end, ok := parseQRange(arg); ok {
				rev := fileRevision(root, file, start, end)
				items = append(items, model.QItem{ID: model.StableRangeID(wid, file, start, end, rev).String(), Revision: rev, Kind: "text", File: file, Line: start, EndLine: end, Origin: "text"})
			}
		}
	}
	ctxLines := 2
	if s.Flags["full"] {
		ctxLines = 50
	}
	if v := s.Options["ctx"]; v != "" {
		ctxLines, _ = strconv.Atoi(v)
	}
	for _, arg := range s.Args {
		if strings.HasPrefix(arg, "ctx=") {
			ctxLines, _ = strconv.Atoi(strings.TrimPrefix(arg, "ctx="))
		} else if strings.HasPrefix(arg, "±") {
			ctxLines, _ = strconv.Atoi(strings.TrimPrefix(arg, "±"))
		} else if arg != "full" {
			ctxLines, _ = strconv.Atoi(arg)
		}
	}
	if ctxLines < 0 {
		ctxLines = -ctxLines
	}
	if ctxLines > 50 {
		ctxLines = 50
	}
	out := make([]model.QItem, 0, len(items))
	for _, item := range items {
		if ctx.Err() != nil {
			return qExecution{items: out, reason: qContextCode(ctx.Err())}
		}
		if item.Missing {
			out = append(out, item)
			continue
		}
		fresh, _ := r.Payload["fresh"].(bool)
		if sessions != nil && !s.Flags["fresh"] && !fresh && sessions.Seen(r.Context.SessionID, item.ID, item.Rev()) {
			item.Text = ""
			item.Seen = true
			out = append(out, item)
			continue
		}
		if item.Kind == "doc" && item.Text != "" {
			item.Read = true
			out = append(out, item)
			continue
		}
		start := max(1, item.Line-ctxLines)
		end := max(start, item.EndLine+ctxLines)
		if item.EndLine == 0 {
			end = start + ctxLines*2
		}
		env, err := a.qCall(ctx, r, "nav.multi-read", map[string]any{"items": []string{fmt.Sprintf("%s:%d-%d", item.File, start, end)}})
		if err != nil || qEnvReason(env) != "" {
			return qExecution{items: out, reason: "stage_failed"}
		}
		raw := jsonValue(env.Items)
		var rows []map[string]any
		_ = json.Unmarshal(raw, &rows)
		if len(rows) > 0 {
			if content, ok := rows[0]["content"].(string); ok {
				item.Text = content
			}
			if file, ok := rows[0]["file"].(string); ok && item.File == "" {
				item.File = file
			}
		}
		if item.Kind != "doc" && item.Line > 0 {
			current := fileRevision(root, item.File, item.Line, max(item.Line, item.EndLine))
			if current != "" {
				if item.Rev() != "" && item.Rev() != current {
					item.Stale = true
				}
				item.Revision = current
			}
		}
		item.Read = item.Text != ""
		out = append(out, item)
	}
	if sessions != nil {
		for _, item := range out {
			if item.Read && item.Text != "" {
				sessions.Remember(r.Context.SessionID, item)
			}
		}
	}
	return qExecution{items: out}
}

func qItemsFromEnvelopeLegacy(env model.Envelope, wid, root string) []model.QItem {
	data := jsonValue(env.Items)
	var rows []map[string]any
	if json.Unmarshal(data, &rows) != nil {
		return nil
	}
	out := make([]model.QItem, 0, len(rows))
	for _, row := range rows {
		file := stringValue(row, "file_path", "file", "path")
		name := stringValue(row, "qualified_name", "name", "symbol", "doc_id", "title")
		kind := stringValue(row, "kind", "layer")
		line := intValue(row, "line", "start_line")
		end := intValue(row, "end_line")
		if file == "" && stringValue(row, "doc_id") != "" {
			file = "_wiki/" + stringValue(row, "doc_id")
		}
		if name == "" {
			name = filepath.Base(file)
		}
		origin := stringValue(row, "origin")
		if origin == "" {
			origin = env.Backend
		}
		it := model.QItem{Kind: kind, Name: name, File: file, Line: line, EndLine: end, Origin: origin, Lang: stringValue(row, "language", "lang"), Parent: stringValue(row, "parent"), Signature: stringValue(row, "signature"), Title: stringValue(row, "title"), Layer: stringValue(row, "layer"), Score: intValue(row, "score")}
		if kind == "doc" || stringValue(row, "doc_id") != "" {
			it.ID = model.StableDocumentID(wid, file, stringValue(row, "doc_id"), stringValue(row, "content_hash")).String()
			it.Kind = "doc"
			it.Revision = stringValue(row, "content_hash")
		} else if file != "" && line > 0 {
			rev := fileRevision(root, file, line, end)
			record := model.SymbolRecord{FilePath: file, Name: name, Kind: kind, QualifiedName: name, SignatureHash: stringValue(row, "signature_hash")}
			it.ID = model.StableSymbolID(wid, record, rev, false).String()
			it.Revision = rev
		} else if file != "" && line > 0 {
			it.ID = model.StableRangeID(wid, file, line, max(line, end), fileRevision(root, file, line, max(line, end))).String()
		}
		if stale, ok := row["stale"].(bool); ok {
			it.Stale = stale
		}
		if text := stringValue(row, "text", "content", "snippet", "evidence"); text != "" {
			it.Text = text
		}
		out = append(out, it)
	}
	return out
}
func fileRevision(root, file string, start, end int) string {
	if root == "" {
		return ""
	}
	path := filepath.Join(root, filepath.FromSlash(file))
	clean := filepath.Clean(path)
	rel, relErr := filepath.Rel(filepath.Clean(root), clean)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ""
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n"), "\n")
	start = max(1, start)
	end = max(start, end)
	if start > len(lines) {
		return ""
	}
	end = min(end, len(lines))
	return model.RevOfLines(lines[start-1 : end])
}
func jsonValue(value any) []byte { data, _ := json.Marshal(value); return data }
func stringValue(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
func intValue(m map[string]any, keys ...string) int {
	for _, key := range keys {
		switch v := m[key].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case json.Number:
			n, _ := v.Int64()
			return int(n)
		}
	}
	return 0
}
func parseQRange(value string) (string, int, int, bool) {
	colon := strings.LastIndex(value, ":")
	dash := strings.LastIndex(value, "-")
	if colon < 1 || dash <= colon {
		return "", 0, 0, false
	}
	start, e1 := strconv.Atoi(value[colon+1 : dash])
	end, e2 := strconv.Atoi(value[dash+1:])
	if e1 != nil || e2 != nil || start < 1 || end < start {
		return "", 0, 0, false
	}
	return value[:colon], start, end, true
}
func qWhere(items []model.QItem, s query.Stage) []model.QItem {
	if len(s.Args) < 1 {
		return items
	}
	expr := strings.Join(s.Args, " ")
	for _, op := range []string{"!~", "!=", "=", "~"} {
		if at := strings.Index(expr, op); at > 0 {
			field := strings.TrimSpace(expr[:at])
			value := strings.Trim(strings.TrimSpace(expr[at+len(op):]), "\"'")
			out := items[:0]
			for _, item := range items {
				got, _ := item.Field(field)
				text := fmt.Sprint(got)
				match := false
				switch op {
				case "=":
					match = text == value
				case "!=":
					match = text != value
				case "~":
					match = strings.Contains(text, value)
				case "!~":
					match = !strings.Contains(text, value)
				}
				if match {
					out = append(out, item)
				}
			}
			return out
		}
	}
	return items
}
func qLimit(items []model.QItem, s query.Stage) []model.QItem {
	n, _ := strconv.Atoi(firstArg(s))
	if n < 0 {
		n = 0
	}
	if n < len(items) {
		return items[:n]
	}
	return items
}
func qUniq(items []model.QItem) []model.QItem {
	seen := map[string]bool{}
	out := items[:0]
	for _, item := range items {
		key := item.ID
		if key == "" {
			key = item.File + ":" + strconv.Itoa(item.Line) + ":" + item.Name
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}
func qSort(items []model.QItem, s query.Stage) []model.QItem {
	field := firstArg(s)
	if field == "" {
		field = "file"
	}
	desc := len(s.Args) > 1 && strings.EqualFold(s.Args[1], "desc")
	sort.SliceStable(items, func(i, j int) bool {
		a, _ := items[i].Field(field)
		b, _ := items[j].Field(field)
		if fmt.Sprint(a) == fmt.Sprint(b) {
			if items[i].Line == items[j].Line {
				return items[i].ID < items[j].ID
			}
			return items[i].Line < items[j].Line
		}
		if desc {
			return fmt.Sprint(a) > fmt.Sprint(b)
		}
		return fmt.Sprint(a) < fmt.Sprint(b)
	})
	return items
}
func truncateQText(text string, n int) string {
	if n > len(text) {
		n = len(text)
	}
	for n > 0 && n < len(text) && !utf8.RuneStart(text[n]) {
		n--
	}
	return text[:n]
}
func qBoundEnvelope(env model.Envelope, items []model.QItem, fields []string, maxBytes int, queryText, wid, workspace, generation string, baseOffset int) model.Envelope {
	for {
		data, _ := json.Marshal(env)
		if len(data) <= maxBytes {
			return env
		}
		rows, ok := env.Items.(model.QItems)
		if !ok || len(rows) == 0 {
			env.Ok = false
			env.Partial = false
			env.Error = &model.EnvelopeError{Kind: "q", Code: "max_bytes_item_exceeded", ReasonCode: "max_bytes_item_exceeded", Message: "max_bytes_item_exceeded", Stage: "projection"}
			env.Reason = "max_bytes_item_exceeded"
			return env
		}
		if len(rows) == 1 && len(items) > 0 && items[0].Text != "" {
			candidate := items[0]
			candidate.TruncatedItem = true
			for n := len(candidate.Text); n > 0; n = n / 2 {
				candidate.Text = truncateQText(candidate.Text, n)
				sensibilidad, _ := rows[0].Get("sensibilidad")
				if sensibilidad == nil {
					sensibilidad = ""
				}
				rows[0] = candidate.Project(fields).With("sensibilidad", sensibilidad)
				env.Items = rows
				data, _ = json.Marshal(env)
				if len(data) <= maxBytes {
					env.Truncated = true
					env.Continuation = qContinuation(queryText, workspace, wid, generation, baseOffset+1)
					return env
				}
			}
			rows = rows[:0]
		} else {
			rows = rows[:len(rows)-1]
		}
		env.Items = rows
		env.Truncated = true
		env.Continuation = qContinuation(queryText, workspace, wid, generation, baseOffset+len(rows))
	}
}
