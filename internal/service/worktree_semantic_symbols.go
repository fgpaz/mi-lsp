package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type physicalSymbol struct {
	model.SymbolRecord
	selectionLine   int
	selectionColumn int
}

// physicalDocumentSymbols asks the existing native backend for structure in the
// physical worktree. Document symbols are structural, not compiler identities.
func (a *App) physicalDocumentSymbols(ctx context.Context, registration model.WorkspaceRegistration, project model.ProjectFile, request model.CommandRequest, file string) ([]physicalSymbol, error) {
	backend, ok := backendForSourcePath(file)
	if !ok || a.Semantic == nil {
		return nil, fmt.Errorf("document symbols unavailable for %s", file)
	}
	if backend == "kotlin-language-server" {
		selector, _ := request.Payload["entrypoint"].(string)
		if strings.TrimSpace(selector) == "" {
			selector, _ = request.Payload["project_path"].(string)
		}
		if strings.TrimSpace(selector) == "" {
			return nil, fmt.Errorf("Kotlin requires an explicit module entrypoint")
		}
	}
	request.Payload = clonePayload(request.Payload)
	request.Payload["file"] = file
	target, envelope, err := a.resolveSemanticTarget(ctx, registration, project, request, "document_symbols", backend)
	if err != nil {
		return nil, err
	}
	if envelope != nil {
		return nil, fmt.Errorf("semantic target unavailable for %s", file)
	}
	absolute := file
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(registration.Root, filepath.FromSlash(file))
	}
	rootReal, err := filepath.EvalSymlinks(registration.Root)
	if err != nil {
		return nil, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(rootReal, absolute)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("document outside physical worktree")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("physical document unavailable")
	}
	payload := map[string]any{"file": absolute}
	if target.Entrypoint.Path != "" {
		switch target.Entrypoint.Kind {
		case model.EntrypointKindSolution:
			payload["solution"] = target.Entrypoint.Path
		case model.EntrypointKindProject:
			payload["project_path"] = target.Entrypoint.Path
		}
	}
	wr := model.WorkerRequest{ProtocolVersion: model.ProtocolVersion, Method: "document_symbols", Workspace: registration.Root, WorkspaceName: registration.Name, BackendType: backend, RepoID: target.Repo.ID, RepoName: target.Repo.Name, RepoRoot: filepath.Join(registration.Root, filepath.FromSlash(target.Repo.Root)), EntrypointID: target.Entrypoint.ID, EntrypointPath: target.Entrypoint.Path, EntrypointType: target.Entrypoint.Kind, Payload: payload}
	response, err := a.Semantic.Call(ctx, registration, wr)
	if err != nil {
		return nil, err
	}
	if !response.Ok {
		return nil, fmt.Errorf("document symbols failed for %s", file)
	}
	out := []physicalSymbol{}
	var locationErr error
	var visit func(map[string]any)
	visit = func(node map[string]any) {
		name, _ := node["name"].(string)
		kind := fmt.Sprint(node["kind"])
		start, end := intFromAny(node["line"], 0), intFromAny(node["end_line"], 0)
		if r, ok := node["range"].(map[string]any); ok {
			if pos, ok := r["start"].(map[string]any); ok {
				start = intFromAny(pos["line"], 0) + 1
			}
			if pos, ok := r["end"].(map[string]any); ok {
				end = intFromAny(pos["line"], 0) + 1
			}
		} else if loc, ok := node["location"].(map[string]any); ok {
			rawURI, ok := loc["uri"].(string)
			parsed, err := url.Parse(rawURI)
			if !ok || err != nil || parsed.Scheme != "file" || (parsed.Host != "" && parsed.Host != "localhost") || parsed.RawQuery != "" || parsed.Fragment != "" {
				locationErr = fmt.Errorf("unsupported symbol location URI")
				return
			}
			location, err := filepath.EvalSymlinks(filepath.FromSlash(parsed.Path))
			if err != nil || location != absolute {
				locationErr = fmt.Errorf("symbol location differs from requested physical document")
				return
			}
			if r, ok := loc["range"].(map[string]any); ok {
				if pos, ok := r["start"].(map[string]any); ok {
					start = intFromAny(pos["line"], 0) + 1
				}
				if pos, ok := r["end"].(map[string]any); ok {
					end = intFromAny(pos["line"], 0) + 1
				}
			}
		}
		selectionLine, selectionColumn := intFromAny(node["selection_line"], 0), intFromAny(node["selection_column"], 0)
		if selection, ok := node["selectionRange"].(map[string]any); ok {
			if pos, ok := selection["start"].(map[string]any); ok {
				selectionLine = intFromAny(pos["line"], 0) + 1
				selectionColumn = intFromAny(pos["character"], 0) + 1
			}
		}
		if _, ok := node["location"].(map[string]any); ok && selectionLine == 0 {
			if loc, ok := node["location"].(map[string]any); ok {
				if r, ok := loc["range"].(map[string]any); ok {
					if pos, ok := r["start"].(map[string]any); ok {
						selectionLine = intFromAny(pos["line"], 0) + 1
						selectionColumn = intFromAny(pos["character"], 0) + 1
					}
				}
			}
		}
		if name != "" && start > 0 {
			out = append(out, physicalSymbol{SymbolRecord: model.SymbolRecord{FilePath: file, RepoID: target.Repo.ID, RepoName: target.Repo.Name, Name: name, Kind: kind, StartLine: start, EndLine: end, Origin: "structural"}, selectionLine: selectionLine, selectionColumn: selectionColumn})
		}
		if children, ok := node["children"].([]any); ok {
			for _, child := range children {
				if m, ok := child.(map[string]any); ok {
					visit(m)
				}
			}
		}
	}
	for _, item := range response.Items {
		if node, ok := item["symbol"].(map[string]any); ok {
			visit(node)
		} else {
			visit(item)
		}
	}
	if locationErr != nil {
		return nil, locationErr
	}
	return out, nil
}

func worktreeSymbolName(symbol string) string {
	if i := strings.LastIndex(symbol, "."); i >= 0 {
		return symbol[i+1:]
	}
	return symbol
}
