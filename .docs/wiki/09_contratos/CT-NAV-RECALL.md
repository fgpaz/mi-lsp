# CT-NAV-RECALL

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-RECALL"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-NAV-RECALL]]'
exports:
  - 'CT-NAV-RECALL'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-NAV-RECALL.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-RECALL.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-NAV-RECALL.md
```

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav recall`.

## Forma de invocacion

```text
mi-lsp nav recall <query> [--workspace <alias>] [--max-items 10] [--token-budget 2000] [--intent formula|evidence|route|explore|learning] [--format compact|json|text|toon] [--map]
```

La CLI acepta una consulta libre y una intencion opcional. Produce un envelope `backend=recall` con candidatos wiki semanticos cuando embeddings estan configurados. Cuando la key, el provider o la config fallan, la guia canonica de fallback es rerutear a `mi-lsp nav wiki search`, no activar un modelo local oculto ni convertir una degradacion lexical en runtime default.

## Configuracion `[embeddings]`

`[embeddings]` en `.mi-lsp/project.toml` habilita recall cuando incluye `base_url` + `model`, salvo `enabled = false`. El bloque completo soportado es:

```toml
[embeddings]
provider = "openai"
base_url = "https://embeddings.example.local/v1"
model = "text-embedding-model"
dim = 1536
api_key_env = "MI_LSP_EMBEDDINGS_API_KEY"
profile = "knowledge-wiki"
batch_size = 32
timeout_ms = 30000
encoding_format = "float"
user_agent = "mi-lsp-embeddings/1.0"
```

El cliente usa payload OpenAI-compatible con `encoding_format = "float"`, header `Accept: application/json`, `User-Agent` configurable y validacion estricta de dimension contra `dim`. La API key se resuelve desde el environment o desde un wrapper como `mkey run`; nunca se imprime ni se guarda en docs.

## Configuracion `[recall.rerank_extension]`

El rerank externo es opcional, local y deshabilitado por defecto:

```toml
[recall.rerank_extension]
enabled = true
command = "mi-lsp-rerank-local"
args = ["--profile", "default"]
timeout_ms = 2000
candidate_count = 50
top_n = 10
max_snippet_chars = 500
```

`command` se ejecuta sin shell y recibe stdin JSON. `args` son literales de configuracion, no templates de secretos. El core no implementa cliente HTTP privado de rerank.

## Payload logico

- `query`: string requerido
- `workspace`: alias o path resoluble
- `max_items`: limite de resultados (default 10)
- `token_budget`: presupuesto de tokens aproximado (default 2000)
- `intent`: una de `formula`, `evidence`, `route`, `explore`, `learning`; default `explore`
- `format`: salida estructurada
- `map`: boolean, agrega mini-mapa de ubicacion documental cuando el valor es true

Cuando `workspace` se omite, el runtime resuelve primero el workspace registrado cuyo root contiene el `caller_cwd` real del invocador. Solo si no hay match puede caer a `last_workspace`, y ese caso debe quedar visible en `warnings`.

## Guia de intenciones

| Intent | Uso recomendado | Efecto esperado |
|---|---|---|
| `formula` | Buscar la definicion canonica, regla, contrato o decision que formula la respuesta. | Prioriza secciones normativas, RF/CT/TECH/DB y chunks con lenguaje definitorio. |
| `evidence` | Reunir evidencia citables para justificar una respuesta o auditoria. | Prioriza trazabilidad, pruebas, acceptance criteria, snippets y referencias documentales. |
| `route` | Elegir la ruta de trabajo o el proximo documento a leer. | Prioriza anchors, flows, RF/TP/CT relacionados y puede usarse con `--map`. |
| `explore` | Exploracion general cuando todavia no se conoce el vocabulario. | Balancea similitud semantica y cobertura documental. |
| `learning` | Preparar onboarding, resumen de conceptos o aprendizaje progresivo. | Prioriza explicaciones, arquitectura, baseline y docs de contexto. |

Regla de autoridad: embeddings descubren candidatos. Un hit `route` o material `route-only` no se convierte por si mismo en fuente final; la respuesta final debe anclarse en el documento canonico o evidencia que el candidato permita abrir.

## Respuesta

Cada item de `backend=recall` contiene:

- `query`: consulta efectiva usada para el embedding
- `intent`: intencion efectiva, normalizada
- `archivo`: ruta relativa al workspace
- `heading`: titulo o numero de seccion del chunk
- `score`: float [0, 1] de similitud de coseno normalizada
- `snippet`: fragmento de contexto
- `start_line`: numero de linea inicial en el archivo original
- `end_line`: numero de linea final en el archivo original
- `why`: razon breve de ranking/reranking; incluye `external_rerank` cuando el hook local externo reordeno ese candidato

El envelope puede contener ademas:

- `warnings`: fallas o degradaciones visibles
- `continuation.reason`: siguiente paso recomendado
- `hint`: accion recomendada cuando embeddings no estan configurados, la API falla o la consulta queda sin resultados
- `truncated`: true cuando `token_budget` recorta items

## Gating y prerequisitos

- **Ungated**: no requiere `governance_blocked=false` ni `docs_index_ready=true` para devolver guidance operacional.
- Hot path directo: no auto-inicia daemon.
- `[embeddings]` esta activo cuando `base_url` + `model` existen y `enabled` no es `false`.
- Si `[embeddings]` no esta activo, devuelve `backend=recall`, `items=[]` y `hint` accionable sin llamar al proveedor.
- Si el provider activo falla por key, endpoint, timeout o payload, el fallback canonico es `mi-lsp nav wiki search "<query>" --workspace <alias> --format toon`.
- Si embeddings no estan activos o fallan, no se invoca `[recall.rerank_extension]`.
- No existe fallback local oculto ni modelo implicito.

## Backends de busqueda

- `recall`: vector similarity sobre chunks wiki enriquecidos con metadata.
- `nav wiki search`: fallback lexical/wiki explicito que el agente debe ejecutar cuando recall no puede usar embeddings.

## Semantica observable

- `score` de embeddings siempre [0, 1] post-normalizacion.
- Ranking determinista: dentro de backend, por score descendente y reranking estable por `intent`.
- Rerank extension se aplica despues del ranking semantico y antes del corte final; si falla, el orden semantico se preserva.
- Top-k = `min(max_items, presupuesto_token / tokens_por_item)`.
- Cuando `token_budget` agota, truncar con `truncated=true` y `next_hint` accionable.
- Cambios de metadata-prefix, texto enriquecido, content hash, `embedding_model` o `embedding_dim` requieren reindex/reembedding.
- Cambios en `[recall.rerank_extension]` no requieren reindex si el modelo/dimension de embeddings no cambia.

## Warnings esperables

- `embeddings_unconfigured` - `[embeddings]` no esta seteado, falta `base_url`/`model`, o `enabled=false`.
- `embeddings_unavailable` - API/provider no disponible; usar `nav wiki search` como fallback.
- `api_timeout` - timeout en embedding API; usar `nav wiki search` como fallback.
- `dimension_mismatch` - el vector devuelto no coincide con `dim`; no reutilizar ni persistir ese resultado.
- `rerank extension <kind>; preserved semantic order` - hook no disponible, timeout, salida invalida, indices invalidos o exit no cero.
- `workspace omitted; multiple registry aliases share root ...`
- `workspace omitted; no registered workspace matched caller cwd ...; falling back to last_workspace=...`

## Errores

- query vacia -> error explicito
- workspace no resoluble -> error explicito
- `index.db` no accesible -> error explicito
- API key invalida o endpoint malformado -> warning/hint accionable, sin imprimir secretos
- salida invalida del hook externo -> warning sanitizado y orden semantico preservado

## Relacion con otros comandos

`nav recall` es complementario a:

- `nav ask`: respuesta docs-first con synthesis; `nav recall` devuelve candidatos semanticos.
- `nav wiki search`: fallback canonico y busqueda wiki lexical gobernada.
- `nav route`: routing de tarea canonico; `nav recall --intent route` descubre candidatos pero no reemplaza el ancla canonica.
- `nav pack`: lectura compacta de los documentos canonicos elegidos.

## Envelope structure

```json
{
  "ok": true,
  "backend": "recall",
  "workspace": "alias",
  "items": [
    {
      "query": "how does recall validate dimensions",
      "intent": "formula",
      "archivo": ".docs/wiki/07_tech/TECH-SEMANTIC-RECALL.md",
      "heading": "## Embedding provider contract",
      "score": 0.87,
      "snippet": "The provider response must match the configured embedding dimension...",
      "start_line": 92,
      "end_line": 104,
      "why": "formula intent boosted contract-like technical language"
    }
  ],
  "warnings": [],
  "stats": {"items": 1, "backend_time_ms": 145},
  "truncated": false
}
```

Si embeddings no estan configurados o fueron apagados explicitamente:

```json
{
  "ok": true,
  "backend": "recall",
  "workspace": "alias",
  "items": [],
  "hint": "embeddings not configured; configure [embeddings] or use 'mi-lsp nav wiki search' for lexical wiki fallback",
  "truncated": false
}
```

## Profile-driven selection

- `embeddings.profile = "knowledge-wiki"`: ranking docs-first, penaliza generic README.
- `embeddings.profile = "spec-driven"`: penaliza hits textuales puros, prioriza docs gobernados RF/CT.
- Selection influencia en token budget y scoring.

## Operaciones adicionales

`mi-lsp workspace status` debe exponer en su envelope:

- `embeddings_enabled: true|false`
- `recall_profile`: "knowledge-wiki" o "spec-driven"
- `embeddings_model`: nombre del modelo actual si enabled
- Si embeddings esta en estado `unconfigured` u `offline`, incluir hint accionable hacia config o `nav wiki search`
