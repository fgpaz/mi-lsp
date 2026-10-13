package worker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const KotlinConfigAuthorityID = "gradle-compileClasspath-resolvedArtifacts/v1"
const KotlinConfigCapture = "explicit-opt-in-offline"
const KotlinConfigResolver = "configurations.compileClasspath.resolvedArtifacts"
const KotlinConfigServerBinding = "contract_pin_not_runtime_proof"
const KotlinConfigClasspathScope = "project-effective"
const KotlinExporterSHA256 = "97fd0a079a12cb921c21404de8415fcdbc7915168fb7d1940836fbbc3579b1b3"
const KotlinInitScriptSHA256 = "2d46c87c31f8c7e5c94064f46d29e6b44ef0b95d46aebe807a50213f896f7bb5"

// KotlinPromotionOracleGaps are the RF-GPH-008 oracles still absent. This list
// is not an env switch and does not grant stable/complete.
func KotlinPromotionOracleGaps() []string {
	return []string{
		"oracle_identity_fixture",
		"oracle_relation_precision_recall",
		"oracle_determinism_30_reruns",
		"oracle_incremental_vs_clean",
		"oracle_cross_rid",
		"oracle_conformance_final_verify",
	}
}

// KotlinPromotionEvidence is the only promotion input. Zero values are the
// current lane. Env, labels and permissions are not fields and are not read.
type KotlinPromotionEvidence struct {
	AuthorityCertified            bool
	IdentityFixtureDigest         string
	RelationPrecisionRecallDigest string
	DeterminismRuns               int
	DeterminismDigest             string
	IncrementalDigest             string
	CrossRIDDigest                string
	ConformanceDigest             string
	FinalVerifyConsumed           bool
}

// KotlinPromotionAllowed is the conservative RF-GPH-008 gate. This revision
// returns false even if a caller fills the struct: FINAL_VERIFY is not consumed
// here and cannot be self-granted.
func KotlinPromotionAllowed(e KotlinPromotionEvidence) (bool, []string) {
	missing := []string{}
	if !e.AuthorityCertified {
		missing = append(missing, "configuration_authority")
	}
	if len(e.IdentityFixtureDigest) != 64 {
		missing = append(missing, "oracle_identity_fixture")
	}
	if len(e.RelationPrecisionRecallDigest) != 64 {
		missing = append(missing, "oracle_relation_precision_recall")
	}
	if e.DeterminismRuns != 30 || len(e.DeterminismDigest) != 64 {
		missing = append(missing, "oracle_determinism_30_reruns")
	}
	if len(e.IncrementalDigest) != 64 {
		missing = append(missing, "oracle_incremental_vs_clean")
	}
	if len(e.CrossRIDDigest) != 64 {
		missing = append(missing, "oracle_cross_rid")
	}
	if !e.FinalVerifyConsumed || len(e.ConformanceDigest) != 64 {
		missing = append(missing, "oracle_conformance_final_verify")
	}
	if len(missing) == 0 {
		missing = append(missing, "promotion_not_granted_this_revision")
	}
	return false, missing
}

type KotlinConfigArtifact struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Coordinate string `json:"coordinate"`
}

type KotlinConfigGap struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

type KotlinConfigReceipt struct {
	ReceiptVersion          int                    `json:"receipt_version"`
	Authority               string                 `json:"authority"`
	Capture                 string                 `json:"capture"`
	Certified               *bool                  `json:"certified"`
	CertificationBlockers   []string               `json:"certification_blockers"`
	Resolver                string                 `json:"resolver"`
	BackupClasspath         *bool                  `json:"backup_classpath"`
	ClasspathScope          string                 `json:"classpath_scope"`
	Calls                   string                 `json:"calls"`
	GraphCompleteness       string                 `json:"graph_completeness"`
	PromotionOraclesMissing []string               `json:"promotion_oracles_missing"`
	ModuleRoot              string                 `json:"module_root"`
	ModuleName              string                 `json:"module_name"`
	SourceSet               string                 `json:"source_set"`
	JDKHome                 string                 `json:"jdk_home"`
	JDKReleaseSHA256        string                 `json:"jdk_release_sha256"`
	JVMTarget               string                 `json:"jvm_target"`
	GradleVersion           string                 `json:"gradle_version"`
	CompilerVersion         string                 `json:"compiler_version"`
	ServerVersion           string                 `json:"server_version"`
	ServerVersionBinding    string                 `json:"server_version_binding"`
	ConfigurationSHA256     string                 `json:"configuration_sha256"`
	ExporterPath            string                 `json:"exporter_path"`
	ExporterSHA256          string                 `json:"exporter_sha256"`
	InitScriptPath          string                 `json:"init_script_path"`
	InitScriptSHA256        string                 `json:"init_script_sha256"`
	EvidenceDigest          string                 `json:"evidence_digest"`
	CaptureCommand          string                 `json:"capture_command"`
	Sources                 []KotlinCompilerInput  `json:"sources"`
	Classpath               []KotlinCompilerInput  `json:"classpath"`
	BuildInputs             []KotlinCompilerInput  `json:"build_inputs"`
	Artifacts               []KotlinConfigArtifact `json:"artifacts"`
	Unrepresentable         []KotlinConfigGap      `json:"unrepresentable"`
}

func kotlinHex64(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}

func kotlinPlain(v string) bool {
	return v != "" && !strings.ContainsAny(v, "\n\r\t")
}

func kotlinStdlibCoordinate(coordinate string) bool {
	parts := strings.Split(coordinate, ":")
	return len(parts) >= 3 && parts[0] == "module" && strings.HasPrefix(parts[2], "kotlin-stdlib")
}

func kotlinConfigEvidenceDigest(r KotlinConfigReceipt) string {
	lines := []string{
		"authority=" + r.Authority,
		"capture=" + r.Capture,
		"resolver=" + r.Resolver,
		"gradle=" + r.GradleVersion,
		"compiler=" + r.CompilerVersion,
		"server=" + r.ServerVersion,
		"jdk_home=" + r.JDKHome,
		"jdk_release=" + r.JDKReleaseSHA256,
		"jvm_target=" + r.JVMTarget,
		"module_name=" + r.ModuleName,
		"module_root=" + r.ModuleRoot,
		"exporter_sha256=" + r.ExporterSHA256,
		"init_script_sha256=" + r.InitScriptSHA256,
		"promotion_oracles=" + strings.Join(r.PromotionOraclesMissing, ","),
	}
	for _, item := range r.Sources {
		lines = append(lines, "source\t"+item.Path+"\t"+item.SHA256)
	}
	for _, item := range r.Classpath {
		lines = append(lines, "classpath\t"+item.Path+"\t"+item.SHA256)
	}
	for _, item := range r.Artifacts {
		lines = append(lines, "artifact\t"+item.Path+"\t"+item.SHA256+"\t"+item.Coordinate)
	}
	for _, item := range r.BuildInputs {
		lines = append(lines, "build_input\t"+item.Path+"\t"+item.SHA256)
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func kotlinSortedInputs(items []KotlinCompilerInput) bool {
	for i, item := range items {
		if !kotlinPlain(item.Path) || !kotlinHex64(item.SHA256) || !filepath.IsAbs(item.Path) {
			return false
		}
		if i > 0 && items[i-1].Path >= item.Path {
			return false
		}
	}
	return true
}

func kotlinHashMatches(path, expected string, limit int64) bool {
	digest, err := kotlinInputHash(path, limit)
	return err == nil && digest == expected
}

func kotlinPinnedScript(path, base, pin string) bool {
	if !filepath.IsAbs(path) || filepath.Base(path) != base || !kotlinPlain(path) {
		return false
	}
	return kotlinHashMatches(path, pin, 2<<20)
}

// loadKotlinConfigAuthority rejects a descriptor that only claims resolved.
// Backup classpath, stdlib-only and a receipt from any other exporter fail closed.
func loadKotlinConfigAuthority(module, configSHA string, main KotlinMainConfiguration) (KotlinConfigReceipt, string, error) {
	fail := func() (KotlinConfigReceipt, string, error) {
		return KotlinConfigReceipt{}, "", errors.New("unavailable: configuration authority not certified")
	}
	path := filepath.Join(module, ".mi-lsp", "kotlin-main-v1.authority.json")
	data, err := readKotlinMetadata(path)
	if err != nil {
		return fail()
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var receipt KotlinConfigReceipt
	if err := dec.Decode(&receipt); err != nil {
		return fail()
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fail()
	}
	if receipt.ReceiptVersion != 1 || receipt.Authority != KotlinConfigAuthorityID || receipt.Capture != KotlinConfigCapture || receipt.Certified == nil || !*receipt.Certified || len(receipt.CertificationBlockers) != 0 || receipt.Resolver != KotlinConfigResolver || receipt.BackupClasspath == nil || *receipt.BackupClasspath || receipt.ClasspathScope != KotlinConfigClasspathScope {
		return fail()
	}
	if receipt.Calls != "unsupported" || receipt.GraphCompleteness != "partial" || !reflect.DeepEqual(receipt.PromotionOraclesMissing, KotlinPromotionOracleGaps()) {
		return fail()
	}
	if receipt.ModuleRoot != module || receipt.ModuleName != main.ModuleName || receipt.SourceSet != "main-jvm" || receipt.JDKHome != main.JDKHome || receipt.JVMTarget != main.JVMTarget || receipt.GradleVersion != "8.10" || receipt.CompilerVersion != KotlinCompilerVersion || receipt.ServerVersion != KotlinForkServerVersion || receipt.ServerVersionBinding != KotlinConfigServerBinding {
		return fail()
	}
	if receipt.ConfigurationSHA256 != configSHA || !kotlinHex64(receipt.JDKReleaseSHA256) || !kotlinHex64(receipt.EvidenceDigest) {
		return fail()
	}
	if !kotlinPinnedScript(receipt.ExporterPath, "export-main-jvm.sh", KotlinExporterSHA256) || receipt.ExporterSHA256 != KotlinExporterSHA256 || !kotlinPinnedScript(receipt.InitScriptPath, "export-main-jvm.init.gradle.kts", KotlinInitScriptSHA256) || receipt.InitScriptSHA256 != KotlinInitScriptSHA256 {
		return fail()
	}
	if receipt.CaptureCommand != fmt.Sprintf("gradle --offline --no-daemon --no-build-cache -p %s -I %s miLspExportMainJvm", module, receipt.InitScriptPath) {
		return fail()
	}
	if receipt.Sources == nil || receipt.Classpath == nil || receipt.BuildInputs == nil || receipt.Artifacts == nil || receipt.Unrepresentable == nil || receipt.CertificationBlockers == nil || receipt.PromotionOraclesMissing == nil {
		return fail()
	}
	if !reflect.DeepEqual(receipt.Sources, main.Sources) || !reflect.DeepEqual(receipt.Classpath, main.Classpath) || !reflect.DeepEqual(receipt.BuildInputs, main.BuildInputs) || len(receipt.Unrepresentable) != 0 {
		return fail()
	}
	if !kotlinSortedInputs(receipt.Sources) || !kotlinSortedInputs(receipt.Classpath) || !kotlinSortedInputs(receipt.BuildInputs) {
		return fail()
	}
	release, err := filepath.EvalSymlinks(filepath.Join(receipt.JDKHome, "release"))
	if err != nil || !kotlinHashMatches(release, receipt.JDKReleaseSHA256, 2<<20) {
		return fail()
	}
	artifacts := map[string]KotlinConfigArtifact{}
	stdlib := 0
	pin := false
	for _, artifact := range receipt.Artifacts {
		if !kotlinPlain(artifact.Coordinate) || !kotlinHex64(artifact.SHA256) || !filepath.IsAbs(artifact.Path) {
			return fail()
		}
		if _, ok := artifacts[artifact.Path]; ok {
			return fail()
		}
		artifacts[artifact.Path] = artifact
		if kotlinStdlibCoordinate(artifact.Coordinate) {
			stdlib++
			if artifact.Coordinate == "module:org.jetbrains.kotlin:kotlin-stdlib:2.1.0" || (strings.HasPrefix(artifact.Coordinate, "module:org.jetbrains.kotlin:kotlin-stdlib-jdk") && strings.HasSuffix(artifact.Coordinate, ":2.1.0")) {
				pin = true
			}
		}
	}
	if !pin || stdlib == len(artifacts) {
		return fail()
	}
	for _, input := range receipt.Classpath {
		artifact, ok := artifacts[input.Path]
		if !ok || artifact.SHA256 != input.SHA256 || !strings.HasSuffix(input.Path, ".jar") || !kotlinHashMatches(input.Path, input.SHA256, 256<<20) {
			return fail()
		}
	}
	if len(artifacts) != len(receipt.Classpath) {
		return fail()
	}
	for _, input := range receipt.Sources {
		if !kotlinHashMatches(input.Path, input.SHA256, 2<<20) {
			return fail()
		}
	}
	seenBuild := map[string]bool{}
	for _, input := range receipt.BuildInputs {
		rel, err := filepath.Rel(module, input.Path)
		if err != nil || !kotlinAllowedBuildInput(rel) || !kotlinHashMatches(input.Path, input.SHA256, 2<<20) {
			return fail()
		}
		seenBuild[filepath.ToSlash(rel)] = true
	}
	if !seenBuild["build.gradle"] && !seenBuild["build.gradle.kts"] {
		return fail()
	}
	if !seenBuild["settings.gradle"] && !seenBuild["settings.gradle.kts"] {
		return fail()
	}
	for _, rel := range kotlinBuildInputNames() {
		path := filepath.Join(module, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || !seenBuild[rel] {
			return fail()
		}
	}
	if kotlinConfigEvidenceDigest(receipt) != receipt.EvidenceDigest {
		return fail()
	}
	sum := sha256.Sum256(data)
	return receipt, hex.EncodeToString(sum[:]), nil
}

func kotlinBuildInputNames() []string {
	return []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "gradle.properties", "gradle/libs.versions.toml", "gradle/wrapper/gradle-wrapper.properties"}
}

func kotlinAllowedBuildInput(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, name := range kotlinBuildInputNames() {
		if rel == name {
			return true
		}
	}
	return false
}
