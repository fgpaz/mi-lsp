package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const KotlinBackend = "kotlin-language-server"
const KotlinServerVersion = "1.3.13"
const KotlinUpstreamCommit = "f532bcc9c6578f5a35009a8b5f281f93d4d131ba"
const KotlinCompilerVersion = "2.1.0"

// The local sidecar is a deployment assertion, not proof that classpath/JDK
// resolution succeeded. All v0 semantic answers remain partial until K2.
type KotlinRuntimeManifest struct {
	ServerVersion         string            `json:"server_version"`
	CompilerVersion       string            `json:"compiler_version"`
	UpstreamCommit        string            `json:"upstream_commit"`
	LauncherSHA256        string            `json:"launcher_sha256"`
	ExtensionVersion      string            `json:"extension_version,omitempty"`
	ExtensionSourceSHA256 string            `json:"extension_source_sha256,omitempty"`
	ExtensionPatchSHA256  string            `json:"extension_patch_sha256,omitempty"`
	DistributionFiles     map[string]string `json:"distribution_files,omitempty"`
}

type KotlinRuntimeConfig struct {
	Launcher    string
	ModuleRoot  string
	Entrypoint  string
	Fingerprint string
	Manifest    KotlinRuntimeManifest
}

type KotlinClient struct {
	*LSPClient
	Runtime KotlinRuntimeConfig
}

// KotlinRuntimeConfiguration performs bounded local metadata reads only. The
// explicit opt-in authorizes later LS initialization, which can execute Gradle.
func KotlinRuntimeConfiguration(workspace model.WorkspaceRegistration, request model.WorkerRequest) (KotlinRuntimeConfig, error) {
	fail := func(detail string) (KotlinRuntimeConfig, error) {
		return KotlinRuntimeConfig{}, fmt.Errorf("unavailable: Kotlin %s", detail)
	}
	if os.Getenv("MI_LSP_KOTLIN_ENABLE") != "1" {
		return fail("disabled; initialization may execute Gradle; set MI_LSP_KOTLIN_ENABLE=1 only in an authorized project")
	}
	if runtime.GOOS == "windows" {
		return fail("requires owned Job Object lifecycle on Windows")
	}
	launcher := strings.TrimSpace(os.Getenv("MI_LSP_KOTLIN_LANGUAGE_SERVER_PATH"))
	if !filepath.IsAbs(launcher) || !isRegularFile(launcher) {
		return fail("requires an absolute local versioned launcher path")
	}
	launcher = filepath.Clean(launcher)
	data, err := readKotlinMetadata(launcher + ".mi-lsp.json")
	if err != nil {
		return fail("missing runtime version manifest")
	}
	var manifest KotlinRuntimeManifest
	if json.Unmarshal(data, &manifest) != nil || (manifest.ServerVersion != KotlinServerVersion && manifest.ServerVersion != KotlinForkServerVersion) || manifest.CompilerVersion != KotlinCompilerVersion || manifest.UpstreamCommit != KotlinUpstreamCommit {
		return fail("incompatible runtime version manifest")
	}
	launcherData, err := readKotlinMetadata(launcher)
	if err != nil {
		return fail("launcher is not bounded readable metadata")
	}
	sum := sha256.Sum256(launcherData)
	if manifest.LauncherSHA256 != hex.EncodeToString(sum[:]) {
		return fail("launcher digest does not match manifest")
	}
	root := request.RepoRoot
	if root == "" {
		root = workspace.Root
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return fail("invalid repo root")
	}
	entry := strings.TrimSpace(request.EntrypointPath)
	if entry == "" {
		return fail("requires an explicit module build.gradle or build.gradle.kts entrypoint")
	}
	if !filepath.IsAbs(entry) {
		entry = filepath.Join(workspace.Root, filepath.FromSlash(entry))
	}
	entry = filepath.Clean(entry)
	name := filepath.Base(entry)
	if name != "build.gradle" && name != "build.gradle.kts" {
		return fail("entrypoint must be a Gradle module build file")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fail("invalid repo root")
	}
	realEntry, err := filepath.EvalSymlinks(entry)
	if err != nil {
		return fail("missing entrypoint")
	}
	rel, err := filepath.Rel(realRoot, realEntry)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fail("entrypoint escapes selected repo")
	}
	module := filepath.Dir(realEntry)
	if info, err := os.Stat(filepath.Join(module, "src", "main")); err != nil || !info.IsDir() {
		return fail("module must declare src/main JVM scope; scripts/variants are unsupported")
	}
	hash := sha256.New()
	field := func(name string, value []byte) {
		fmt.Fprintf(hash, "%d:%s%d:", len(name), name, len(value))
		_, _ = hash.Write(value)
	}
	field("manifest", data)
	field("launcher", launcherData)
	field("launcher_path", []byte(launcher))
	field("module", []byte(module))
	field("entrypoint", []byte(realEntry))
	field("java_home", []byte(os.Getenv("JAVA_HOME")))
	for _, name := range []string{"JAVA_OPTS", "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "GRADLE_OPTS", "GRADLE_USER_HOME"} {
		field(name, []byte(os.Getenv(name)))
	}
	field("gradle_daemon_policy", []byte("org.gradle.daemon=false"))
	if request.Method == "graph_observe" {
		if manifest.ServerVersion != KotlinForkServerVersion || manifest.ExtensionVersion != KotlinObservationVersion {
			return fail("requires honest compiler observation fork metadata; stock server unsupported")
		}
		if err := verifyKotlinForkDistribution(launcher, manifest); err != nil {
			return fail("fork distribution provenance invalid")
		}
		resolved, err := kotlinConfigurationFingerprint(module)
		if err != nil {
			return fail("resolved main JVM configuration unavailable or changed")
		}
		field("compiler_only", []byte("true"))
		field("resolved_main_configuration", resolved)
	}
	build, err := readKotlinMetadata(realEntry)
	if err != nil {
		return fail("cannot read module build metadata")
	}
	field("build", build)
	for dir := module; ; dir = filepath.Dir(dir) {
		for _, name := range []string{"settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts", "gradle.properties", "gradlew", "gradle/wrapper/gradle-wrapper.properties", "gradle/libs.versions.toml"} {
			path := filepath.Join(dir, filepath.FromSlash(name))
			value, err := readKotlinMetadata(path)
			if errors.Is(err, os.ErrNotExist) {
				field(path, nil)
				continue
			}
			if err != nil {
				return fail("invalid Gradle metadata")
			}
			field(path, value)
		}
		if dir == realRoot {
			break
		}
		if filepath.Dir(dir) == dir {
			return fail("module is outside repo")
		}
	}
	return KotlinRuntimeConfig{Launcher: launcher, ModuleRoot: module, Entrypoint: realEntry, Fingerprint: hex.EncodeToString(hash.Sum(nil)), Manifest: manifest}, nil
}
func readKotlinMetadata(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return nil, errors.New("metadata exceeds bound")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2<<20 {
		return nil, errors.New("metadata exceeds bound")
	}
	return data, nil
}

func NewKotlinClient(workspace model.WorkspaceRegistration, request model.WorkerRequest) (*KotlinClient, error) {
	config, err := KotlinRuntimeConfiguration(workspace, request)
	if err != nil {
		return nil, err
	}
	workspace.Root = config.ModuleRoot
	var initOptions map[string]any
	var serverArgs []string
	if request.Method == "graph_observe" {
		initOptions = map[string]any{"miLspCompilerOnly": true}
		serverArgs = []string{"--miLspCompilerOnly"}
	}
	return &KotlinClient{LSPClient: NewLSPClient(LSPConfig{ServerCmd: config.Launcher, ServerArgs: serverArgs, Backend: KotlinBackend, StrictCapabilities: true, RequireExplicitPosition: true, ExpectedServerVersion: config.Manifest.ServerVersion, InitOptions: initOptions}, workspace), Runtime: config}, nil
}
func (c *KotlinClient) Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	if err := ctx.Err(); err != nil {
		return model.WorkerResponse{}, err
	}
	switch request.Method {
	case "status":
		// Metadata status is not an initialize, conformance or publication claim.
		authority, certified := "unverified", false
		if cfg, err := ReadKotlinObservationConfiguration(c.Runtime.ModuleRoot); err == nil && cfg.Receipt.Certified != nil && *cfg.Receipt.Certified {
			authority, certified = cfg.Receipt.Authority, true
		}
		allowed, gaps := KotlinPromotionAllowed(KotlinPromotionEvidence{AuthorityCertified: certified})
		return model.WorkerResponse{Ok: true, Backend: KotlinBackend, Partial: true, Reason: "explicit_incomplete", Items: []map[string]any{{
			"server_version": c.Runtime.Manifest.ServerVersion, "compiler_version": c.Runtime.Manifest.CompilerVersion,
			"extension_version": c.Runtime.Manifest.ExtensionVersion, "state": "configured_unverified",
			"graph_domain": "main-jvm_named_declarations_local_references", "calls": "unsupported",
			"graph_stageable": allowed, "configuration_authority": authority, "conformance": "unverified",
			"promotion_oracles_missing": gaps,
		}}, Warnings: []string{"Kotlin calls unsupported; partial/experimental compiler domain is not a complete project graph"}}, nil
	case "graph_observe":
		return c.observeCompiler(ctx, request)
	case "get_context", "find_refs", "find_definition", "document_symbols":
	default:
		return model.WorkerResponse{Ok: false, Backend: KotlinBackend, ErrorCode: "unsupported_operation", Error: "unsupported Kotlin operation; resolved-call export is unavailable"}, nil
	}
	if file, ok := request.Payload["file"].(string); ok && !filepath.IsAbs(file) {
		payload := make(map[string]any, len(request.Payload))
		for key, value := range request.Payload {
			payload[key] = value
		}
		payload["file"] = filepath.Join(request.Workspace, filepath.FromSlash(file))
		request.Payload = payload
	}
	file, _ := request.Payload["file"].(string)
	if file != "" {
		real, err := filepath.EvalSymlinks(file)
		if err != nil {
			return model.WorkerResponse{}, err
		}
		rel, err := filepath.Rel(filepath.Join(c.Runtime.ModuleRoot, "src", "main"), real)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return model.WorkerResponse{}, errors.New("unavailable: file outside selected module src/main")
		}
	}
	response, err := c.LSPClient.Call(ctx, request)
	if err == nil && response.Ok {
		response.Partial = true
		response.Reason = "explicit_incomplete"
		response.Warnings = append(response.Warnings, "partial: classpath/JDK/target and compiler identity are not attested by K1; PSI symbols are structural only")
	}
	return response, err
}

// RequestExtension is the K2 transport seam, not an implemented compiler API.
// K2 must require its exact advertised experimental capability and validate the
// full observation before returning/publishing compiler facts.
func (c *KotlinClient) RequestExtension(ctx context.Context, method string, requiredCapability string, params any) (json.RawMessage, error) {
	if !strings.HasPrefix(method, "kotlin/") {
		return nil, errors.New("unsupported_operation: extension namespace")
	}
	for !c.mu.TryLock() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer c.mu.Unlock()
	if err := c.startContext(ctx); err != nil {
		return nil, err
	}
	var experimental map[string]json.RawMessage
	if json.Unmarshal(c.capabilities["experimental"], &experimental) != nil || requiredCapability == "" || string(experimental[requiredCapability]) != "true" {
		return nil, errors.New("unsupported_operation: exact compiler extension capability not advertised")
	}
	return c.sendRequest(method, params)
}
