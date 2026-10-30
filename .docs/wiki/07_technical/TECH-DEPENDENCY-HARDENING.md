---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-DEPENDENCY-HARDENING
kind: technical
status: active
title: TECH-DEPENDENCY-HARDENING
links:
  code:
  - path: scripts/release/install-local.sh
    role: implementation
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-DEPENDENCY-HARDENING
kind: technical
audience: dual
imports: [.docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md]
exports: [TECH-DEPENDENCY-HARDENING]
agent_must_read: [.docs/wiki/07_technical/TECH-DEPENDENCY-HARDENING.md, worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj]
agent_may_edit: [.docs/wiki/07_technical/TECH-DEPENDENCY-HARDENING.md]
agent_must_not_edit: [worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj, scripts/release/, .docs/ae/repo-policy.yaml]
verify: ["Leer .docs/wiki/07_technical/TECH-DEPENDENCY-HARDENING.md y .docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md; cotejar las afirmaciones con la fuente publicada worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Una auditoría registre paquetes vulnerables o NU1903, o se proponga suppression/relajar el gate de release.]
evidence: [.docs/wiki/07_technical/TECH-DEPENDENCY-HARDENING.md, .docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md, worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj]
```

## Resumen

Documenta la postura de hardening de dependencias y bootstrap del worker .NET, incluyendo el tratamiento canónico de advisories NuGet del worker Roslyn y la estrategia de resolucion/instalacion desde cualquier `cwd`.

### Resumen
Documenta la postura de hardening de dependencias y bootstrap del worker .NET, incluyendo el tratamiento canónico de advisories NuGet del worker Roslyn y la estrategia de resolucion/instalacion desde cualquier `cwd`.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

Volver a [07_technical.md](../07_technical.md).

### Responsable y alcance
- Owner logico: C# semantic backend
- Scope: dependencias del worker Roslyn, hardening de supply chain, empaquetado por RID y criterios de bootstrap/remediacion
- Non-goals: lockfile reproducible multi-registro o SBOM formal en esta fase

### Runtime o subsistema

#### Postura canónica

- Remediar advisories; no suprimirlos silenciosamente.
- Preferir upgrade del paquete raiz que arrastra la dependencia vulnerable.
- Mantener alineadas las versiones Roslyn/MSBuild con el mínimo override necesario.
- Si se agregan referencias directas `Microsoft.Build.*` en un proyecto con `MSBuildLocator`, deben declararse con `ExcludeAssets="runtime"`.
- Si un advisory transitorio no queda cubierto por el upgrade raiz, se permite un pin directo del paquete vulnerable a la version parcheada mínima publicada por Microsoft.

#### Bootstrap y empaquetado canónico

- La CLI resuelve el worker C# contra el ejecutable/distribucion actual; si corre dentro del repo `mi-lsp`, puede caer a `dev-local` desde `worker-dotnet/`.
- Orden de selección canónico para queries: `bundle -> installed -> dev-local`, resuelto por presencia de archivos y sin probe de compatibilidad en el hot path.
- Compatibilidad mínima significa que el worker responde al probe `status` con `protocol_version` aceptado por la CLI; ese probe queda reservado para `worker status` y diagnóstico explicito.
- `worker install` debe copiar el bundle por RID cuando existe; solo usa `dotnet publish` como ruta de desarrollo o remediacion desde source.
- El empaquetado local/release debe materializar `dist/<rid>/mi-lsp(.exe)` + `dist/<rid>/workers/<rid>/` para que la CLI instalada vea el mismo layout que en validacion.
- `scripts/release/install-local.sh` debe tratar la CLI, el bundle local y `~/.mi-lsp/workers/<rid>` como una sola activación: preservar el worker global anterior hasta que `worker install`, `version` y `worker status` terminen, y restaurarlo ante fallos o señales. El directorio global corresponde a `os.UserHomeDir()/.mi-lsp/workers/<rid>`; si se solapa con un destino local, la instalación debe rechazarse antes de reemplazar archivos.
- Los instaladores publicos `scripts/install/install.ps1|sh` deben consumir el asset GoReleaser por RID y preservar ese mismo layout después de verificar checksum.
- El bundle del worker debe copiar el directorio completo de `dotnet publish` por RID; `PublishSingleFile` no es una variante soportada para Roslyn/MSBuild porque rompe la carga de dependencias en consultas semánticas reales.
- Los artefactos locales `bin/workers/<rid>` dentro del repo no se consideran bundle de distribucion canónico para consultas; se evita preferirlos por encima del fallback `dev-local`.
- El `tool_root` se deriva del ejecutable/distribucion o del repo `mi-lsp`; no debe depender del `cwd` del workspace consultado.
- Si el candidato real seleccionado falla por bootstrap/arranque, el caller reintenta una sola vez con el siguiente candidato determinista y, si no recupera, devuelve error enriquecido con `source/path` y remediacion `mi-lsp worker install`.
- Todos los subprocessos no interactivos asociados al worker, `git`, `rg`, `node/tsserver` y auto-start del daemon deben usar la politica comun de ocultamiento en Windows para evitar consolas extra.

#### Secuencia de remediacion aplicada

1. Elevar:
   - `Microsoft.CodeAnalysis.CSharp.Workspaces`
   - `Microsoft.CodeAnalysis.Workspaces.MSBuild`
   a `5.0.0`
2. Rebuild y volver a auditar el grafo transitivo.
3. Como el advisory persistio en la resolución intermedia, fijar referencias directas a:
   - `Microsoft.Build`
   - `Microsoft.Build.Framework`
   - `Microsoft.Build.Tasks.Core`
   - `Microsoft.Build.Utilities.Core`
   en `17.14.28`
4. Declarar `ExcludeAssets="runtime"` en esas referencias para respetar la carga via `Microsoft.Build.Locator`.
5. El pin previo `10.0.10` resolvía los advisories originales, pero la auditoría de release volvió a detectar vulnerabilidades altas en XML y PKCS. No se distribuye ese resultado.
6. La referencia efectiva `System.Security.Cryptography.Xml` queda fijada en `10.0.14`, cuyo grafo requiere `System.Security.Cryptography.Pkcs` `10.0.14`; la auditoría vulnerable sigue siendo gate de release.
7. La auditoria de vulnerabilidades del worker es un gate: un advisory vulnerable o un build con `NU1903` no se promociona como listo.
8. No se registra resultado de auditoria ni estado de release sin ejecutar el comando correspondiente y conservar evidencia sanitizada.

- **Marcador estable**: TECH-DEPENDENCY-HARDENING.vulnerability-gate
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/TECH-DEPENDENCY-HARDENING.md
  - worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj
- **Comprobación prevista**:
  - dotnet list worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj package --vulnerable --include-transitive
  - dotnet build worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj --configuration Release
- **Condiciones de rechazo**:
  - vulnerable_package_reported=true
  - NU1903_present=true
  - audit_evidence_missing=true
- **package**:
  - **name**: System.Security.Cryptography.Xml
  - **pinned_version**: 10.0.14
- **audit**:
  - **vulnerable_scan**: release_blocking
  - **build_warning_NU1903**: release_blocking
  - **result_claim**: forbidden_without_command_and_sanitized_evidence

#### Cierres no aceptados

- suppressions
- bajar audit level
- ignorar warnings en CI
- copiar manualmente assemblies vulnerables al output

### Dependencias e interacciones

- `worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj`
- build local/CI del worker
- compatibilidad con Roslyn Workspace API
- `Microsoft.Build.Locator`
- layout de distribucion canónico `dist/<rid>/mi-lsp(.exe)` + `dist/<rid>/workers/<rid>/`
- scripts operativos `scripts/release/build-dist.ps1`, `scripts/release/build-workers.ps1`, `scripts/release/install-local.ps1`, `scripts/release/ae-release-binaries.ps1` y `scripts/install/*` para build/install/release-distribution reproducible
- `.goreleaser.yaml` y `.github/workflows/release.yml` como pipeline publica canónica de empaquetado por RID
- la capa [[AE-RELEASE-DISTRIBUTION]] exige cerrar cambios release-visible con build de `win-arm64`, `win-x64`, `linux-arm64`, `linux-x64`, refresh local/WSL cuando aplique, evidencia de SHA/provenance y publicación por tag limpio o waiver explícito

### Fallos y notas operativas
> `wiki_source_table_exception: true`. La siguiente tabla es una comparacion humana de sintomas y mitigaciones; el gate normativo de vulnerabilidades vive en `TECH-DEPENDENCY-HARDENING.vulnerability-gate`.

- **Riesgo**: Advisory persistente; **Sintoma**: `dotnet build` emite `NU1903`; **Mitigacion canonica**: pin directo Microsoft.Build* en version corregida
- **Riesgo**: Advisory transitorio en `System.Security.Cryptography.Xml`; **Sintoma**: restore/build alerta `GHSA-37gx-xxp4-5rgx` o `GHSA-w3x6-4m5h-cxqf`; **Mitigacion canonica**: pin directo a `System.Security.Cryptography.Xml` `10.0.14` o versión superior parcheada, con PKCS transitivo auditado
- **Riesgo**: Incompatibilidad Roslyn/MSBuild; **Sintoma**: fallas al cargar workspace; **Mitigacion canonica**: subir versions en bloque controlado
- **Riesgo**: Error de MSBuildLocator; **Sintoma**: assemblies MSBuild copiados al output; **Mitigacion canonica**: `ExcludeAssets="runtime"`
- **Riesgo**: “Solucion” por suppression; **Sintoma**: build verde pero inseguro; **Mitigacion canonica**: prohibido por politica
- **Riesgo**: Worker instalado stale o incompatible; **Sintoma**: `Dll was not found.`, `Unknown method 'status'` o mismatch de protocolo; **Mitigacion canonica**: `worker install` y selección de candidato compatible por RID
- **Riesgo**: CLI ejecutada desde otro repo; **Sintoma**: el worker se resuelve contra el workspace ajeno; **Mitigacion canonica**: resolver `tool_root` desde ejecutable/distribucion o repo `mi-lsp`, no desde el `cwd`
- **Riesgo**: Artefacto local `bin/workers` viejo; **Sintoma**: probe superficial verde pero consultas Roslyn fallan; **Mitigacion canonica**: en source repo preferir `dev-local`; no tratar `bin/workers/<rid>` como bundle canónico
- **Riesgo**: Release parcial por RID; **Sintoma**: una maquina ARM64 o x64 sigue ejecutando revision vieja; **Mitigacion canonica**: `ae-release-binaries.ps1` debe construir todos los RIDs, refrescar local/WSL y publicar tag limpio para que GitHub Releases entregue assets nuevos
- **Riesgo**: Binario Windows lockeado por daemon; **Sintoma**: `Copy-Item` falla sobre `C:\Users\<user>\bin\mi-lsp.exe`; **Mitigacion canonica**: `install-local.ps1` detiene el daemon existente antes de copiar y reintenta reemplazo/remocion
- **Riesgo**: Reemplazo local Unix del CLI en ejecución; **Sintoma**: sobrescribir el ejecutable causa `ETXTBSY` o deja un archivo truncado; **Mitigacion canonica**: `install-local.sh` prepara CLI y worker junto al destino, activa el CLI mediante rename y conserva ambos artefactos anteriores para rollback

### Referencias de contexto
- [CT-DAEMON-WORKER.md](contracts/CT-DAEMON-WORKER.md)
