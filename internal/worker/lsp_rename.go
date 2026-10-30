package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type lspWorkspaceSymbol struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	ContainerName string `json:"containerName"`
	Location      struct {
		URI   string    `json:"uri"`
		Range *lspRange `json:"range"`
	} `json:"location"`
}

type lspRenameTextEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

const (
	renameSelectorCandidateLimit  = 8
	renameSelectorSourceByteLimit = 2 << 20
)

type lspRenameDocumentChange struct {
	TextDocument *struct {
		URI     string `json:"uri"`
		Version *int   `json:"version"`
	} `json:"textDocument"`
	Edits []lspRenameTextEdit `json:"edits"`
	Kind  string              `json:"kind"`
}

// renameSymbol returns a versioned gopls workspace edit only. The service owns
// validation, atomic staging, application, rollback, and post-edit diagnostics.
func (c *LSPClient) supportsPrepareRename() bool {
	raw := bytes.TrimSpace(c.capabilities["renameProvider"])
	if len(raw) == 0 || bytes.Equal(raw, []byte("false")) || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var provider struct {
		PrepareProvider bool `json:"prepareProvider"`
	}
	return json.Unmarshal(raw, &provider) == nil && provider.PrepareProvider
}

func (c *LSPClient) resolveRenameSymbol(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	if c.backendName() != "gopls" || !c.supportsPrepareRename() {
		return model.WorkerResponse{}, errors.New("unsupported_operation: safe gopls prepareRename is unavailable")
	}
	workspaceSymbolCapability := bytes.TrimSpace(c.capabilities["workspaceSymbolProvider"])
	if len(workspaceSymbolCapability) == 0 || bytes.Equal(workspaceSymbolCapability, []byte("false")) || bytes.Equal(workspaceSymbolCapability, []byte("null")) {
		return model.WorkerResponse{}, errors.New("unsupported_operation: gopls did not advertise workspace/symbol")
	}
	symbol, _ := request.Payload["symbol"].(string)
	if !token.IsIdentifier(symbol) {
		return model.WorkerResponse{}, errors.New("invalid_input: semantic rename resolution requires a Go identifier")
	}
	if err := ctx.Err(); err != nil {
		return model.WorkerResponse{}, err
	}
	raw, err := c.sendRequest("workspace/symbol", map[string]any{"query": symbol})
	if err != nil {
		return model.WorkerResponse{}, fmt.Errorf("unavailable: gopls workspace/symbol failed: %w", err)
	}
	if len(raw) > 1<<20 {
		return model.WorkerResponse{}, errors.New("work_limit: workspace/symbol response exceeds 1 MiB")
	}
	var symbols []lspWorkspaceSymbol
	if len(raw) == 0 || json.Unmarshal(raw, &symbols) != nil {
		return model.WorkerResponse{}, errors.New("invalid_response: gopls workspace/symbol returned an invalid list")
	}
	matching := make([]lspWorkspaceSymbol, 0, len(symbols))
	for _, candidate := range symbols {
		if candidate.Name == symbol {
			matching = append(matching, candidate)
		}
	}
	const candidateLimit = 32
	candidateItems := make([]map[string]any, 0, min(len(matching), candidateLimit))
	if len(matching) > candidateLimit {
		for _, candidate := range matching[:candidateLimit] {
			item := map[string]any{"name": candidate.Name, "kind": candidate.Kind, "container_name": candidate.ContainerName}
			if candidate.Location.Range != nil {
				item["line"] = candidate.Location.Range.Start.Line + 1
				item["column"] = candidate.Location.Range.Start.Character + 1
			}
			if candidate.Location.URI != "" {
				path, pathErr := nativeRenameURIToPath(candidate.Location.URI)
				if pathErr != nil {
					return model.WorkerResponse{}, pathErr
				}
				if file, pathErr := diagnosticFile(c.workspace.Root, map[string]any{"file": path}); pathErr == nil && !workerPathContainsGit(file, c.workspace.Root) {
					if relative, relErr := filepath.Rel(c.workspace.Root, file); relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
						item["file"] = filepath.ToSlash(relative)
					}
				}
			}
			candidateItems = append(candidateItems, item)
		}
		return model.WorkerResponse{Ok: false, ErrorCode: "work_limit", Items: candidateItems}, nil
	}
	verifiedItems := make([]map[string]any, 0, renameSelectorCandidateLimit)
	seen := map[string]bool{}
	var sourceBytes int64
	for _, candidate := range matching {
		if len(verifiedItems) >= renameSelectorCandidateLimit {
			break
		}
		if err := ctx.Err(); err != nil {
			return model.WorkerResponse{}, err
		}
		path, pathErr := nativeRenameURIToPath(candidate.Location.URI)
		if pathErr != nil {
			return model.WorkerResponse{}, pathErr
		}
		if candidate.Location.Range == nil {
			return model.WorkerResponse{}, errors.New("explicit_incomplete: workspace symbol omitted a resolvable range")
		}
		file, err := diagnosticFile(c.workspace.Root, map[string]any{"file": path})
		if err != nil || workerPathContainsGit(file, c.workspace.Root) || !strings.EqualFold(filepath.Ext(file), ".go") {
			return model.WorkerResponse{}, errors.New("invalid_path: workspace symbol escaped the allowed Go workspace")
		}
		start := candidate.Location.Range.Start
		key := filepath.Clean(file) + fmt.Sprintf("\x00%d:%d", start.Line, start.Character)
		if seen[key] {
			continue
		}
		seen[key] = true
		if start.Line < 0 || start.Line >= 1<<24 || start.Character < 0 || start.Character >= 1<<20 {
			return model.WorkerResponse{}, errors.New("invalid_response: workspace symbol position is out of bounds")
		}
		line, column := start.Line+1, start.Character+1
		remainingBytes := int64(renameSelectorSourceByteLimit) - sourceBytes
		if remainingBytes <= 0 {
			break
		}
		if err := c.openDocumentForSelector(file, remainingBytes); err != nil {
			if len(verifiedItems) > 0 {
				break
			}
			return model.WorkerResponse{Ok: false, ErrorCode: "work_limit"}, nil
		}
		opened, ok := c.openDocs[filepath.Clean(file)]
		if !ok || int64(len(opened.content)) > remainingBytes {
			return model.WorkerResponse{Ok: false, ErrorCode: "work_limit"}, nil
		}
		if !utf8.ValidString(opened.content) {
			return model.WorkerResponse{}, errors.New("explicit_incomplete: workspace-symbol source is not valid UTF-8")
		}
		sourceBytes += int64(len(opened.content))
		preparedRaw, err := c.sendRequest("textDocument/prepareRename", map[string]any{
			"textDocument": map[string]any{"uri": buildFileURI(file)},
			"position":     lspPositionMap(line, column),
		})
		if err != nil {
			return model.WorkerResponse{}, fmt.Errorf("explicit_incomplete: could not verify every workspace-symbol candidate with prepareRename: %w", err)
		}
		trimmed := bytes.TrimSpace(preparedRaw)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}
		var prepared struct {
			Placeholder string `json:"placeholder"`
		}
		if json.Unmarshal(trimmed, &prepared) != nil {
			return model.WorkerResponse{}, errors.New("invalid_response: prepareRename candidate result was malformed")
		}
		if prepared.Placeholder != symbol {
			continue
		}
		relative, err := filepath.Rel(c.workspace.Root, file)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return model.WorkerResponse{}, errors.New("invalid_path: workspace-symbol candidate is outside the worker root")
		}
		digest := sha256.Sum256([]byte(opened.content))
		selector := model.NativeSymbolSelector{
			Version: 1, Workspace: request.WorkspaceName, RepoID: request.RepoID,
			WorkspaceRoot: request.Workspace, RepoRoot: request.RepoRoot,
			File: filepath.ToSlash(relative), Line: line, Column: column, Name: symbol,
			SHA256: hex.EncodeToString(digest[:]),
		}
		workspacePhysical, pathErr := filepath.EvalSymlinks(selector.WorkspaceRoot)
		repoPhysical, repoErr := filepath.EvalSymlinks(selector.RepoRoot)
		filePhysical, fileErr := filepath.EvalSymlinks(file)
		if pathErr != nil || repoErr != nil || fileErr != nil || !workerPathWithin(workspacePhysical, repoPhysical) || !workerPathWithin(repoPhysical, filePhysical) {
			continue
		}
		selector.WorkspaceRoot = filepath.Clean(workspacePhysical)
		selector.RepoRoot = filepath.Clean(repoPhysical)
		selector.File, err = filepath.Rel(selector.RepoRoot, filePhysical)
		if err != nil {
			continue
		}
		selector.File = filepath.ToSlash(selector.File)
		opaque, err := model.EncodeNativeSymbolSelector(selector)
		if err != nil {
			continue
		}
		verifiedItems = append(verifiedItems, map[string]any{"name": symbol, "symbol": opaque, "file": filepath.ToSlash(relative), "line": line, "column": column, "kind": candidate.Kind, "container_name": candidate.ContainerName})
	}
	if len(verifiedItems) == 0 {
		return model.WorkerResponse{Ok: false, ErrorCode: "no_semantic_match", Items: verifiedItems}, nil
	}
	// A returned workspace/symbol subset does not prove global uniqueness.
	return model.WorkerResponse{Ok: false, ErrorCode: "selection_required", Items: verifiedItems}, nil
}

func workerPathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (c *LSPClient) renameSymbol(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	if c.backendName() != "gopls" {
		return model.WorkerResponse{}, errors.New("unsupported_operation: semantic rename is currently enabled only for gopls")
	}
	if err := ctx.Err(); err != nil {
		return model.WorkerResponse{}, err
	}
	if !c.supportsPrepareRename() {
		return model.WorkerResponse{}, errors.New("unsupported_operation: gopls did not advertise safe prepareRename support")
	}
	symbol, _ := request.Payload["symbol"].(string)
	newName, _ := request.Payload["new_name"].(string)
	fileValue, _ := request.Payload["file"].(string)
	line := intFromPayload(request.Payload, "line", 0)
	column := intFromPayload(request.Payload, "column", 0)
	if !token.IsIdentifier(symbol) || !token.IsIdentifier(newName) || symbol == newName || fileValue == "" || line < 1 || column < 1 {
		return model.WorkerResponse{}, errors.New("invalid_input: rename requires distinct Go identifiers and an explicit UTF-16 position")
	}
	file, err := diagnosticFile(c.workspace.Root, map[string]any{"file": fileValue})
	if err != nil {
		return model.WorkerResponse{}, err
	}
	if workerPathContainsGit(file, c.workspace.Root) {
		return model.WorkerResponse{}, errors.New("invalid_path: rename target is inside .git")
	}
	expectedSelectorDigest, _ := request.Payload["selector_sha256"].(string)
	files, err := renameInputFiles(request.Payload["files"], file, c.workspace.Root)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	for _, candidate := range files {
		var openErr error
		if expectedSelectorDigest != "" && filepath.Clean(candidate) == filepath.Clean(file) {
			openErr = c.openDocumentForSelector(candidate, maxLSPDocumentBytes)
		} else {
			openErr = c.openDocument(candidate)
		}
		if openErr != nil {
			return model.WorkerResponse{}, fmt.Errorf("unavailable: could not open rename source: %w", openErr)
		}
	}
	if expected := expectedSelectorDigest; expected != "" {
		opened, ok := c.openDocs[filepath.Clean(file)]
		if !ok {
			return model.WorkerResponse{}, errors.New("explicit_incomplete: selector source snapshot is unavailable")
		}
		digest := sha256.Sum256([]byte(opened.content))
		if hex.EncodeToString(digest[:]) != expected {
			return model.WorkerResponse{}, errors.New("explicit_incomplete: selector source snapshot is stale")
		}
	}
	if err := validateUTF16Position(file, line, column); err != nil {
		return model.WorkerResponse{}, fmt.Errorf("invalid_position: %w", err)
	}
	position := lspPositionMap(line, column)
	params := map[string]any{"textDocument": map[string]any{"uri": buildFileURI(file)}, "position": position}
	preparedRaw, err := c.sendRequest("textDocument/prepareRename", params)
	if err != nil {
		return model.WorkerResponse{}, fmt.Errorf("unsupported_operation: prepareRename failed: %w", err)
	}
	var prepared struct {
		Placeholder string `json:"placeholder"`
	}
	if len(preparedRaw) == 0 || bytes.Equal(bytes.TrimSpace(preparedRaw), []byte("null")) || json.Unmarshal(preparedRaw, &prepared) != nil || prepared.Placeholder != symbol {
		return model.WorkerResponse{}, errors.New("unsupported_operation: prepareRename did not confirm the selected symbol")
	}
	referenceFiles, err := c.nativeRenameReferenceFiles(file, line, column)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	seenFiles := map[string]bool{}
	for _, candidate := range files {
		seenFiles[filepath.Clean(candidate)] = true
	}
	for _, referenceFile := range referenceFiles {
		key := filepath.Clean(referenceFile)
		if seenFiles[key] {
			continue
		}
		if len(files) >= maxLSPOpenDocuments {
			return model.WorkerResponse{}, errors.New("work_limit: native rename references exceed 32 files")
		}
		if err := c.openDocument(referenceFile); err != nil {
			return model.WorkerResponse{}, fmt.Errorf("unavailable: could not open verified rename reference: %w", err)
		}
		seenFiles[key] = true
		files = append(files, referenceFile)
	}
	params["newName"] = newName
	workspaceEdit, err := c.sendRequest("textDocument/rename", params)
	if err != nil {
		return model.WorkerResponse{}, fmt.Errorf("unavailable: gopls rename request failed: %w", err)
	}
	var plan struct {
		Changes         map[string]json.RawMessage `json:"changes"`
		DocumentChanges []json.RawMessage          `json:"documentChanges"`
	}
	if len(workspaceEdit) == 0 || json.Unmarshal(workspaceEdit, &plan) != nil || len(plan.Changes) != 0 || len(plan.DocumentChanges) == 0 || len(plan.DocumentChanges) > maxLSPOpenDocuments {
		return model.WorkerResponse{}, errors.New("unsupported_operation: gopls did not return a bounded versioned documentChanges plan")
	}
	versions := map[string]int{}
	snapshots := map[string]string{}
	seen := map[string]bool{}
	totalEdits := 0
	for _, rawEdit := range plan.DocumentChanges {
		var document lspRenameDocumentChange
		if json.Unmarshal(rawEdit, &document) != nil || document.TextDocument == nil || document.TextDocument.URI == "" || document.TextDocument.Version == nil || len(document.Edits) == 0 || document.Kind != "" {
			return model.WorkerResponse{}, errors.New("unsupported_operation: rename plan includes an unversioned edit or resource operation")
		}
		uri := document.TextDocument.URI
		if seen[uri] {
			return model.WorkerResponse{}, errors.New("invalid_response: duplicate document in gopls rename plan")
		}
		seen[uri] = true
		path, err := nativeRenameURIToPath(uri)
		if err != nil {
			return model.WorkerResponse{}, err
		}
		validated, err := diagnosticFile(c.workspace.Root, map[string]any{"file": path})
		if err != nil || workerPathContainsGit(validated, c.workspace.Root) {
			return model.WorkerResponse{}, errors.New("invalid_path: gopls rename plan contains a path outside the allowed workspace")
		}
		opened, ok := c.openDocs[filepath.Clean(validated)]
		if !ok || opened.version != *document.TextDocument.Version || !utf8.ValidString(opened.content) {
			return model.WorkerResponse{}, errors.New("explicit_incomplete: gopls rename plan version does not match the opened source snapshot")
		}
		totalEdits += len(document.Edits)
		if totalEdits > 5000 {
			return model.WorkerResponse{}, errors.New("work_limit: gopls rename plan exceeds 5000 edits")
		}
		converted := make([][2]int, 0, len(document.Edits))
		for _, edit := range document.Edits {
			if edit.NewText != newName || strings.ContainsAny(edit.NewText, "\r\n\x00") {
				return model.WorkerResponse{}, errors.New("unsupported_operation: gopls rename plan contains a non-identifier edit")
			}
			start, okStart := workerUTF16ByteOffset([]byte(opened.content), edit.Range.Start.Line, edit.Range.Start.Character)
			end, okEnd := workerUTF16ByteOffset([]byte(opened.content), edit.Range.End.Line, edit.Range.End.Character)
			if !okStart || !okEnd || start > end || string(opened.content[start:end]) != symbol {
				return model.WorkerResponse{}, errors.New("explicit_incomplete: gopls edit does not match the confirmed symbol snapshot")
			}
			converted = append(converted, [2]int{start, end})
		}
		sort.Slice(converted, func(i, j int) bool { return converted[i][0] < converted[j][0] })
		for i := 1; i < len(converted); i++ {
			if converted[i][0] < converted[i-1][1] {
				return model.WorkerResponse{}, errors.New("invalid_response: overlapping edits in gopls rename plan")
			}
		}
		digest := sha256.Sum256([]byte(opened.content))
		versions[uri] = opened.version
		snapshots[uri] = hex.EncodeToString(digest[:])
	}
	referenceItems := make([]map[string]any, 0, len(files))
	for _, candidate := range files {
		referenceItems = append(referenceItems, map[string]any{"file": candidate})
	}
	return model.WorkerResponse{
		Ok: true, Backend: c.backendName(), WorkspaceEdit: workspaceEdit, Items: referenceItems,
		DocumentSnapshots: snapshots, DocumentVersions: versions, RenamePlaceholder: prepared.Placeholder,
		Stats: model.Stats{Files: len(plan.DocumentChanges), Symbols: totalEdits},
	}, nil
}

// nativeRenameURIToPath applies native rename's local-only URI policy before
// delegating valid file path decoding to the legacy worker helper.
func nativeRenameURIToPath(uri string) (string, error) {
	if !strings.HasPrefix(uri, "file://") {
		return "", errors.New("invalid_path: native rename requires a file URI")
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return "", errors.New("invalid_path: native rename received a malformed file URI")
	}
	if parsed.User != nil || (parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost")) {
		return "", errors.New("invalid_path: native rename rejects non-local file URI authorities and userinfo")
	}
	path := uriToPath(uri)
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("invalid_path: native rename file URI has no absolute local path")
	}
	return filepath.Clean(path), nil
}

// nativeRenameReferenceFiles keeps raw LSP authorities available for validation
// before native rename turns references into local paths.
func (c *LSPClient) nativeRenameReferenceFiles(file string, line, column int) ([]string, error) {
	raw, err := c.sendSemanticRequest("textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": buildFileURI(file)},
		"position":     lspPositionMap(line, column),
		"context":      map[string]any{"includeDeclaration": true},
	})
	if err != nil {
		return nil, fmt.Errorf("unavailable: gopls references request failed: %w", err)
	}
	var locations []lspLocation
	if len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &locations); err != nil {
			return nil, fmt.Errorf("invalid_response: gopls references response is malformed: %w", err)
		}
	}
	if len(locations) == 0 {
		return nil, errors.New("explicit_incomplete: gopls returned no semantic references for native rename")
	}
	files := []string{file}
	seen := map[string]bool{filepath.Clean(file): true}
	for _, location := range locations {
		path, err := nativeRenameURIToPath(location.URI)
		if err != nil {
			return nil, err
		}
		validated, err := diagnosticFile(c.workspace.Root, map[string]any{"file": path})
		if err != nil || workerPathContainsGit(validated, c.workspace.Root) {
			return nil, errors.New("invalid_path: native rename reference escaped the allowed workspace")
		}
		key := filepath.Clean(validated)
		if seen[key] {
			continue
		}
		if len(files) >= maxLSPOpenDocuments {
			return nil, errors.New("work_limit: native rename references exceed 32 files")
		}
		seen[key] = true
		files = append(files, validated)
	}
	return files, nil
}

func renameInputFiles(raw any, anchor, root string) ([]string, error) {
	var paths []string
	switch value := raw.(type) {
	case []string:
		paths = append(paths, value...)
	case []any:
		for _, item := range value {
			path, ok := item.(string)
			if !ok {
				return nil, errors.New("invalid_input: rename files must be paths")
			}
			paths = append(paths, path)
		}
	case nil:
	default:
		return nil, errors.New("invalid_input: rename files must be an array")
	}
	paths = append(paths, anchor)
	if len(paths) > maxLSPOpenDocuments {
		return nil, errors.New("work_limit: rename plan exceeds the open-document limit")
	}
	result, seen := make([]string, 0, len(paths)), map[string]bool{}
	for _, path := range paths {
		validated, err := diagnosticFile(root, map[string]any{"file": path})
		if err != nil || workerPathContainsGit(validated, root) {
			return nil, errors.New("invalid_path: rename source is outside the selected workspace")
		}
		key := filepath.Clean(validated)
		if !seen[key] {
			seen[key] = true
			result = append(result, validated)
		}
	}
	if len(result) > maxLSPOpenDocuments {
		return nil, errors.New("work_limit: rename plan exceeds the open-document limit")
	}
	return result, nil
}

func workerUTF16ByteOffset(data []byte, line, character int) (int, bool) {
	if line < 0 || character < 0 || !utf8.Valid(data) {
		return 0, false
	}
	start, currentLine := 0, 0
	for currentLine < line {
		next := bytes.IndexByte(data[start:], '\n')
		if next < 0 {
			return 0, false
		}
		start += next + 1
		currentLine++
	}
	end := len(data)
	if next := bytes.IndexByte(data[start:], '\n'); next >= 0 {
		end = start + next
	}
	if end > start && data[end-1] == '\r' {
		end--
	}
	units, offset := 0, start
	for offset < end {
		if units == character {
			return offset, true
		}
		r, size := utf8.DecodeRune(data[offset:end])
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > character {
			return 0, false
		}
		units += width
		offset += size
	}
	return offset, units == character
}

func workerPathContainsGit(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.EqualFold(segment, ".git") {
			return true
		}
	}
	return false
}
