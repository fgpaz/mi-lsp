---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-TS-BACKEND
kind: technical
status: active
title: TECH-TS-BACKEND
---

## Resumen

Define la estrategia técnica del backend TypeScript/JavaScript en `mi-lsp`: catálogo repo-local siempre disponible y backend semántico opcional via `tsserver`.

### Resumen
Define la estrategia técnica del backend TypeScript/JavaScript en `mi-lsp`: catálogo repo-local siempre disponible y backend semántico opcional via `tsserver`.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

Volver a [07_technical.md](../07_technical.md).

### Responsable y alcance
- Owner logico: TS semantic backend
- Scope: TS/JS/Next discovery, semántica opcional, reglas de routing
- Non-goals: equivalencia total con Roslyn en v1.1

### Runtime o subsistema

#### Capa siempre-on

- `walker` + ignores
- extractor estructural repo-local
- catálogo liviano en `index.db`
- busqueda textual con cadena de fallback robusta
- composer local de `slice_text` para `nav context`

##### Cadena de fallback para busqueda

La busqueda textual implementa un fallback automático cuando `rg` no está disponible:

1. **`rg` binario**: intenta usar `ripgrep` si existe y es accesible
2. **Go native walker**: fallback a `searchPatternGo` implementado en Go que:
   - Respeta `.milspignore` y patrones de ignore globales
   - Filtra archivos binarios automáticamente
   - Usa walker existente sin dependencias externas
3. **`MI_LSP_RG` env var**: permite override de la ruta exacta de `rg` si se necesita

Ventaja: nunca falla la busqueda, siempre hay fallback nativo con performance aceptable.
Si `rg` encuentra cero matches y devuelve exit code `1`, el contrato sigue siendo `ok=true` con lista vacia.

#### Capa semántica opcional

- Runtime `tsserver` por `(workspace, backend_type=tsserver)` cuando Node/TypeScript existan.
- Uso recomendado:
  - `nav context` sobre archivos TS/TSX/JS/JSX como enriquecimiento del slice
  - `nav refs` en consultas con posicion o backend explícito
- Si `tsserver` no existe:
  - fallback a catalog/text
  - warning explícito
  - `backend` debe reflejar el backend realmente usado
  - el slice textual sigue siendo obligatorio para `nav context`

#### Regla de routing

- **Query**: `nav symbols`; **Default TS behavior**: catálogo repo-local
- **Query**: `nav overview`; **Default TS behavior**: catálogo + filesystem
- **Query**: `nav search`; **Default TS behavior**: `ripgrep` con normalización de cero matches
- **Query**: `nav context`; **Default TS behavior**: slice local + `tsserver` si disponible, si no catalog/text
- **Query**: `nav refs`; **Default TS behavior**: `tsserver` si disponible y query resoluble, si no warning + fallback

### Dependencias e interacciones

- `indexer/extractor_ts.go`
- `indexer/walker.go`
- `output formatter`
- runtime pool del daemon
- bridge/protocolo con `tsserver`

### Fallos y notas operativas
- **Riesgo**: Semántica inconsistente; **Sintoma**: refs pobres por nombre; **Mitigacion canonica**: requerir posicion o backend explícito
- **Riesgo**: Node ausente; **Sintoma**: backend no inicia; **Mitigacion canonica**: fallback con warning accionable
- **Riesgo**: Drift index/semantics; **Sintoma**: resultados mixtos; **Mitigacion canonica**: explicitar `backend` y `warnings`
- **Riesgo**: `tsserver` ausente en `nav context`; **Sintoma**: sin hover util; **Mitigacion canonica**: conservar `slice_text` y degradar a `catalog` o `text`

### Referencias de contexto
- [CT-DAEMON-WORKER.md](contracts/CT-DAEMON-WORKER.md)
- [CT-CLI-DAEMON-ADMIN.md](contracts/CT-CLI-DAEMON-ADMIN.md)
