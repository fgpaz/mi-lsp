# TECH-DEPENDENCY-HARDENING

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Summary

Documenta la postura de hardening de dependencias y bootstrap del worker .NET, incluyendo el tratamiento canonico del advisory `GHSA-h4j7-5rxr-p4wc` y la estrategia de resolucion/instalacion del worker Roslyn desde cualquier `cwd`.

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
5. Resultado esperado: `dotnet build worker-dotnet/MiLsp.Worker.sln` sin `NU1903`.

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
- scripts operativos `scripts/release/build-dist.ps1`, `scripts/release/build-workers.ps1` e `scripts/release/install-local.ps1` para build/install reproducible
- `.goreleaser.yaml` y `.github/workflows/release.yml` como pipeline publica canonica de empaquetado por RID

## Failure modes y notas operativas

| Riesgo | Sintoma | Mitigacion canonica |
|---|---|---|
| Advisory persistente | `dotnet build` emite `NU1903` | pin directo Microsoft.Build* en version corregida |
| Incompatibilidad Roslyn/MSBuild | fallas al cargar workspace | subir versions en bloque controlado |
| Error de MSBuildLocator | assemblies MSBuild copiados al output | `ExcludeAssets="runtime"` |
| “Solucion” por suppression | build verde pero inseguro | prohibido por politica |
| Worker instalado stale o incompatible | `Dll was not found.`, `Unknown method 'status'` o mismatch de protocolo | `worker install` y seleccion de candidato compatible por RID |
| CLI ejecutada desde otro repo | el worker se resuelve contra el workspace ajeno | resolver `tool_root` desde ejecutable/distribucion o repo `mi-lsp`, no desde el `cwd` |
| Artefacto local `bin/workers` viejo | probe superficial verde pero consultas Roslyn fallan | en source repo preferir `dev-local`; no tratar `bin/workers/<rid>` como bundle canonico |

## Related docs

- [CT-DAEMON-WORKER.md](../09_contratos/CT-DAEMON-WORKER.md)



