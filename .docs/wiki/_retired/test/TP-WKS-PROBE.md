---
doc_id: TP-WKS-PROBE
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_kind: canonical-test-plan
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: TP-WKS-PROBE
kind: test-plan
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-007]]'
  - '[[CT-WORKSPACE-PROBE]]'
  - '[[TP-WKS]]'
exports:
  - TP-WKS-PROBE
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-007.md
  - .docs/wiki/09_contratos/CT-WORKSPACE-PROBE.md
  - .docs/wiki/06_pruebas/TP-WKS.md
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - go test ./internal/workspace ./internal/store ./internal/service ./internal/cli
  - gofmt -d -- $(git diff --name-only --diff-filter=ACMRTUXB HEAD -- '*.go')
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - any_test_failed=true
  - snapshot_changed_without_explicit_fixture=true
  - side_effects=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
---

# TP-WKS-PROBE - Plan de pruebas del Batch B

wiki_source_table_exception: true

## [TP-WKS-PROBE-B01] Oracle y presupuesto

```toon
block_id: TP-WKS-PROBE-B01
kind: normative
source_of_truth: TP-WKS-PROBE
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
plan_id: TP-WKS-PROBE
acceptance_oracle: all-targeted-go-tests-pass-and-filesystem-snapshot-equal
mutation_policy: tests_may_mutate_temp_dirs_only
production_paths: forbidden
```

## [TP-WKS-PROBE-B02] Identidad y resolución

```toon
block_id: TP-WKS-PROBE-B02
kind: normative
source_of_truth: TP-WKS-PROBE
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
normative: identity_and_resolution
```

| Caso | Tipo | Cobertura | Resultado esperado |
|---|---|---|---|
| TC-WKS-PROBE-001 | negativo | alias explícito inexistente | error `WKS_SELECTOR_NOT_FOUND`, sin fallback a caller_cwd |
| TC-WKS-PROBE-002 | negativo | alias explícito stale | error `WKS_SELECTOR_STALE`, sin fallback |
| TC-WKS-PROBE-003 | positivo | selector omitido dentro de root registrado | `resolution_source=caller_cwd` y warning/provenance auditable |
| TC-WKS-PROBE-004 | positivo | selector path explícito relativo | resuelve relativo al caller CWD capturado |
| TC-WKS-PROBE-005 | plataforma | casing | Windows/macOS agrupan casing equivalente; Unix conserva roots distintos |
| TC-WKS-PROBE-006 | plataforma | symlink/junction | identidad canónica converge solo cuando el host permite evaluar el enlace |
| TC-WKS-PROBE-007 | negativo | root físico inexistente | estado stale/unknown explícito, nunca alias alternativo |
| TC-WKS-PROBE-018 | negativo | worktree Git anidado bajo parent registrado | usa `git_top_level`, nunca selecciona parent léxico |
| TC-WKS-PROBE-019 | negativo | root Git no registrado | resolución sintética/path del Git root, sin leer estado del parent |
| TC-WKS-PROBE-020 | positivo | parent y worktree registrados | gana el root canónico exacto del worktree |
| TC-WKS-PROBE-021 | plataforma | `.git` archivo de worktree | se detecta como root Git válido |
| TC-WKS-PROBE-022 | positivo | `git_common_dir` compartido | roots y operational paths permanecen distintos |

## [TP-WKS-PROBE-B03] Estado híbrido

```toon
block_id: TP-WKS-PROBE-B03
kind: normative
source_of_truth: TP-WKS-PROBE
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
normative: hybrid_state
```

| Caso | Tipo | Cobertura | Resultado esperado |
|---|---|---|---|
| TC-WKS-PROBE-008 | positivo | config repo-local | `portable_config=repo-local` |
| TC-WKS-PROBE-009 | positivo | estado operativo local | path resuelto por máquina, sin mover legacy |
| TC-WKS-PROBE-010 | negativo | legacy existente | `migration_status=legacy_present`, bytes y mtime sin cambios |

## [TP-WKS-PROBE-B04] Probe no mutante

```toon
block_id: TP-WKS-PROBE-B04
kind: normative
source_of_truth: TP-WKS-PROBE
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
normative: non_mutating_probe
```

| Caso | Tipo | Cobertura | Resultado esperado |
|---|---|---|---|
| TC-WKS-PROBE-011 | positivo | sin daemon y sin DB | status `absent` o `partial`, exit controlado, sin archivos nuevos |
| TC-WKS-PROBE-012 | positivo | DB existente | abre `mode=ro`, `db_mode=ro`, no crea WAL/SHM |
| TC-WKS-PROBE-013 | negativo | DB ausente | no crea `.mi-lsp`, `index.db`, WAL ni SHM |
| TC-WKS-PROBE-014 | negativo | home sin registry | no crea `~/.mi-lsp` ni escribe registry |
| TC-WKS-PROBE-015 | positivo | snapshot antes/después | lista, bytes y mtimes iguales fuera de fixtures temporales |
| TC-WKS-PROBE-016 | positivo | salida estructurada | `side_effects:false`, envelope estable y provenance mínima |
| TC-WKS-PROBE-017 | negativo | telemetría/snapshot/migración | ningún registro ni artefacto generado |
| TC-WKS-PROBE-023 | negativo | worktree Git y parent con fixtures distintos | no lee config/DB del parent y snapshots antes/después iguales |
| TC-WKS-PROBE-024 | negativo | Git top-level no disponible después de detectar contexto Git | error tipado o raíz sintética, nunca fallback léxico al parent |

## [TP-WKS-PROBE-B05] Comandos de verificación

```toon
block_id: TP-WKS-PROBE-B05
kind: normative
source_of_truth: TP-WKS-PROBE
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
evidence:
  - .docs/wiki/06_pruebas/TP-WKS-PROBE.md
commands:
  format: dynamic_touched_go_gofmt_from_git_diff
  tests: go test ./internal/workspace ./internal/store ./internal/service ./internal/cli
  snapshot: before_after_parent_worktree_home_and_temp_fixture
  forbidden_paths: [src, cmd, worker-dotnet, deployment, real_user_home]
```

La única oracle de formato para todos los Go tocados es la orden dinámica y no mutante declarada en `verify`. El conjunto se obtiene desde Git para incluir cambios staged y unstaged; no se aceptan listas manuales ni invocaciones mutantes del formateador, porque la verificación no debe mutar archivos. Los casos que requieren symlink/junction se marcan skip únicamente cuando el sistema operativo o permisos no los permiten; el motivo queda en el resultado de test.
