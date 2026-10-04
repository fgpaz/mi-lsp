---
doc_id: TP-WKS
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_kind: canonical-test-plan
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: TP-WKS
kind: test-plan
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-007]]'
exports:
  - TP-WKS
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL.md
  - .docs/wiki/04_RF/RF-WKS-007.md
  - .docs/wiki/06_matriz_pruebas_RF.md
  - .docs/wiki/06_pruebas/TP-WKS.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-WKS.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - go test -count=1 ./internal/service ./internal/workspace ./internal/cli
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
stop_if:
  - any_test_failed=true
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - wiki_source_verdict=BLOCKED
evidence:
  - .docs/wiki/03_FL.md
  - .docs/wiki/04_RF/RF-WKS-007.md
  - .docs/wiki/06_matriz_pruebas_RF.md
  - .docs/wiki/06_pruebas/TP-WKS.md
  - internal/service/auto_register_test.go
  - internal/workspace/autoregister_test.go
---

# TP-WKS

wiki_source_table_exception: true

La tabla de casos `TC-WKS-001..047` se conserva como índice humano histórico; los casos nuevos `TC-WKS-048..060` tienen autoridad únicamente en sus bloques TOON normativos, sin duplicarse en esa tabla.

## Cobertura objetivo

- RF-WKS-001
- RF-WKS-002
- RF-WKS-003
- RF-WKS-004
- RF-WKS-005
- RF-WKS-006
- RF-WKS-007
- RF-WKS-008
- RF-WKS-009

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-WKS-001 | positivo | RF-WKS-001 | registra workspace compatible con alias explícito y conserva un entrypoint Go anidado mediante una ruta exacta relativa al workspace, por ejemplo `runtime/go.mod`; el selector puede ser repo-local explícito (`go.mod`) o un ID generado de `WorkspaceEntrypoint` resuelto por repo+entrypoint exactos y rebasado/revalidado como `go.mod` seguro y regular; selector vacío conserva el fallback del root; un selector explícito `go.work` se rechaza y un root solo con `go.work` omite Go; cualquier selector/ID desconocido, malformado, inseguro, inexistente o no regular queda omitido sin fallback |
| TC-WKS-002 | positivo | RF-WKS-001 | registra workspace con alias derivado del root |
| TC-WKS-003 | negativo | RF-WKS-001 | rechaza path inexistente o layout incompatible sin side effects |
| TC-WKS-004 | positivo | RF-WKS-001 | detecta workspace Python con `pyproject.toml` y reporta `language: python` |
| TC-WKS-005 | positivo | RF-WKS-001 | detecta workspace mixto Python+TS y reporta ambos lenguajes |
| TC-WKS-006 | positivo | RF-WKS-002 | workspace add sin --no-index indexa automaticamente |
| TC-WKS-007 | positivo | RF-WKS-002 | workspace add con --no-index salta indexing |
| TC-WKS-008 | negativo | RF-WKS-002 | workspace add indexa pero falla → warning, registro exitoso |
| TC-WKS-009 | positivo | RF-WKS-003 | `init` registra el repo actual, indexa y devuelve `next_steps` para `nav ask` |
| TC-WKS-010 | negativo | RF-WKS-003 | `init` rechaza un path incompatible sin registro parcial |
| TC-WKS-011 | positivo | RF-WKS-004 | `mi-lsp` devuelve home content-first por default y `mi-lsp --classic` vuelve a help generica |
| TC-WKS-012 | positivo | RF-WKS-004 | `workspace status` emite vista preview-first por default y `workspace status --full` re-expande detalle |
| TC-WKS-013 | negativo | RF-WKS-004 | `--axi` y `--classic` juntos fallan con error claro |
| TC-WKS-014 | positivo | RF-WKS-005 | `workspace status` expone `governance_profile`, `governance_sync`, `governance_index_sync` y `governance_blocked`; la raíz de gobernanza validada puede declarar `imports: []` |
| TC-WKS-015 | negativo | RF-WKS-005 | si falta `00_gobierno_documental.md` o la gobernanza es inválida, el repo entra en `blocked mode`; contratos no raíz con `imports: []` siguen bloqueados |
| TC-WKS-016 | positivo | RF-WKS-004 | `TestAXIFalseDisablesDefaultAXISurface`: `--axi=false` explícito deshabilita AXI incluso en superficies AXI-default (Wave 3b hard disable) |
| TC-WKS-017 | positivo | RF-WKS-004 | `workspace list` preserva aliases duplicados del mismo root sin deduplicar ni borrar registros |
| TC-WKS-018 | positivo | RF-WKS-004 | `workspace list --group-by-root` agrupa por root y expone `alias_count`, `aliases`, `canonical_alias`, `selection_reason`, `kind` y warnings |
| TC-WKS-019 | positivo | RF-WKS-005 | `workspace status` sin `--workspace` resuelve por `caller_cwd` dentro del worktree/workspace registrado y expone `workspace_source=caller_cwd` |
| TC-WKS-020 | positivo | RF-WKS-005 | `workspace status --workspace <alias>` explicito gana sobre `caller_cwd`, pero emite warning si el CWD pertenece a otro root registrado |
| TC-WKS-021 | positivo | RF-WKS-004 | `workspace doctor` reporta aliases que comparten root exacto sin mutar `registry.toml` |
| TC-WKS-022 | positivo | RF-WKS-004 | `workspace doctor` reporta familias de worktrees que comparten `git common dir` pero tienen roots fisicos distintos |
| TC-WKS-023 | positivo | RF-WKS-004 | `workspace prune --stale --dry-run` lista aliases con root inexistente sin modificar `registry.toml` |
| TC-WKS-024 | positivo | RF-WKS-004 | `workspace prune --stale --apply` remueve solo aliases con root inexistente y no borra worktrees ni indices |
| TC-WKS-025 | positivo | RF-WKS-005 | `workspace status --full` refresca memoria stale cuando `auto_sync` esta habilitado y la conserva stale con `--no-auto-sync` |
| TC-WKS-026 | positivo | RF-WKS-001, RF-WKS-002 | `workspace doctor` agrega `health` y `next_actions` accionables sin mutar registry, worktrees ni indices |
| TC-WKS-027 | positivo | RF-WKS-006 | `TestRootCommandExposesVersionCommand` + `TestBuildVersionInfoUsesRuntimeProvenance`: `mi-lsp version` existe, no requiere workspace/daemon y expone Go/runtime/protocol/RID/path/hash provenance |
| TC-WKS-028 | positivo | RF-WKS-006 | `TestRenderTextVersionInfo`: la salida `text` de version es legible y los formatos estructurados conservan el envelope `backend=version` |
| TC-WKS-029 | positivo | RF-WKS-001, RF-WKS-002 | `TestDoctorWorkspacesBinaryRevisionDriftAction`: `workspace doctor` detecta drift de revision entre binarios `mi-lsp` visibles y agrega `review_binary_version_drift` sin podar ni modificar PATH |
| TC-WKS-030 | positivo | RF-WKS-006 | `scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror` valida el gate AE sin publicar, exige artefactos por RID cuando no se salta build y reporta paths/SHA para provenance |
| TC-WKS-031 | positivo | RF-WKS-004 | `workspace hygiene` reporta `backend=registry-hygiene`, aliases stale, familias de worktrees y acciones seguras sin mutar registry por default |
| TC-WKS-032 | positivo | RF-WKS-004 | `workspace hygiene --apply-safe` remueve solo aliases con root inexistente via la logica existente de poda segura y conserva aliases vivos, roots, worktrees e indices |
| TC-WKS-033 | positivo | RF-WKS-005, RF-QRY-013 | `workspace status`/`nav governance` exponen detalles de stale index con timestamps comparados y comando de reindex accionable |
| TC-WKS-034 | positivo | RF-WKS-004 | `TestDoctorWorkspacesReportsGitCaseCollisions`: `workspace doctor` detecta paths versionados que difieren solo por casing y emite `fix_git_case_collisions` sin mutar registry ni worktrees |
| TC-WKS-035 | positivo | RF-WKS-005 | `TestExecuteWorkspaceStatusAgentReadsExplicitAliasOutsideCallerCWDWithWarning` + `TestExecuteNavGovernanceAgentReadsExplicitAliasOutsideCallerCWDWithWarning`: caller agente con `--workspace` fuera del `caller_cwd` LEE sin `--allow-cross-workspace`, con warning de mismatch y sin registrar override |
| TC-WKS-036 | positivo | RF-WKS-005 | `TestExecuteWorkspaceStatusAgentAllowsExplicitCrossWorkspaceOverride`: `--allow-cross-workspace` permite el uso intencional y conserva warning auditable |
| TC-WKS-061 | negativo | RF-WKS-005 | `TestCrossWorkspaceWriteAgentStillRefusedWithoutOverride`: `index.start`, `workspace.link` y `prepare.create` cross-workspace de un caller agente reciben `workspace_cross_workspace_refused` con hint hacia `--allow-cross-workspace` |
| TC-WKS-062 | positivo | RF-WKS-005 | `TestCrossWorkspaceWriteAgentOverrideIsRecorded`: el override en una escritura procede, emite warning y agrega una línea JSONL a `~/.mi-lsp/overrides.log` |
| TC-WKS-063 | negativo | RF-WKS-005, RF-IDX-001 | `TestCrossWorkspaceReadIsMarkedForSideEffectGating` y `TestCrossWorkspaceReadDoesNotStartReindexWithoutOverride`: una lectura cross-workspace de un harness se marca; sin override no lanza el reindex en segundo plano ni registra nada; con `--allow-cross-workspace` lo lanza y agrega `index.autoindex` a `overrides.log` |
| TC-WKS-037 | positivo | RF-WKS-004 | `TestWorkspaceHygieneReportsLiveReadinessIssuesWithoutPruning`: `workspace hygiene` reporta aliases vivos sin gobernanza/docs como `workspace_readiness_issues` y `--apply-safe` no los remueve |
| TC-WKS-038 | positivo | RF-WKS-008 | `TestResolveCanonsRelativeFromWorkspaceRoot`: `[[RF-WKS-008|canon]]` con `root="../wiki-repo/Ingenieria"` se resuelve desde la raíz del workspace, no desde cwd ni `.mi-lsp/` |
| TC-WKS-039 | positivo | RF-WKS-008 | `TestCanonLinksRoundTripSaveLoadRegistry` + `workspace link <alias> --role`: persiste `CanonLinks` en `registry.toml` |
| TC-WKS-040 | positivo | RF-WKS-008 | `index --docs-only` recorre markdown `../` de raíces `[[RF-WKS-008|canon]]` válidas y omite roots inválidos con warning |
| TC-WKS-041 | negativo | RF-WKS-008 | rechaza root absoluto, `~/`, UNC o `C:`; `escape_max` omitido=1 y `0` prohíbe parent escape extra |
| TC-WKS-042 | negativo | RF-WKS-008 | `TestResolveCanonsRejectsSymlinkComponent`: symlink o junction en el path resuelto falla cerrado |
| TC-WKS-043 | negativo | RF-WKS-008 | `TestIndexWorkspaceDocsSkipsCanonSymlinkMarkdown`: un `.md` symlink dentro de una raíz `[[RF-WKS-008|canon]]` no se indexa |
| TC-WKS-044 | positivo | RF-WKS-009 | `TestWorkspaceWhichCommandDoesNotUseDaemon`: `workspace which` existe, expone `--cwd` y nunca dispara el hook de ejecucion de operacion (no toca daemon) |
| TC-WKS-045 | positivo | RF-WKS-009 | `TestWorkspaceWhichPreservesWindowsBackslashPath` + `TestLiteralWorkspacePathDoesNotUnquoteJSONEscapes`: un root Windows con backslash simple se conserva literal en la salida JSON, sin colapsar segmentos ni interpretar `\r`/`\m`/`\p` como escapes |
| TC-WKS-046 | positivo | RF-WKS-009 | `TestWorkspaceWhichResolvesCWDAndPathSelector`: resuelve por `--workspace` explicito (alias o path), por cwd dentro del root registrado y por `last_workspace`, con `source` correcto en cada caso |
| TC-WKS-047 | negativo | RF-WKS-009 | documentado en `internal/cli/workspace_which.go:68,89` (`resolveWorkspaceWhich`): un `--workspace` explicito no registrado, o la ausencia de selector con cwd fuera de cualquier root y sin `last_workspace` configurado, devuelven error explicito; sin test Go nombrado que cubra ambas ramas de error (gap residual, ver auditoria) |

## Casos normativos de auto-registro

Los casos `TC-WKS-048..060` se definen únicamente en estos bloques TOON; las filas históricas `TC-WKS-001..047` permanecen intactas en la tabla anterior.

### TC-WKS-048

```toon
block_id: TC-WKS-048
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterSkipsIndexWithoutCommits$'
evidence:
  - internal/service/auto_register_test.go
case_id: TC-WKS-048
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/service/auto_register_test.go
    test: TestExecuteAutoRegisterSkipsIndexWithoutCommits
expected:
  auto_register_warning: auto_register_index_skipped
  query_continues: true
  index_start: forbidden
```

### TC-WKS-049

```toon
block_id: TC-WKS-049
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterOptOut$'
evidence:
  - internal/service/auto_register_test.go
case_id: TC-WKS-049
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/service/auto_register_test.go
    test: TestExecuteAutoRegisterOptOut
coverage_scope:
  covered_inputs: [request.Context.NoAutoRegister, MI_LSP_NO_AUTO_REGISTER=1]
  cli_flag_to_daemon_e2e: pending_no_automated_oracle
expected:
  auto_register: disabled
  index_start: disabled
```

### TC-WKS-050

```toon
block_id: TC-WKS-050
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegistersOnFirstQuery$'
evidence:
  - internal/service/auto_register_test.go
case_id: TC-WKS-050
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/service/auto_register_test.go
    test: TestExecuteAutoRegistersOnFirstQuery
expected:
  first_query_registers: true
  index_start: one_background_job
  repeated_query_duplicate_job: false
```

### TC-WKS-051

```toon
block_id: TC-WKS-051
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterNameCollisionUsesDeterministicSuffix|TestAutoRegisterReusesExistingAliasForSameRoot'
evidence:
  - internal/workspace/autoregister_test.go
case_id: TC-WKS-051
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterNameCollisionUsesDeterministicSuffix
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterReusesExistingAliasForSameRoot
expected:
  collision_alias: deterministic_suffix
  existing_alias_for_same_root: reused
  existing_alias_overwrite: forbidden
```

### TC-WKS-052

```toon
block_id: TC-WKS-052
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterConcurrentQueriesIndexOnce$'
evidence:
  - internal/service/auto_register_test.go
case_id: TC-WKS-052
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/service/auto_register_test.go
    test: TestExecuteAutoRegisterConcurrentQueriesIndexOnce
expected:
  concurrent_queries_same_root: one_registration
  index_start: one_background_job
```

### TC-WKS-053

```toon
block_id: TC-WKS-053
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterRetriesAfterProjectCreationFailure$'
  - go test -count=1 ./internal/service -run '^TestExecuteRetriesPartialAutoRegistrationAndStartsIndex$'
evidence:
  - internal/workspace/autoregister_test.go
  - internal/service/auto_register_test.go
case_id: TC-WKS-053
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterRetriesAfterProjectCreationFailure
  - file: internal/service/auto_register_test.go
    test: TestExecuteRetriesPartialAutoRegistrationAndStartsIndex
expected:
  failed_project_creation_leaves_partial_registration: false
  retry_completes_registration: true
  index_start_after_recovery: true
```

### TC-WKS-054

```toon
block_id: TC-WKS-054
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterFailureDoesNotBlockNavMultiRead$'
evidence:
  - internal/service/auto_register_test.go
case_id: TC-WKS-054
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/service/auto_register_test.go
    test: TestExecuteAutoRegisterFailureDoesNotBlockNavMultiRead
expected:
  nav_multi_read_completes: true
  auto_register_failure_reported_as_warning: true
  registry_persisted_on_failure: false
```

### TC-WKS-055

```toon
block_id: TC-WKS-055
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterByRequestedPath$'
evidence:
  - internal/workspace/autoregister_test.go
case_id: TC-WKS-055
case_type: negative
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterByRequestedPath
expected:
  unknown_alias_or_nonexistent_path: not_registered
```

### TC-WKS-056

```toon
block_id: TC-WKS-056
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterByRequestedPath$'
evidence:
  - internal/workspace/autoregister_test.go
case_id: TC-WKS-056
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterByRequestedPath
expected:
  requested_existing_git_repository: registered
  alias: derived_from_git_root
```

### TC-WKS-057

```toon
block_id: TC-WKS-057
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterNeverRegistersHomeOrRoot|TestAutoRegisterSkipsNonGitDirectory'
evidence:
  - internal/workspace/autoregister_test.go
case_id: TC-WKS-057
case_type: negative
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterNeverRegistersHomeOrRoot
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterSkipsNonGitDirectory
expected:
  home_registered: false
  filesystem_root_registered: false
  non_git_directory_registered: false
```

### TC-WKS-058

```toon
block_id: TC-WKS-058
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterKeepsExistingProjectToml|TestAutoRegisterFirstQueryInUnregisteredGitRepo|TestSaveRegistryPreservesRestrictivePermissions'
evidence:
  - internal/workspace/autoregister_test.go
  - internal/workspace/registry.go
  - internal/workspace/registry_test.go
case_id: TC-WKS-058
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterKeepsExistingProjectToml
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterFirstQueryInUnregisteredGitRepo
  - file: internal/workspace/registry_test.go
    test: TestSaveRegistryPreservesRestrictivePermissions
expected:
  existing_project_toml_overwritten: false
  last_workspace_changed: false
  new_registry_mode_argument: "0600"
  existing_registry_permission_bits: preserved
  permission_test_scope: non_windows_checks_mode_0600_on_create_and_after_explicit_chmod
```

### TC-WKS-059

```toon
block_id: TC-WKS-059
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterTwoProcesses$'
evidence:
  - internal/workspace/autoregister_test.go
case_id: TC-WKS-059
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterTwoProcesses
expected:
  process_count: 2
  roots: distinct_same_basename
  registry_entries_survive: true
```

### TC-WKS-060

```toon
block_id: TC-WKS-060
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterLockTimeoutWithLiveHolder$'
evidence:
  - internal/workspace/autoregister_test.go
case_id: TC-WKS-060
case_type: negative
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterLockTimeoutWithLiveHolder
expected:
  live_holder_timeout_error: registry_lock_timeout
  retry_after_lock_release: succeeds
```

### TC-WKS-064

```toon
block_id: TC-WKS-064
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterLinkedWorktreeUsesRegisteredMainRoot|TestAutoRegisterLinkedWorktreeWithoutRegisteredMainFailsClosed'
  - go test -count=1 ./internal/service -run '^TestExecuteLinkedWorktreeWithoutRegisteredMainDoesNotIndex$'
evidence:
  - internal/workspace/autoregister_test.go
  - internal/service/auto_register_test.go
case_id: TC-WKS-064
case_type: positive_and_negative
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterLinkedWorktreeUsesRegisteredMainRoot
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterLinkedWorktreeWithoutRegisteredMainFailsClosed
  - file: internal/service/auto_register_test.go
    test: TestExecuteLinkedWorktreeWithoutRegisteredMainDoesNotIndex
expected:
  registered_main: resolve_without_worktree_registration
  unregistered_main: explicit_incomplete_reason_code_invalid_workspace_without_index
  registry_consistent: true
```

### TC-WKS-065

```toon
block_id: TC-WKS-065
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterForceBypassesDefaultScope|TestAutoRegisterTempRootIsDeniedEvenWhenHomeContainsRepo|TestGarbageCollectRegistryRetainsRootsWithAmbiguousErrors|TestPruneStaleWorkspacesDryRunAndApply'
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterForceOverrideIsReported$'
evidence:
  - internal/workspace/autoregister_test.go
  - internal/workspace/registry_test.go
  - internal/service/auto_register_test.go
case_id: TC-WKS-065
case_type: positive_and_negative
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterForceBypassesDefaultScope
  - file: internal/workspace/autoregister_test.go
    test: TestAutoRegisterTempRootIsDeniedEvenWhenHomeContainsRepo
  - file: internal/workspace/registry_test.go
    test: TestGarbageCollectRegistryRetainsRootsWithAmbiguousErrors
  - file: internal/workspace/registry_test.go
    test: TestPruneStaleWorkspacesDryRunAndApply
  - file: internal/service/auto_register_test.go
    test: TestExecuteAutoRegisterForceOverrideIsReported
expected:
  outside_HOME_default_registration: false
  temporary_HOME_default_registration: false
  force_override: explicit_and_reported
  prune_only_definitely_missing_root: true
  ambiguous_errors_and_empty_roots: retained
  existing_directories_and_worktrees_deleted: false
```

### TC-WKS-066

```toon
block_id: TC-WKS-066
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/cli -run '^TestWorkspacePruneDefaultsToSafeStalePreview$'
evidence:
  - internal/cli/workspace.go
  - internal/cli/workspace_test.go
case_id: TC-WKS-066
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/cli/workspace_test.go
    test: TestWorkspacePruneDefaultsToSafeStalePreview
expected:
  command_without_flags: accepted
  stale_only: true
  default_mode: dry_run
  compatibility_flag_stale: accepted
```

### TC-WKS-067

```toon
block_id: TC-WKS-067
kind: normative
source_of_truth: normative
verify:
  - go test -count=1 ./internal/workspace ./internal/service -run 'TestListWorkspacesGarbageCollectsMissingRoots|TestResolveWorkspaceSelectionRejectsStaleLastWorkspace|TestProbeNestedGitWorktreeDoesNotReadLexicalParentState|TestExecuteWorkspaceStatusUnregisteredNestedGitWorktreeKeepsPhysicalRoot'
evidence:
  - internal/workspace/registry.go
  - internal/workspace/registry_test.go
  - internal/service/workspace_ops.go
case_id: TC-WKS-067
case_type: positive
requirement_id: RF-WKS-007
test_bindings:
  - file: internal/workspace/registry_test.go
    test: TestListWorkspacesGarbageCollectsMissingRoots
  - file: internal/workspace/registry_test.go
    test: TestResolveWorkspaceSelectionRejectsStaleLastWorkspace
  - file: internal/service/probe_test.go
    test: TestProbeNestedGitWorktreeDoesNotReadLexicalParentState
  - file: internal/service/workspace_resolution_test.go
    test: TestExecuteWorkspaceStatusUnregisteredNestedGitWorktreeKeepsPhysicalRoot
  - file: internal/service/workspace_resolution_test.go
    test: TestExecuteWorkspaceStatusExplicitDotUsesGitAwareCallerRoot
expected:
  public_list_gc_persisted: true
  ordinary_resolution_gc_persisted: true
  ordinary_read_only_unregistered_linked_worktree_with_registered_main: resolve_main_alias
  stale_last_selector_error_preserved: WKS_SELECTOR_STALE
  read_only_resolution_and_doctor_hygiene_preview_mutate_registry: false
  ordinary_resolution_git_failure_for_linked_marker: registered_containment_only
  status_probe_preserve_unregistered_linked_worktree_physical_root_without_registration_or_index: true
  status_probe_git_failure_preserve_physical_root_without_parent_state_inspection: true
```
