---
wiki_contract: WIKI-CONTRACT-v1
id: TP-HARNESS-DIAG-001
kind: test
status: draft
title: "Verificar diagnósticos por backend y aislamiento de cambios"
links:
  verifies: [RF-HARNESS-DIAG-001]
  code:
    - path: internal/service/diagnostics_test.go
    - path: internal/worker/diagnostics_test.go
    - path: internal/cli/nav_diagnostics_test.go
    - path: internal/output/diagnostics_test.go
---

## Alcance
TESTING_DEFERRED_UNTIL_IMPLEMENTATION_COMPLETE. Esta lane agrega casos/fixtures, pero no los ejecuta ni afirma aceptación. La única FINAL_VERIFY es del padre.

## Casos
- CLI: un archivo o `--changed` exclusivamente; `--base` requiere cambios; `--max-files` y offset acotados; payload exacto para `nav.diagnostics`.
- Git: staged, unstaged, eliminación, untracked con espacios/salto de línea, gitignore y base de merge; refs parecidas a flags no pasan como argumentos de proceso.
- Routing: un diff Go+TS genera ambos backends, con errores primero aunque el backend de warnings termine antes. Sin backend: `partial`, omisión tipada y nunca lista limpia.
- Aislamiento: path fuera del root, symlink externo, archivo eliminado y dos worktrees del mismo repo. `file_missing` explica ausencia en la raíz; `invalid_path` conserva el rechazo externo. Ningún resultado del checkout principal satisface el otro worktree.
- LSP: informe pull `full` vacío es válido; `null` o `unchanged` sin informe previo no lo son; push para otra URI se descarta; publicación vacía se distingue de ausencia; todo push conserva `partial`.
- tsserver: syntactic/semantic/suggestion DiagnosticsSync, coordenadas `startLocation` uno-based, código y severidad. En fallo de spawn/protocolo, comprobar etapa, argv y stderr acotado/sanitizado; nunca inferir limpio.
- Presupuesto: errores delante de warnings, totales previos al límite, salida humana corta y razones de omisión conservadas.

## Smoke semántico real, una vez integrado
Usar `testdata/diagnostics/` en workspaces de QA aislados (copiar cada subcarpeta fuera del checkout para no modificar el fixture). Instalar solo backends ya autorizados, no sustituir su ausencia con grep/build/test. Consultar cada archivo erróneo por CLI y `lsp.diagnostics` vía provider; observar backend real, mensaje, severidad, path y coordenadas. Corregir el error en la copia y repetir para descartar buffers viejos.

- Go: `go/bad.go`, asignación string→int (gopls).
- TS: `typescript/bad.ts`, asignación string→number con tsconfig strict (tsserver).
- Python: `python/bad.py`, asignación str→int con type checking basic (Pyright).
- C#: `roslyn/Broken.cs`, asignación string→int en `Diagnostics.csproj` (.NET 10/Roslyn). Debe usarse el worker compilado con el método nuevo.
- Rust: `rust/src/main.rs`, expresión incompleta; diagnóstico nativo de rust-analyzer, siempre cobertura parcial sin cargo check.

## Oráculo y límites
La evidencia del padre debe separar protocolos simulados de diagnósticos de backend real. Falta de binario o SDK implica `unavailable`, no PASS; parser nativo Rust no demuestra cobertura de `rustc`. Push sin barrera puede terminar con snapshot parcial incluso después de corregir: no declarar limpio completo. Un timeout tiene que devolver omisión antes del plazo global.

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-HARNESS-DIAG-001
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-HARNESS-DIAG-001.md
exports:
  - TP-HARNESS-DIAG-001
agent_must_read:
  - .docs/wiki/06_tests/TP-HARNESS-DIAG-001.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-HARNESS-DIAG-001.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-HARNESS-DIAG-001.md
  - .docs/wiki/07_technical/TECH-HARNESS-DIAG-001.md
verify:
  - .docs/wiki/06_tests/TP-HARNESS-DIAG-001.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - backend_unavailable_is_reported_as_pass
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-HARNESS-DIAG-001.md
  - .docs/wiki/05_requirements/RF-HARNESS-DIAG-001.md
```
