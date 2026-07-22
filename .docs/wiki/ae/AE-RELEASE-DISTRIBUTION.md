# AE-RELEASE-DISTRIBUTION

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-RELEASE-DISTRIBUTION"
id: "AE-RELEASE-DISTRIBUTION"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-DEPENDENCY-HARDENING]]'
  - '[[TECH-GRAPH-NATIVE]]'
  - '[[DB-SYMBOL-EDGE-GRAPH]]'
  - '[[CT-GRAPH-CLI]]'
  - '[[CT-MILX-V1]]'
  - '[[09_contratos_tecnicos]]'
  - '[[AE-EVIDENCE-POLICY]]'
exports:
  - 'AE-RELEASE-DISTRIBUTION'
agent_must_read:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - .docs/wiki/07_tech/TECH-DEPENDENCY-HARDENING.md
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/09_contratos_tecnicos.md
agent_may_edit:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - scripts/release/ae-release-binaries.ps1
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - pwsh ./scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror
  - mi-lsp nav governance --workspace <alias> --format toon
stop_if:
  - governance_blocked=true
  - release-visible work lacks provenance evidence
evidence:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - scripts/release/ae-release-binaries.ps1
```

## Release Distribution Gate

```toon
doc_id: AE-RELEASE-DISTRIBUTION
block_id: AE-RELEASE-DISTRIBUTION.gate
kind: policy
source_of_truth: this
gate_id: ae_release_distribution
applies_when:
  - CLI behavior can drift between source and installed binary
  - worker bootstrap or bundled worker layout changes
  - release, install, version, doctor, daemon, or cross-OS behavior changes
  - search routing, safe-degrade planner, telemetry export, attribution, or version provenance fields change
  - an agent claims final readiness after modifying binary-producing code
  - graph identity/schema, generation publication/recovery, adapters, query envelopes, federation, MILX host or pack behavior changes
  - global daemon-routing flags (`--no-daemon`, `--no-auto-daemon`) or daemon request deadline semantics change
required_targets:
  local_current_machine:
    windows_arm64: C:/Users/fgpaz/bin/mi-lsp.exe
    wsl_linux:
      detection: "wsl sh -lc 'whoami; printf %s \"$HOME\"; uname -m'"
      install_paths:
        - "$HOME/.local/bin/mi-lsp"
        - "$HOME/bin/mi-lsp"
  release_assets:
    - win-arm64
    - win-x64
    - linux-arm64
    - linux-x64
    - osx-arm64
    - osx-x64
  public_install_scripts:
    - scripts/install/install.ps1
    - scripts/install/install.sh
    - scripts/install/install-agent.ps1
    - scripts/install/install-agent.sh
  optional_skill_mirror:
    roots:
      - C:/Users/fgpaz/.agents/skills/mi-lsp
      - C:/repos/buho/assets/skills/mi-lsp
    files:
      - bin/mi-lsp-win-x64.exe
      - bin/mi-lsp-linux-x64
  public_install_contract:
    latest_release: GitHub releases/latest
    checksum_asset: mi-lsp_<version>_checksums.txt
    archive_layout: mi-lsp(.exe) plus workers/<rid> inside the release archive
    agent_install: npx skills add fgpaz/mi-lsp --skill mi-lsp -g -a codex -a claude-code -y
    macos_mapping: install.sh resolves darwin-* release archives and maps bundled workers to osx-*; explicit darwin-* and osx-* aliases remain accepted
    worker_manifest_validation: install.sh requires a Python 3 JSON parser and validates schema, RID, protocol, file_count, paths, sizes, and SHA-256 hashes before install
    no_silent_auto_update: true
  code_signing_posture:
    decision: deferred  # SEC-11 / FD3 (2026-06-10)
    rationale: >
      CLI and worker artifacts are integrity-verified via published SHA256 checksums
      (mi-lsp_<version>_checksums.txt) which install scripts validate before extraction.
      Authenticode/code-signing is NOT applied: it requires a CA code-signing certificate
      plus CI signing secrets and mainly improves Windows SmartScreen reputation for wide
      non-technical distribution. Revisit when distributing broadly to non-technical users.
    integrity_today: sha256_checksums_verified_pre_extract
    revisit_trigger: broad_non_technical_distribution
default_command: scripts/release/ae-release-binaries.ps1
publish_command:
  shape: "pwsh ./scripts/release/ae-release-binaries.ps1 -Clean -Publish -Tag <vX.Y.Z>"
  effect: "build all RIDs, refresh local installs, verify provenance, and push the release tag that triggers GitHub release upload"
stop_if:
  - current worktree is dirty and Publish is requested
  - tag does not point at HEAD when Publish is requested
  - any required RID artifact is missing
  - worker verification accepts a WorkersRoot that is not exactly the six allowlisted RID directories
  - worker protocol probe reads are unbounded before timeout enforcement
  - public install script references an asset name not produced by GoReleaser
  - install.sh maps a Darwin host or darwin-*|osx-* alias to an archive/worker RID pair not produced by the release
  - release-platform-mapping runs without literal `pwsh` in release mode (same executable contract as GoReleaser verification)
  - public install script extracts before checksum verification
  - public installer accepts a worker manifest with the wrong schema, RID, or protocol
  - PowerShell extraction skips per-entry lexical, root-reparse, or destination-reparse checks
  - install-agent bypasses npx with an ungoverned folder-copy fallback
  - local ARM64 install was skipped without waiver on this workstation
  - WSL install was skipped without waiver when WSL is available
  - local executable remains locked after daemon stop and copy retries
  - telemetry/planner/provenance changes lack installed-path `version`, `worker status`, and `admin export --summary` evidence or explicit waiver
  - graph-native release lacks TP-GPH, migration rollback, cross-RID, no-MCP/no-network and both-comparator Victory evidence
  - direct/daemon routing flags or dial-only timeout behavior lack focused CLI/daemon verification
  - any target RID produces different NodeKey/cross-RID/determinism digest for the same fixture
  - an extension or pack can write the primary graph/wiki, access network/MCP/secrets, or survive cleanup
verify:
  - powershell -File ./scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror
  - sh scripts/tests/install-platform-mapping.sh
  - MI_LSP_RELEASE=1 sh scripts/tests/release-platform-mapping.sh
  - sh scripts/install/install.sh --rid linux-x64 --validate-worker-manifest <manifest> --worker-root <worker-root>
  - mi-lsp version --format toon
  - mi-lsp admin export --recent --summary --by-route --by-client --by-hint --by-failure-stage --format toon
  - mi-lsp nav wiki validate-source --workspace <alias> --format toon
evidence:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - scripts/release/ae-release-binaries.ps1
```

## Contrato de release para el lock de arranque del daemon

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: AE-RELEASE-DISTRIBUTION
block_id: AE-RELEASE-DISTRIBUTION.daemon-start-lock
kind: release-contract
audience: llm-first
source_of_truth: this
imports:
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
  - .docs/wiki/09_contratos_tecnicos.md
exports:
  - daemon_start_lock_release_invariants
evidence:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
  - internal/daemon/start_lock.go
  - internal/daemon/start_lock_windows.go
  - internal/daemon/start_lock_unix.go
  - internal/daemon/process_liveness_windows.go
  - internal/daemon/start_lock_test.go
verify:
  - go test ./internal/daemon/... -run 'Test(StartLock|ProcessExists)'
  - git diff --check
stop_if:
  - start_guard_not_persistent=true
  - start_guard_removed=true
  - start_lock_create_without_O_CREATE_O_EXCL=true
  - start_lock_metadata_not_versioned_pid_nonce=true
  - start_lock_descriptor_not_closed_after_metadata=true
  - start_lock_operation_outside_guard=true
  - live_or_unknown_owner_reclaimed=true
  - ambiguous_windows_liveness_reclaimed=true
  - legacy_empty_lock_reclaimed_at_or_before_5m=true
  - close_without_matching_pid_nonce=true
  - fail_open_on_liveness_error=true
start_lock:
  guard:
    path: start.guard
    persistence: persistent_never_removed
    os_exclusive_lock:
      windows: LockFileEx
      unix: flock
    serializes: [create, inspect, reclaim, Close]
  lock_file:
    path: start.lock
    create_flags: O_CREATE|O_EXCL|O_RDWR
    mode: 0600
    metadata:
      version: 1
      fields: [pid, nonce]
      descriptor_close: after_versioned_metadata_sync
    pid_valid_range: 1..math.MaxInt32
  reclaim:
    live_owner: preserve
    dead_owner: reclaim
    metadata_unknown: preserve
    legacy_empty_only_after: 5m
    windows_liveness:
      ERROR_INVALID_PARAMETER: nonexistent
      ACCESS_DENIED: alive
      ambiguous_error: alive
      exit_code_error: alive
    errors: fail_closed
  close:
    under_guard: true
    remove_only_when_pid_and_nonce_match: true
    replacement_with_different_metadata: preserve
release_status:
  claim: not_declared_by_document_update
```

Este bloque fija invariantes que deben conservarse en cualquier artefacto distribuido; la actualización documental no declara un resultado de release ni sustituye la evidencia de ejecución.

## Graph-Native Release Gate

```toon
doc_id: AE-RELEASE-DISTRIBUTION
block_id: AE-RELEASE-DISTRIBUTION.graph_native_gate
kind: release-gate
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - .docs/wiki/06_pruebas/TP-GPH.md
  - scripts/release/ae-release-binaries.ps1
applies_when:
  - graph-kernel-or-identity-code-changes
  - graph-SQLite-schema-migration-publication-or-recovery-changes
  - compiler-adapter-or-backend-maturity-changes
  - graph-query-impact-wiki-code-or-global-federation-changes
  - MILX-host-protocol-capability-or-pack-changes
required_canon:
  - RF-GPH-001-through-RF-GPH-011
  - TP-GPH-001-through-TP-GPH-007
  - TECH-GRAPH-NATIVE
  - DB-SYMBOL-EDGE-GRAPH
  - CT-GRAPH-CLI
  - CT-MILX-V1
required_evidence:
  - all-prior-slice-joins-PASS
  - NodeKey-and-cross-RID-golden-vectors
  - atomic-publish-crash-recovery-and-migration-rollback
  - direct-daemon-read-only-parity
  - no-MCP-no-network-no-graph-write-security
  - 30-repetition-raw-results-for-current-Graphify-and-previous-mi-lsp
  - correctness-precision-recall-negative-violations-determinism-tokens-warm-p95-peak-RSS-incrementality
cross_rid_matrix:
  required_RIDs: [win-arm64, win-x64, linux-arm64, linux-x64]
  oracle: identical-canonical-identity-and-output-for-same-fixture
release_artifacts:
  core: mi-lsp-bundle-per-RID
  worker: workers-per-RID-when-affected
  packs: versioned-manifest-plus-executable-sha256-when-shipped
stop_if:
  - any-required-metric-unavailable-or-threshold-failed
  - Graphify-or-previous-mi-lsp-comparator-missing
  - fewer-than-30-repetitions
  - rollback-or-crash-recovery-not-exercised
  - cross-RID-conflict
  - authority-inversion-or-dangling-edge
  - capability-escape-MCP-network-secret-or-primary-write
  - installed-binary-provenance-does-not-match-reviewed-source
publish: forbidden-until-G10-ae-close-APPROVED
```

## Campaña Harness-first única

```toon
doc_id: AE-RELEASE-DISTRIBUTION
block_id: AE-RELEASE-DISTRIBUTION.harness_first_campaign
kind: campaign-gate
source_of_truth: docs/benchmarks/HARNESS_FIRST_CAMPAIGN.json
imports:
  - docs/benchmarks/HARNESS_FIRST.md
  - docs/benchmarks/HARNESS_FIRST_CAMPAIGN.json
  - scripts/bench/harness_first/runner.py
evidence:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - docs/benchmarks/HARNESS_FIRST_CAMPAIGN.json
  - scripts/bench/harness_first/tests/test_runner.py
verify:
  - python -m scripts.bench.harness_first.runner --manifest docs/benchmarks/HARNESS_FIRST_CAMPAIGN.json --output <evidence-dir> --dry-run
  - python -m unittest discover -s scripts/bench/harness_first/tests -p "test_*.py"
stop_if:
  - campaign_status=NOT_RUN_or_FAIL
  - retry_attempted=true
  - provenance_not_exact=true
  - rss_not_observed_for_all_query_and_worker_processes=true
campaign:
  schema: harness-first-campaign/v1
  campaign_id: harness-first-final
  authorized_runner: scripts/bench/harness_first/runner.py
  execution: one_run_per_candidate_mode
  retry: forbidden
  status: NOT_RUN
  status_rule: remains_NOT_RUN_until_real_run
  comparator: none
  victory_lab_comparison: forbidden
queries:
  manifest: [wiki-pack, explain-change, workspace-map, related]
  direct_only: [wiki-pack, explain-change, workspace-map]
  daemon_capable: [related]
  explain_change_preview_required: true
  freshness_rank_required: [workspace-map, related]
  related_parity: direct_and_daemon
contract_gates:
  preview_sections: [change, affected, callers, callees, tests, contracts, wiki]
  expansion_fields: [command, reason]
  expansion_command_prefix: "mi-lsp nav "
  freshness_exact_state: current
  traversal: fail_closed
  measurements: [output_bytes, estimated_tokens, peak_rss]
  telemetry: sanitized_allowlisted_only
release_targets:
  required_RIDs: [win-arm64, win-x64, linux-arm64, linux-x64, osx-arm64, osx-x64]
  local_preferred: win-arm64
  remote_readback:
    logical_rids: [osx-arm64, osx-x64]
    asset_rids: [darwin-arm64, darwin-x64]
    worker_paths: [workers/osx-arm64, workers/osx-x64]
    rule: map_logical_osx_to_darwin_assets_without_renaming_worker_paths
provenance:
  source_worktree: clean_required
  vcs_revision: exact_40_or_64_hex_required
  vcs_modified: false_required
  worker_status: usable_evidence_required
artifacts:
  persisted: [report.json, report.yaml, marker]
  raw_stdout_stderr_payload: forbidden
```

La presencia de tests del runner, del manifest o del contrato no equivale a una ejecución de campaña. No se registra `PASS` ni métricas reales hasta que exista una única corrida autorizada con evidencia sanitizada.

## Readback de assets Darwin y workers macOS

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: AE-RELEASE-DISTRIBUTION
block_id: AE-RELEASE-DISTRIBUTION.darwin-readback
kind: release-readback-contract
audience: llm-first
source_of_truth: this
imports:
  - .docs/wiki/07_tech/TECH-DEPENDENCY-HARDENING.md
  - .docs/wiki/09_contratos_tecnicos.md
exports:
  - darwin_release_readback_mapping
evidence:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - scripts/release/ae-release-binaries.ps1
  - scripts/release/platform-mapping.sh
  - scripts/tests/release-platform-mapping.sh
verify:
  - pwsh ./scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror
  - sh scripts/tests/install-platform-mapping.sh
stop_if:
  - logical_osx_rid_compared_as_darwin_worker_rid=true
  - darwin_asset_missing=true
  - worker_osx_path_lost=true
readback:
  logical_rids: [osx-arm64, osx-x64]
  release_asset_rids: [darwin-arm64, darwin-x64]
  archive_mapping:
    osx-arm64: darwin-arm64
    osx-x64: darwin-x64
  worker_layout:
    osx-arm64: workers/osx-arm64
    osx-x64: workers/osx-x64
  invariant: asset_name_and_worker_runtime_name_are_distinct_but_mapped
release_status:
  campaign_result: not_claimed_by_document_presence
  pass_or_metrics: require_real_evidence
```

El readback compara el RID lógico que usa el worker con el nombre del asset publicado: `osx-*` se busca en assets `darwin-*`, mientras el contenido conserva `workers/osx-*`. Esta traducción es de validación y empaquetado; no autoriza afirmar una campaña ejecutada.

## WSL Worker Execution Audit

```toon
doc_id: AE-RELEASE-DISTRIBUTION
block_id: AE-RELEASE-DISTRIBUTION.wsl_worker_execution_audit
kind: policy
source_of_truth: this
applies_when:
  - historical WSL worker execution audit
  - release-visible WSL worker evidence is reviewed
required_inventory_fields:
  - distro_name
  - distro_state
  - wsl_version
  - detected_user_or_waiver
  - detected_home_or_waiver
  - cli_install_paths_reviewed
  - worker_paths_reviewed
  - read_only_scope
  - skipped_mutating_checks
required_attribution_fields:
  - MI_LSP_CLIENT_NAME
  - MI_LSP_SESSION_ID
  - mi_lsp_preflight.alias
  - mi_lsp_preflight.ae_canon.status
stop_if:
  - WSL distro must be started only to read history
  - audit would rewrite shell history, telemetry, install paths, or worker state
  - WSL skip has no waiver or scope-limit note
  - worker status is treated as enough without client/session attribution
verify:
  - mi-lsp nav governance --workspace <alias> --format toon
evidence:
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/wsl-execution-inventory.yaml
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/worker-session-attribution-matrix.yaml
```

## Operational Notes

`scripts/release/ae-release-binaries.ps1` is the maintained entrypoint for this gate. By default it builds all six RIDs and refreshes the current host install. On Windows it also refreshes the active WSL install when WSL is present and the matching Linux RID was built.

`scripts/install/install.ps1` and `scripts/install/install.sh` are the public CLI install/update entrypoints. They consume GitHub `releases/latest`, map the host to a published Windows, Linux, or Darwin archive, verify the release checksum before extraction, preserve the bundled worker layout (`darwin-*` archive to `osx-*` worker), and run `version` plus `worker status` probes.

`scripts/install/install-agent.ps1` and `scripts/install/install-agent.sh` compose the CLI installer with `npx skills add`; they do not copy skill folders directly. A weekly release check from the skill may notify about newer releases, but binary update remains explicit user action.

The local install path must stop an existing `mi-lsp daemon` before replacing the executable and worker bundle, then retry copy/removal briefly to absorb Windows file-lock lag.
WSL install defaults are detected from the active distro user and `$HOME`; pass `-WslInstallPaths` only when the target distro uses non-standard paths.

Publishing is explicit. The script only pushes a tag when `-Publish -Tag <tag>` is passed, the worktree is clean, and the tag points at `HEAD`. The GitHub release workflow remains the public upload mechanism for release assets.
