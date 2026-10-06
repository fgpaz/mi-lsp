---
doc_id: RF-WKS-007
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_kind: canonical-requirement
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: RF-WKS-007
kind: requirement
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-001]]'
  - '[[RF-WKS-005]]'
  - '[[RF-WKS-006]]'
  - '[[TP-WKS]]'
  - '[[CT-WORKSPACE-PROBE]]'
  - '[[TP-WKS-PROBE]]'
exports:
  - RF-WKS-007
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-001.md
  - .docs/wiki/04_RF/RF-WKS-005.md
  - .docs/wiki/04_RF/RF-WKS-006.md
  - .docs/wiki/04_RF/RF-WKS-007.md
  - .docs/wiki/06_pruebas/TP-WKS.md
  - .docs/wiki/09_contratos/CT-WORKSPACE-PROBE.md
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-007.md
  - .docs/wiki/06_pruebas/TP-WKS.md
  - .docs/wiki/09_contratos/CT-WORKSPACE-PROBE.md
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
  - internal/service/app.go
verify:
  - gofmt -d -- $(git diff --name-only --diff-filter=ACMRTUXB HEAD -- '*.go')
  - go test ./internal/workspace ./internal/store ./internal/service ./internal/cli
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - explicit_selector_fallback=true
  - probe_side_effects=true
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
  - .docs/wiki/06_pruebas/TP-WKS.md
  - .docs/wiki/09_contratos/CT-WORKSPACE-PROBE.md
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
---

# RF-WKS-007 - Identidad fail-closed, estado híbrido y probe no mutante

## [RF-WKS-007-B01] Hoja de ejecución

```toon
block_id: RF-WKS-007-B01
kind: normative
source_of_truth: RF-WKS-007
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
requirement_id: RF-WKS-007
feature: workspace-identity-and-probe
status: implemented-in-batch
priority: high
actors: [CLI, service, workspace, store, agent]
acceptance_oracle: TP-WKS-PROBE
contract: CT-WORKSPACE-PROBE
```

## [RF-WKS-007-B02] Identidad física y raíz de display

```toon
block_id: RF-WKS-007-B02
kind: normative
source_of_truth: RF-WKS-007
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
identity:
  display_root: original-user-facing-root
  canonical_root: absolute-clean-evaluated-root
  comparison:
    windows: case-insensitive
    darwin: case-insensitive-by-default
    unix: case-sensitive
  symlink_policy: canonical_root_evaluates-existing-symlinks
  selector_kinds: [omitted, alias, path]
```

La raíz mostrada conserva una forma estable y legible para el usuario. La identidad física se calcula de forma absoluta, limpia y, cuando el path existe, con evaluación de symlink/junction. La comparación solo ignora casing en plataformas con semántica de filesystem insensible al casing. Dos paths físicamente distintos no se agrupan por una normalización que solo baje a minúsculas.
La respuesta para agentes identifica en su primera línea el workspace seleccionado. Esto hace visible el alias o selector efectivo sin modificar la identidad física ni la resolución fail-closed.

## [RF-WKS-007-B03] Resolución fail-closed

```toon
block_id: RF-WKS-007-B03
kind: normative
source_of_truth: RF-WKS-007
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
resolution:
  explicit_alias:
    unknown: error
    stale: error
    fallback_to_caller_cwd: forbidden
  explicit_path:
    missing: error
    stale_registration: error
    linked_worktree:
      main_registered: resolve_main_alias_without_worktree_registration
      ordinary_read_only_unregistered: resolve_main_alias_when_available
      status_probe: preserve_physical_root_read_only_without_registration_or_index
      main_unregistered: explicit_incomplete_invalid_workspace_without_index
  omitted_selector:
    git_top_level: determine_first_in_normal_and_read_only
    registered_git_root: exact_canonical_match_preferred
    unregistered_git_root: auto_register_then_resolve_within_home_only
    linked_worktree: resolve_registered_main_root_without_registering_worktree
    linked_worktree_main_unregistered: explicit_incomplete_invalid_workspace_without_index
    synthetic_read_only_root: forbidden_for_linked_worktree
    lexical_parent_fallback_for_git: forbidden
    non_git_directory: preserve_registered_containment
    precedence: [git_top_level, caller_cwd, same_root_alias_policy, last_workspace]
    git_top_level_failure: typed_or_synthetic_without_lexical_parent_fallback
    source: auditable
    warning_on_fallback: required
  provenance:
    source: required
    selector_kind: required
    display_root: required
    canonical_root: required
```

Un selector explícito inválido o stale nunca puede convertirse silenciosamente en el workspace del `caller_cwd`. La resolución omitida conserva la precedencia contextual existente y expone `source` y warnings suficientes para auditoría.
Los aliases heredados que apunten a la misma raíz física siguen siendo válidos y se conservan; la presentación compacta no los migra, elimina ni redirige. Un alias explícito desconocido o stale sigue fallando sin sustituirse por el workspace del cwd.

Cuando el `caller_cwd` o un path explícito está dentro de un linked worktree, la resolución compara `git dir` y `git common dir` como rutas absolutas, canónicas y normalizadas. Si el root principal está registrado, devuelve ese alias y no crea ni indexa un alias para el linked worktree. Si no lo está, devuelve `explicit_incomplete` con `reason_code=invalid_workspace` y no inicia indexación. `MI_LSP_AUTOREGISTER=force` opta explícitamente por registrar el linked worktree como raíz física independiente.

### Auto-registro en la primera consulta

Las operaciones que requieren workspace (`nav.*`, `index.*`, `info`, `workspace.status`) ejecutan un único camino compartido por CLI y daemon (`App.Execute`, `internal/service/auto_register.go` y `internal/workspace/autoregister.go`), de modo que sirve igual a `mi-lsp mcp`, al plugin milsp y al hijo persistente de mi-mcp.

La especificación normativa del auto-registro, incluida la respuesta cuando el repo aún no tiene commits, el alcance del opt-out, linked worktrees, force y GC del registry, está agrupada en el bloque TOON `RF-WKS-007-B08`. Los casos de verificación se definen normativamente en `TP-WKS` (`TC-WKS-048..060`, `TC-WKS-064..066`). El enlace al flag CLI se apoya en evidencia de código; `TC-WKS-049` no declara cobertura E2E de su propagación.

## [RF-WKS-007-B04] Estado híbrido portable/local

```toon
block_id: RF-WKS-007-B04
kind: normative
source_of_truth: RF-WKS-007
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
state:
  portable:
    location: repo-local
    examples: [project.toml, configuration, migration_status]
  operational:
    location: machine-local-resolved-path
    examples: [index.db, daemon_runtime, snapshots, telemetry]
  legacy:
    automatic_move: forbidden
    automatic_delete: forbidden
    migration_status: explicit
```

La configuración que debe viajar con el repositorio permanece repo-local. El estado operativo se resuelve a una ruta local de la máquina y no se incorpora a la configuración portable. El estado legacy se conserva durante esta ola; cualquier compatibilidad o migración se informa explícitamente y no mueve ni borra archivos automáticamente. Un worktree y su parent, aun cuando compartan `git_common_dir`, conservan roots canónicos y operational paths distintos.

## [RF-WKS-007-B05] Probe

```toon
block_id: RF-WKS-007-B05
kind: normative
source_of_truth: RF-WKS-007
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
probe:
  daemon_required: false
  database_required: false
  statuses: [absent, current, stale, unknown, partial]
  output:
    structured: true
    side_effects: false
    provenance: version-compatible
  forbidden:
    - MkdirAll
    - schema_migration
    - journal_mode_WAL
    - wal_shm_creation
    - registry_write
    - snapshots
    - telemetry
```

`mi-lsp probe` puede ejecutarse sin daemon y sin DB. Si existe una DB, solo puede abrirla en modo SQLite `ro` real; si no existe, debe reportar evidencia ausente sin crear directorios ni archivos. El probe no inicializa schema, no aplica migraciones, no escribe registry, no crea snapshots, no envía telemetría y devuelve `side_effects:false` en salida estructurada.

## [RF-WKS-007-B06] Estados y evidencia

```toon
block_id: RF-WKS-007-B06
kind: normative
source_of_truth: RF-WKS-007
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
status_meaning:
  absent: required_evidence_missing
  current: observed_state_matches_expected
  stale: observed_state_exists_but_is_older_or_mismatched
  unknown: evidence_unavailable_or_unreadable
  partial: some_checks_available_and_others_missing
```

Los estados son diagnósticos, no instrucciones implícitas de reparación. La salida incluye evidencia mínima, migration status, resolución del workspace y provenance reutilizable de `mi-lsp version` cuando corresponda.

## [RF-WKS-007-B07] No-regresión y pruebas

```toon
block_id: RF-WKS-007-B07
kind: normative
source_of_truth: RF-WKS-007
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
normative: snapshots-before-after-alias-casing-symlink-db-missing-read-only
format_oracle: dynamic_touched_go_gofmt_from_git_diff
```

Las pruebas cubren snapshots antes/después de parent, worktree, home y fixtures temporales, alias inexistente y stale, resolución contextual sin selector, casing por plataforma, symlink/junction cuando el host lo permite, DB inexistente y apertura read-only sin WAL/SHM. Toda prueba que observe una mutación del filesystem o registry hace fallar el batch. La única oracle de formato para todos los Go tocados es la orden dinámica y no mutante declarada en `verify`: descubre el conjunto completo desde Git, no usa listas manuales y nunca escribe archivos.

## [RF-WKS-007-B08] Auto-registro en la primera consulta

```toon
block_id: RF-WKS-007-B08
kind: normative
source_of_truth: RF-WKS-007
verify:
  - go test -count=1 ./internal/service ./internal/workspace ./internal/cli
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/04_RF/RF-WKS-007.md
  - .docs/wiki/06_pruebas/TP-WKS.md
  - internal/cli/root.go
  - internal/cli/workspace.go
  - internal/cli/workspace_test.go
  - internal/service/auto_register.go
  - internal/service/auto_register_test.go
  - internal/service/probe.go
  - internal/service/probe_test.go
  - internal/service/workspace_ops.go
  - internal/service/workspace_resolution_test.go
  - internal/workspace/autoregister.go
  - internal/workspace/autoregister_test.go
  - internal/workspace/registry.go
  - internal/workspace/registry_test.go
operations_requiring_workspace: [nav.*, index.*, info, workspace.status]
selector:
  omitted: caller_cwd
  requested_path: existing_path_that_is_not_a_registered_alias
  auto_register_only_inside_unregistered_git_repository: true
  auto_register_default_scope: inside_HOME_only
  outside_HOME_or_tmp: no_auto_register_by_default
  linked_worktree:
    identify_by: normalized_absolute_canonical_git_common_dir_and_git_dir
    when_main_registered: resolve_main_alias_without_worktree_registration
    when_main_unregistered: explicit_incomplete_reason_code_invalid_workspace_without_index
    force_override: MI_LSP_AUTOREGISTER=force
    force_bypass: default_scope_and_linked_worktree_restrictions
    force_log: required_unambiguous_warning
registry_gc:
  triggers: [workspace_list, workspace_resolve]
  removable_only_when: root_is_definitively_missing
  ambiguous_permission_or_io_errors: retain_and_report_skipped
  cleanup_log: sanitized_counts_without_paths
  empty_root: retain_and_report_skipped
  stale_selector_diagnostic: capture_before_gc_and_preserve_WKS_SELECTOR_STALE
  diagnostic_snapshot_without_gc: [workspace_doctor, workspace_hygiene_preview, workspace_hygiene_readiness, read_only_resolution]
  ordinary_read_only_git_failure_for_linked_marker: registered_containment_only
  status_probe_linked_worktree: preserve_physical_root_read_only_without_registration_or_index
  status_probe_git_failure_for_linked_marker: preserve_physical_root_without_parent_state_inspection
  forbidden_side_effects: [delete_directory, delete_cache, delete_worktree]
  manual_command: mi-lsp workspace prune --stale
registration:
  root: git_top_level
  project_toml:
    create_only_if_missing: true
    preserve_existing: true
  last_workspace_changed: false
  default_excluded: [$HOME, filesystem_root, path_outside_git_repository, path_outside_HOME, path_inside_temp_dir]
  force_bypasses: [path_outside_HOME, path_inside_temp_dir, linked_worktree_default_resolution]
  force_never_bypasses: [$HOME, filesystem_root, path_outside_git_repository]
  probe_registers: false
alias:
  default: git_root_basename
  collision: deterministic_basename_plus_sha256_root_prefix
  collision_suffix_expands_if_needed: true
  overwrite_existing_alias: forbidden
  alias_for_same_root: reuse_existing
query_with_commits:
  response: immediate
  response_content: [catalog_or_text, possibly_partial]
  index_start: background_deduplicated_by_root
query_without_commits:
  registration_persisted: true
  query_continues: true
  response_mode: text_search
  index_start: forbidden
  warning: auto_register_index_skipped
  warning_message: "repository has no commits yet; not indexing until the first commit (results use text search)"
registration_failure:
  query_blocked: false
  warning: auto_register_failed
registry:
  write: atomic_temp_file_then_rename
  read_modify_write_lock: registry.lock
  permissions:
    new_file_mode_argument: "0600"
    existing_file_mode: preserve_existing_permission_bits
    test_assertion_scope: non_windows_checks_new_mode_and_explicit_0600_update
    source_behavior: internal/workspace/registry.go
    test_binding: TestSaveRegistryPreservesRestrictivePermissions
opt_out:
  environment_variable: MI_LSP_NO_AUTO_REGISTER=1
  cli_option: --no-auto-register
  cli_source_evidence: internal/cli/root.go
  cli_mapping: QueryOptions.NoAutoRegister
  cli_request_path: CommandRequest.Context.NoAutoRegister
  daemon_request_path: executeOperation_passes_request_to_daemon.ExecuteWithDialTimeout
  cli_daemon_e2e_coverage: pending_no_automated_oracle
  service_test_inputs: [request.Context.NoAutoRegister, MI_LSP_NO_AUTO_REGISTER=1]
case_ids: [TC-WKS-048, TC-WKS-049, TC-WKS-050, TC-WKS-051, TC-WKS-052, TC-WKS-053, TC-WKS-054, TC-WKS-055, TC-WKS-056, TC-WKS-057, TC-WKS-058, TC-WKS-059, TC-WKS-060, TC-WKS-064, TC-WKS-065, TC-WKS-066, TC-WKS-067]
```
