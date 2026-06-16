# TP-WKS

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TP-WKS"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TP-WKS]]'
exports:
  - 'TP-WKS'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/06_pruebas/TP-WKS.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-WKS.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-WKS.md
```

## Cobertura objetivo

- RF-WKS-001
- RF-WKS-002
- RF-WKS-003
- RF-WKS-004
- RF-WKS-005
- RF-WKS-006

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-WKS-001 | positivo | RF-WKS-001 | registra workspace compatible con alias explicito |
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
| TC-WKS-014 | positivo | RF-WKS-005 | `workspace status` expone `governance_profile`, `governance_sync`, `governance_index_sync` y `governance_blocked` |
| TC-WKS-015 | negativo | RF-WKS-005 | si falta `00_gobierno_documental.md` o la gobernanza es invalida, el repo entra en `blocked mode` |
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
| TC-WKS-035 | negativo | RF-WKS-005 | `TestExecuteWorkspaceStatusAgentRejectsExplicitAliasOutsideCallerCWD`: caller agente con `--workspace` fuera del `caller_cwd` recibe `workspace_cross_workspace_refused` y hint hacia `--allow-cross-workspace` |
| TC-WKS-036 | positivo | RF-WKS-005 | `TestExecuteWorkspaceStatusAgentAllowsExplicitCrossWorkspaceOverride`: `--allow-cross-workspace` permite el uso intencional y conserva warning auditable |
| TC-WKS-037 | positivo | RF-WKS-004 | `TestWorkspaceHygieneReportsLiveReadinessIssuesWithoutPruning`: `workspace hygiene` reporta aliases vivos sin gobernanza/docs como `workspace_readiness_issues` y `--apply-safe` no los remueve |
