package actions

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

// El modo immutable no consulta WAL. Rechazar un WAL activo evita afirmar un
// resultado obsoleto y también evita que una inspección cree archivos SHM.
func catalogReadyReadOnly(root string) (bool, error) {
	path := store.WorkspaceDBPath(root)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if info, err := os.Stat(path + "-wal"); err == nil && info.Size() > 0 {
		return false, errors.New("WAL presente")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	db, err := store.OpenReadOnlyExisting(root, path)
	if err != nil {
		return false, err
	}
	defer db.Close()
	ctx := context.Background()
	active, ok, err := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveCatalogGeneration)
	if err != nil {
		return false, err
	}
	if ok && strings.TrimSpace(active) != "" {
		return true, nil
	}
	indexed, ok, err := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaIndexedAt)
	if err != nil {
		return false, err
	}
	if !ok || strings.TrimSpace(indexed) == "" {
		return false, nil
	}
	files, ok, err := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaTotalFiles)
	if err != nil {
		return false, err
	}
	return ok && strings.TrimSpace(files) != "", nil
}

func inspectWorker(toolRoot string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(toolRoot) == "" {
		toolRoot, _ = worker.ResolveToolRoot(".")
	}
	rid := worker.ResolveRID()
	dirs := []string{}
	// Las distribuciones contienen workers/; el repositorio de fuente no es un bundle.
	if _, err := os.Stat(filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker", "MiLsp.Worker.csproj")); errors.Is(err, os.ErrNotExist) {
		dirs = append(dirs, filepath.Join(toolRoot, "workers", rid), filepath.Join(toolRoot, "bin", "workers", rid))
	}
	dirs = append(dirs, filepath.Join(home, ".mi-lsp", "workers", rid))
	exe := "MiLsp.Worker"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	for _, dir := range dirs {
		dll := filepath.Join(dir, "MiLsp.Worker.dll")
		if isFrameworkConfig(filepath.Join(dir, "MiLsp.Worker.runtimeconfig.json")) && probeWorker(dll) {
			return true, nil
		}
	}
	for _, dir := range dirs {
		if probeWorker(filepath.Join(dir, exe)) {
			return true, nil
		}
	}
	return false, nil
}

func isFrameworkConfig(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	type framework struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	var cfg struct {
		RuntimeOptions struct {
			Framework  *framework      `json:"framework"`
			Frameworks []framework     `json:"frameworks"`
			Included   json.RawMessage `json:"includedFrameworks"`
		} `json:"runtimeOptions"`
	}
	if json.Unmarshal(body, &cfg) != nil || cfg.RuntimeOptions.Included != nil {
		return false
	}
	list := cfg.RuntimeOptions.Frameworks
	if cfg.RuntimeOptions.Framework != nil {
		list = append(list, *cfg.RuntimeOptions.Framework)
	}
	if len(list) == 0 {
		return false
	}
	for _, f := range list {
		if strings.TrimSpace(f.Name) == "" || strings.TrimSpace(f.Version) == "" {
			return false
		}
	}
	return true
}

// El handshake no pasa por un shell y no divulga stderr ni salida del worker.
func probeWorker(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	command := path
	args := []string{}
	if strings.HasSuffix(strings.ToLower(path), ".dll") {
		if !isFrameworkConfig(filepath.Join(filepath.Dir(path), "MiLsp.Worker.runtimeconfig.json")) {
			return false
		}
		command = "dotnet"
		args = []string{path}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = filepath.Dir(path)
	processutil.ConfigureNonInteractiveCommand(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return false
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err = cmd.Start(); err != nil {
		return false
	}
	defer func() {
		_ = in.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	req := model.WorkerRequest{ProtocolVersion: model.ProtocolVersion, Method: "status", Workspace: ".", WorkspaceName: "probe", BackendType: "roslyn"}
	if worker.WriteFrame(in, req) != nil {
		return false
	}
	_ = in.Close()
	var response model.WorkerResponse
	if worker.ReadFrame(out, &response) != nil || !response.Ok {
		return false
	}
	for _, item := range response.Items {
		if version, ok := item["protocol_version"].(string); ok && strings.TrimSpace(version) != "" && version != model.ProtocolVersion {
			return false
		}
	}
	return true
}
