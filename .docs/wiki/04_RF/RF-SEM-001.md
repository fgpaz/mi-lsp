# RF-SEM-001 - Configurar backend de embeddings pluggable OpenAI-compatible por workspace

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-SEM-001"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-SEM-001]]'
exports:
  - 'RF-SEM-001'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-SEM-001.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-SEM-001.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-SEM-001.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-SEM-001 |
| Titulo | Configurar backend de embeddings pluggable OpenAI-compatible por workspace |
| Actores | Desarrollador, Skill, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-SEM-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace registrado con `.mi-lsp/project.toml` accesible | funcional | obligatorio |
| Backend embeddings disponible (OpenAI-compatible o offline) | tecnica | opcional |
| Perfil knowledge-wiki efectivo | operativa | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `provider` | string | no | `.mi-lsp/project.toml [embeddings]` | e.g. `openai`, `azure`, `custom` o vacio | RF-SEM-001 |
| `base_url` | string | no | `.mi-lsp/project.toml [embeddings]` | valid HTTP(S) URL o vacio | RF-SEM-001 |
| `model` | string | no | `.mi-lsp/project.toml [embeddings]` | e.g. `text-embedding-3-small`, `bge-m3` o vacio | RF-SEM-001 |
| `dim` | entero | no | `.mi-lsp/project.toml [embeddings]` | > 0 (e.g. 1024 para bge-m3) o vacio | RF-SEM-001 |
| `api_key_env` | string | no | `.mi-lsp/project.toml [embeddings]` | env var name (e.g. `MI_LSP_EMBEDDINGS_API_KEY`) o vacio | RF-SEM-001 |
| `profile` | enum | no | `.mi-lsp/project.toml [embeddings]` | `spec-driven` o `knowledge-wiki`; default `knowledge-wiki` | RF-SEM-001 |
| `batch_size` | entero | no | `.mi-lsp/project.toml [embeddings]` | >= 1, default 10 | RF-SEM-001 |
| `timeout_ms` | entero | no | `.mi-lsp/project.toml [embeddings]` | > 0, default 30000 | RF-SEM-001 |

## 4. Process Steps (Happy Path)

1. CLI recibe comando `nav recall` o similar.
2. Core lee `.mi-lsp/project.toml` seccion `[embeddings]` si existe.
3. Si `[embeddings]` section no existe, usa default offlineion-lexical).
4. Si `provider` esta configurado, carga cliente OpenAI-compatible con credenciales de env var.
5. Valida conexion a backend (timeout `timeout_ms`).
6. Si validacion falla, registra warning y cae a fallback offline-lexical.
7. Config se mantiene viva para toda sesion de CLI.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | interno | configuracion cargada exitosamente |
| `backend` | string | interno | `embeddings` si es activo, `text-index` si es fallback |
| `warnings` | lista | usuario/skill | diagnostico de config fallida o fallback activado |
| `config_effective` | objeto | diagnostico | snapshot de config usado (sin credenciales) |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `SEM_INVALID_CONFIG` | config esta malformada | `project.toml [embeddings]` invalido | warning + fallback a offline |
| `SEM_PROVIDER_UNREACHABLE` | backend no responde | timeout o error de red | warning + fallback a offline |
| `SEM_API_KEY_MISSING` | api_key_env especificado pero env var vacia | `$MI_LSP_EMBEDDINGS_API_KEY` no seteada | warning + fallback a offline |

## 7. Special Cases and Variants

- Si no hay seccion `[embeddings]` en `project.toml`, default es offline-lexical (no error).
- Si `provider` es vacio o `null`, skip inicializacion de cliente y usa offline directo.
- Si backend esta accesible pero timeout en validacion, registra warning y cae a offline.
- Config puede ser override parcial por CLI flags (future expansion).

## 8. Data Model Impact

- Config almacenado en `EmbeddingsConfig` struct inmutable durante sesion
- Credenciales nunca persistidas en disco; siempre via env var

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Cargar config valida desde project.toml
  Given un workspace con [embeddings] section en project.toml valida
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then la config se carga exitosamente
  And backend = "embeddings"

Scenario: Caer a offline si backend no responde
  Given config que apunta a un backend no disponible
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then se registra warning de provider unreachable
  And backend = "text-index" (fallback)
  And la busqueda continua con offline-lexical

Scenario: Usar default offline si no hay config
  Given un workspace sin seccion [embeddings]
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then no hay error, backend = "text-index"
  And se usa online-lexical sin intentar remote
```

## 10. Test Traceability

- Positivo: `TP-SEM / TC-SEM-001`
- Positivo: `TP-SEM / TC-SEM-002`
- Negativo: `TP-SEM / TC-SEM-003`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir configuracion global (`~/.mi-lsp/`); es por workspace
  - no exponer credenciales en output
  - no fallar si backend no disponible
- Decisiones cerradas:
  - fallback offline-lexical es silencioso + warning
  - credenciales via env var, nunca en `project.toml`
- TODO explicit = 0
- Fuera de alcance:
  - certificados SSL custom (future)
  - proxy HTTP (future)
