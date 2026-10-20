---
wiki_contract: WIKI-CONTRACT-v1
id: TP-WKS
kind: test
status: active
title: TP-WKS
links:
  verifies:
  - RF-IDX-001
  - RF-QRY-013
  - RF-WKS-001
  - RF-WKS-002
  - RF-WKS-003
  - RF-WKS-004
  - RF-WKS-005
  - RF-WKS-006
  - RF-WKS-007
  - RF-WKS-008
  - RF-WKS-009
  code:
  - path: internal/workspace/autoregister_test.go
    role: test
---

## Alcance

RF-IDX-001, RF-QRY-013, RF-WKS-001, RF-WKS-002, RF-WKS-003, RF-WKS-004, RF-WKS-005, RF-WKS-006, RF-WKS-007, RF-WKS-008, RF-WKS-009. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

wiki_source_table_exception: true

La tabla de casos `TC-WKS-001..047` se conserva como índice humano histórico; los casos nuevos `TC-WKS-048..060` tienen autoridad únicamente en sus bloques TOON normativos, sin duplicarse en esa tabla.

### Reparación fuente doctor/versión — 2026-10-13

No cambia la tabla histórica ni agrega bindings de tests ficticios. Las condiciones siguientes son obligaciones de fuente, no resultados de una campaña nueva:

- **Marcador estable**: tp-wks-doctor-version-source-repair
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace <alias> --ids TP-WKS --format toon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WKS.md
  - internal/cli/root.go
  - internal/cli/version.go
- **contracts**:
  - CT-CLI-DAEMON-ADMIN
  - CT-Q-V1
- **requirements**:
  - RF-WKS-006
- **Estado del comportamiento**: explicit_incomplete
- **budget**: exhausted_FINAL1_audit1_recheck1
- **original_doctor_result**: FAIL_double_JSON
- **remaining_LSP_03_04_05_06**: NOT_RUN
- **source_obligations**:
  - **doctor_failed_checks**: one_envelope_and_nonzero_exit
  - **formats**:
    - text
    - json
    - toon
  - **unprinted_errors**: normal_outer_handler
  - **release_version**: exact_without_v_binary_and_dynamic_provider
  - **candidate_generator**: same_release_version_for_provider_and_tool_card
- **runtime_schema_windows_conformance**: not_claimed
- **owner**: coord-herramientas_coord-mi-lsp
- **expires_at**: 2026-10-18 23:00:00+00:00

Los planes y comandos históricos de este documento no autorizan ejecuciones adicionales para LSPstd. Publicar fuente acotada no convierte el FAIL retenido en PASS.

### Cobertura objetivo

- RF-WKS-001
- RF-WKS-002
- RF-WKS-003
- RF-WKS-004
- RF-WKS-005
- RF-WKS-006
- RF-WKS-007
- RF-WKS-008
- RF-WKS-009

### Escenarios
- **Caso**: TC-WKS-001; **Tipo**: positivo; **RF**: RF-WKS-001; **Descripción**: registra workspace compatible con alias explícito y conserva un entrypoint Go anidado mediante una ruta exacta relativa al workspace, por ejemplo `runtime/go.mod`; el selector puede ser repo-local explícito (`go.mod`) o un ID generado de `WorkspaceEntrypoint` resuelto por repo+entrypoint exactos y rebasado/revalidado como `go.mod` seguro y regular; selector vacío conserva el fallback del root; un selector explícito `go.work` se rechaza y un root solo con `go.work` omite Go; cualquier selector/ID desconocido, malformado, inseguro, inexistente o no regular queda omitido sin fallback
  - RF-WKS-001 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: registra workspace compatible con alias explícito y conserva un entrypoint Go anidado mediante una ruta exacta relativa al workspace, por ejemplo `runtime/go.mod`; el selector puede ser repo-local explícito (`go.mod`) o un ID generado de `WorkspaceEntrypoint` resuelto por repo+entrypoint exactos y rebasado/revalidado como `go.mod` seguro y regular; selector vacío conserva el fallback del root; un selector explícito `go.work` se rechaza y un root solo con `go.work` omite Go; cualquier selector/ID desconocido, malformado, inseguro, inexistente o no regular queda omitido sin fallback. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-002; **Tipo**: positivo; **RF**: RF-WKS-001; **Descripción**: registra workspace con alias derivado del root
  - RF-WKS-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: registra workspace con alias derivado del root. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-003; **Tipo**: negativo; **RF**: RF-WKS-001; **Descripción**: rechaza path inexistente o layout incompatible sin side effects
  - RF-WKS-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: rechaza path inexistente o layout incompatible sin side effects. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-004; **Tipo**: positivo; **RF**: RF-WKS-001; **Descripción**: detecta workspace Python con `pyproject.toml` y reporta `language: python`
  - RF-WKS-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: detecta workspace Python con `pyproject.toml` y reporta `language: python`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-005; **Tipo**: positivo; **RF**: RF-WKS-001; **Descripción**: detecta workspace mixto Python+TS y reporta ambos lenguajes
  - RF-WKS-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: detecta workspace mixto Python+TS y reporta ambos lenguajes. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-006; **Tipo**: positivo; **RF**: RF-WKS-002; **Descripción**: workspace add sin --no-index indexa automáticamente
  - RF-WKS-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: workspace add sin --no-index indexa automáticamente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-007; **Tipo**: positivo; **RF**: RF-WKS-002; **Descripción**: workspace add con --no-index salta indexing
  - RF-WKS-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: workspace add con --no-index salta indexing. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-008; **Tipo**: negativo; **RF**: RF-WKS-002; **Descripción**: workspace add indexa pero falla → warning, registro exitoso
  - RF-WKS-002 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: workspace add indexa pero falla → warning, registro exitoso. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-009; **Tipo**: positivo; **RF**: RF-WKS-003; **Descripción**: `init` registra el repo actual, indexa y devuelve `next_steps` para `nav ask`
  - RF-WKS-003 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `init` registra el repo actual, indexa y devuelve `next_steps` para `nav ask`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-010; **Tipo**: negativo; **RF**: RF-WKS-003; **Descripción**: `init` rechaza un path incompatible sin registro parcial
  - RF-WKS-003 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `init` rechaza un path incompatible sin registro parcial. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-011; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `mi-lsp` devuelve home content-first por default y `mi-lsp --classic` vuelve a help generica
  - RF-WKS-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `mi-lsp` devuelve home content-first por default y `mi-lsp --classic` vuelve a help generica. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-012; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace status` emite vista preview-first por default y `workspace status --full` re-expande detalle
  - RF-WKS-004 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `workspace status` emite vista preview-first por default y `workspace status --full` re-expande detalle. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-013; **Tipo**: negativo; **RF**: RF-WKS-004; **Descripción**: `--axi` y `--classic` juntos fallan con error claro
  - RF-WKS-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `--axi` y `--classic` juntos fallan con error claro. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-014; **Tipo**: positivo; **RF**: RF-WKS-005; **Descripción**: `workspace status` expone `governance_profile`, `governance_sync`, `governance_index_sync` y `governance_blocked`; la raíz de gobernanza validada puede declarar `imports: []`
  - RF-WKS-005 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `workspace status` expone `governance_profile`, `governance_sync`, `governance_index_sync` y `governance_blocked`; la raíz de gobernanza validada puede declarar `imports: []`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-015; **Tipo**: negativo; **RF**: RF-WKS-005; **Descripción**: si falta `00_governance.md` o la gobernanza es inválida, el repo entra en `blocked mode`; contratos no raíz con `imports: []` siguen bloqueados
  - RF-WKS-005 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: si falta `00_governance.md` o la gobernanza es inválida, el repo entra en `blocked mode`; contratos no raíz con `imports: []` siguen bloqueados. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-016; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `TestAXIFalseDisablesDefaultAXISurface`: `--axi=false` explícito deshabilita AXI incluso en superficies AXI-default (Wave 3b hard disable)
  - RF-WKS-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `TestAXIFalseDisablesDefaultAXISurface`: `--axi=false` explícito deshabilita AXI incluso en superficies AXI-default (Wave 3b hard disable). No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-017; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace list` preserva aliases duplicados del mismo root sin deduplicar ni borrar registros
  - RF-WKS-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `workspace list` preserva aliases duplicados del mismo root sin deduplicar ni borrar registros. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-018; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace list --group-by-root` agrupa por root y expone `alias_count`, `aliases`, `canonical_alias`, `selection_reason`, `kind` y warnings
  - RF-WKS-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `workspace list --group-by-root` agrupa por root y expone `alias_count`, `aliases`, `canonical_alias`, `selection_reason`, `kind` y warnings. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-019; **Tipo**: positivo; **RF**: RF-WKS-005; **Descripción**: `workspace status` sin `--workspace` resuelve por `caller_cwd` dentro del worktree/workspace registrado y expone `workspace_source=caller_cwd`
  - RF-WKS-005 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `workspace status` sin `--workspace` resuelve por `caller_cwd` dentro del worktree/workspace registrado y expone `workspace_source=caller_cwd`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-020; **Tipo**: positivo; **RF**: RF-WKS-005; **Descripción**: `workspace status --workspace <alias>` explícito gana sobre `caller_cwd`, pero emite warning si el CWD pertenece a otro root registrado
  - RF-WKS-005 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `workspace status --workspace <alias>` explícito gana sobre `caller_cwd`, pero emite warning si el CWD pertenece a otro root registrado. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-021; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace doctor` reporta aliases que comparten root exacto sin mutar `registry.toml`
  - RF-WKS-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `workspace doctor` reporta aliases que comparten root exacto sin mutar `registry.toml`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-022; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace doctor` reporta familias de worktrees que comparten `git common dir` pero tienen roots fisicos distintos
  - RF-WKS-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `workspace doctor` reporta familias de worktrees que comparten `git common dir` pero tienen roots fisicos distintos. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-023; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace prune --stale --dry-run` lista aliases con root inexistente sin modificar `registry.toml`
  - RF-WKS-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `workspace prune --stale --dry-run` lista aliases con root inexistente sin modificar `registry.toml`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-024; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace prune --stale --apply` remueve solo aliases con root inexistente y no borra worktrees ni índices
  - RF-WKS-004 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `workspace prune --stale --apply` remueve solo aliases con root inexistente y no borra worktrees ni índices. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-025; **Tipo**: positivo; **RF**: RF-WKS-005; **Descripción**: `workspace status --full` refresca memoria stale cuando `auto_sync` esta habilitado y la conserva stale con `--no-auto-sync`
  - RF-WKS-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `workspace status --full` refresca memoria stale cuando `auto_sync` esta habilitado y la conserva stale con `--no-auto-sync`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-026; **Tipo**: positivo; **RF**: RF-WKS-001, RF-WKS-002; **Descripción**: `workspace doctor` agrega `health` y `next_actions` accionables sin mutar registry, worktrees ni índices
  - RF-WKS-001 — Criterio 1; RF-WKS-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `workspace doctor` agrega `health` y `next_actions` accionables sin mutar registry, worktrees ni índices. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-027; **Tipo**: positivo; **RF**: RF-WKS-006; **Descripción**: `TestRootCommandExposesVersionCommand` + `TestBuildVersionInfoUsesRuntimeProvenance`: `mi-lsp version` existe, no requiere workspace/daemon y expone Go/runtime/protocol/RID/path/hash provenance
  - RF-WKS-006 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `TestRootCommandExposesVersionCommand` + `TestBuildVersionInfoUsesRuntimeProvenance`: `mi-lsp version` existe, no requiere workspace/daemon y expone Go/runtime/protocol/RID/path/hash provenance. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-028; **Tipo**: positivo; **RF**: RF-WKS-006; **Descripción**: `TestRenderTextVersionInfo`: la salida `text` de version es legible y los formatos estructurados conservan el envelope `backend=version`
  - RF-WKS-006 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `TestRenderTextVersionInfo`: la salida `text` de version es legible y los formatos estructurados conservan el envelope `backend=version`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-029; **Tipo**: positivo; **RF**: RF-WKS-001, RF-WKS-002; **Descripción**: `TestDoctorWorkspacesBinaryRevisionDriftAction`: `workspace doctor` detecta drift de revision entre binarios `mi-lsp` visibles y agrega `review_binary_version_drift` sin podar ni modificar PATH
  - RF-WKS-001 — Criterio 1; RF-WKS-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestDoctorWorkspacesBinaryRevisionDriftAction`: `workspace doctor` detecta drift de revision entre binarios `mi-lsp` visibles y agrega `review_binary_version_drift` sin podar ni modificar PATH. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-030; **Tipo**: positivo; **RF**: RF-WKS-006; **Descripción**: `scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror` valida el gate AE sin publicar, exige artefactos por RID cuando no se salta build y reporta paths/SHA para provenance
  - RF-WKS-006 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror` valida el gate AE sin publicar, exige artefactos por RID cuando no se salta build y reporta paths/SHA para provenance. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-031; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace hygiene` reporta `backend=registry-hygiene`, aliases stale, familias de worktrees y acciones seguras sin mutar registry por default
  - RF-WKS-004 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `workspace hygiene` reporta `backend=registry-hygiene`, aliases stale, familias de worktrees y acciones seguras sin mutar registry por default. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-032; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `workspace hygiene --apply-safe` remueve solo aliases con root inexistente via la logica existente de poda segura y conserva aliases vivos, roots, worktrees e índices
  - RF-WKS-004 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `workspace hygiene --apply-safe` remueve solo aliases con root inexistente via la logica existente de poda segura y conserva aliases vivos, roots, worktrees e índices. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-033; **Tipo**: positivo; **RF**: RF-WKS-005, RF-QRY-013; **Descripción**: `workspace status`/`nav governance` exponen detalles de stale index con timestamps comparados y comando de reindex accionable
  - RF-QRY-013 — Criterio 5; RF-WKS-005 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `workspace status`/`nav governance` exponen detalles de stale index con timestamps comparados y comando de reindex accionable. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-034; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `TestDoctorWorkspacesReportsGitCaseCollisions`: `workspace doctor` detecta paths versionados que difieren solo por casing y emite `fix_git_case_collisions` sin mutar registry ni worktrees
  - RF-WKS-004 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `TestDoctorWorkspacesReportsGitCaseCollisions`: `workspace doctor` detecta paths versionados que difieren solo por casing y emite `fix_git_case_collisions` sin mutar registry ni worktrees. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-035; **Tipo**: positivo; **RF**: RF-WKS-005; **Descripción**: `TestExecuteWorkspaceStatusAgentReadsExplicitAliasOutsideCallerCWDWithWarning` + `TestExecuteNavGovernanceAgentReadsExplicitAliasOutsideCallerCWDWithWarning`: caller agente con `--workspace` fuera del `caller_cwd` LEE sin `--allow-cross-workspace`, con warning de mismatch y sin registrar override
  - RF-WKS-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestExecuteWorkspaceStatusAgentReadsExplicitAliasOutsideCallerCWDWithWarning` + `TestExecuteNavGovernanceAgentReadsExplicitAliasOutsideCallerCWDWithWarning`: caller agente con `--workspace` fuera del `caller_cwd` LEE sin `--allow-cross-workspace`, con warning de mismatch y sin registrar override. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-036; **Tipo**: positivo; **RF**: RF-WKS-005; **Descripción**: `TestExecuteWorkspaceStatusAgentAllowsExplicitCrossWorkspaceOverride`: `--allow-cross-workspace` permite el uso intencional y conserva warning auditable
  - RF-WKS-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestExecuteWorkspaceStatusAgentAllowsExplicitCrossWorkspaceOverride`: `--allow-cross-workspace` permite el uso intencional y conserva warning auditable. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-061; **Tipo**: negativo; **RF**: RF-WKS-005; **Descripción**: `TestCrossWorkspaceWriteAgentStillRefusedWithoutOverride`: `index.start`, `workspace.link` y `prepare.create` cross-workspace de un caller agente reciben `workspace_cross_workspace_refused` con hint hacia `--allow-cross-workspace`
  - RF-WKS-005 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `TestCrossWorkspaceWriteAgentStillRefusedWithoutOverride`: `index.start`, `workspace.link` y `prepare.create` cross-workspace de un caller agente reciben `workspace_cross_workspace_refused` con hint hacia `--allow-cross-workspace`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-062; **Tipo**: positivo; **RF**: RF-WKS-005; **Descripción**: `TestCrossWorkspaceWriteAgentOverrideIsRecorded`: el override en una escritura procede, emite warning y agrega una línea JSONL a `~/.mi-lsp/overrides.log`
  - RF-WKS-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestCrossWorkspaceWriteAgentOverrideIsRecorded`: el override en una escritura procede, emite warning y agrega una línea JSONL a `~/.mi-lsp/overrides.log`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-063; **Tipo**: negativo; **RF**: RF-WKS-005, RF-IDX-001; **Descripción**: `TestCrossWorkspaceReadIsMarkedForSideEffectGating` y `TestCrossWorkspaceReadDoesNotStartReindexWithoutOverride`: una lectura cross-workspace de un harness se marca; sin override no lanza el reindex en segundo plano ni registra nada; con `--allow-cross-workspace` lo lanza y agrega `index.autoindex` a `overrides.log`
  - RF-IDX-001 — Criterio 3; RF-WKS-005 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `TestCrossWorkspaceReadIsMarkedForSideEffectGating` y `TestCrossWorkspaceReadDoesNotStartReindexWithoutOverride`: una lectura cross-workspace de un harness se marca; sin override no lanza el reindex en segundo plano ni registra nada; con `--allow-cross-workspace` lo lanza y agrega `index.autoindex` a `overrides.log`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-037; **Tipo**: positivo; **RF**: RF-WKS-004; **Descripción**: `TestWorkspaceHygieneReportsLiveReadinessIssuesWithoutPruning`: `workspace hygiene` reporta aliases vivos sin gobernanza/docs como `workspace_readiness_issues` y `--apply-safe` no los remueve
  - RF-WKS-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `TestWorkspaceHygieneReportsLiveReadinessIssuesWithoutPruning`: `workspace hygiene` reporta aliases vivos sin gobernanza/docs como `workspace_readiness_issues` y `--apply-safe` no los remueve. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-038; **Tipo**: positivo; **RF**: RF-WKS-008; **Descripción**: `TestResolveCanonsRelativeFromWorkspaceRoot`: `[[RF-WKS-008|canon]]` con `root="../wiki-repo/Ingenieria"` se resuelve desde la raíz del workspace, no desde cwd ni `.mi-lsp/`
  - RF-WKS-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestResolveCanonsRelativeFromWorkspaceRoot`: `[[RF-WKS-008|canon]]` con `root="../wiki-repo/Ingenieria"` se resuelve desde la raíz del workspace, no desde cwd ni `.mi-lsp/`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-039; **Tipo**: positivo; **RF**: RF-WKS-008; **Descripción**: `TestCanonLinksRoundTripSaveLoadRegistry` + `workspace link <alias> --role`: persiste `CanonLinks` en `registry.toml`
  - RF-WKS-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestCanonLinksRoundTripSaveLoadRegistry` + `workspace link <alias> --role`: persiste `CanonLinks` en `registry.toml`. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-040; **Tipo**: positivo; **RF**: RF-WKS-008; **Descripción**: `index --docs-only` recorre markdown `../` de raíces `[[RF-WKS-008|canon]]` válidas y omite roots inválidos con warning
  - RF-WKS-008 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `index --docs-only` recorre markdown `../` de raíces `[[RF-WKS-008|canon]]` válidas y omite roots inválidos con warning. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-041; **Tipo**: negativo; **RF**: RF-WKS-008; **Descripción**: rechaza root absoluto, `~/`, UNC o `C:`; `escape_max` omitido=1 y `0` prohíbe parent escape extra
  - RF-WKS-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: rechaza root absoluto, `~/`, UNC o `C:`; `escape_max` omitido=1 y `0` prohíbe parent escape extra. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-042; **Tipo**: negativo; **RF**: RF-WKS-008; **Descripción**: `TestResolveCanonsRejectsSymlinkComponent`: symlink o junction en el path resuelto falla cerrado
  - RF-WKS-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestResolveCanonsRejectsSymlinkComponent`: symlink o junction en el path resuelto falla cerrado. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-043; **Tipo**: negativo; **RF**: RF-WKS-008; **Descripción**: `TestIndexWorkspaceDocsSkipsCanonSymlinkMarkdown`: un `.md` symlink dentro de una raíz `[[RF-WKS-008|canon]]` no se indexa
  - RF-WKS-008 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `TestIndexWorkspaceDocsSkipsCanonSymlinkMarkdown`: un `.md` symlink dentro de una raíz `[[RF-WKS-008|canon]]` no se indexa. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-044; **Tipo**: positivo; **RF**: RF-WKS-009; **Descripción**: `TestWorkspaceWhichCommandDoesNotUseDaemon`: `workspace which` existe, expone `--cwd` y nunca dispara el hook de ejecución de operación (no toca daemon)
  - RF-WKS-009 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `TestWorkspaceWhichCommandDoesNotUseDaemon`: `workspace which` existe, expone `--cwd` y nunca dispara el hook de ejecución de operación (no toca daemon). No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-045; **Tipo**: positivo; **RF**: RF-WKS-009; **Descripción**: `TestWorkspaceWhichPreservesWindowsBackslashPath` + `TestLiteralWorkspacePathDoesNotUnquoteJSONEscapes`: un root Windows con backslash simple se conserva literal en la salida JSON, sin colapsar segmentos ni interpretar `\r`/`\m`/`\p` como escapes
  - RF-WKS-009 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `TestWorkspaceWhichPreservesWindowsBackslashPath` + `TestLiteralWorkspacePathDoesNotUnquoteJSONEscapes`: un root Windows con backslash simple se conserva literal en la salida JSON, sin colapsar segmentos ni interpretar `\r`/`\m`/`\p` como escapes. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-046; **Tipo**: positivo; **RF**: RF-WKS-009; **Descripción**: `TestWorkspaceWhichResolvesCWDAndPathSelector`: resuelve por `--workspace` explícito (alias o path), por cwd dentro del root registrado y por `last_workspace`, con `source` correcto en cada caso
  - RF-WKS-009 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `TestWorkspaceWhichResolvesCWDAndPathSelector`: resuelve por `--workspace` explícito (alias o path), por cwd dentro del root registrado y por `last_workspace`, con `source` correcto en cada caso. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-047; **Tipo**: negativo; **RF**: RF-WKS-009; **Descripción**: documentado en `internal/cli/workspace_which.go:68,89` (`resolveWorkspaceWhich`): un `--workspace` explícito no registrado, o la ausencia de selector con cwd fuera de cualquier root y sin `last_workspace` configurado, devuelven error explícito; sin test Go nombrado que cubra ambas ramas de error (gap residual, ver auditoria)
  - RF-WKS-009 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: documentado en `internal/cli/workspace_which.go:68,89` (`resolveWorkspaceWhich`): un `--workspace` explícito no registrado, o la ausencia de selector con cwd fuera de cualquier root y sin `last_workspace` configurado, devuelven error explícito; sin test Go nombrado que cubra ambas ramas de error (gap residual, ver auditoria). No acredita una ejecución de esta migración.

### Casos normativos de auto-registro

Los casos `TC-WKS-048..060` se definen únicamente en estos bloques TOON; las filas históricas `TC-WKS-001..047` permanecen intactas en la tabla anterior.

#### TC-WKS-048
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-048
- **Comprobación prevista**:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterSkipsIndexWithoutCommits$'
- **Evidencia referenciada**:
  - internal/service/auto_register.go
- **case_id**: TC-WKS-048
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/service/auto_register.go
  - **test**: TestExecuteAutoRegisterSkipsIndexWithoutCommits
- **expected**:
  - **auto_register_warning**: auto_register_index_skipped
  - **query_continues**: true
  - **index_start**: forbidden

#### TC-WKS-049
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-049
- **Comprobación prevista**:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterOptOut$'
- **Evidencia referenciada**:
  - internal/service/auto_register.go
- **case_id**: TC-WKS-049
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/service/auto_register.go
  - **test**: TestExecuteAutoRegisterOptOut
- **coverage_scope**:
  - **covered_inputs**:
    - request.Context.NoAutoRegister
    - MI_LSP_NO_AUTO_REGISTER=1
  - **cli_flag_to_daemon_e2e**: pending_no_automated_oracle
- **expected**:
  - **auto_register**: disabled
  - **index_start**: disabled

#### TC-WKS-050
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-050
- **Comprobación prevista**:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegistersOnFirstQuery$'
- **Evidencia referenciada**:
  - internal/service/auto_register.go
- **case_id**: TC-WKS-050
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/service/auto_register.go
  - **test**: TestExecuteAutoRegistersOnFirstQuery
- **expected**:
  - **first_query_registers**: true
  - **index_start**: one_background_job
  - **repeated_query_duplicate_job**: false

#### TC-WKS-051
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-051
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterNameCollisionUsesDeterministicSuffix|TestAutoRegisterReusesExistingAliasForSameRoot'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
- **case_id**: TC-WKS-051
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterNameCollisionUsesDeterministicSuffix
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterReusesExistingAliasForSameRoot
- **expected**:
  - **collision_alias**: deterministic_suffix
  - **existing_alias_for_same_root**: reused
  - **existing_alias_overwrite**: forbidden

#### TC-WKS-052
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-052
- **Comprobación prevista**:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterConcurrentQueriesIndexOnce$'
- **Evidencia referenciada**:
  - internal/service/auto_register.go
- **case_id**: TC-WKS-052
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/service/auto_register.go
  - **test**: TestExecuteAutoRegisterConcurrentQueriesIndexOnce
- **expected**:
  - **concurrent_queries_same_root**: one_registration
  - **index_start**: one_background_job

#### TC-WKS-053
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-053
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterRetriesAfterProjectCreationFailure$'
  - go test -count=1 ./internal/service -run '^TestExecuteRetriesPartialAutoRegistrationAndStartsIndex$'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
  - internal/service/auto_register.go
- **case_id**: TC-WKS-053
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterRetriesAfterProjectCreationFailure
  - **file**: internal/service/auto_register.go
  - **test**: TestExecuteRetriesPartialAutoRegistrationAndStartsIndex
- **expected**:
  - **failed_project_creation_leaves_partial_registration**: false
  - **retry_completes_registration**: true
  - **index_start_after_recovery**: true

#### TC-WKS-054
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-054
- **Comprobación prevista**:
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterFailureDoesNotBlockNavMultiRead$'
- **Evidencia referenciada**:
  - internal/service/auto_register.go
- **case_id**: TC-WKS-054
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/service/auto_register.go
  - **test**: TestExecuteAutoRegisterFailureDoesNotBlockNavMultiRead
- **expected**:
  - **nav_multi_read_completes**: true
  - **auto_register_failure_reported_as_warning**: true
  - **registry_persisted_on_failure**: false

#### TC-WKS-055
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-055
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterByRequestedPath$'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
- **case_id**: TC-WKS-055
- **case_type**: negative
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterByRequestedPath
- **expected**:
  - **unknown_alias_or_nonexistent_path**: not_registered

#### TC-WKS-056
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-056
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterByRequestedPath$'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
- **case_id**: TC-WKS-056
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterByRequestedPath
- **expected**:
  - **requested_existing_git_repository**: registered
  - **alias**: derived_from_git_root

#### TC-WKS-057
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-057
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterNeverRegistersHomeOrRoot|TestAutoRegisterSkipsNonGitDirectory'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
- **case_id**: TC-WKS-057
- **case_type**: negative
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterNeverRegistersHomeOrRoot
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterSkipsNonGitDirectory
- **expected**:
  - **home_registered**: false
  - **filesystem_root_registered**: false
  - **non_git_directory_registered**: false

#### TC-WKS-058
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-058
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterKeepsExistingProjectToml|TestAutoRegisterFirstQueryInUnregisteredGitRepo|TestSaveRegistryPreservesRestrictivePermissions'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
  - internal/workspace/registry.go
  - internal/workspace/registry.go
- **case_id**: TC-WKS-058
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterKeepsExistingProjectToml
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterFirstQueryInUnregisteredGitRepo
  - **file**: internal/workspace/registry.go
  - **test**: TestSaveRegistryPreservesRestrictivePermissions
- **expected**:
  - **existing_project_toml_overwritten**: false
  - **last_workspace_changed**: false
  - **new_registry_mode_argument**: 0600
  - **existing_registry_permission_bits**: preserved
  - **permission_test_scope**: non_windows_checks_mode_0600_on_create_and_after_explicit_chmod

#### TC-WKS-059
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-059
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterTwoProcesses$'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
- **case_id**: TC-WKS-059
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterTwoProcesses
- **expected**:
  - **process_count**: 2
  - **roots**: distinct_same_basename
  - **registry_entries_survive**: true

#### TC-WKS-060
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-060
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run '^TestAutoRegisterLockTimeoutWithLiveHolder$'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
- **case_id**: TC-WKS-060
- **case_type**: negative
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterLockTimeoutWithLiveHolder
- **expected**:
  - **live_holder_timeout_error**: registry_lock_timeout
  - **retry_after_lock_release**: succeeds

#### TC-WKS-064
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-064
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterLinkedWorktreeUsesRegisteredMainRoot|TestAutoRegisterLinkedWorktreeWithoutRegisteredMainFailsClosed'
  - go test -count=1 ./internal/service -run '^TestExecuteLinkedWorktreeWithoutRegisteredMainDoesNotIndex$'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
  - internal/service/auto_register.go
- **case_id**: TC-WKS-064
- **case_type**: positive_and_negative
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterLinkedWorktreeUsesRegisteredMainRoot
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterLinkedWorktreeWithoutRegisteredMainFailsClosed
  - **file**: internal/service/auto_register.go
  - **test**: TestExecuteLinkedWorktreeWithoutRegisteredMainDoesNotIndex
- **expected**:
  - **registered_main**: resolve_without_worktree_registration
  - **unregistered_main**: explicit_incomplete_reason_code_invalid_workspace_without_index
  - **registry_consistent**: true

#### TC-WKS-065
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-065
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace -run 'TestAutoRegisterForceBypassesDefaultScope|TestAutoRegisterTempRootIsDeniedEvenWhenHomeContainsRepo|TestGarbageCollectRegistryRetainsRootsWithAmbiguousErrors|TestPruneStaleWorkspacesDryRunAndApply'
  - go test -count=1 ./internal/service -run '^TestExecuteAutoRegisterForceOverrideIsReported$'
- **Evidencia referenciada**:
  - internal/workspace/autoregister_test.go
  - internal/workspace/registry.go
  - internal/service/auto_register.go
- **case_id**: TC-WKS-065
- **case_type**: positive_and_negative
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterForceBypassesDefaultScope
  - **file**: internal/workspace/autoregister_test.go
  - **test**: TestAutoRegisterTempRootIsDeniedEvenWhenHomeContainsRepo
  - **file**: internal/workspace/registry.go
  - **test**: TestGarbageCollectRegistryRetainsRootsWithAmbiguousErrors
  - **file**: internal/workspace/registry.go
  - **test**: TestPruneStaleWorkspacesDryRunAndApply
  - **file**: internal/service/auto_register.go
  - **test**: TestExecuteAutoRegisterForceOverrideIsReported
- **expected**:
  - **outside_HOME_default_registration**: false
  - **temporary_HOME_default_registration**: false
  - **force_override**: explicit_and_reported
  - **prune_only_definitely_missing_root**: true
  - **ambiguous_errors_and_empty_roots**: retained
  - **existing_directories_and_worktrees_deleted**: false

#### TC-WKS-066
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-066
- **Comprobación prevista**:
  - go test -count=1 ./internal/cli -run '^TestWorkspacePruneDefaultsToSafeStalePreview$'
- **Evidencia referenciada**:
  - internal/cli/workspace.go
  - internal/cli/workspace.go
- **case_id**: TC-WKS-066
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/cli/workspace.go
  - **test**: TestWorkspacePruneDefaultsToSafeStalePreview
- **expected**:
  - **command_without_flags**: accepted
  - **stale_only**: true
  - **default_mode**: dry_run
  - **compatibility_flag_stale**: accepted

#### TC-WKS-067
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-067
- **Comprobación prevista**:
  - go test -count=1 ./internal/workspace ./internal/service -run 'TestListWorkspacesGarbageCollectsMissingRoots|TestResolveWorkspaceSelectionRejectsStaleLastWorkspace|TestProbeNestedGitWorktreeDoesNotReadLexicalParentState|TestExecuteWorkspaceStatusUnregisteredNestedGitWorktreeKeepsPhysicalRoot'
- **Evidencia referenciada**:
  - internal/workspace/registry.go
  - internal/workspace/registry.go
  - internal/service/workspace_ops.go
- **case_id**: TC-WKS-067
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **test_bindings**:
  - **file**: internal/workspace/registry.go
  - **test**: TestListWorkspacesGarbageCollectsMissingRoots
  - **file**: internal/workspace/registry.go
  - **test**: TestResolveWorkspaceSelectionRejectsStaleLastWorkspace
  - **file**: internal/service/probe.go
  - **test**: TestProbeNestedGitWorktreeDoesNotReadLexicalParentState
  - **file**: internal/service/workspace_ops.go
  - **test**: TestExecuteWorkspaceStatusUnregisteredNestedGitWorktreeKeepsPhysicalRoot
  - **file**: internal/service/workspace_ops.go
  - **test**: TestExecuteWorkspaceStatusExplicitDotUsesGitAwareCallerRoot
- **expected**:
  - **public_list_gc_persisted**: true
  - **ordinary_resolution_gc_persisted**: true
  - **ordinary_read_only_unregistered_linked_worktree_with_registered_main**: resolve_main_alias
  - **stale_last_selector_error_preserved**: WKS_SELECTOR_STALE
  - **read_only_resolution_and_doctor_hygiene_preview_mutate_registry**: false
  - **ordinary_resolution_git_failure_for_linked_marker**: registered_containment_only
  - **status_probe_preserve_unregistered_linked_worktree_physical_root_without_registration_or_index**: true
  - **status_probe_git_failure_preserve_physical_root_without_parent_state_inspection**: true

#### TC-WKS-068
  - RF-IDX-001 — Criterio 1; RF-QRY-013 — Criterio 3; RF-WKS-001 — Criterio 1; RF-WKS-002 — Criterio 1; RF-WKS-003 — Criterio 2; RF-WKS-004 — Criterio 3; RF-WKS-005 — Criterio 1; RF-WKS-006 — Criterio 5; RF-WKS-007 — Criterio 3; RF-WKS-008 — Criterio 1; RF-WKS-009 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-WKS-068
- **Comprobación prevista**:
  - manual_evidence_pending
- **Evidencia referenciada**:
  - internal/workspace/registry.go
  - internal/cli/git_frontier.go
- **case_id**: TC-WKS-068
- **case_type**: positive
- **requirement_id**: RF-WKS-007
- **verification_mode**: manual_evidence
- **Estado del comportamiento**: pending
- **test_bindings**:
- **expected**:
  - **explicit_path_read_only_unregistered_linked_worktree_with_registered_main**: preserve_physical_root_without_registration_or_index
  - **nav_git_frontier_explicit_physical_path**: git_head_from_target_root
  - **nav_git_frontier_generations**: from_target_existing_store_only_or_typed_missing_never_main_inherited
  - **omitted_selector_linked_worktree_with_registered_main**: resolve_main_alias_unchanged
  - **explicit_alias_main_from_linked_worktree_cwd**: resolve_main_alias_unchanged
  - **invalid_explicit_path**: fail_closed_without_caller_cwd_substitution
- **manual_evidence**:
  - **fixture**: linked worktree físico con main registrado en registry y sin alias del worktree
  - **command**: mi-lsp nav git-frontier --workspace <ruta-absoluta-del-linked-worktree> --format json
  - **assert_git_commit**: coincide con git -C <ruta> rev-parse HEAD
  - **assert_generations**: null o valores del store bajo <ruta>/.mi-lsp; generation_reasons tipados si falta DB; nunca valores del main salvo que el path sea el main
  - **assert_omitted_control**: mi-lsp nav git-frontier desde cwd del linked sin --workspace resuelve alias del main cuando está registrado
