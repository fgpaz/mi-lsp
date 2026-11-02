---
wiki_contract: WIKI-CONTRACT-v1
id: CT-WORKSPACE-PROBE
kind: contract
status: active
title: CT-WORKSPACE-PROBE - Contrato de identidad, estado y probe
links:
  supports:
  - RF-WKS-007
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: CT-WORKSPACE-PROBE
kind: read-only-cli-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-WORKSPACE-STATUS.md]
exports: [CT-WORKSPACE-PROBE]
agent_must_read: [.docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md, internal/service/probe.go]
agent_may_edit: [.docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md]
agent_must_not_edit: [internal/service/workspace_probe.go, .mi-lsp/index.db, registry.toml]
verify: ["Leer .docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md y .docs/wiki/07_technical/TECH-WORKSPACE-STATUS.md; cotejar las afirmaciones con la fuente publicada internal/service/probe.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Alias explícito inválido reciba fallback, root Git se resuelva por contención lexical o probe escriba config/registry/índice.]
evidence: [.docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md, .docs/wiki/07_technical/TECH-WORKSPACE-STATUS.md, internal/service/probe.go]
```

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### [CT-WORKSPACE-PROBE-B01] Envelope

- **Marcador estable**: CT-WORKSPACE-PROBE-B01
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md
- **schema**: mi-lsp/workspace-probe/v1
- **command**: mi-lsp probe
- **backend**: workspace-probe
- **ok**: boolean
- **side_effects**: false
- **protocol_version**: string
- **items**:
  - ProbeReport
- **warnings**:
  - string
- **error**: optional EnvelopeError

`side_effects` es un literal booleano `false`; no es una promesa textual. El envelope mantiene los formatos estructurados soportados por la CLI.

### [CT-WORKSPACE-PROBE-B02] ProbeReport

- **Marcador estable**: CT-WORKSPACE-PROBE-B02
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md
- **ProbeReport**:
  - **Estado del comportamiento**: absent|current|stale|unknown|partial
  - **workspace**:
    - **selector**: string
    - **selector_kind**: omitted|alias|path
    - **resolution_source**: caller_cwd|git_top_level|explicit|path|last_workspace
    - **display_root**: string
    - **canonical_root**: string
  - **state**:
    - **config**: portable|missing|unknown
    - **operational**: local|legacy|missing|unknown
    - **database**: absent|present_ro|unreadable|not_checked
    - **migration_status**: not_needed|legacy_present|pending|unknown
  - **Evidencia referenciada**:
    - **files**:
      - string
    - **mtimes**: map
    - **db_mode**: ro|not_opened
  - **provenance**:
    - **command**: mi-lsp
    - **version**: optional VersionInfo

Los paths de evidencia son informativos y no habilitan mutación. `canonical_root` es la identidad física; `display_root` es la forma presentada al usuario.

### [CT-WORKSPACE-PROBE-B03] Selector

- **Marcador estable**: CT-WORKSPACE-PROBE-B03
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md
- **selector_rules**:
  - **explicit_alias_not_found**: WKS_SELECTOR_NOT_FOUND
  - **explicit_alias_stale**: WKS_SELECTOR_STALE
  - **explicit_path_not_found**: WKS_SELECTOR_NOT_FOUND
  - **omitted_selector**: contextual_resolution_with_provenance
  - **omitted_git_top_level**: read_only_git_rev_parse_first
  - **registered_git_root**: exact_canonical_root_only
  - **unregistered_git_root**: synthetic_path_resolution_without_registry_write
  - **git_top_level_failure**: typed_or_synthetic_without_lexical_parent_fallback
  - **lexical_parent_fallback_for_git**: forbidden
  - **non_git_containment**: preserved
  - **explicit_fallback**: forbidden

Los errores anteriores son estables y deben conservarse en la salida estructurada. El caller CWD solo participa en resolución omitida o para interpretar un path explícito relativo, nunca para reparar un alias explícito inválido.

### [CT-WORKSPACE-PROBE-B04] Read-only DB

- **Marcador estable**: CT-WORKSPACE-PROBE-B04
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md
- **database**:
  - **precondition**: file_exists
  - **dsn**: file:<path>?mode=ro
  - **schema_creation**: forbidden
  - **pragmas**:
    - **journal_mode**: forbidden
    - **wal_checkpoint**: forbidden
    - **query_only**: allowed
    - **foreign_keys**: allowed
  - **filesystem**:
    - **create_directory**: forbidden
    - **create_db**: forbidden
    - **create_wal**: forbidden
    - **create_shm**: forbidden

Si la DB no existe, el servicio reporta `database: absent` sin llamar a `MkdirAll` ni abrir el driver. Cuando existe, el modo `ro` evita crear la DB o sidecars; ninguna rutina de configuración de escritura se reutiliza en probe.

### [CT-WORKSPACE-PROBE-B05] Estado híbrido y legacy

- **Marcador estable**: CT-WORKSPACE-PROBE-B05
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md
- **state_contract**:
  - **portable_config**: repo-local
  - **operational_state**: machine-local
  - **legacy_move**: forbidden
  - **legacy_delete**: forbidden
  - **migration_status**: always_reported

La compatibilidad legacy se expone como `legacy_present` o `unknown`; no se realiza migración automática durante probe. Un `git_common_dir` compartido no permite reutilizar la identidad ni el operational path de otro root.

### [CT-WORKSPACE-PROBE-B06] Provenance y telemetría

- **Marcador estable**: CT-WORKSPACE-PROBE-B06
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-WORKSPACE-PROBE.md
- **provenance**:
  - **source**: mi-lsp version
  - **required_when**: binary_metadata_available
- **telemetry**:
  - **record**: forbidden
- **snapshots**:
  - **create**: forbidden

El probe reutiliza el modelo de provenance de `mi-lsp version` sin duplicar identidad ni alterar el resultado de version. Probe no registra operaciones ni ejecuta limpieza de retención.

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
