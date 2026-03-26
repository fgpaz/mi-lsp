# TECH-PYTHON-BACKEND

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Summary

Define la estrategia tecnica del backend Python en `mi-lsp`: indexacion repo-local con tree-sitter pure Go y backend semantico opcional via `pyright-langserver` (cliente LSP generico reutilizable).

## Owner and scope

- Owner logico: Python semantic backend
- Scope: Python discovery, indexacion AST, semantica opcional, reglas de routing
- Non-goals: soporte completo de type checking o virtual environments en v1.1

## Runtime o subsistema

### Capa siempre-on (indexacion)

- `walker` + ignores (extensiones `.py`, `.pyi`)
- extractor Python via tree-sitter pure Go (`github.com/odvcencio/gotreesitter`)
- simbolos extraidos: `class`, `function`, `method`, `decorated_definition`
- catalogo liviano en `index.db` con `language: "python"`
- busqueda textual con cadena de fallback robusta (igual que TS)

### Tree-sitter

- Dependencia: `github.com/odvcencio/gotreesitter v0.9.2`
- Pure Go, zero CGO, cross-compila a Windows ARM64
- Incluye gramatica Python con external scanner para indentation
- Si el parsing falla, el extractor retorna `nil` (degradacion graceful)

### Capa semantica opcional

- Runtime `pyright-langserver` via cliente LSP generico (`LSPClient`)
- Discovery de Pyright (en orden):
  1. `pyright-langserver` en PATH (cubre pip install y npm global)
  2. `node_modules/.bin/pyright-langserver` local
  3. npm global bin (Windows `.cmd` variant)
- Lifecycle LSP: `initialize` -> `initialized` -> `textDocument/didOpen` -> queries -> `shutdown` -> `exit`
- Init options: `pythonPath` detectado automaticamente (`python3` > `python`)
- Si `pyright-langserver` no existe:
  - fallback a catalog/text
  - warning explicito
  - `backend` refleja el backend realmente usado

### Cliente LSP generico

El backend Python usa un cliente LSP generico (`LSPClient`) que es reutilizable para futuros backends:

- Framing: JSON-RPC 2.0 con Content-Length (compartido con tsserver)
- Configurable via `LSPConfig{ServerCmd, ServerArgs, InitOptions}`
- Aplicable a `gopls`, `rust-analyzer`, etc. sin cambios de framing

### Regla de routing

| Query | Default Python behavior |
|---|---|
| `nav symbols` | catalogo repo-local |
| `nav overview` | catalogo + filesystem |
| `nav search` | `ripgrep` |
| `nav context` | `pyright` si disponible, si no catalog/text |
| `nav refs` | `pyright` si disponible, si no text fallback |

### Routing por archivo vs workspace

- Si el archivo es `.py`/`.pyi`, se usa `pyright` directamente
- Si el workspace solo tiene Python (sin C#/TS), `find_refs` usa `pyright`
- Si es mixto (Python + C#/TS), el routing se resuelve por extension del archivo

## Dependencias e interacciones

- `indexer/extractor_python.go` (tree-sitter)
- `indexer/walker.go` (extensiones `.py`, `.pyi`)
- `worker/lsp_protocol.go` (framing JSON-RPC 2.0)
- `worker/lsp_client.go` (cliente LSP generico)
- `worker/pyright.go` (config + discovery)
- `worker/runtime_client.go` (case "pyright")
- `workspace/topology.go` (deteccion Python markers)
- `service/semantic.go` (routing `isPythonFile`)
- `daemon/lifecycle.go` (warm Python backends)

## Python markers detectados

- Extensiones: `.py`, `.pyi`
- Config files: `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt`, `poetry.lock`, `pipfile`, `pipfile.lock`

## Failure modes y notas operativas

| Riesgo | Sintoma | Mitigacion canonica |
|---|---|---|
| gotreesitter parse failure | extractor retorna nil | degradacion graceful, archivo se indexa sin simbolos |
| Pyright no instalado | backend no inicia | fallback con warning accionable |
| Python no instalado | `pythonPath` default a "python" | warning pero Pyright puede funcionar sin |
| Repos grandes Python | indexacion lenta | gotreesitter es ~3.9x mas lento que C pero aceptable para indexing |

## Related docs

- [TECH-TS-BACKEND.md](TECH-TS-BACKEND.md)
- [CT-DAEMON-WORKER.md](../09_contratos/CT-DAEMON-WORKER.md)
- [CT-CLI-DAEMON-ADMIN.md](../09_contratos/CT-CLI-DAEMON-ADMIN.md)
