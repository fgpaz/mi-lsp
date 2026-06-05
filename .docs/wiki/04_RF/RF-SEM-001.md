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
| Backend embeddings disponible (OpenAI-compatible; Nan/Qwen3 recomendado para recall operativo) | tecnica | opcional |
| Perfil knowledge-wiki efectivo | operativa | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `enabled` | bool | no | `.mi-lsp/project.toml [embeddings]` | omitido = activo si `base_url` + `model`; `false` = apagado explicito | RF-SEM-001 |
| `provider` | string | no | `.mi-lsp/project.toml [embeddings]` | e.g. `openai`, `azure`, `custom`, `nan` o vacio | RF-SEM-001 |
| `base_url` | string | no | `.mi-lsp/project.toml [embeddings]` | valid HTTP(S) URL o vacio | RF-SEM-001 |
| `model` | string | no | `.mi-lsp/project.toml [embeddings]` | e.g. `qwen3-embedding`, `text-embedding-3-large` o vacio | RF-SEM-001 |
| `dim` | entero | no | `.mi-lsp/project.toml [embeddings]` | > 0; `4096` para `qwen3-embedding` | RF-SEM-001 |
| `api_key_env` | string | no | `.mi-lsp/project.toml [embeddings]` | env var name (e.g. `MI_LSP_EMBEDDINGS_API_KEY`) o vacio | RF-SEM-001 |
| `profile` | enum | no | `.mi-lsp/project.toml [embeddings]` | `spec-driven` o `knowledge-wiki`; default `knowledge-wiki` | RF-SEM-001 |
| `batch_size` | entero | no | `.mi-lsp/project.toml [embeddings]` | >= 1, default 10 | RF-SEM-001 |
| `timeout_ms` | entero | no | `.mi-lsp/project.toml [embeddings]` | > 0, default 30000 | RF-SEM-001 |
| `encoding_format` | string | no | `.mi-lsp/project.toml [embeddings]` | OpenAI-compatible; default operativo `float` | RF-SEM-001 |
| `user_agent` | string | no | `.mi-lsp/project.toml [embeddings]` | header HTTP seguro; default del cliente si se omite | RF-SEM-001 |

Ejemplo operativo Nan/Qwen3:

```toml
[embeddings]
provider = "openai"
base_url = "https://api.nan.builders/v1"
model = "qwen3-embedding"
dim = 4096
api_key_env = "NAN_API_KEY"
profile = "knowledge-wiki"
batch_size = 32
timeout_ms = 30000
encoding_format = "float"
user_agent = "mi-lsp-embeddings/1.0"
```

`api_key_env` nombra una variable de entorno. El valor real de la key nunca va en `project.toml`, logs, wiki, output de CLI ni evidencia; para ejecuciones locales usar env del shell o `mkey run`.

## 4. Process Steps (Happy Path)

1. CLI recibe comando `nav recall` o similar.
2. Core lee `.mi-lsp/project.toml` seccion `[embeddings]` si existe.
3. Si `[embeddings]` no existe, falta `base_url`/`model`, o `enabled=false`, devuelve hint y no llama al backend.
4. Si `base_url` + `model` existen y `enabled` esta omitido o `true`, carga cliente OpenAI-compatible con credenciales desde `api_key_env`.
5. Ejecuta embedding del query o de chunks de wiki con timeout `timeout_ms`, payload `encoding_format = "float"` cuando no haya override y headers `Accept: application/json` + `User-Agent`.
6. Si la llamada falla, registra warning y orienta el fallback canonico a `mi-lsp nav wiki search`; no existe fallback BGE oculto.
7. Config efectiva se resuelve desde `.mi-lsp/project.toml` por operacion.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | interno | configuracion cargada exitosamente |
| `backend` | string | interno | `recall` si usa embeddings; fallback documental recomendado: `nav wiki search` |
| `warnings` | lista | usuario/skill | diagnostico de config fallida o fallback activado |
| `config_effective` | objeto | diagnostico | snapshot de config usado (sin credenciales) |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `SEM_INVALID_CONFIG` | config esta malformada | `project.toml [embeddings]` invalido | warning + fallback a `nav wiki search` |
| `SEM_PROVIDER_UNREACHABLE` | backend no responde | timeout o error de red | warning + fallback a `nav wiki search` |
| `SEM_API_KEY_MISSING` | api_key_env especificado pero env var vacia | variable nombrada en `api_key_env` no seteada | warning + fallback a `nav wiki search` |
| `SEM_DIMENSION_MISMATCH` | proveedor responde otra dimension | vector recibido no coincide con `dim` | error de embedding y fallback documental |

## 7. Special Cases and Variants

- Si no hay seccion `[embeddings]` en `project.toml`, default es `embeddings_unconfigured` con hint accionable (no error).
- Si hay `base_url` + `model` y `enabled` esta omitido, la config esta activa.
- Si `enabled=false`, la config queda apagada aunque existan `base_url` + `model`.
- Si `provider` es vacio o `null`, la compatibilidad se decide por `base_url` + `model`; no se habilita un proveedor local implicito.
- Si backend esta accesible pero timeout en validacion, registra warning y orienta a `nav wiki search`.
- Config puede ser override parcial por CLI flags (future expansion).
- `encoding_format` omitido se resuelve a `float` para proveedores OpenAI-compatible que lo soportan.
- `user_agent` omitido usa el default seguro del cliente.

## 8. Data Model Impact

- Config almacenado en `EmbeddingsBlock` / cliente embeddings durante la operacion
- Credenciales nunca persistidas en disco; siempre via env var

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Cargar config valida desde project.toml
  Given un workspace con [embeddings] section en project.toml valida
  And la config incluye base_url y model
  And enabled esta omitido
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then la config se carga exitosamente
  And backend = "recall"
  And el payload usa encoding_format = "float"
  And la dimension recibida coincide con dim

Scenario: Apagar embeddings con kill switch explicito
  Given un workspace con [embeddings] section en project.toml valida
  And enabled = false
  When ejecuto "mi-lsp index --workspace <alias> --docs-only"
  Then no se llama al provider de embeddings
  And wiki_chunk_embeddings no recibe filas nuevas

Scenario: Caer a offline si backend no responde
  Given config que apunta a un backend no disponible
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then se registra warning de provider unreachable
  And la guidance recomienda `mi-lsp nav wiki search`
  And no se usa un fallback BGE oculto

Scenario: Usar default offline si no hay config
  Given un workspace sin seccion [embeddings]
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then no hay error, backend = "recall"
  And items = []
  And hint menciona embeddings
  And no se intenta remote
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
  - fallback documental seguro es `nav wiki search`
  - credenciales via env var, nunca en `project.toml`
- TODO explicit = 0
- Fuera de alcance:
  - certificados SSL custom (future)
  - proxy HTTP (future)
