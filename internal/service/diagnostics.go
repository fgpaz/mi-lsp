package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

const diagnosticsMaxPaths = 256

type diagnosticJob struct {
	path    string
	request model.WorkerRequest
}
type diagnosticResult struct {
	path     string
	response model.WorkerResponse
	err      error
}

func (a *App) diagnostics(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	registration, project, err := diagnosticsWorkspace(ctx, request)
	if err != nil {
		return diagnosticsFailure("", "invalid_workspace", "physical workspace resolution unavailable; select the intended workspace explicitly"), nil
	}
	changed := boolPayload(request.Payload, "changed")
	file := stringPayload(request.Payload, "file")
	if changed == (file != "") {
		return model.Envelope{}, errors.New("diagnostics requires one file or --changed")
	}
	if !changed && stringPayload(request.Payload, "base") != "" {
		return model.Envelope{}, errors.New("--base requires --changed")
	}
	maxFiles := intFromAny(request.Payload["max_files"], 32)
	if maxFiles < 1 || maxFiles > 64 {
		return model.Envelope{}, errors.New("max_files must be between 1 and 64")
	}
	offset := intFromAny(request.Payload["offset"], 0)
	if offset < 0 {
		return model.Envelope{}, errors.New("offset must be nonnegative")
	}
	paths := []string{file}
	pathLimit := false
	if changed {
		paths, pathLimit, err = diagnosticsChangedPaths(ctx, registration.Root, stringPayload(request.Payload, "base"))
		if err != nil {
			code := "unavailable"
			if ctx.Err() != nil {
				code = "timeout"
			}
			var stable *model.StableError
			if errors.As(err, &stable) && stable.Code == "work_limit" {
				code = "work_limit"
			}
			return diagnosticsFailure(registration.Name, code, "git change discovery unavailable or work limit reached; request files individually; no clean result inferred"), nil
		}
	}
	summary := &model.DiagnosticsSummary{FilesRequested: len(paths), Backends: map[string]int{}}
	env := model.Envelope{Ok: true, ContractVersion: "diagnostics/v1", Workspace: registration.Name, Backend: "semantic-diagnostics", Items: []model.Diagnostic{}, Diagnostics: summary}
	omit := func(path, code, reason string) {
		env.Partial = true
		env.Omissions = append(env.Omissions, model.EnvelopeOmission{Path: path, ErrorCode: code, Reason: reason})
	}
	if pathLimit {
		env.Truncated = true
		omit("", "work_limit", "changed path discovery exceeded 256 paths; use individual files")
	}
	groups := map[string][]diagnosticJob{}
	admitted := 0
	for _, path := range paths {
		absolute, relative, code := diagnosticsPath(registration.Root, path)
		if code != "" {
			omit(path, code, "source unavailable or outside selected workspace")
			continue
		}
		if admitted >= maxFiles {
			omit(relative, "work_limit", "max_files reached; request this file individually")
			continue
		}
		perFile := request
		perFile.Payload = map[string]any{"file": relative}
		// No blanket backend override for mixed-language diffs. Reuse normal source
		// routing, never the catalog/text fallback used by navigation.
		backend, supported := backendForSourcePath(relative)
		if !supported || backend == "kotlin-language-server" {
			omit(relative, "unsupported_operation", "language has no diagnostic backend")
			continue
		}
		if request.Context.BackendHint != "" && request.Context.BackendHint != backend {
			omit(relative, "unsupported_operation", "backend override does not match source language")
			continue
		}
		target, ambiguity, targetErr := a.resolveSemanticTarget(ctx, registration, project, perFile, "diagnostics", backend)
		if targetErr != nil || ambiguity != nil {
			omit(relative, "unavailable", "semantic target/entrypoint unavailable")
			continue
		}
		repoRoot := filepath.Join(registration.Root, filepath.FromSlash(target.Repo.Root))
		if _, _, code := diagnosticsPath(registration.Root, repoRoot); code != "" {
			// diagnosticsPath expects a regular file. Validate directory separately.
			physical, e := filepath.EvalSymlinks(repoRoot)
			if e != nil || !diagnosticsInside(registration.Root, physical) {
				omit(relative, "invalid_path", "semantic repo outside workspace")
				continue
			}
		}
		payload := map[string]any{"file": absolute}
		if target.Entrypoint.Kind == model.EntrypointKindSolution {
			payload["solution"] = target.Entrypoint.Path
		}
		if target.Entrypoint.Kind == model.EntrypointKindProject {
			payload["project_path"] = target.Entrypoint.Path
		}
		if target.Entrypoint.Path != "" {
			candidate := target.Entrypoint.Path
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(registration.Root, filepath.FromSlash(candidate))
			}
			physical, e := filepath.EvalSymlinks(candidate)
			if e != nil || !diagnosticsInside(registration.Root, physical) {
				omit(relative, "invalid_path", "semantic entrypoint outside workspace or missing")
				continue
			}
		}
		wr := model.WorkerRequest{ProtocolVersion: model.ProtocolVersion, Method: "diagnostics", Workspace: registration.Root, WorkspaceName: registration.Name, BackendType: backend, RepoID: target.Repo.ID, RepoName: target.Repo.Name, RepoRoot: repoRoot, EntrypointID: target.Entrypoint.ID, EntrypointPath: target.Entrypoint.Path, EntrypointType: target.Entrypoint.Kind, Payload: payload}
		if backend == "rust-analyzer" {
			wr.EntrypointID += "::diagnostics-native"
		}
		groups[backend] = append(groups[backend], diagnosticJob{relative, wr})
		admitted++
	}
	// Five backend lanes are independent. Requests sharing a backend are bounded
	// and sequential to avoid fighting over its runtime's mutable document state.
	results := make(chan diagnosticResult, admitted)
	var wg sync.WaitGroup
	for _, jobs := range groups {
		wg.Add(1)
		go func(jobs []diagnosticJob) {
			defer wg.Done()
			timedOut := false
			for _, job := range jobs {
				if timedOut {
					results <- diagnosticResult{path: job.path, response: model.WorkerResponse{Backend: job.request.BackendType}, err: context.DeadlineExceeded}
					continue
				}
				callCtx, done := context.WithTimeout(ctx, 8*time.Second)
				response, err := a.diagnosticsCall(callCtx, registration, job.request)
				if response.Backend == "" {
					response.Backend = job.request.BackendType
				}
				done()
				timedOut = errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
				results <- diagnosticResult{job.path, response, err}
			}
		}(jobs)
	}
	wg.Wait()
	close(results)
	collected := []diagnosticResult{}
	for result := range results {
		collected = append(collected, result)
	}
	sort.Slice(collected, func(i, j int) bool { return collected[i].path < collected[j].path })
	items := []model.Diagnostic{}
	seen := map[string]bool{}
	for _, result := range collected {
		response := result.response
		if result.err != nil || !response.Ok {
			code := "unavailable"
			if errors.Is(result.err, context.DeadlineExceeded) || errors.Is(result.err, context.Canceled) {
				code = "timeout"
			}
			if response.ErrorCode != "" {
				code = response.ErrorCode
			}
			if result.err != nil {
				for _, candidate := range []string{"unsupported_operation", "invalid_path", "file_missing", "work_limit"} {
					if strings.HasPrefix(result.err.Error(), candidate+":") {
						code = candidate
						break
					}
				}
			}
			omit(result.path, code, response.Backend+" semantic diagnostic request failed; no clean result inferred")
			continue
		}
		summary.FilesChecked++
		summary.Backends[response.Backend]++
		if response.Partial {
			omit(result.path, "explicit_incomplete", response.Reason)
		}
		for _, warning := range response.Warnings {
			env.Warnings = appendStringIfMissing(env.Warnings, warning)
		}
		for _, raw := range response.Items {
			encoded, e := json.Marshal(raw)
			if e != nil {
				omit(result.path, "invalid_response", "diagnostic could not be decoded")
				continue
			}
			var d model.Diagnostic
			if json.Unmarshal(encoded, &d) != nil || d.Line < 1 || d.Column < 1 || d.Message == "" || diagnosticSeverityRank(d.Severity) > 3 {
				omit(result.path, "invalid_response", "diagnostic schema invalid")
				continue
			}
			switch d.Code.(type) {
			case nil, string, float64:
			default:
				omit(result.path, "invalid_response", "diagnostic code must be string or number")
				continue
			}
			_, rel, code := diagnosticsPath(registration.Root, d.Path)
			if code != "" || rel != result.path {
				omit(result.path, "invalid_response", "backend diagnostic escaped requested file")
				continue
			}
			d.Path = rel
			keyBytes, _ := json.Marshal(d)
			key := string(keyBytes)
			if seen[key] {
				continue
			}
			seen[key] = true
			switch d.Severity {
			case "error":
				summary.Errors++
			case "warning":
				summary.Warnings++
			case "information":
				summary.Information++
			case "hint":
				summary.Hints++
			}
			items = append(items, d)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Severity != b.Severity {
			return diagnosticSeverityRank(a.Severity) < diagnosticSeverityRank(b.Severity)
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Message != b.Message {
			return a.Message < b.Message
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		ac, _ := json.Marshal(a.Code)
		bc, _ := json.Marshal(b.Code)
		return string(ac) < string(bc)
	})
	total := len(items)
	if offset > total {
		offset = total
	}
	items = items[offset:]
	limit := request.Context.MaxItems
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if len(items) > limit {
		items = items[:limit]
		env.Truncated = true
	}
	if env.Truncated || offset > 0 {
		hint := fmt.Sprintf("diagnostics snapshot: %d total; rerun with --offset %d or request an omitted file; raise --max-files/--token-budget if needed", total, offset+len(items))
		env.NextHint = &hint
	}
	summary.Returned = len(items)
	env.Items = items
	env.Stats = model.Stats{Files: summary.FilesChecked, Symbols: total}
	env.Hint = fmt.Sprintf("%d error(s), %d warning(s); %d/%d files checked", summary.Errors, summary.Warnings, summary.FilesChecked, summary.FilesRequested)
	if env.Partial {
		env.Degraded = true
		env.Reason = "explicit_incomplete"
		env.Hint += "; partial, not a clean bill of health"
	}
	if summary.FilesRequested > 0 && summary.FilesChecked == 0 {
		env.Ok = false
		// No checked source is a failed query, with an actionable typed cause.
		// A generic partial flag alone loses the reason at the CLI/MCP boundary.
		code, detail := "unavailable", "no semantic source could be checked"
		if len(env.Omissions) > 0 {
			code, detail = env.Omissions[0].ErrorCode, env.Omissions[0].Reason
			for _, omission := range env.Omissions[1:] {
				if omission.ErrorCode != code {
					code, detail = "unavailable", "multiple diagnostic failures; inspect omissions for each source"
					break
				}
			}
		}
		env.Reason = code
		env.Error = &model.EnvelopeError{Kind: "diagnostics", Code: code, ReasonCode: code, Message: detail, Detail: detail}
	}
	return boundDiagnostics(env, request.Context), nil
}

// Startup/discovery may not yet honor context. Never let one backend hold the
// aggregate indefinitely; its lane stops admitting work after the first timeout.
func (a *App) diagnosticsCall(ctx context.Context, reg model.WorkspaceRegistration, req model.WorkerRequest) (model.WorkerResponse, error) {
	if err := ctx.Err(); err != nil {
		return model.WorkerResponse{}, err
	}
	type result struct {
		response model.WorkerResponse
		err      error
	}
	completed := make(chan result, 1)
	go func() { r, e := a.Semantic.Call(ctx, reg, req); completed <- result{r, e} }()
	select {
	case r := <-completed:
		return r.response, r.err
	case <-ctx.Done():
		return model.WorkerResponse{}, ctx.Err()
	}
}

func diagnosticsFailure(workspaceName, code, reason string) model.Envelope {
	return model.Envelope{ContractVersion: "diagnostics/v1", Ok: false, Partial: true, Degraded: true, Reason: code, Hint: reason, Workspace: workspaceName, Backend: "semantic-diagnostics", Items: []model.Diagnostic{}, Diagnostics: &model.DiagnosticsSummary{Backends: map[string]int{}}, Error: &model.EnvelopeError{Kind: code, Code: code, ReasonCode: code, Message: reason}, Omissions: []model.EnvelopeOmission{{ErrorCode: code, Reason: reason}}}
}

func diagnosticSeverityRank(s string) int {
	switch s {
	case "error":
		return 0
	case "warning":
		return 1
	case "information":
		return 2
	case "hint":
		return 3
	default:
		return 4
	}
}

func diagnosticsInside(root, path string) bool {
	physical, e := filepath.EvalSymlinks(root)
	if e != nil {
		return false
	}
	rel, e := filepath.Rel(physical, path)
	return e == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func diagnosticsPath(root, path string) (string, string, string) {
	if path == "" || strings.ContainsRune(path, 0) {
		return "", "", "invalid_path"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	path = filepath.Clean(path)
	if !diagnosticsInside(root, path) {
		return "", "", "invalid_path"
	}
	physical, e := filepath.EvalSymlinks(path)
	if os.IsNotExist(e) {
		return "", "", "file_missing"
	}
	if e != nil || !diagnosticsInside(root, physical) {
		return "", "", "invalid_path"
	}
	info, e := os.Stat(physical)
	if e != nil || !info.Mode().IsRegular() {
		return "", "", "invalid_path"
	}
	if info.Size() > 2<<20 {
		return "", "", "work_limit"
	}
	rel, e := filepath.Rel(root, path)
	if e != nil {
		return "", "", "invalid_path"
	}
	return path, filepath.ToSlash(rel), ""
}

// Keep linked worktrees physically distinct even when the registry aliases the
// canonical checkout. Only equal git common directories authorize this mapping.
func diagnosticsWorkspace(ctx context.Context, request model.CommandRequest) (model.WorkspaceRegistration, model.ProjectFile, error) {
	resolution, err := workspace.ResolveWorkspaceSelectionReadOnlyPhysical(request.Context.Workspace, request.Context.CallerCWD)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, err
	}
	reg := resolution.Registration
	var project model.ProjectFile
	if request.Context.CallerCWD != "" {
		top, e := diagnosticsGit(ctx, request.Context.CallerCWD, "rev-parse", "--show-toplevel")
		if e == nil {
			root := strings.TrimSpace(string(top))
			selectedCommon, e1 := diagnosticsGit(ctx, reg.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
			callerCommon, e2 := diagnosticsGit(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
			if e1 == nil && e2 == nil && bytes.Equal(bytes.TrimSpace(selectedCommon), bytes.TrimSpace(callerCommon)) {
				reg.Root = root
			} else if request.Context.Workspace == "" {
				return reg, project, errors.New("invalid_workspace: caller git root differs from selected workspace; select a workspace explicitly")
			}
		}
	}
	physical, e := filepath.EvalSymlinks(reg.Root)
	if e != nil {
		return reg, project, errors.New("invalid_workspace: physical root unavailable")
	}
	reg.Root = physical
	project, err = workspace.LoadProjectTopology(reg.Root, reg)
	return reg, project, err
}

type diagnosticsGitBuffer struct {
	bytes.Buffer
	limited bool
}

func (b *diagnosticsGitBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		b.limited = true
		return 0, model.NewStableError("work_limit")
	}
	return b.Buffer.Write(p)
}
func diagnosticsGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false"}, args...)...)
	processutil.ConfigureNonInteractiveCommand(cmd)
	cmd.Dir = root
	// Ambient git overrides may not redirect diagnostics into another checkout.
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	var output diagnosticsGitBuffer
	cmd.Stdout = &output
	err := cmd.Run()
	if output.limited {
		return nil, model.NewStableError("work_limit")
	}
	if err != nil {
		return nil, fmt.Errorf("unavailable: git diagnostic path discovery failed: %w", err)
	}
	return output.Bytes(), nil
}
func diagnosticsChangedPaths(ctx context.Context, root, base string) ([]string, bool, error) {
	commands := [][]string{{"diff", "--no-ext-diff", "--no-textconv", "--name-only", "--no-renames", "-z", "--"}, {"diff", "--cached", "--no-ext-diff", "--no-textconv", "--name-only", "--no-renames", "-z", "--"}, {"ls-files", "--others", "--exclude-standard", "-z", "--"}}
	if base != "" {
		// Resolve to an object id before it reaches diff; refs are not argv options.
		resolved, err := diagnosticsGit(ctx, root, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
		if err != nil {
			return nil, false, err
		}
		merge, err := diagnosticsGit(ctx, root, "merge-base", strings.TrimSpace(string(resolved)), "HEAD")
		if err != nil {
			return nil, false, err
		}
		commands = append(commands, []string{"diff", "--no-ext-diff", "--no-textconv", "--name-only", "--no-renames", "-z", strings.TrimSpace(string(merge)), "--"})
	}
	paths := map[string]bool{}
	for _, args := range commands {
		output, err := diagnosticsGit(ctx, root, args...)
		if err != nil {
			return nil, false, err
		}
		for _, p := range bytes.Split(output, []byte{0}) {
			if len(p) > 0 {
				paths[string(p)] = true
			}
		}
	}
	ordered := make([]string, 0, len(paths))
	for p := range paths {
		ordered = append(ordered, p)
	}
	// Spend discovery capacity on semantic source paths before documentation or
	// generated metadata; every discarded path still has an explicit limit reason.
	sort.Slice(ordered, func(i, j int) bool {
		bi, si := backendForSourcePath(ordered[i])
		bj, sj := backendForSourcePath(ordered[j])
		si = si && bi != "kotlin-language-server"
		sj = sj && bj != "kotlin-language-server"
		if si != sj {
			return si
		}
		return ordered[i] < ordered[j]
	})
	truncated := len(ordered) > diagnosticsMaxPaths
	if truncated {
		ordered = ordered[:diagnosticsMaxPaths]
	}
	return ordered, truncated, nil
}
