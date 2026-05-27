package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	defaultEditPlanMaxFileBytes = 1_000_000
	defaultEditPlanMaxDiffChars = 200_000
)

var editPlanBinaryExtensions = map[string]struct{}{
	".7z": {}, ".bin": {}, ".bmp": {}, ".db": {}, ".dll": {}, ".dylib": {}, ".exe": {}, ".gif": {},
	".gz": {}, ".ico": {}, ".jar": {}, ".jpeg": {}, ".jpg": {}, ".pdf": {}, ".png": {}, ".sqlite": {},
	".sqlite3": {}, ".tar": {}, ".webp": {}, ".zip": {},
}

type editPlanResolvedTarget struct {
	Target     model.EditPlanTarget
	RelPath    string
	AbsPath    string
	Before     []byte
	Hash       string
	StartLine  int
	EndLine    int
	StartByte  int
	EndByte    int
	LineCount  int
	LineEnding string
}

type editPlanFileState struct {
	RelPath string
	AbsPath string
	Before  []byte
	After   []byte
}

func (a *App) editPlan(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	packetText := strings.TrimSpace(stringPayload(request.Payload, "packet"))
	if packetText == "" {
		packetText = strings.TrimSpace(stringPayload(request.Payload, "stdin"))
	}
	if packetText == "" {
		return model.Envelope{}, errors.New("edit-plan packet is required; pass --stdin or --packet <file>")
	}
	strict, _ := request.Payload["strict"].(bool)
	includeContent, _ := request.Payload["include_content"].(bool)
	applyRequested, _ := request.Payload["apply"].(bool)
	experimentalApply, _ := request.Payload["experimental_apply"].(bool)
	if applyRequested && !experimentalApply {
		return model.Envelope{}, errors.New("--apply requires --experimental-apply")
	}

	packet, err := parseEditPlanPacket(packetText, strict)
	if err != nil {
		return model.Envelope{}, err
	}
	targets, guardrails, evidence, err := validateEditPlanPacket(registration.Root, &packet, strict, applyRequested, includeContent)
	if err != nil {
		return model.Envelope{}, err
	}
	fileStates := editPlanInitialFileStates(targets)
	var operationResults []model.EditPlanOperationResult
	if editPlanVersion(&packet) == model.EditPlanVersionV2 {
		operationResults, err = applyEditPlanV2InMemory(&packet, targets, fileStates)
	} else {
		operationResults, err = applyEditPlanInMemory(&packet, targets, fileStates)
	}
	if err != nil {
		return model.Envelope{}, err
	}
	diff := buildEditPlanDiff(fileStates)
	filesChanged := countChangedEditPlanFiles(fileStates)

	maxDiffChars := packet.Constraints.MaxDiffChars
	if maxDiffChars <= 0 {
		maxDiffChars = request.Context.MaxChars
	}
	if maxDiffChars <= 0 {
		maxDiffChars = defaultEditPlanMaxDiffChars
	}
	truncated := false
	var nextHint *string
	if len(diff) > maxDiffChars {
		diff = diff[:maxDiffChars]
		truncated = true
		hint := "diff truncated; rerun with --max-chars or lower constraints.max_diff_chars for a smaller packet"
		nextHint = &hint
	}

	mode := "dry_run"
	applyStatus := model.EditPlanApplyStatus{Requested: applyRequested}
	if applyRequested {
		if err := requireCleanGitWorkspace(ctx, registration.Root); err != nil {
			return model.Envelope{}, err
		}
		if err := revalidateEditPlanHashes(registration.Root, targets); err != nil {
			return model.Envelope{}, err
		}
		changedFiles, err := writeEditPlanFiles(fileStates)
		if err != nil {
			return model.Envelope{}, err
		}
		mode = "applied"
		applyStatus.Applied = true
		applyStatus.Files = changedFiles
		applyStatus.Message = "files written; no stage, commit, formatter, chmod, rename, or delete operation was performed"
	}
	if !applyRequested {
		applyStatus.Message = "dry-run only; no files were written"
	}

	result := model.EditPlanResult{
		PatchPacket:  packet,
		Diff:         diff,
		FilesChanged: filesChanged,
		Operations:   operationResults,
		Evidence:     evidence,
		Guardrails:   guardrails,
		ApplyStatus:  applyStatus,
	}
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "edit-plan",
		Mode:      mode,
		Items:     []model.EditPlanResult{result},
		Truncated: truncated,
		NextHint:  nextHint,
		Stats:     model.Stats{Files: filesChanged, Ms: time.Since(started).Milliseconds()},
	}, nil
}

func parseEditPlanPacket(packetText string, strict bool) (model.EditPlanRequest, error) {
	var packet model.EditPlanRequest
	packetText = strings.TrimPrefix(packetText, "\ufeff")
	decoder := json.NewDecoder(strings.NewReader(packetText))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(&packet); err != nil {
		return packet, fmt.Errorf("invalid edit-plan packet JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return packet, errors.New("invalid edit-plan packet JSON: multiple top-level values")
	}
	return packet, nil
}

func validateEditPlanPacket(root string, packet *model.EditPlanRequest, strict bool, applyRequested bool, includeContent bool) (map[string]editPlanResolvedTarget, []model.EditPlanGuardrail, []model.EditPlanEvidence, error) {
	version := editPlanVersion(packet)
	switch version {
	case model.EditPlanVersionV1, model.EditPlanVersionV2:
		packet.Version = version
	default:
		return nil, nil, nil, fmt.Errorf("unsupported edit-plan version %q; expected %q or %q", packet.Version, model.EditPlanVersionV1, model.EditPlanVersionV2)
	}
	if len(packet.Targets) == 0 {
		return nil, nil, nil, errors.New("edit-plan requires at least one target")
	}
	if len(packet.Operations) == 0 {
		return nil, nil, nil, errors.New("edit-plan requires at least one operation")
	}
	maxFileBytes := packet.Constraints.MaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = defaultEditPlanMaxFileBytes
	}
	denyPaths := editPlanDenyPaths(packet.Constraints.DenyPaths)
	guardrails := []model.EditPlanGuardrail{
		{Code: "dry_run_default", Status: "active", Message: "dry-run is the default; writes require --apply --experimental-apply"},
		{Code: "no_stage_commit_format", Status: "active", Message: "edit-plan never stages, commits, formats, renames, chmods, or deletes directories"},
		{Code: "path_denylist", Status: "active", Message: "blocked paths include .git/**, .mi-lsp/**, read-model.toml, binaries, and configured deny_paths"},
	}
	if version == model.EditPlanVersionV2 {
		guardrails = append(guardrails, model.EditPlanGuardrail{Code: "go_ast_only", Status: "active", Message: "edit-plan-v2 implements Go AST operations only; C#, TypeScript, and Python return language_not_supported"})
	}
	if applyRequested {
		guardrails = append(guardrails, model.EditPlanGuardrail{Code: "apply_clean_git", Status: "active", Message: "apply requires a clean git workspace and revalidated hashes"})
	}

	targetIDs := map[string]struct{}{}
	targets := make(map[string]editPlanResolvedTarget, len(packet.Targets))
	evidence := make([]model.EditPlanEvidence, 0, len(packet.Targets)*2)
	for i := range packet.Targets {
		target := packet.Targets[i]
		id := strings.TrimSpace(target.ID)
		if id == "" {
			return nil, nil, nil, errors.New("target id is required")
		}
		if _, exists := targetIDs[id]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate target id %q", id)
		}
		targetIDs[id] = struct{}{}
		absPath, relPath, err := resolveEditPlanPath(root, target.Path, denyPaths)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("target %s: %w", id, err)
		}
		info, err := os.Stat(absPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("target %s: file must exist and be readable: %w", id, err)
		}
		if info.IsDir() {
			return nil, nil, nil, fmt.Errorf("target %s: directories are not supported", id)
		}
		if info.Size() > int64(maxFileBytes) {
			return nil, nil, nil, fmt.Errorf("target %s: file exceeds max_file_bytes (%d)", id, maxFileBytes)
		}
		before, err := os.ReadFile(absPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("target %s: read failed: %w", id, err)
		}
		if isEditPlanBinaryPath(relPath, before) {
			return nil, nil, nil, fmt.Errorf("target %s: binary files are blocked", id)
		}
		if version == model.EditPlanVersionV2 {
			language, err := normalizeEditPlanTargetLanguage(target.Language, relPath)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("target %s: %w", id, err)
			}
			target.Language = language
			packet.Targets[i].Language = language
			if target.Range.StartLine <= 0 && target.Range.EndLine <= 0 {
				target.Range.StartLine = 1
				packet.Targets[i].Range.StartLine = 1
			}
		}
		start, end, lineCount, err := editPlanLineRangeOffsets(before, target.Range.StartLine, target.Range.EndLine)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("target %s: %w", id, err)
		}
		if target.Range.EndLine <= 0 {
			target.Range.EndLine = lineCount
			packet.Targets[i].Range.EndLine = lineCount
		}
		actualHash := editPlanHash(before)
		expectedHash := strings.TrimSpace(target.ExpectedHash)
		if expectedHash == "" {
			if applyRequested || strict || packet.Constraints.RequireEvidence {
				return nil, nil, nil, fmt.Errorf("target %s: expected_hash is required for apply, strict mode, or require_evidence", id)
			}
			guardrails = append(guardrails, model.EditPlanGuardrail{Code: "missing_expected_hash", Status: "warning", Message: "target " + id + " has no expected_hash; dry-run validated current bytes only"})
		} else if !editPlanHashMatches(expectedHash, actualHash) {
			return nil, nil, nil, fmt.Errorf("target %s: expected_hash mismatch", id)
		}
		packet.Targets[i] = target
		resolved := editPlanResolvedTarget{
			Target:     target,
			RelPath:    relPath,
			AbsPath:    absPath,
			Before:     before,
			Hash:       actualHash,
			StartLine:  target.Range.StartLine,
			EndLine:    packet.Targets[i].Range.EndLine,
			StartByte:  start,
			EndByte:    end,
			LineCount:  lineCount,
			LineEnding: detectEditPlanLineEnding(before),
		}
		targets[id] = resolved
		evidence = append(evidence,
			model.EditPlanEvidence{Kind: "target_hash", Path: relPath, Value: "sha256:" + actualHash},
			model.EditPlanEvidence{Kind: "target_range", Path: relPath, Value: fmt.Sprintf("%d-%d", resolved.StartLine, resolved.EndLine)},
		)
		if includeContent {
			evidence = append(evidence, model.EditPlanEvidence{Kind: "target_content", Path: relPath, Value: editPlanEvidenceContent(before, start, end)})
		}
	}

	operationIDs := map[string]struct{}{}
	rangeOperationByTarget := map[string]string{}
	for _, operation := range packet.Operations {
		id := strings.TrimSpace(operation.ID)
		if id == "" {
			return nil, nil, nil, errors.New("operation id is required")
		}
		if _, exists := operationIDs[id]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate operation id %q", id)
		}
		operationIDs[id] = struct{}{}
		targetID := strings.TrimSpace(operation.TargetID)
		if targetID == "" {
			return nil, nil, nil, fmt.Errorf("operation %s: target_id is required", id)
		}
		if _, ok := targets[targetID]; !ok {
			return nil, nil, nil, fmt.Errorf("operation %s: unknown target_id %q", id, targetID)
		}
		target := targets[targetID]
		if err := validateEditPlanOperation(operation, version, target); err != nil {
			return nil, nil, nil, fmt.Errorf("operation %s: %w", id, err)
		}
		if version == model.EditPlanVersionV1 && editPlanOperationUsesWholeRange(operation.Kind) {
			if previous := rangeOperationByTarget[targetID]; previous != "" {
				return nil, nil, nil, fmt.Errorf("operation %s overlaps target range already used by operation %s", id, previous)
			}
			rangeOperationByTarget[targetID] = id
		}
	}
	return targets, guardrails, evidence, nil
}

func editPlanVersion(packet *model.EditPlanRequest) string {
	return strings.TrimSpace(packet.Version)
}

func normalizeEditPlanTargetLanguage(language string, relPath string) (string, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	switch language {
	case "":
		switch strings.ToLower(filepath.Ext(relPath)) {
		case ".go":
			language = "go"
		case ".cs":
			language = "csharp"
		case ".ts", ".tsx", ".js", ".jsx":
			language = "typescript"
		case ".py":
			language = "python"
		default:
			return "", fmt.Errorf("language is required for edit-plan-v2 target %q", relPath)
		}
	case "cs":
		language = "csharp"
	case "ts", "tsx", "javascript", "js", "jsx":
		language = "typescript"
	case "py":
		language = "python"
	}
	switch language {
	case "go", "csharp", "typescript", "python":
		return language, nil
	default:
		return "", fmt.Errorf("unsupported target language %q", language)
	}
}

func editPlanDenyPaths(configured []string) []string {
	result := []string{".git/**", ".mi-lsp/**", ".docs/wiki/_mi-lsp/read-model.toml"}
	for _, value := range configured {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func resolveEditPlanPath(root string, requested string, denyPaths []string) (string, string, error) {
	if strings.TrimSpace(requested) == "" {
		return "", "", errors.New("path is required")
	}
	if strings.ContainsAny(requested, "\r\n") {
		return "", "", errors.New("path contains newline")
	}
	root = filepath.Clean(root)
	absPath := requested
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(root, filepath.FromSlash(requested))
	}
	absPath = filepath.Clean(absPath)
	if !editPlanPathInsideRoot(root, absPath) {
		return "", "", errors.New("path outside workspace root")
	}
	if evaluated, err := filepath.EvalSymlinks(absPath); err == nil {
		evaluatedRoot := root
		if resolvedRoot, rootErr := filepath.EvalSymlinks(root); rootErr == nil {
			evaluatedRoot = resolvedRoot
		}
		if !editPlanPathInsideRoot(evaluatedRoot, evaluated) {
			return "", "", errors.New("symlink resolves outside workspace root")
		}
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", "", errors.New("path outside workspace root")
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if deniedBy, denied := editPlanDeniedBy(rel, denyPaths); denied {
		return "", "", fmt.Errorf("path denied by %s", deniedBy)
	}
	return absPath, rel, nil
}

func editPlanPathInsideRoot(root string, absPath string) bool {
	root = filepath.Clean(root)
	absPath = filepath.Clean(absPath)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
		absPath = strings.ToLower(absPath)
	}
	return absPath == root || strings.HasPrefix(absPath, root+string(os.PathSeparator))
}

func editPlanDeniedBy(relPath string, denyPaths []string) (string, bool) {
	relPath = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(relPath)), "./")
	for _, pattern := range denyPaths {
		normalized := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(pattern)), "./")
		if normalized == "" {
			continue
		}
		if strings.HasSuffix(normalized, "/**") {
			prefix := strings.TrimSuffix(normalized, "/**")
			if relPath == prefix || strings.HasPrefix(relPath, prefix+"/") {
				return pattern, true
			}
		}
		if relPath == normalized {
			return pattern, true
		}
		if ok, _ := pathpkg.Match(normalized, relPath); ok {
			return pattern, true
		}
	}
	return "", false
}

func isEditPlanBinaryPath(relPath string, content []byte) bool {
	if _, blocked := editPlanBinaryExtensions[strings.ToLower(filepath.Ext(relPath))]; blocked {
		return true
	}
	return bytes.IndexByte(content, 0) >= 0
}

func validateEditPlanOperation(operation model.EditPlanOperation, version string, target editPlanResolvedTarget) error {
	if version == model.EditPlanVersionV2 {
		return validateEditPlanV2Operation(operation, target)
	}
	switch operation.Kind {
	case "replace_literal":
		if operation.Find == "" {
			return errors.New("find is required for replace_literal")
		}
	case "replace_regex_limited":
		if operation.Find == "" {
			return errors.New("find is required for replace_regex_limited")
		}
		if operation.MaxReplacements <= 0 {
			return errors.New("max_replacements is required for replace_regex_limited")
		}
		if strings.ContainsAny(operation.Find, "\r\n") {
			return errors.New("multiline regex is not allowed")
		}
		if _, err := regexp.Compile(operation.Find); err != nil {
			return fmt.Errorf("invalid regex: %w", err)
		}
	case "insert_before", "insert_after", "replace_range":
		if operation.Content == "" {
			return errors.New("content is required")
		}
	case "delete_range":
	default:
		return fmt.Errorf("unsupported operation kind %q", operation.Kind)
	}
	return nil
}

func validateEditPlanV2Operation(operation model.EditPlanOperation, target editPlanResolvedTarget) error {
	if target.Target.Language != "go" {
		return fmt.Errorf("language_not_supported: AST backend for language %q is not implemented; use edit-plan-v1 textual operations or a future backend", target.Target.Language)
	}
	switch operation.Kind {
	case "replace_go_function", "replace_go_function_body", "insert_go_function_after":
		if target.Target.Symbol == nil || strings.TrimSpace(target.Target.Symbol.Name) == "" {
			return errors.New("symbol.name is required for Go function operations")
		}
		if strings.TrimSpace(operation.Content) == "" {
			return errors.New("content is required")
		}
	case "ensure_go_import", "remove_go_import":
		if editPlanOperationImportPath(operation) == "" {
			return errors.New("import_path or content is required")
		}
	default:
		return fmt.Errorf("unsupported edit-plan-v2 operation kind %q", operation.Kind)
	}
	return nil
}

func editPlanOperationImportPath(operation model.EditPlanOperation) string {
	if trimmed := strings.TrimSpace(operation.ImportPath); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(operation.Content)
}

func editPlanOperationUsesWholeRange(kind string) bool {
	switch kind {
	case "insert_before", "insert_after", "delete_range", "replace_range":
		return true
	default:
		return false
	}
}

func editPlanInitialFileStates(targets map[string]editPlanResolvedTarget) map[string]*editPlanFileState {
	fileStates := map[string]*editPlanFileState{}
	for _, target := range targets {
		if _, exists := fileStates[target.RelPath]; exists {
			continue
		}
		fileStates[target.RelPath] = &editPlanFileState{
			RelPath: target.RelPath,
			AbsPath: target.AbsPath,
			Before:  append([]byte(nil), target.Before...),
			After:   append([]byte(nil), target.Before...),
		}
	}
	return fileStates
}

func applyEditPlanInMemory(packet *model.EditPlanRequest, targets map[string]editPlanResolvedTarget, fileStates map[string]*editPlanFileState) ([]model.EditPlanOperationResult, error) {
	results := make([]model.EditPlanOperationResult, 0, len(packet.Operations))
	for _, operation := range packet.Operations {
		target := targets[operation.TargetID]
		state := fileStates[target.RelPath]
		start, end, _, err := editPlanLineRangeOffsets(state.After, target.Target.Range.StartLine, target.Target.Range.EndLine)
		if err != nil {
			return nil, fmt.Errorf("operation %s: target range no longer valid after previous operations: %w", operation.ID, err)
		}
		lineEnding := detectEditPlanLineEnding(state.After)
		result := model.EditPlanOperationResult{ID: operation.ID, Kind: operation.Kind, TargetID: operation.TargetID, Path: target.RelPath, Status: "planned"}
		afterText := string(state.After)
		prefix := afterText[:start]
		rangeText := afterText[start:end]
		suffix := afterText[end:]
		switch operation.Kind {
		case "replace_literal":
			count := strings.Count(rangeText, operation.Find)
			if count == 0 {
				if packet.Constraints.RequireCleanMatch {
					return nil, fmt.Errorf("operation %s: literal match not found", operation.ID)
				}
				result.Status = "no_match"
				results = append(results, result)
				continue
			}
			if operation.MaxReplacements > 0 && count > operation.MaxReplacements {
				return nil, fmt.Errorf("operation %s: literal matched %d times, above max_replacements %d", operation.ID, count, operation.MaxReplacements)
			}
			replace := normalizeEditPlanLineEndings(operation.Replace, lineEnding)
			limit := -1
			if operation.MaxReplacements > 0 {
				limit = operation.MaxReplacements
			}
			rangeText = strings.Replace(rangeText, operation.Find, replace, limit)
			state.After = []byte(prefix + rangeText + suffix)
			result.Replacements = count
		case "replace_regex_limited":
			re, err := regexp.Compile(operation.Find)
			if err != nil {
				return nil, fmt.Errorf("operation %s: invalid regex: %w", operation.ID, err)
			}
			matches := re.FindAllStringIndex(rangeText, -1)
			if len(matches) == 0 {
				if packet.Constraints.RequireCleanMatch {
					return nil, fmt.Errorf("operation %s: regex match not found", operation.ID)
				}
				result.Status = "no_match"
				results = append(results, result)
				continue
			}
			if len(matches) > operation.MaxReplacements {
				return nil, fmt.Errorf("operation %s: regex matched %d times, above max_replacements %d", operation.ID, len(matches), operation.MaxReplacements)
			}
			replace := normalizeEditPlanLineEndings(operation.Replace, lineEnding)
			rangeText = re.ReplaceAllString(rangeText, replace)
			state.After = []byte(prefix + rangeText + suffix)
			result.Replacements = len(matches)
		case "insert_before":
			content := normalizeEditPlanLineEndings(operation.Content, lineEnding)
			state.After = []byte(prefix + content + rangeText + suffix)
		case "insert_after":
			content := normalizeEditPlanLineEndings(operation.Content, lineEnding)
			state.After = []byte(prefix + rangeText + content + suffix)
		case "delete_range":
			state.After = []byte(prefix + suffix)
		case "replace_range":
			content := normalizeEditPlanLineEndings(operation.Content, lineEnding)
			state.After = []byte(prefix + content + suffix)
		default:
			return nil, fmt.Errorf("operation %s: unsupported operation kind %q", operation.ID, operation.Kind)
		}
		result.Status = "ok"
		results = append(results, result)
	}
	return results, nil
}

func editPlanLineRangeOffsets(content []byte, startLine int, endLine int) (int, int, int, error) {
	if startLine <= 0 {
		return 0, 0, 0, errors.New("range.start_line must be positive")
	}
	starts := []int{0}
	for i, b := range content {
		if b == '\n' && i+1 < len(content) {
			starts = append(starts, i+1)
		}
	}
	lineCount := len(starts)
	if len(content) == 0 {
		lineCount = 1
		starts = []int{0}
	}
	if endLine <= 0 {
		endLine = lineCount
	}
	if endLine < startLine {
		return 0, 0, lineCount, errors.New("range.end_line must be >= start_line")
	}
	if startLine > lineCount || endLine > lineCount {
		return 0, 0, lineCount, fmt.Errorf("range %d-%d outside file line count %d", startLine, endLine, lineCount)
	}
	start := starts[startLine-1]
	end := len(content)
	if endLine < lineCount {
		end = starts[endLine]
	}
	return start, end, lineCount, nil
}

func detectEditPlanLineEnding(content []byte) string {
	if bytes.Contains(content, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

func normalizeEditPlanLineEndings(value string, lineEnding string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if lineEnding == "\r\n" {
		value = strings.ReplaceAll(value, "\n", "\r\n")
	}
	return value
}

func editPlanHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func editPlanHashMatches(expected string, actual string) bool {
	expected = strings.TrimSpace(strings.ToLower(expected))
	expected = strings.TrimPrefix(expected, "sha256:")
	return expected == strings.ToLower(actual)
}

func editPlanEvidenceContent(content []byte, start int, end int) string {
	if start < 0 || end < start || end > len(content) {
		return ""
	}
	value := string(content[start:end])
	if len(value) > 8_000 {
		return value[:8_000]
	}
	return value
}

func buildEditPlanDiff(fileStates map[string]*editPlanFileState) string {
	paths := make([]string, 0, len(fileStates))
	for relPath, state := range fileStates {
		if !bytes.Equal(state.Before, state.After) {
			paths = append(paths, relPath)
		}
	}
	sort.Strings(paths)
	var builder strings.Builder
	for _, relPath := range paths {
		state := fileStates[relPath]
		builder.WriteString("diff --git a/")
		builder.WriteString(relPath)
		builder.WriteString(" b/")
		builder.WriteString(relPath)
		builder.WriteString("\n--- a/")
		builder.WriteString(relPath)
		builder.WriteString("\n+++ b/")
		builder.WriteString(relPath)
		builder.WriteString("\n")
		beforeLines := editPlanDiffLines(state.Before)
		afterLines := editPlanDiffLines(state.After)
		builder.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", len(beforeLines), len(afterLines)))
		for _, line := range beforeLines {
			builder.WriteString("-")
			builder.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				builder.WriteString("\n")
			}
		}
		for _, line := range afterLines {
			builder.WriteString("+")
			builder.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				builder.WriteString("\n")
			}
		}
	}
	return builder.String()
}

func editPlanDiffLines(content []byte) []string {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	if normalized == "" {
		return []string{}
	}
	lines := strings.SplitAfter(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func countChangedEditPlanFiles(fileStates map[string]*editPlanFileState) int {
	count := 0
	for _, state := range fileStates {
		if !bytes.Equal(state.Before, state.After) {
			count++
		}
	}
	return count
}

func requireCleanGitWorkspace(ctx context.Context, root string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("apply requires a clean git workspace: %w", err)
	}
	if strings.TrimSpace(string(output)) != "" {
		return errors.New("apply requires a clean git workspace; commit or stash changes first")
	}
	return nil
}

func revalidateEditPlanHashes(root string, targets map[string]editPlanResolvedTarget) error {
	for id, target := range targets {
		content, err := os.ReadFile(target.AbsPath)
		if err != nil {
			return fmt.Errorf("target %s: re-read failed before apply: %w", id, err)
		}
		if editPlanHash(content) != target.Hash {
			return fmt.Errorf("target %s: hash changed before apply", id)
		}
		if !editPlanPathInsideRoot(root, target.AbsPath) {
			return fmt.Errorf("target %s: path escaped workspace before apply", id)
		}
	}
	return nil
}

func writeEditPlanFiles(fileStates map[string]*editPlanFileState) ([]string, error) {
	paths := make([]string, 0, len(fileStates))
	for relPath, state := range fileStates {
		if !bytes.Equal(state.Before, state.After) {
			paths = append(paths, relPath)
		}
	}
	sort.Strings(paths)
	backups := map[string][]byte{}
	modes := map[string]os.FileMode{}
	written := make([]string, 0, len(paths))
	for _, relPath := range paths {
		state := fileStates[relPath]
		info, err := os.Stat(state.AbsPath)
		if err != nil {
			return written, fmt.Errorf("stat before write %s: %w", relPath, err)
		}
		backups[state.AbsPath] = append([]byte(nil), state.Before...)
		modes[state.AbsPath] = info.Mode()
		if err := replaceEditPlanFile(state.AbsPath, state.After, info.Mode()); err != nil {
			rollbackEditPlanFiles(backups, modes)
			return written, fmt.Errorf("write %s failed; rollback attempted: %w", relPath, err)
		}
		written = append(written, relPath)
	}
	return written, nil
}

func replaceEditPlanFile(absPath string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(absPath)
	base := filepath.Base(absPath)
	tmp, err := os.CreateTemp(dir, "."+base+".mi-lsp-edit-plan-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := os.Remove(absPath); err != nil {
			return err
		}
	}
	return os.Rename(tmpPath, absPath)
}

func rollbackEditPlanFiles(backups map[string][]byte, modes map[string]os.FileMode) {
	paths := make([]string, 0, len(backups))
	for path := range backups {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		_ = os.WriteFile(path, backups[path], modes[path])
	}
}
