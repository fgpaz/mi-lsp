---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-024
kind: requirement
status: active
title: Búsqueda rg passthrough con anotación opcional por línea
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/grep.go
    role: behavior
  - path: internal/daemon/state_store.go
    role: behavior
---

## Propósito

Búsqueda rg passthrough con anotación opcional por línea

## Comportamiento ordenado

- **Marcador estable**: rf-qry-024-grep-v1
- **Estado del comportamiento**: implemented
- **contract**: grep-v1
- **Propósito**: mantener la semántica de ripgrep y añadir contexto de símbolos sin cambiar resultados de búsqueda
- **Entradas**:
  - **command**: mi-lsp grep [rg-args...]
  - **passthrough**: ejecutar el binario rg real desde MI_LSP_RG o PATH con los argumentos restantes
  - **flags_milsp**:
    - --rg-compat
    - --annotate
- **Comportamiento**:
  - Mantener selección, orden, globs, tipos y exit codes 0/1/2 de rg.
  - Por defecto anotar únicamente líneas de match de archivos de código indexados cuando stdout va directamente al agente.
  - Añadir a la línea un sufijo tabulado ⟦clase símbolo⟧; clase es def, ref, com o str y símbolo es el qualified_name contenedor cuando exista.
  - Si no hay catálogo, el archivo no es código soportado o ningún match pertenece a código indexado, entregar la salida original de rg.
  - Resolver por cwd/ruta la mejor workspace registrada sin mutar el registry. Si no hay catálogo, emitir por stderr una guía única con alias y `mi-lsp index --workspace <alias>`; si no está registrado, recomendar `mi-lsp workspace add <ruta> --name <alias> --no-index` y luego indexar explícitamente. No registrar ni reindexar en silencio.
  - Ante timeout, catálogo inválido o fallo recuperado del anotador, conservar íntegra la salida de rg y su exit code.
  - --rg-compat entrega salida rg pura; --annotate fuerza anotación incluso si stdout está redirigido.
  - Los modos de salida incompatibles con anotación (-l, -c, --json, --files, -o, --vimgrep) se mantienen como rg puro salvo --annotate explícito.
  - La telemetría registra operación grep y bytes de salida, nunca patrón, texto de match ni contenido de archivo.
- **Salidas**:
  - **stdout**: coincidencias de rg, potencialmente anotadas por línea
  - **stderr**: errores originales de rg y advertencia local única que nombra el alias probable, la ausencia de registro o el índice faltante
  - **exit_status**: código original de rg
- **Implementación**:
  - internal/grepx/
  - internal/cli/grep.go
  - internal/daemon/state_store.go
- **Comprobaciones previstas**:
  - TP-QRY/TC-QRY-192
  - TP-QRY/TC-QRY-193
  - TP-QRY/TC-QRY-194
- **Comprobación prevista**:
  - go test ./internal/grepx
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-QRY-024 --format toon
- **Evidencia referenciada**:
  - .docs/wiki/05_requirements/RF-QRY-024.md
  - internal/grepx/catalog_test.go
- **Límites**:
  - No alterar salida o código de salida en --rg-compat.
  - Nunca hacer que una falla de anotación convierta un éxito/fallo de rg en otro resultado.
  - No persistir patrones ni contenido de resultados en telemetría.
- **durable_evidence**:
  - .docs/wiki/06_tests/TP-QRY.md

## Superficies

- mi-lsp grep [rg-args...]

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. Mantener selección, orden, globs, tipos y exit codes 0/1/2 de rg.
2. Por defecto anotar únicamente líneas de match de archivos de código indexados cuando stdout va directamente al agente.
3. Añadir a la línea un sufijo tabulado ⟦clase símbolo⟧; clase es def, ref, com o str y símbolo es el qualified_name contenedor cuando exista.
4. Si no hay catálogo, el archivo no es código soportado o ningún match pertenece a código indexado, entregar la salida original de rg.
5. Resolver por cwd/ruta la mejor workspace registrada sin mutar el registry. Si no hay catálogo, emitir por stderr una guía única con alias y `mi-lsp index --workspace <alias>`; si no está registrado, recomendar `mi-lsp workspace add <ruta> --name <alias> --no-index` y luego indexar explícitamente. No registrar ni reindexar en silencio.
6. Ante timeout, catálogo inválido o fallo recuperado del anotador, conservar íntegra la salida de rg y su exit code.
7. --rg-compat entrega salida rg pura; --annotate fuerza anotación incluso si stdout está redirigido.
8. Los modos de salida incompatibles con anotación (-l, -c, --json, --files, -o, --vimgrep) se mantienen como rg puro salvo --annotate explícito.
9. La telemetría registra operación grep y bytes de salida, nunca patrón, texto de match ni contenido de archivo.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-024
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-024
- Búsqueda rg passthrough con anotación opcional por línea
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-024.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-024.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/cli/grep.go
- internal/daemon/state_store.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-QRY.md, ejecutar el
  comando publicado: go test ./internal/model ./internal/service ./internal/cli ./internal/indexer
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-QRY-024.
evidence:
- .docs/wiki/05_requirements/RF-QRY-024.md
- .docs/wiki/06_tests/TP-QRY.md
```
