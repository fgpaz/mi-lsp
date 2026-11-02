---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WKS-006
kind: requirement
status: active
title: RF-WKS-006 - Exponer provenance del binario con `mi-lsp version`
links:
  implements:
  - FL-BOOT-01
  code:
  - path: internal/cli/root.go
    role: behavior
  - path: internal/cli/version.go
    role: behavior
  - path: internal/daemon/server.go
    role: behavior
  - path: internal/model/build_metadata.go
    role: behavior
  - path: internal/model/types.go
    role: behavior
  - path: internal/output/formatter.go
    role: behavior
---

## Propósito

RF-WKS-006 - Exponer provenance del binario con `mi-lsp version`

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WKS-006
- **Campo**: Título; **Valor**: Exponer provenance del binario con `mi-lsp version`
- **Campo**: Actores; **Valor**: Desarrollador, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-BOOT-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Binario `mi-lsp` ejecutable; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Metadata Go build info disponible; **Tipo**: técnica; **Estado requerido**: opcional
- **Condición**: Workspace registrado; **Tipo**: funcional; **Estado requerido**: no requerido
- **Condición**: Daemon corriendo; **Tipo**: técnica; **Estado requerido**: no requerido

### 3. Secuencia principal
1. El usuario o agente ejecuta `mi-lsp version`.
2. La CLI lee metadata local del ejecutable con `runtime/debug.ReadBuildInfo`, `runtime.GOOS`, `runtime.GOARCH`, `os.Executable` y hash SHA256 del binario cuando el archivo puede abrirse.
3. La CLI deriva `worker_rid` desde la misma logica de RID que usa la instalación de workers.
4. Sin `--format` explícito, la CLI emite una salida `text` corta y legible.
5. Con `--format compact|json|toon|yaml`, la CLI emite el envelope estable con `backend=version`.

### 5. Casos especiales y variantes
- Si la metadata VCS no existe, la salida estructurada omite esos campos y la salida `text` muestra `unknown` para revision/modified.
- `version` no resuelve workspace, no toca registry, no consulta daemon y no requiere worker instalado.
- GoReleaser inyecta `{{ .Version }}` (sin prefijo `v`, igual a `provider_version` dinámico) y `{{ .FullCommit }}` mediante `-X` en `internal/model.BuildVersion` y `internal/model.BuildRevision`. CLI y daemon comparten esos valores; el estado del daemon no queda en `(devel)` cuando el binario empaquetado ya tiene versión.
- `version` complementa `go version -m <path>`: el primero prueba la superficie CLI activa; el segundo sigue siendo una comprobacion externa util de provenance.
- `vcs_modified=false` indica que el binario fue construido desde un árbol limpio; no prueba por si solo que mirrors o instalación local esten sincronizados.

### Estado

`implemented`

### Notas de implementacion

- Comando: `internal/cli/version.go`
- Registro Cobra: `internal/cli/root.go`
- Modelo: `internal/model/types.go` (`VersionInfo`)
- Render text: `internal/output/formatter.go`
- Cobertura de tests: `TestReleaseLdflagsMetadataOverridesGoBuildInfo`, `TestRenderTextVersionInfo`

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 4. Salidas
- **Campo**: `command`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: identifica el ejecutable
- **Campo**: `version`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: version del tag inyectada por GoReleaser; en builds locales, version del módulo Go (puede ser `(devel)`)
- **Campo**: `module_path`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: módulo que construyo el binario
- **Campo**: `go_version`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: toolchain Go usada
- **Campo**: `goos` / `goarch`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: plataforma real del binario
- **Campo**: `protocol_version`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: version de protocolo visible
- **Campo**: `worker_rid`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: RID efectivo para workers
- **Campo**: `tool_root`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: root de herramienta resuelto
- **Campo**: `cli_path`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: ruta del ejecutable en uso
- **Campo**: `executable_sha256`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: hash del binario en uso cuando se puede calcular
- **Campo**: `vcs_revision` / `vcs_time` / `vcs_modified`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: provenance VCS embebido si existe

### 6. Impacto en el modelo de datos
- `VersionInfo`
- `QueryEnvelope`

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. El usuario o agente ejecuta `mi-lsp version`.
2. La CLI lee metadata local del ejecutable con `runtime/debug.ReadBuildInfo`, `runtime.GOOS`, `runtime.GOARCH`, `os.Executable` y hash SHA256 del binario cuando el archivo puede abrirse.
3. La CLI deriva `worker_rid` desde la misma logica de RID que usa la instalación de workers.
4. Sin `--format` explícito, la CLI emite una salida `text` corta y legible.
5. Con `--format compact|json|toon|yaml`, la CLI emite el envelope estable con `backend=version`.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WKS-006
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-BOOT-01.md
exports:
- RF-WKS-006
- RF-WKS-006 - Exponer provenance del binario con `mi-lsp version`
agent_must_read:
- .docs/wiki/05_requirements/RF-WKS-006.md
- .docs/wiki/04_flows/FL-BOOT-01.md
- .docs/wiki/06_tests/TP-WKS.md
- .docs/wiki/06_tests/TP-WKS-PROBE.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WKS-006.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/cli/root.go
- internal/cli/version.go
- internal/daemon/server.go
- internal/model/build_metadata.go
- internal/model/types.go
- internal/output/formatter.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-WKS.md, ejecutar el
  comando publicado: go test -count=1 ./internal/service -run ''^TestExecuteAutoRegisterSkipsIndexWithoutCommits$''
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-WKS-006.
evidence:
- .docs/wiki/05_requirements/RF-WKS-006.md
- .docs/wiki/06_tests/TP-WKS.md
- .docs/wiki/06_tests/TP-WKS-PROBE.md
```
