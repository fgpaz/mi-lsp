# TECH-GOVERNANCE-PROFILES

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-GOVERNANCE-PROFILES"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-GOVERNANCE-PROFILES]]'
exports:
  - 'TECH-GOVERNANCE-PROFILES'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_tech/TECH-GOVERNANCE-PROFILES.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-GOVERNANCE-PROFILES.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-GOVERNANCE-PROFILES.md
```

## Proposito

Detallar el modelo tecnico de gobernanza profile-aware de `mi-lsp`.

## Invariantes

- `00_gobierno_documental.md` es la autoridad humana.
- El bloque YAML embebido es la fuente estructurada.
- `.docs/wiki/_mi-lsp/read-model.toml` es una proyeccion versionada, obligatoria y auto-sincronizable.
- El perfil visible se compila internamente a `base + overlays`.
- Si la gobernanza esta ambigua, invalida o stale, el workspace queda bloqueado para superficies docs-first.

## Perfiles visibles

- `ordered_wiki`
- `spec_backend`
- `spec_full`
- `custom`

## Compilacion interna

- `ordered_wiki` -> base `ordered_wiki`
- `spec_backend` -> base `ordered_wiki` + overlays `spec_core`, `technical`
- `spec_full` -> base `ordered_wiki` + overlays `spec_core`, `technical`, `uxui`
- `custom` -> extiende un perfil canónico y agrega overlays dentro del schema validado

## Gate operativo

- `workspace status` siempre expone estado de gobernanza
- `nav governance` siempre esta permitido
- `nav ask` y `nav pack` bloquean cuando la gobernanza no es valida
- Si la proyeccion cambia, el workspace debe reindexarse antes de continuar con consultas docs-first

## Validación del Kernel v2

- El canon universal se resuelve desde `<kernel_home>/canon`; no se duplica en el repositorio.
- `.docs/ae/repo-policy.yaml` declara un alias exacto en `tracker.provider` (`Linear`, `Plane`, `Azure Boards`, `Jira` o `None`) y exactamente un bloque correspondiente (`linear`, `plane`, `azure_boards`, `jira` o `none`).
- `None` solo requiere `tracker.none.mode: local-only`; no requiere ni admite campos de proveedores externos o aliases legacy.
- Un proveedor externo requiere dentro de su bloque `base_url`, `workspace`, `key_env` (solo nombre de variable de entorno) y al menos un proyecto con `key`.
- Los aliases neutrales `tracker.base_url`, `tracker.conf_file`, `tracker.key_env`, `tracker.workspace` y `tracker.projects` son inválidos porque impiden un render determinístico entre providers.
- `repo.structure_rules` debe contener al menos una regla concreta del repositorio.

## Artefactos afectados

- `.docs/ae/repo-policy.yaml`
- `.docs/wiki/00_gobierno_documental.md`
- `.docs/wiki/_mi-lsp/read-model.toml`
- `internal/docgraph/*`
- `internal/service/ask.go`
- `internal/service/pack.go`
- `internal/service/workspace_ops.go`
