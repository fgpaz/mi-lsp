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
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const KotlinForkServerVersion = "1.3.13+mi-lsp.obs2"
const KotlinObservationMethod = "kotlin/miLsp/compilerObservations/v2"
const KotlinObservationCapability = "miLspCompilerObservationsV2"
const KotlinObservationVersion = "mi-lsp-compiler-observations/v2"

type KotlinCompilerInput struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type KotlinMainConfiguration struct {
	SchemaVersion   int                   `json:"schema_version"`
	Resolution      string                `json:"resolution"`
	ModuleRoot      string                `json:"module_root"`
	ModuleName      string                `json:"module_name"`
	SourceSet       string                `json:"source_set"`
	JDKHome         string                `json:"jdk_home"`
	JVMTarget       string                `json:"jvm_target"`
	GradleVersion   string                `json:"gradle_version"`
	CompilerVersion string                `json:"compiler_version"`
	ServerVersion   string                `json:"server_version"`
	Sources         []KotlinCompilerInput `json:"sources"`
	Classpath       []KotlinCompilerInput `json:"classpath"`
	BuildInputs     []KotlinCompilerInput `json:"build_inputs"`
}
type KotlinObservationConfiguration struct {
	Path          string
	SHA256        string
	Main          KotlinMainConfiguration
	Receipt       KotlinConfigReceipt
	ReceiptSHA256 string
}

// Explicit resolved descriptor plus the pinned exporter receipt. A JSON file that
// only says resolved, a backup classpath, or stdlib-only is not authority.
func ReadKotlinObservationConfiguration(module string) (KotlinObservationConfiguration, error) {
	path := filepath.Join(module, ".mi-lsp", "kotlin-main-v1.json")
	data, err := readKotlinMetadata(path)
	if err != nil {
		return KotlinObservationConfiguration{}, errors.New("unavailable: explicit resolved main JVM descriptor required")
	}
	var config KotlinMainConfiguration
	if json.Unmarshal(data, &config) != nil || config.SchemaVersion != 1 || config.Resolution != "resolved" || config.SourceSet != "main-jvm" || config.ModuleRoot != module || config.ModuleName == "" || config.JVMTarget == "" || config.GradleVersion == "" || !filepath.IsAbs(config.JDKHome) || config.CompilerVersion != KotlinCompilerVersion || config.ServerVersion != KotlinForkServerVersion {
		return KotlinObservationConfiguration{}, errors.New("unavailable: main JVM descriptor contract mismatch")
	}
	if len(config.Sources) == 0 || len(config.Sources) > 512 || len(config.Classpath) == 0 || len(config.Classpath) > 256 || len(config.BuildInputs) == 0 || len(config.BuildInputs) > 128 {
		return KotlinObservationConfiguration{}, errors.New("unavailable: descriptor bounds")
	}
	seen := map[string]bool{}
	for _, group := range [][]KotlinCompilerInput{config.Sources, config.Classpath, config.BuildInputs} {
		for _, input := range group {
			digest, parseErr := model.ParseGraphDigest(input.SHA256)
			if !filepath.IsAbs(input.Path) || seen[input.Path] || parseErr != nil || digest == (model.GraphDigest{}) {
				return KotlinObservationConfiguration{}, errors.New("unavailable: descriptor input invalid")
			}
			seen[input.Path] = true
		}
	}
	for _, input := range config.Sources {
		real, err := filepath.EvalSymlinks(input.Path)
		if err != nil {
			return KotlinObservationConfiguration{}, errors.New("unavailable: missing source")
		}
		rel, err := filepath.Rel(filepath.Join(module, "src", "main"), real)
		ext := filepath.Ext(real)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || (ext != ".kt" && ext != ".java") {
			return KotlinObservationConfiguration{}, errors.New("unavailable: source outside main JVM module")
		}
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	receipt, receiptSHA, err := loadKotlinConfigAuthority(module, sha, config)
	if err != nil {
		return KotlinObservationConfiguration{}, err
	}
	return KotlinObservationConfiguration{Path: path, SHA256: sha, Main: config, Receipt: receipt, ReceiptSHA256: receiptSHA}, nil
}

func kotlinInputHash(path string, limit int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return "", errors.New("unavailable: compiler input bound")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, limit+1))
	if err != nil || n > limit {
		return "", errors.New("unavailable: compiler input read")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (c *KotlinClient) observeCompiler(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	fail := func(code string) (model.WorkerResponse, error) {
		return model.WorkerResponse{Backend: KotlinBackend, ErrorCode: code, Error: "Kotlin compiler observation unavailable", Reason: "explicit_incomplete"}, nil
	}
	if c.Runtime.Manifest.ServerVersion != KotlinForkServerVersion || c.Runtime.Manifest.ExtensionVersion != KotlinObservationVersion || c.config.InitOptions["miLspCompilerOnly"] != true {
		return fail("GPH_BACKEND_CAPABILITY_UNSUPPORTED")
	}
	config, err := ReadKotlinObservationConfiguration(c.Runtime.ModuleRoot)
	if err != nil {
		return fail("GPH_KOTLIN_CONFIG_UNAVAILABLE")
	}
	current, err := KotlinRuntimeConfiguration(model.WorkspaceRegistration{Root: request.Workspace}, request)
	if err != nil || current.Fingerprint != c.Runtime.Fingerprint {
		c.Close()
		return model.WorkerResponse{}, errors.New("unavailable: Kotlin configuration changed; runtime invalidated")
	}
	raw, err := c.RequestExtension(ctx, KotlinObservationMethod, KotlinObservationCapability, map[string]any{"module_root": c.Runtime.ModuleRoot, "configuration_path": config.Path, "configuration_sha256": config.SHA256})
	if err != nil {
		return model.WorkerResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		c.Close()
		return model.WorkerResponse{}, err
	}
	// Keep exact compiler export as derived transport, not an invented batch/NodeKey.
	var observation map[string]any
	if json.Unmarshal(raw, &observation) != nil {
		c.Close()
		return fail("GPH_BACKEND_PROVENANCE_MISSING")
	}
	return model.WorkerResponse{Ok: true, Backend: KotlinBackend, Partial: true, Reason: "explicit_incomplete", Items: []map[string]any{{"compiler_observation": observation}}, Warnings: []string{"compiler raw v2; authority receipt checked; promotion oracles missing; partial not stageable; calls unsupported"}}, nil
}

func verifyKotlinForkDistribution(launcher string, manifest KotlinRuntimeManifest) error {
	root, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(launcher)))
	if err != nil {
		return err
	}
	if len(manifest.DistributionFiles) == 0 || len(manifest.DistributionFiles) > 512 || manifest.ExtensionSourceSHA256 == "" || manifest.ExtensionPatchSHA256 == "" {
		return errors.New("unavailable: fork provenance missing")
	}
	if manifest.DistributionFiles["mi-lsp-provenance/MiLspCompilerObservations.kt"] != manifest.ExtensionSourceSHA256 || manifest.DistributionFiles["mi-lsp-provenance/upstream.patch"] != manifest.ExtensionPatchSHA256 {
		return errors.New("unavailable: fork provenance mismatch")
	}
	libs := 0
	var total int64
	for name, expected := range manifest.DistributionFiles {
		if name == "" || strings.Contains(name, "\\\\") || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name || strings.HasPrefix(name, "../") {
			return errors.New("unavailable: distribution path")
		}
		path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("unavailable: distribution escape")
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		total += info.Size()
		if total > 1<<30 {
			return errors.New("unavailable: distribution bound")
		}
		actual, err := kotlinInputHash(path, 256<<20)
		if err != nil || actual != expected {
			return errors.New("unavailable: distribution changed")
		}
		if strings.HasPrefix(name, "lib/") && strings.HasSuffix(name, ".jar") {
			libs++
		}
	}
	if libs == 0 {
		return errors.New("unavailable: fork libraries missing")
	}
	return nil
}

func kotlinConfigurationFingerprint(module string) ([]byte, error) {
	config, err := ReadKotlinObservationConfiguration(module)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	fmt.Fprintln(hash, config.SHA256)
	fmt.Fprintln(hash, config.ReceiptSHA256)
	fmt.Fprintln(hash, config.Receipt.JDKHome)
	fmt.Fprintln(hash, config.Receipt.JDKReleaseSHA256)
	fmt.Fprintln(hash, config.Receipt.ExporterSHA256)
	fmt.Fprintln(hash, config.Receipt.InitScriptSHA256)
	for _, group := range [][]KotlinCompilerInput{config.Main.Sources, config.Main.Classpath, config.Main.BuildInputs} {
		for _, input := range group {
			fmt.Fprintf(hash, "%d:%s:%s\n", len(input.Path), input.Path, input.SHA256)
		}
	}
	return hash.Sum(nil), nil
}
