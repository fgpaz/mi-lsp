# Superficie mínima desde execute

La fuente es `internal/cli/provider_manifest.go`; `provider-manifest.json` es un snapshot distribuido, no otro contrato editable. El padre lo regenera desde su binario integrado con `MI_LSP_BINARY=/ruta/mi-lsp sh scripts/provider-snapshot.sh` antes de publicar. BUILD no compila ni acredita ese snapshot como evidencia runtime.

| Alias real en sandbox | Operación canónica | argv por operación |
|---|---|---|
| lsp.intent | intent | nav intent |
| lsp.read | multi-read | nav multi-read |
| lsp.search | search | nav search |
| lsp.refs | refs | nav refs |
| lsp.diagnostics | diagnostics | nav diagnostics |
| fs.find | find-files | find-files |

Cada operación tiene `invocation` v2 propio con `input_mapping.fields` ya soportado por el loader. La única extensión solicitada a H3 es `model_alias:{namespace,name}`; C165 es dueño de su implementación genérica. No añade otro schema de argumentos, wrapper de dominio ni nuevo MCP tool. El provider por sí solo no acredita aliases ejecutables: requieren H3 publicado con ese soporte, no CLI129. El nombre target sigue siendo canónico y namespace sigue siendo milsp. `milsp.q`, `milsp.intent`, `milsp.multiRead` y catálogo avanzado siguen existiendo.

- Diagnostics: `file` XOR `changed:true`; `base` solo con changed; `max_files` 1–64, `offset` >=0. La CLI valida exclusión. Contrato JSON `diagnostics/v1`. Campos comunes `workspace`, `max_items`, `token_budget`.
- Find: `pattern` obligatorio; nombres con guion inicial funcionan en CLI directa con `--`, pero el mapping H3 inspeccionado aún los rechaza honestamente; no se coloca un `--` no soportado en fixed_args. Necesidad genérica de delimitador registrada para C165.
- Find: `top` 1–1000, `budget_bytes` 512–1048576, `literal`, `refresh`, más campos comunes. Contrato `find-files/v1`; no se afirma fallback semántico ni completitud sobre listados truncados.
- Read: `ranges` array de `path:start-end`.
- Refs: `symbol` obligatorio; `file`, `line`, `column` opcionales.
- Search: `pattern` obligatorio, `include_content` opcional.
- Intent: `question` obligatorio; `top`, `offset`, `repo` opcionales.

Default de navegación: 20 items / 2000 tokens aproximados, ampliables explícitamente. Find aplica además límite JSON de 4096 bytes por defecto. `mi-lsp models` deriva schema/nombres del manifiesto; `nav --help` y `provider-manifest` conservan catálogo completo.

Despliegue por harness/usuario: [guía única](../../skills/mi-lsp/references/all-harnesses.md). No cambiar caches durante BUILD. Runtime manual y medición de ambos hosts/harness los acredita la FINAL_VERIFY del padre. El enganche tras-edit automático pertenece a jefe-harness-web: invocaciones manuales jamás lo acreditan.
