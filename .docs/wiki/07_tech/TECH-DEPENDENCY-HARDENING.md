# TECH-DEPENDENCY-HARDENING

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-DEPENDENCY-HARDENING"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-DEPENDENCY-HARDENING]]'
exports:
  - 'TECH-DEPENDENCY-HARDENING'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_tech/TECH-DEPENDENCY-HARDENING.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-DEPENDENCY-HARDENING.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-DEPENDENCY-HARDENING.md
```

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Summary

Documenta la postura de hardening de dependencias y bootstrap del worker .NET, incluyendo el tratamiento canonico de advisories NuGet del worker Roslyn y la estrategia de resolucion/instalacion desde cualquier `cwd`.

## Owner and scope

- Owner logico: C# semantic backend
- Scope: dependencias del worker Roslyn, hardening de supply chain, empaquetado por RID y criterios de bootstrap/remediacion
- Non-goals: lockfile reproducible multi-registro o SBOM formal en esta fase

## Runtime o subsistema

### Postura canonica

- Remediar advisories; no suprimirlos silenciosamente.
- Preferir upgrade del paquete raiz que arrastra la dependencia vulnerable.
- Mantener alineadas las versiones Roslyn/MSBuild con el minimo override necesario.
- Si se agregan referencias directas `Microsoft.Build.*` en un proyecto con `MSBuildLocator`, deben declararse con `ExcludeAssets="runtime"`.
- Si un advisory transitorio no queda cubierto por el upgrade raiz, se permite un pin directo del paquete vulnerable a la version parcheada minima publicada por Microsoft.

### Bootstrap y empaquetado canonico

- La CLI resuelve el worker C# contra el ejecutable/distribucion actual; si corre dentro del repo `mi-lsp`, puede caer a `dev-local` desde `worker-dotnet/`.
- Orden de seleccion canonico para queries: `bundle -> installed -> dev-local`, resuelto por presencia de archivos y sin probe de compatibilidad en el hot path.
- Compatibilidad minima significa que el worker responde al probe `status` con `protocol_version` aceptado por la CLI; ese probe queda reservado para `worker status` y diagnostico explicito.
- `worker install` debe copiar el bundle por RID cuando existe; solo usa `dotnet publish` como ruta de desarrollo o remediacion desde source.
- El empaquetado local/release debe materializar `dist/<rid>/mi-lsp(.exe)` + `dist/<rid>/workers/<rid>/` para que la CLI instalada vea el mismo layout que en validacion.
- El bundle del worker debe copiar el directorio completo de `dotnet publish` por RID; `PublishSingleFile` no es una variante soportada para Roslyn/MSBuild porque rompe la carga de dependencias en consultas semanticas reales.
- Los artefactos locales `bin/workers/<rid>` dentro del repo no se consideran bundle de distribucion canonico para consultas; se evita preferirlos por encima del fallback `dev-local`.
- El `tool_root` se deriva del ejecutable/distribucion o del repo `mi-lsp`; no debe depender del `cwd` del workspace consultado.
- Si el candidato real seleccionado falla por bootstrap/arranque, el caller reintenta una sola vez con el siguiente candidato determinista y, si no recupera, devuelve error enriquecido con `source/path` y remediacion `mi-lsp worker install`.
- Todos los subprocessos no interactivos asociados al worker, `git`, `rg`, `node/tsserver` y auto-start del daemon deben usar la politica comun de ocultamiento en Windows para evitar consolas extra.

### Secuencia de remediacion aplicada

1. Elevar:
   - `Microsoft.CodeAnalysis.CSharp.Workspaces`
   - `Microsoft.CodeAnalysis.Workspaces.MSBuild`
   a `5.0.0`
2. Rebuild y volver a auditar el grafo transitivo.
3. Como el advisory persistio en la resolucion intermedia, fijar referencias directas a:
   - `Microsoft.Build`
   - `Microsoft.Build.Framework`
   - `Microsoft.Build.Tasks.Core`
   - `Microsoft.Build.Utilities.Core`
   en `17.14.28`
4. Declarar `ExcludeAssets="runtime"` en esas referencias para respetar la carga via `Microsoft.Build.Locator`.
5. Para `GHSA-37gx-xxp4-5rgx` y `GHSA-w3x6-4m5h-cxqf`, fijar `System.Security.Cryptography.Xml` en `10.0.6`, version parcheada para la linea .NET 10.
6. Resultado esperado: `dotnet list worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj package --vulnerable --include-transitive` sin paquetes vulnerables y build release sin `NU1903`.

### Cierres no aceptados

- suppressions
- bajar audit level
- ignorar warnings en CI
- copiar manualmente assemblies vulnerables al output

## Dependencias e interacciones

- `worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj`
- build local/CI del worker
- compatibilidad con Roslyn Workspace API
- `Microsoft.Build.Locator`
- layout de distribucion canonico `dist/<rid>/mi-lsp(.exe)` + `dist/<rid>/workers/<rid>/`
- scripts operativos `scripts/release/build-dist.ps1`, `scripts/release/build-workers.ps1`, `scripts/release/install-local.ps1` y `scripts/release/ae-release-binaries.ps1` para build/install/release-distribution reproducible
- `.goreleaser.yaml` y `.github/workflows/release.yml` como pipeline publica canonica de empaquetado por RID
- la capa [[AE-RELEASE-DISTRIBUTION]] exige cerrar cambios release-visible con build de `win-arm64`, `win-x64`, `linux-arm64`, `linux-x64`, refresh local/WSL cuando aplique, evidencia de SHA/provenance y publicacion por tag limpio o waiver explicito

## Failure modes y notas operativas

| Riesgo | Sintoma | Mitigacion canonica |
|---|---|---|
| Advisory persistente | `dotnet build` emite `NU1903` | pin directo Microsoft.Build* en version corregida |
| Advisory transitorio en `System.Security.Cryptography.Xml` | restore/build alerta `GHSA-37gx-xxp4-5rgx` o `GHSA-w3x6-4m5h-cxqf` | pin directo a `System.Security.Cryptography.Xml` `10.0.6` o version superior parcheada |
| Incompatibilidad Roslyn/MSBuild | fallas al cargar workspace | subir versions en bloque controlado |
| Error de MSBuildLocator | assemblies MSBuild copiados al output | `ExcludeAssets="runtime"` |
| “Solucion” por suppression | build verde pero inseguro | prohibido por politica |
| Worker instalado stale o incompatible | `Dll was not found.`, `Unknown method 'status'` o mismatch de protocolo | `worker install` y seleccion de candidato compatible por RID |
| CLI ejecutada desde otro repo | el worker se resuelve contra el workspace ajeno | resolver `tool_root` desde ejecutable/distribucion o repo `mi-lsp`, no desde el `cwd` |
| Artefacto local `bin/workers` viejo | probe superficial verde pero consultas Roslyn fallan | en source repo preferir `dev-local`; no tratar `bin/workers/<rid>` como bundle canonico |
| Release parcial por RID | una maquina ARM64 o x64 sigue ejecutando revision vieja | `ae-release-binaries.ps1` debe construir todos los RIDs, refrescar local/WSL y publicar tag limpio para que GitHub Releases entregue assets nuevos |
| Binario Windows lockeado por daemon | `Copy-Item` falla sobre `C:\Users\fgpaz\bin\mi-lsp.exe` | `install-local.ps1` detiene el daemon existente antes de copiar y reintenta reemplazo/remocion |

## Related docs

- [CT-DAEMON-WORKER.md](../09_contratos/CT-DAEMON-WORKER.md)
