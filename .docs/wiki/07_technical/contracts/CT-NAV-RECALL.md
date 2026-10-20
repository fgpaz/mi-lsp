---
wiki_contract: WIKI-CONTRACT-v1
id: CT-NAV-RECALL
kind: contract
status: active
title: CT-NAV-RECALL
---

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

### Frontera
Usuario/agente -> CLI publica `mi-lsp nav recall`.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### Forma de invocacion

```text
mi-lsp nav recall <query> [--workspace <alias>] [--max-items 10] [--token-budget 2000] [--intent formula|evidence|route|explore|learning] [--format compact|json|text|toon] [--map]
```

La CLI acepta una consulta libre y una intención opcional. Produce un envelope `backend=recall` con candidatos wiki semánticos cuando embeddings estan configurados. Cuando la key, el provider o la config fallan, la guia canónica de fallback es rerutear a `mi-lsp nav wiki search`, no activar un modelo local oculto ni convertir una degradacion lexical en runtime default.

### Configuración `[embeddings]`

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
index_timeout_ms = 0
encoding_format = "float"
user_agent = "mi-lsp-embeddings/1.0"
```

El cliente usa payload OpenAI-compatible con `encoding_format = "float"`, header `Accept: application/json`, `User-Agent` configurable y validación estricta de dimensión contra `dim`. La API key se resuelve desde el environment o desde un wrapper como `mkey run`; nunca se imprime ni se guarda en docs. `index_timeout_ms` es opcional: cuando vale 0 u omitido, el timeout de indexación con embeddings se estima desde corpus documental, `batch_size` y `timeout_ms`.

### Configuración `[recall.rerank_extension]`

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

`command` se ejecuta sin shell y recibe stdin JSON. `args` son literales de configuración, no templates de secretos. El core no implementa cliente HTTP privado de rerank.

### Payload logico

- `query`: string requerido
- `workspace`: alias o path resoluble
- `max_items`: límite de resultados (default 10)
- `token_budget`: presupuesto de tokens aproximado (default 2000)
- `intent`: una de `formula`, `evidence`, `route`, `explore`, `learning`; default `explore`
- `format`: salida estructurada
- `map`: boolean, agrega mini-mapa de ubicacion documental cuando el valor es true

Cuando `workspace` se omite, el runtime resuelve primero el workspace registrado cuyo root contiene el `caller_cwd` real del invocador. Solo si no hay match puede caer a `last_workspace`, y ese caso debe quedar visible en `warnings`.

### Guia de intenciones

- **Intent**: `formula`; **Uso recomendado**: Buscar la definicion canónica, regla, contrato o decision que formula la respuesta.; **Efecto esperado**: Prioriza secciones normativas, RF/CT/TECH/DB y chunks con lenguaje definitorio.
- **Intent**: `evidence`; **Uso recomendado**: Reunir evidencia citables para justificar una respuesta o auditoria.; **Efecto esperado**: Prioriza trazabilidad, pruebas, acceptance criteria, snippets y referencias documentales.
- **Intent**: `route`; **Uso recomendado**: Elegir la ruta de trabajo o el proximo documento a leer.; **Efecto esperado**: Prioriza anchors, flows, RF/TP/CT relacionados y puede usarse con `--map`.
- **Intent**: `explore`; **Uso recomendado**: Exploracion general cuando todavía no se conoce el vocabulario.; **Efecto esperado**: Balancea similitud semántica y cobertura documental.
- **Intent**: `learning`; **Uso recomendado**: Preparar onboarding, resumen de conceptos o aprendizaje progresivo.; **Efecto esperado**: Prioriza explicaciones, arquitectura, baseline y docs de contexto.

Regla de autoridad: embeddings descubren candidatos. Un hit `route` o material `route-only` no se convierte por si mismo en fuente final; la respuesta final debe anclarse en el documento canónico o evidencia que el candidato permita abrir.

### Respuesta

Cada item de `backend=recall` contiene:

- `query`: consulta efectiva usada para el embedding
- `intent`: intención efectiva, normalizada
- `archivo`: ruta relativa al workspace
- `heading`: título o número de seccion del chunk
- `score`: float [0, 1] de similitud de coseno normalizada
- `snippet`: fragmento de contexto
- `start_line`: número de línea inicial en el archivo original
- `end_line`: número de línea final en el archivo original
- `why`: razon breve de ranking/reranking; incluye `external_rerank` cuando el hook local externo reordeno ese candidato

El envelope puede contener ademas:

- `warnings`: fallas o degradaciones visibles
- `continuation.reason`: siguiente paso recomendado
- `hint`: acción recomendada cuando embeddings no estan configurados, la API falla o la consulta queda sin resultados
- `truncated`: true cuando `token_budget` recorta items

### Gating y prerequisitos

- **Ungated**: no requiere `governance_blocked=false` ni `docs_index_ready=true` para devolver guidance operacional.
- Hot path directo: no auto-inicia daemon.
- `[embeddings]` esta activo cuando `base_url` + `model` existen y `enabled` no es `false`.
- Si `[embeddings]` no esta activo, devuelve `backend=recall`, `items=[]` y `hint` accionable sin llamar al proveedor.
- Si el provider activo falla por key, endpoint, timeout o payload, el fallback canónico es `mi-lsp nav wiki search "<query>" --workspace <alias> --format toon`.
- Si embeddings no estan activos o fallan, no se invoca `[recall.rerank_extension]`.
- No existe fallback local oculto ni modelo implicito.

### Backends de busqueda

- `recall`: vector similarity sobre chunks wiki enriquecidos con metadata.
- `nav wiki search`: fallback lexical/wiki explícito que el agente debe ejecutar cuando recall no puede usar embeddings.

### Semántica observable

- `score` de embeddings siempre [0, 1] post-normalizacion.
- Ranking determinista: dentro de backend, por score descendente y reranking estable por `intent`.
- Rerank extension se aplica después del ranking semántico y antes del corte final; si falla, el orden semántico se preserva.
- Top-k = `min(max_items, presupuesto_token / tokens_por_item)`.
- Cuando `token_budget` agota, truncar con `truncated=true` y `next_hint` accionable.
- Cambios de metadata-prefix, texto enriquecido, content hash, `embedding_model` o `embedding_dim` requieren reindex/reembedding.
- El backfill de embeddings persiste lotes confirmados y reutiliza chunks ya guardados con mismo hash/modelo/dimension; una corrida interrumpida puede retomarse sin rehacer todos los vectores.
- Durante backfill, `index status` expone `current_stage=embeddings` y progreso `N/M chunks embedded`; errores por deadline/cancelacion usan `error.stage=embeddings`.
- Cambios en `[recall.rerank_extension]` no requieren reindex si el modelo/dimension de embeddings no cambia.

### Relacion con otros comandos

`nav recall` es complementario a:

- `nav ask`: respuesta docs-first con synthesis; `nav recall` devuelve candidatos semanticos.
- `nav wiki search`: fallback canónico y busqueda wiki lexical gobernada.
- `nav route`: routing de tarea canónico; `nav recall --intent route` descubre candidatos pero no reemplaza el ancla canonica.
- `nav pack`: lectura compacta de los documentos canónicos elegidos.

### Envelope structure

```json
{
  "ok": true,
  "backend": "recall",
  "workspace": "alias",
  "items": [
    {
      "query": "how does recall validate dimensions",
      "intent": "formula",
      "archivo": ".docs/wiki/07_technical/TECH-SEMANTIC-RECALL.md",
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

### Profile-driven selection

- `embeddings.profile = "knowledge-wiki"`: ranking docs-first, penaliza generic README.
- `embeddings.profile = "spec-driven"`: penaliza hits textuales puros, prioriza docs gobernados RF/CT.
- Selection influencia en token budget y scoring.

### Operaciones adicionales

`mi-lsp workspace status` debe exponer en su envelope:

- `embeddings_enabled: true|false`
- `recall_profile`: "knowledge-wiki" o "spec-driven"
- `embeddings_model`: nombre del modelo actual si enabled
- Si embeddings esta en estado `unconfigured` u `offline`, incluir hint accionable hacia config o `nav wiki search`

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

### Warnings esperables

- `embeddings_unconfigured` - `[embeddings]` no esta seteado, falta `base_url`/`model`, o `enabled=false`.
- `embeddings_unavailable` - API/provider no disponible; usar `nav wiki search` como fallback.
- `api_timeout` - timeout en embedding API; usar `nav wiki search` como fallback.
- `dimension_mismatch` - el vector devuelto no coincide con `dim`; no reutilizar ni persistir ese resultado.
- `rerank extension <kind>; preserved semantic order` - hook no disponible, timeout, salida inválida, índices invalidos o exit no cero.
- `workspace omitted; multiple registry aliases share root ...`
- `workspace omitted; no registered workspace matched caller cwd ...; falling back to last_workspace=...`

### Errores

- query vacia -> error explícito
- workspace no resoluble -> error explícito
- `index.db` no accesible -> error explícito
- API key inválida o endpoint malformado -> warning/hint accionable, sin imprimir secretos
- salida inválida del hook externo -> warning sanitizado y orden semántico preservado

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
