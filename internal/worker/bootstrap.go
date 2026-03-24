package worker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var (
	lookPathFn          = exec.LookPath
	executablePathFn    = os.Executable
	userHomeDirFn       = os.UserHomeDir
	probeWorkerBinaryFn = probeWorkerBinary
)

type LaunchSpec struct {
	Command string
	Args    []string
	WorkDir string
}

type WorkerCandidateStatus struct {
	Source          string `json:"source,omitempty"`
	Path            string `json:"path,omitempty"`
	ProtocolVersion string `json:"protocol_version,omitempty"`
	Compatible      bool   `json:"compatible,omitempty"`
	Error           string `json:"error,omitempty"`
}

type WorkerRuntimeInfo struct {
	RID          string                `json:"rid,omitempty"`
	ToolRoot     string                `json:"tool_root,omitempty"`
	ToolRootKind string                `json:"tool_root_kind,omitempty"`
	Dotnet       bool                  `json:"dotnet,omitempty"`
	InstallHint  string                `json:"install_hint,omitempty"`
	Selected     WorkerCandidateStatus `json:"selected,omitempty"`
	Bundled      WorkerCandidateStatus `json:"bundled,omitempty"`
	Installed    WorkerCandidateStatus `json:"installed,omitempty"`
	DevLocal     WorkerCandidateStatus `json:"dev_local,omitempty"`
}

type workerProbeResult struct {
	ProtocolVersion string
	Compatible      bool
	Err             error
}

type cachedWorkerProbe struct {
	key    string
	result workerProbeResult
}

var workerProbeCache sync.Map

func ResolveRID() string {
	osPart := runtime.GOOS
	switch osPart {
	case "windows":
		osPart = "win"
	case "darwin":
		osPart = "osx"
	}

	archPart := runtime.GOARCH
	switch archPart {
	case "amd64":
		archPart = "x64"
	}

	return osPart + "-" + archPart
}

func workerBinaryName() string {
	if runtime.GOOS == "windows" {
		return "MiLsp.Worker.exe"
	}
	return "MiLsp.Worker"
}

func HasDotnet() bool {
	_, err := lookPathFn("dotnet")
	return err == nil
}

func DiscoverRepoRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(current); statErr == nil && !info.IsDir() {
		current = filepath.Dir(current)
	}
	for {
		if isToolRepoRoot(current) {
			return current, nil
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}
	return "", errors.New("could not determine mi-lsp repo root")
}

func isToolRepoRoot(dir string) bool {
	markers := []string{
		filepath.Join(dir, "worker-dotnet", "MiLsp.Worker", "MiLsp.Worker.csproj"),
		filepath.Join(dir, "cmd", "mi-lsp", "main.go"),
	}
	for _, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

func ResolveToolRoot(start string) (string, string) {
	distributionRoot := ""
	if execPath, err := executablePathFn(); err == nil {
		execDir := filepath.Dir(execPath)
		if repoRoot, repoErr := DiscoverRepoRoot(execDir); repoErr == nil {
			return repoRoot, "repo"
		}
		if absoluteDir, absErr := filepath.Abs(execDir); absErr == nil {
			distributionRoot = absoluteDir
		} else {
			distributionRoot = execDir
		}
	}
	if repoRoot, err := DiscoverRepoRoot(start); err == nil {
		return repoRoot, "repo"
	}
	if distributionRoot != "" {
		return distributionRoot, "distribution"
	}
	if absoluteStart, err := filepath.Abs(start); err == nil {
		return absoluteStart, "distribution"
	}
	return start, "distribution"
}
func InstalledWorkerDir() (string, error) {
	return InstalledWorkerDirForRID(ResolveRID())
}

func InstalledWorkerDirForRID(rid string) (string, error) {
	globalDir, err := userHomeDirFn()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(globalDir, ".mi-lsp", "workers", rid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func FindInstalledWorkerBinary() (string, error) {
	return FindInstalledWorkerBinaryForRID(ResolveRID())
}

func FindInstalledWorkerBinaryForRID(rid string) (string, error) {
	dir, err := InstalledWorkerDirForRID(rid)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(dir, workerBinaryName())
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", errors.New("installed worker binary not found")
}

func FindBundledWorkerBinary(toolRoot string) (string, error) {
	return FindBundledWorkerBinaryForRID(toolRoot, ResolveRID())
}

func FindBundledWorkerBinaryForRID(toolRoot string, rid string) (string, error) {
	for _, dir := range bundledWorkerDirs(toolRoot, rid) {
		candidate := filepath.Join(dir, workerBinaryName())
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("bundled worker binary not found")
}

func bundledWorkerDirs(toolRoot string, rid string) []string {
	cleaned := filepath.Clean(toolRoot)
	if detectToolRootKind(cleaned) == "repo" {
		return nil
	}
	return []string{
		filepath.Join(cleaned, "workers", rid),
		filepath.Join(cleaned, "bin", "workers", rid),
	}
}

func InspectWorkerRuntime(toolRoot string, rid string) WorkerRuntimeInfo {
	if rid == "" {
		rid = ResolveRID()
	}
	if strings.TrimSpace(toolRoot) == "" {
		toolRoot, _ = ResolveToolRoot(".")
	}
	info := WorkerRuntimeInfo{
		RID:          rid,
		ToolRoot:     filepath.Clean(toolRoot),
		ToolRootKind: detectToolRootKind(toolRoot),
		Dotnet:       HasDotnet(),
		InstallHint:  "Run `mi-lsp worker install` to refresh the bundled/global worker.",
	}

	bundledPath, bundledErr := FindBundledWorkerBinaryForRID(toolRoot, rid)
	if bundledErr != nil {
		info.Bundled = WorkerCandidateStatus{Source: "bundle", Error: bundledErr.Error()}
	} else {
		info.Bundled = inspectBinaryCandidate("bundle", bundledPath)
	}

	installedPath, installedErr := FindInstalledWorkerBinaryForRID(rid)
	if installedErr != nil {
		info.Installed = WorkerCandidateStatus{Source: "installed", Error: installedErr.Error()}
	} else {
		info.Installed = inspectBinaryCandidate("installed", installedPath)
	}

	devSpec, devPath, devErr := resolveDevLocalLaunchSpec(toolRoot)
	if devErr != nil {
		info.DevLocal = WorkerCandidateStatus{Source: "dev-local", Error: devErr.Error()}
	} else {
		info.DevLocal = WorkerCandidateStatus{Source: "dev-local", Path: devPath, Compatible: devSpec.Command != ""}
	}

	switch {
	case info.Bundled.Compatible:
		info.Selected = info.Bundled
	case info.Installed.Compatible:
		info.Selected = info.Installed
	case info.DevLocal.Compatible:
		info.Selected = info.DevLocal
	case info.Bundled.Path != "":
		info.Selected = info.Bundled
	case info.Installed.Path != "":
		info.Selected = info.Installed
	default:
		info.Selected = info.DevLocal
	}

	return info
}

func detectToolRootKind(toolRoot string) string {
	if isToolRepoRoot(toolRoot) {
		return "repo"
	}
	return "distribution"
}

func inspectBinaryCandidate(source string, path string) WorkerCandidateStatus {
	result := probeWorkerBinaryFn(path)
	status := WorkerCandidateStatus{
		Source:          source,
		Path:            path,
		ProtocolVersion: result.ProtocolVersion,
		Compatible:      result.Compatible,
	}
	if result.Err != nil {
		status.Error = result.Err.Error()
	}
	return status
}

func ResolveLaunchSpec(toolRoot string) (LaunchSpec, error) {
	if strings.TrimSpace(toolRoot) == "" {
		toolRoot, _ = ResolveToolRoot(".")
	}
	runtimeInfo := InspectWorkerRuntime(toolRoot, ResolveRID())
	switch runtimeInfo.Selected.Source {
	case "bundle":
		if runtimeInfo.Bundled.Compatible && runtimeInfo.Bundled.Path != "" {
			return LaunchSpec{Command: runtimeInfo.Bundled.Path, WorkDir: toolRoot}, nil
		}
	case "installed":
		if runtimeInfo.Installed.Compatible && runtimeInfo.Installed.Path != "" {
			return LaunchSpec{Command: runtimeInfo.Installed.Path, WorkDir: toolRoot}, nil
		}
	case "dev-local":
		if spec, _, err := resolveDevLocalLaunchSpec(toolRoot); err == nil {
			return spec, nil
		}
	}

	if runtimeInfo.Bundled.Compatible && runtimeInfo.Bundled.Path != "" {
		return LaunchSpec{Command: runtimeInfo.Bundled.Path, WorkDir: toolRoot}, nil
	}
	if runtimeInfo.Installed.Compatible && runtimeInfo.Installed.Path != "" {
		return LaunchSpec{Command: runtimeInfo.Installed.Path, WorkDir: toolRoot}, nil
	}
	if spec, _, err := resolveDevLocalLaunchSpec(toolRoot); err == nil {
		return spec, nil
	}

	reasons := make([]string, 0, 3)
	if runtimeInfo.Bundled.Error != "" {
		reasons = append(reasons, "bundle: "+runtimeInfo.Bundled.Error)
	}
	if runtimeInfo.Installed.Error != "" {
		reasons = append(reasons, "installed: "+runtimeInfo.Installed.Error)
	}
	if runtimeInfo.DevLocal.Error != "" {
		reasons = append(reasons, "dev-local: "+runtimeInfo.DevLocal.Error)
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no compatible worker candidate was found")
	}
	return LaunchSpec{}, fmt.Errorf("no compatible roslyn worker available (%s). Run `mi-lsp worker install` to refresh the bundled/global worker", strings.Join(reasons, "; "))
}

func resolveDevLocalLaunchSpec(toolRoot string) (LaunchSpec, string, error) {
	if !HasDotnet() {
		return LaunchSpec{}, "", errors.New("dotnet is not available for dev-local worker execution")
	}

	dllCandidates := []string{
		filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker", "bin", "Debug", "net10.0", "MiLsp.Worker.dll"),
		filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker", "bin", "Release", "net10.0", "MiLsp.Worker.dll"),
	}
	for _, candidate := range dllCandidates {
		if _, err := os.Stat(candidate); err == nil {
			return LaunchSpec{Command: "dotnet", Args: []string{candidate}, WorkDir: toolRoot}, candidate, nil
		}
	}

	projectPath := filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker", "MiLsp.Worker.csproj")
	if _, err := os.Stat(projectPath); err == nil {
		return LaunchSpec{Command: "dotnet", Args: []string{"run", "--project", projectPath, "--no-launch-profile"}, WorkDir: toolRoot}, projectPath, nil
	}
	return LaunchSpec{}, "", errors.New("dev-local worker project was not found")
}

func InstallWorker(toolRoot string, rid string) (string, error) {
	if rid == "" {
		rid = ResolveRID()
	}
	if strings.TrimSpace(toolRoot) == "" {
		toolRoot, _ = ResolveToolRoot(".")
	}

	targetDir, err := InstalledWorkerDirForRID(rid)
	if err != nil {
		return "", err
	}

	if bundledPath, bundleErr := FindBundledWorkerBinaryForRID(toolRoot, rid); bundleErr == nil {
		if err := copyWorkerDistribution(filepath.Dir(bundledPath), targetDir); err != nil {
			return "", err
		}
		if binaryPath, err := FindInstalledWorkerBinaryForRID(rid); err == nil {
			return binaryPath, nil
		}
	}

	if HasDotnet() {
		projectPath := filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker", "MiLsp.Worker.csproj")
		if _, statErr := os.Stat(projectPath); statErr == nil {
			command := exec.Command(
				"dotnet",
				"publish",
				projectPath,
				"-c", "Release",
				"-r", rid,
				"--self-contained", "true",
				"-o", targetDir,
			)
			command.Dir = toolRoot
			output, err := command.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("dotnet publish failed: %w: %s", err, string(output))
			}
		}
	}
	if binaryPath, err := FindInstalledWorkerBinaryForRID(rid); err == nil {
		return binaryPath, nil
	}
	return "", errors.New("worker install could not find a bundled worker and no source project was available; run from a mi-lsp distribution with workers/<rid> or publish the worker from source")
}

func copyWorkerDistribution(sourceDir string, targetDir string) error {
	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(sourceDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())
		if entry.IsDir() {
			if err := copyWorkerDistribution(sourcePath, targetPath); err != nil {
				return err
			}
			continue
		}
		body, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, body, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func probeWorkerBinary(path string) workerProbeResult {
	info, err := os.Stat(path)
	if err != nil {
		return workerProbeResult{Err: err}
	}
	cacheKey := fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())
	if cached, ok := workerProbeCache.Load(path); ok {
		probe := cached.(cachedWorkerProbe)
		if probe.key == cacheKey {
			return probe.result
		}
	}
	result := probeWorkerBinaryOnce(path)
	workerProbeCache.Store(path, cachedWorkerProbe{key: cacheKey, result: result})
	return result
}

func probeWorkerBinaryOnce(path string) workerProbeResult {
	command := exec.Command(path)
	command.Dir = filepath.Dir(path)
	var stderr bytes.Buffer
	command.Stderr = &stderr

	stdin, err := command.StdinPipe()
	if err != nil {
		return workerProbeResult{Err: err}
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return workerProbeResult{Err: err}
	}
	if err := command.Start(); err != nil {
		return workerProbeResult{Err: err}
	}
	defer func() {
		_ = stdin.Close()
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_, _ = command.Process.Wait()
	}()

	request := model.WorkerRequest{ProtocolVersion: model.ProtocolVersion, Method: "status", Workspace: ".", WorkspaceName: "probe", BackendType: "roslyn"}
	if err := WriteFrame(stdin, request); err != nil {
		return workerProbeResult{Err: err}
	}
	_ = stdin.Close()

	type probeResponse struct {
		response model.WorkerResponse
		err      error
	}
	responseCh := make(chan probeResponse, 1)
	go func() {
		var response model.WorkerResponse
		responseCh <- probeResponse{response: response, err: ReadFrame(stdout, &response)}
	}()

	select {
	case result := <-responseCh:
		if result.err != nil {
			if message := strings.TrimSpace(stderr.String()); message != "" {
				return workerProbeResult{Err: errors.New(message)}
			}
			return workerProbeResult{Err: result.err}
		}
		protocolVersion := extractWorkerProtocolVersion(result.response)
		if !result.response.Ok {
			if result.response.Error != "" {
				return workerProbeResult{ProtocolVersion: protocolVersion, Err: errors.New(result.response.Error)}
			}
			return workerProbeResult{ProtocolVersion: protocolVersion, Err: errors.New("worker probe returned ok=false")}
		}
		if protocolVersion != "" && protocolVersion != model.ProtocolVersion {
			return workerProbeResult{ProtocolVersion: protocolVersion, Err: fmt.Errorf("protocol version mismatch: cli=%s worker=%s", model.ProtocolVersion, protocolVersion)}
		}
		return workerProbeResult{ProtocolVersion: firstNonEmpty(protocolVersion, model.ProtocolVersion), Compatible: true}
	case <-time.After(3 * time.Second):
		return workerProbeResult{Err: errors.New("worker probe timed out")}
	}
}

func extractWorkerProtocolVersion(response model.WorkerResponse) string {
	for _, item := range response.Items {
		if value, ok := item["protocol_version"].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

