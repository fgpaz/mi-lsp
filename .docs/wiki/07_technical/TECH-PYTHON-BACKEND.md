---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-PYTHON-BACKEND
kind: technical
status: active
title: TECH-PYTHON-BACKEND
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-PYTHON-BACKEND
kind: technical-backend
audience: dual
imports: [.docs/wiki/07_technical/TECH-TS-BACKEND.md, .docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md]
exports: [TECH-PYTHON-BACKEND]
agent_must_read: [.docs/wiki/07_technical/TECH-PYTHON-BACKEND.md, internal/indexer/extractor_python.go, internal/worker/pyright.go]
agent_may_edit: [.docs/wiki/07_technical/TECH-PYTHON-BACKEND.md]
agent_must_not_edit: [internal/indexer/extractor_python.go, internal/worker/pyright.go, .docs/wiki/_mi-lsp/read-model.toml]
verify: ["Leer .docs/wiki/07_technical/TECH-PYTHON-BACKEND.md y .docs/wiki/07_technical/TECH-TS-BACKEND.md; cotejar las afirmaciones con la fuente publicada internal/indexer/extractor_python.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [La propuesta convierte fallback en éxito semántico o atribuye disponibilidad de Pyright sin comprobación.]
evidence: [.docs/wiki/07_technical/TECH-PYTHON-BACKEND.md, .docs/wiki/07_technical/TECH-TS-BACKEND.md, internal/indexer/extractor_python.go]
```

## Resumen

Define la estrategia técnica del backend Python en `mi-lsp`: indexación repo-local lexical acotada y backend semántico opcional via `pyright-langserver` (cliente LSP generico reutilizable).

### Resumen
Define la estrategia técnica del backend Python en `mi-lsp`: indexación repo-local lexical acotada y backend semántico opcional via `pyright-langserver` (cliente LSP generico reutilizable).

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

Volver a [07_technical.md](../07_technical.md).

### Responsable y alcance
- Owner logico: Python semantic backend
- Scope: Python discovery, indexación lexical de catálogo, semántica opcional, reglas de routing
- Non-goals: soporte completo de type checking o virtual environments en v1.1

### Runtime o subsistema

#### Capa siempre-on (indexación)

- `walker` + ignores (extensiones `.py`, `.pyi`)
- extractor Python lexical por líneas en `indexer/extractor_python.go`
- símbolos extraidos: `class`, `function`, `method`; decoradores se toleran y se asocian al `def` siguiente por indentacion
- catálogo liviano en `index.db` con `language: "python"`
- busqueda textual con cadena de fallback robusta (igual que TS)

#### Extracción lexical

- Sin parser AST en el hot path de catálogo: el extractor usa regex + indentacion para evitar bloqueos de parser por archivo.
- La salida prioriza símbolos navegables y cancelabilidad operativa sobre fidelidad AST completa.
- Si una forma Python no se reconoce, el archivo conserva `FileRecord` y puede seguir cubierto por `nav search` textual o Pyright cuando este disponible.

#### Capa semántica opcional

- Runtime `pyright-langserver` via cliente LSP generico (`LSPClient`)
- Discovery de Pyright (en orden):
  1. `pyright-langserver` en PATH (cubre pip install y npm global)
  2. `node_modules/.bin/pyright-langserver` local
  3. npm global bin (Windows `.cmd` variant)
- Lifecycle LSP: `initialize` -> `initialized` -> `textDocument/didOpen` -> queries -> `shutdown` -> `exit`
- Init options: `pythonPath` detectado automáticamente (`python3` > `python`)
- Si `pyright-langserver` no existe:
  - fallback a catalog/text
  - warning explícito
  - `backend` refleja el backend realmente usado

#### Cliente LSP generico

El backend Python usa un cliente LSP generico (`LSPClient`) que es reutilizable para futuros backends:

- Framing: JSON-RPC 2.0 con Content-Length (compartido con tsserver)
- Configurable via `LSPConfig{ServerCmd, ServerArgs, InitOptions}`
- Aplicable a `gopls`, `rust-analyzer`, etc. sin cambios de framing

#### Regla de routing

- **Query**: `nav symbols`; **Default Python behavior**: catálogo repo-local
- **Query**: `nav overview`; **Default Python behavior**: catálogo + filesystem
- **Query**: `nav search`; **Default Python behavior**: `ripgrep`
- **Query**: `nav context`; **Default Python behavior**: `pyright` si disponible, si no catalog/text
- **Query**: `nav refs`; **Default Python behavior**: `pyright` si disponible, si no text fallback

#### Routing por archivo vs workspace

- Si el archivo es `.py`/`.pyi`, se usa `pyright` directamente
- Si el workspace solo tiene Python (sin C#/TS), `find_refs` usa `pyright`
- Si es mixto (Python + C#/TS), el routing se resuelve por extension del archivo

### Dependencias e interacciones

- `indexer/extractor_python.go` (extracción lexical acotada)
- `indexer/walker.go` (extensiones `.py`, `.pyi`)
- `worker/lsp_protocol.go` (framing JSON-RPC 2.0)
- `worker/lsp_client.go` (cliente LSP generico)
- `worker/pyright.go` (config + discovery)
- `worker/runtime_client.go` (case "pyright")
- `workspace/topology.go` (deteccion Python markers)
- `service/semantic.go` (routing `isPythonFile`)
- `daemon/lifecycle.go` (warm Python backends)

### Python markers detectados

- Extensiones: `.py`, `.pyi`
- Config files: `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt`, `poetry.lock`, `pipfile`, `pipfile.lock`

### Fallos y notas operativas
- **Riesgo**: Forma Python no reconocida por el extractor lexical; **Sintoma**: símbolo ausente en catálogo; **Mitigacion canonica**: fallback textual y Pyright opcional; el archivo conserva `FileRecord`
- **Riesgo**: Pyright no instalado; **Sintoma**: backend no inicia; **Mitigacion canonica**: fallback con warning accionable
- **Riesgo**: Python no instalado; **Sintoma**: `pythonPath` default a "python"; **Mitigacion canonica**: warning pero Pyright puede funcionar sin
- **Riesgo**: Repos grandes Python; **Sintoma**: indexación larga; **Mitigacion canonica**: extractor lexical acotado por líneas y cancelacion cooperativa por archivo

### Referencias de contexto
- [TECH-TS-BACKEND.md](TECH-TS-BACKEND.md)
- [CT-DAEMON-WORKER.md](contracts/CT-DAEMON-WORKER.md)
- [CT-CLI-DAEMON-ADMIN.md](contracts/CT-CLI-DAEMON-ADMIN.md)
