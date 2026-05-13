---
id: RF-WKS-006
title: Exponer provenance del binario con `mi-lsp version`
implements:
  - internal/cli/version.go
  - internal/cli/root.go
  - internal/model/types.go
  - internal/output/formatter.go
tests:
  - internal/cli/root_test.go
  - internal/output/formatter_test.go
---

# RF-WKS-006 - Exponer provenance del binario con `mi-lsp version`

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WKS-006"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-006]]'
exports:
  - 'RF-WKS-006'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-006.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-006.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WKS-006.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-006 |
| Titulo | Exponer provenance del binario con `mi-lsp version` |
| Actores | Desarrollador, Skill, Agente, CLI/Core |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Binario `mi-lsp` ejecutable | funcional | obligatorio |
| Metadata Go build info disponible | tecnica | opcional |
| Workspace registrado | funcional | no requerido |
| Daemon corriendo | tecnica | no requerido |

## 3. Process Steps (Happy Path)

1. El usuario o agente ejecuta `mi-lsp version`.
2. La CLI lee metadata local del ejecutable con `runtime/debug.ReadBuildInfo`, `runtime.GOOS`, `runtime.GOARCH`, `os.Executable` y hash SHA256 del binario cuando el archivo puede abrirse.
3. La CLI deriva `worker_rid` desde la misma logica de RID que usa la instalacion de workers.
4. Sin `--format` explicito, la CLI emite una salida `text` corta y legible.
5. Con `--format compact|json|toon|yaml`, la CLI emite el envelope estable con `backend=version`.

## 4. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `command` | string | usuario/skill | identifica el ejecutable |
| `version` | string | usuario/skill | version de modulo Go o `(devel)` |
| `module_path` | string | usuario/skill | modulo que construyo el binario |
| `go_version` | string | usuario/skill | toolchain Go usada |
| `goos` / `goarch` | string | usuario/skill | plataforma real del binario |
| `protocol_version` | string | usuario/skill | version de protocolo visible |
| `worker_rid` | string | usuario/skill | RID efectivo para workers |
| `tool_root` | string | usuario/skill | root de herramienta resuelto |
| `cli_path` | string | usuario/skill | ruta del ejecutable en uso |
| `executable_sha256` | string | usuario/skill | hash del binario en uso cuando se puede calcular |
| `vcs_revision` / `vcs_time` / `vcs_modified` | string | usuario/skill | provenance VCS embebido si existe |

## 5. Special Cases and Variants

- Si la metadata VCS no existe, la salida estructurada omite esos campos y la salida `text` muestra `unknown` para revision/modified.
- `version` no resuelve workspace, no toca registry, no consulta daemon y no requiere worker instalado.
- `version` complementa `go version -m <path>`: el primero prueba la superficie CLI activa; el segundo sigue siendo una comprobacion externa util de provenance.
- `vcs_modified=false` indica que el binario fue construido desde un arbol limpio; no prueba por si solo que mirrors o instalacion local esten sincronizados.

## 6. Data Model Impact

- `VersionInfo`
- `QueryEnvelope`

## Estado

`implemented`

## Notas de implementacion

- Comando: `internal/cli/version.go`
- Registro Cobra: `internal/cli/root.go`
- Modelo: `internal/model/types.go` (`VersionInfo`)
- Render text: `internal/output/formatter.go`
- Cobertura de tests: `TestRootCommandExposesVersionCommand`, `TestBuildVersionInfoUsesRuntimeProvenance`, `TestRenderTextVersionInfo`
