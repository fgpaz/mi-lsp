# TECH-RUST-BACKEND

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-RUST-BACKEND"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-RUST-BACKEND]]'
exports:
  - 'TECH-RUST-BACKEND'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_tech/TECH-RUST-BACKEND.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-RUST-BACKEND.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-RUST-BACKEND.md
```

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Propósito y alcance

Define el soporte de Rust en `mi-lsp`: detección de Cargo, catálogo lexical local y semántica opcional mediante `rust-analyzer`. No promete análisis de tipos ni observación graph-native para Rust.

## Detección e indexación

- La extensión `.rs` se reconoce como código Rust para catálogo, búsqueda y lectura.
- `Cargo.toml` identifica proyectos Rust. Un manifiesto raíz mantiene el paquete y sus directorios fuente bajo un único repo incluso si no hay metadatos Git; si declara `[workspace]`, los crates miembros también quedan en ese repo y sus manifiestos se conservan como entrypoints.
- El extractor lexical acotado indexa funciones, métodos, structs, enums, traits, alias de tipo, constantes, estáticos, módulos y macros `macro_rules!` habituales. No compila el crate ni sustituye a un parser; los archivos permanecen disponibles para `nav search` aunque una forma no se catalogue.
- `nav symbols`, `nav intent`, `nav search` y `nav multi-read` operan sobre el catálogo/archivos sin iniciar `rust-analyzer`.

## Backend semántico

- `nav context` y `nav refs` pueden usar el cliente LSP genérico (`LSPClient`) con `rust-analyzer`.
- Selección automática por `.rs`; `--backend rust-analyzer` fuerza el backend.
- Búsqueda del ejecutable: `MI_LSP_RUST_ANALYZER_PATH`, `PATH`, `~/.cargo/bin/rust-analyzer`, `bin/rust-analyzer` o `.bin/rust-analyzer` dentro del workspace.
- `textDocument/references` devuelve referencias semánticas; `nav refs` conserva su ancla de catálogo cuando el usuario no indica un archivo. La definición sigue disponible como símbolo catalogado.
- Si el ejecutable falta o el LSP falla, se conserva el slice local y se degrada con warning a catálogo/texto; una indisponibilidad puede poner este backend en cooldown corto por repo.
- El daemon solo calienta Rust cuando el ejecutable está disponible y el workspace declara Rust.

## Límites

- El catálogo lexical es aproximado: no resuelve macros, expansión condicional, imports, tipos ni nombres homónimos.
- No se agrega un adapter graph-native de Rust ni se atribuyen relaciones `calls`/`references` al grafo persistido.
- La calidad de referencias depende de `rust-analyzer` y de que Cargo pueda cargar el workspace y sus dependencias locales.

## Trazabilidad

- Detección: [[RF-WKS-001]]
- Indexación: [[RF-IDX-001]]
- Routing de contexto y referencias: [[RF-QRY-002]]
- Navegación semántica: [[RF-QRY-006]]
