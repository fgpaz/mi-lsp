# TECH-TS-BACKEND

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Summary

Define la estrategia tecnica del backend TypeScript/JavaScript en `mi-lsp`: catalogo repo-local siempre disponible y backend semantico opcional via `tsserver`.

## Owner and scope

- Owner logico: TS semantic backend
- Scope: TS/JS/Next discovery, semantica opcional, reglas de routing
- Non-goals: equivalencia total con Roslyn en v1.1

## Runtime o subsistema

### Capa siempre-on

- `walker` + ignores
- extractor estructural repo-local
- catalogo liviano en `index.db`
- busqueda textual con cadena de fallback robusta
- composer local de `slice_text` para `nav context`

#### Cadena de fallback para busqueda

La busqueda textual implementa un fallback automatico cuando `rg` no está disponible:

1. **`rg` binario**: intenta usar `ripgrep` si existe y es accesible
2. **Go native walker**: fallback a `searchPatternGo` implementado en Go que:
   - Respeta `.milspignore` y patrones de ignore globales
   - Filtra archivos binarios automaticamente
   - Usa walker existente sin dependencias externas
3. **`MI_LSP_RG` env var**: permite override de la ruta exacta de `rg` si se necesita

Ventaja: nunca falla la busqueda, siempre hay fallback nativo con performance aceptable.
Si `rg` encuentra cero matches y devuelve exit code `1`, el contrato sigue siendo `ok=true` con lista vacia.

### Capa semantica opcional

- Runtime `tsserver` por `(workspace, backend_type=tsserver)` cuando Node/TypeScript existan.
- Uso recomendado:
  - `nav context` sobre archivos TS/TSX/JS/JSX como enriquecimiento del slice
  - `nav refs` en consultas con posicion o backend explicito
- Si `tsserver` no existe:
  - fallback a catalog/text
  - warning explicito
  - `backend` debe reflejar el backend realmente usado
  - el slice textual sigue siendo obligatorio para `nav context`

### Regla de routing

| Query | Default TS behavior |
|---|---|
| `nav symbols` | catalogo repo-local |
| `nav overview` | catalogo + filesystem |
| `nav search` | `ripgrep` con normalizacion de cero matches |
| `nav context` | slice local + `tsserver` si disponible, si no catalog/text |
| `nav refs` | `tsserver` si disponible y query resoluble, si no warning + fallback |

## Dependencias e interacciones

- `indexer/extractor_ts.go`
- `indexer/walker.go`
- `output formatter`
- runtime pool del daemon
- bridge/protocolo con `tsserver`

## Failure modes y notas operativas

| Riesgo | Sintoma | Mitigacion canonica |
|---|---|---|
| Semantica inconsistente | refs pobres por nombre | requerir posicion o backend explicito |
| Node ausente | backend no inicia | fallback con warning accionable |
| Drift index/semantics | resultados mixtos | explicitar `backend` y `warnings` |
| `tsserver` ausente en `nav context` | sin hover util | conservar `slice_text` y degradar a `catalog` o `text` |

## Related docs

- [CT-DAEMON-WORKER.md](../09_contratos/CT-DAEMON-WORKER.md)
- [CT-CLI-DAEMON-ADMIN.md](../09_contratos/CT-CLI-DAEMON-ADMIN.md)
