# Task L8: .NET worker — detección de proyecto duplicado

## Shared Context
**Goal:** Que el worker Roslyn detecte nombres de proyecto duplicados en una solution ANTES de cargar, y falle rápido con diagnóstico accionable (causa raíz de los 70s de nav.refs).
**Stack:** .NET 10, Roslyn MSBuildWorkspace.
**Architecture:** Worktree `C:/wt/v050-l8-dotnet-worker`, branch `v050/l8-dotnet-worker`. Único dueño de `worker-dotnet/MiLsp.Worker/*.cs`. Independiente del código Go.

## Locked Decisions
- Antes de `OpenSolutionAsync`, parsear el `.sln` y detectar nombres de proyecto duplicados en la misma carpeta de solución; si hay, devolver un error estructurado `solution_config_error` con los nombres ofensores, sin intentar la carga completa.
- No cambiar el contrato stdin/stdout salvo agregar el nuevo error code; el lado Go (L5) ya cachea el fallo.
- Agregar nota de follow-up (no implementar) sobre code-signing del worker en el verdict.

## Task Metadata
```yaml
id: L8
depends_on: [T2]
agent_type: ps-dotnet10
goal_id: G1
github_issues: []
expected_outcome: "Una solution con proyectos duplicados produce un error claro inmediato en vez de colgar 70s."
files:
  - modify: worker-dotnet/MiLsp.Worker/RoslynService.cs
  - modify: worker-dotnet/MiLsp.Worker/Program.cs
  - modify: worker-dotnet/MiLsp.Worker/ProtocolModels.cs
complexity: medium
done_when:
  - "dotnet build worker-dotnet/MiLsp.Worker exits 0"
  - "a test/smoke .sln with duplicate project names returns solution_config_error fast (<5s)"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L8-verdict.yaml"
stop_if:
  - ".NET 10 SDK is not installed — record cleanup_blocked; AUD-04 still closes on the Go timeout side (L5)"
```

## Reference
`discovery.yaml` AUD-04. `worker-dotnet/MiLsp.Worker/RoslynService.cs:215-233,285` (LoadSolutionAsync/ResolveSolutionPath). El `.sln` real que falló: BuhoSalud.sln (líneas 20/42/62/82/102 con `Shared.Contract` duplicado).

## Prompt
Editá SOLO `worker-dotnet/MiLsp.Worker/*.cs`. Antes de `OpenSolutionAsync` en `RoslynService.cs`, leé el `.sln`, extraé los `Project(...)` y sus nombres dentro de cada carpeta de solución, y si hay nombres duplicados en la misma carpeta, devolvé un error estructurado nuevo (`solution_config_error`) con la lista de duplicados — sin llamar a `OpenSolutionAsync`. Agregá el error code en `ProtocolModels.cs`. No cambies el resto del protocolo. En el verdict, anotá la nota de follow-up de code-signing (no implementar ahora).

## Execution Procedure
1. `cd C:/wt/v050-l8-dotnet-worker`; `git merge --no-edit main`.
2. Verificá `dotnet --version` (necesita SDK .NET 10). Si falta, STOP con `cleanup_blocked` en verdict.
3. Implementá la detección + error code.
4. `dotnet build worker-dotnet/MiLsp.Worker`.
5. Smoke con un `.sln` de prueba con duplicados.
6. Commit. `L8-verdict.yaml`.

## Skeleton
```csharp
var dupes = ParseSolutionProjects(slnPath)
    .GroupBy(p => (p.SolutionFolder, p.Name))
    .Where(g => g.Count() > 1).Select(g => g.Key.Name).ToList();
if (dupes.Count > 0)
    return Error("solution_config_error", $"duplicate project names: {string.Join(", ", dupes)}");
```

## Verify
`dotnet build worker-dotnet/MiLsp.Worker` → Build succeeded

## Commit
`feat(worker): detect duplicate solution project names, fail fast (AUD-04 .NET side)`
