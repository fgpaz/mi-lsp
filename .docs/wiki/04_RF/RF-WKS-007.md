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
  - .docs/wiki/09_contratos/CT-WORKSPACE-PROBE.md
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-007.md
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
  omitted_selector:
    git_top_level: determine_first_in_normal_and_read_only
    registered_git_root: exact_canonical_match_preferred
    unregistered_git_root: synthetic_read_only_root
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
