---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - FL-WIKI-01
allowed_paths:
  - .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hermes-wiki-global.ps1
forbidden_paths:
  - .docs/wiki/**
  - internal/**
  - worker-dotnet/**
verify:
  - test -f .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hermes-wiki-global.ps1
  - "pwsh -NoProfile -File hermes-wiki-global.ps1 -Subcommand inventory -Verbatim -- --all-workspaces --format toon  ->  ok=true (smoke local con --hosts-file <example>)"
stop_if:
  - tailscale CLI no instalado en la máquina del ejecutor (el script debe seguir funcionando con transport=local)
secret_scan: clean
---

# Task T16: Wrapper PowerShell Hermes-side para orquestación cross-host

## Shared Context
**Goal:** Script de orquestación del lado Hermes que lee `~/.hermes/hosts.yaml`, invoca `mi-lsp <subcommand> --all-workspaces` en cada host (local + tailscale-ssh), mergea los TOONs y devuelve un envelope unificado.
**Stack:** PowerShell 7+.
**Architecture:** Vive en `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hermes-wiki-global.ps1` (companion folder). El humano lo copia a `~/.hermes/global-search.ps1` o lo invoca desde donde quiera.

## Locked Decisions
- Script PowerShell único (sin dependencias externas excepto `powershell-yaml` o parsing manual).
- Params: `-Subcommand` (search/inventory/route/trace/pack), `-Args` (remainder pasado a mi-lsp), `-HostsFile` (default `~/.hermes/hosts.yaml`).
- Por host: `transport=local` corre `mi-lsp` directo; `transport=tailscale-ssh` corre `tailscale ssh <ssh_target> -- mi-lsp`.
- Paralelismo entre hosts: `Start-ThreadJob` o `ForEach-Object -Parallel`.
- Anota `host:<name>` en cada item del merge.
- Fallo de host = entry en `hosts_failed[]` con `reason`; no aborta.

## Task Metadata
```yaml
id: T16
depends_on: [T15]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "Script PowerShell que orquesta cross-host y devuelve un envelope mergeado existe y corre."
files:
  - create: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hermes-wiki-global.ps1
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example
complexity: high
done_when:
  - "el script existe y es ejecutable: pwsh -NoProfile -File hermes-wiki-global.ps1 -Subcommand inventory"
  - "con HostsFile apuntando al example, el script ejecuta el primer host (local) e imprime envelope mergeado"
  - "si un host falla (transport=tailscale-ssh y target inválido), aparece en hosts_failed[]; no aborta"
  - "items del merge tienen campo host<>''"
evidence_expected:
  - "Output del script con -Subcommand inventory contra hosts.yaml.example"
  - "Demostración de fallo no-fatal (target inexistente)"
stop_if:
  - "mi-lsp no está en PATH del ejecutor — pedir cómo proceder"
```

## Reference
- T15 hosts.yaml.example.
- Patrón TOON merge: items planos ordenados; stats agregados.

## Prompt

Sos el ejecutor de T16 (ps-worker). Crear UN script PowerShell.

1. Crear `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hermes-wiki-global.ps1` con el contenido del Skeleton (adaptado).
2. **Parseo del hosts.yaml**: usar `powershell-yaml` si está; si no, hacer parseo simple line-by-line (el formato es estable).
3. **Invocación por host**:
   - `transport=local`: `& mi-lsp $Args2Pass` capturando stdout.
   - `transport=tailscale-ssh`: `& tailscale ssh $sshTarget -- mi-lsp @Args2Pass` con timeout `$defaults.timeout_seconds`.
4. **Captura de stderr y exit code**: si exit != 0, anotar en `hosts_failed[]` con reason = "exit=$LASTEXITCODE" o "timeout".
5. **Paralelo**: usar `ForEach-Object -Parallel` o `Start-ThreadJob` con `$defaults.semaphore_hosts` jobs simultáneos.
6. **Merge**: parsear cada envelope TOON (parsing simple — el output de mi-lsp es estable). Concatenar `items[]` y anotar `host` por item. Acumular `workspaces_queried` y `workspaces_failed` cross-host. Agregar campo nuevo `hosts_queried` y `hosts_failed`.
7. **Output**: TOON unificado a stdout.
8. **Smoke test**:
   ```powershell
   pwsh -NoProfile -File .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hermes-wiki-global.ps1 `
       -Subcommand inventory `
       -HostsFile .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example `
       -- --all-workspaces --format toon
   ```
   Verificar que la salida tiene items, hosts_queried, y `ok=true`.
9. Commit: `feat(hermes): add PowerShell wrapper for cross-host wiki nav orchestration`.
10. Reportar.

## Execution Procedure
1. Leer hosts.yaml.example.
2. Escribir el script.
3. Smoke con HostsFile=example.
4. Smoke con target inválido para validar fallo no-fatal.
5. Commit.
6. Reportar.

## Skeleton

```powershell
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Subcommand,   # search | inventory | route | trace | pack
    [string]$HostsFile = "$HOME/.hermes/hosts.yaml",
    [Parameter(ValueFromRemainingArguments)][string[]]$Args2Pass
)

# Parse hosts.yaml (simple parsing or powershell-yaml)
function Read-HostsFile([string]$Path) {
    if (-not (Test-Path $Path)) { throw "hosts file not found: $Path" }
    Import-Module powershell-yaml -ErrorAction SilentlyContinue
    $raw = Get-Content $Path -Raw
    if (Get-Module powershell-yaml) {
        return ConvertFrom-Yaml $raw
    } else {
        # fallback minimal parser — solo cubre el shape declarado
        throw "powershell-yaml module not found; install with: Install-Module powershell-yaml -Scope CurrentUser"
    }
}

$cfg = Read-HostsFile $HostsFile
$timeout = [int]($cfg.defaults.timeout_seconds ?? 5)
$semaphore = [int]($cfg.defaults.semaphore_hosts ?? 2)

$cliArgs = @("nav", "wiki", $Subcommand) + $Args2Pass

$jobs = foreach ($h in $cfg.hosts) {
    Start-ThreadJob -ThrottleLimit $semaphore -ScriptBlock {
        param($Host, $CliArgs, $TimeoutSec)
        try {
            $cmd = $null; $outErr = $null
            if ($Host.transport -eq "local") {
                $cmd = & mi-lsp @CliArgs 2>&1
            } elseif ($Host.transport -eq "tailscale-ssh") {
                $bin = $Host.mi_lsp_bin ?? "mi-lsp"
                $cmd = & tailscale ssh $Host.ssh_target -- $bin @CliArgs 2>&1
            } else {
                throw "unknown transport: $($Host.transport)"
            }
            if ($LASTEXITCODE -ne 0) {
                return @{ host = $Host.name; failed = $true; reason = "exit=$LASTEXITCODE"; output = $cmd }
            }
            return @{ host = $Host.name; failed = $false; output = $cmd }
        } catch {
            return @{ host = $Host.name; failed = $true; reason = $_.Exception.Message }
        }
    } -ArgumentList $h, $cliArgs, $timeout
}

$results = $jobs | Wait-Job -Timeout ($timeout * 2) | Receive-Job

# Merge envelopes (TOON parse + items con host annotation)
$mergedItems = @()
$wsQueried = 0
$wsFailed = @()
$hostsQueried = @()
$hostsFailed = @()

foreach ($r in $results) {
    if ($r.failed) {
        $hostsFailed += @{ name = $r.host; reason = $r.reason }
        continue
    }
    $hostsQueried += $r.host
    # parsear el TOON output (parsing simple por regex o ConvertFrom-Yaml si el TOON parsea como YAML válido — habitualmente sí)
    # extraer items[] y stats
    # ... etiquetar cada item con host = $r.host
    # ... acumular wsQueried y wsFailed
}

# Emitir TOON merged
@{
    ok = $true
    items = $mergedItems
    stats = @{
        workspaces_queried = $wsQueried
        workspaces_failed  = $wsFailed
        hosts_queried      = $hostsQueried
        hosts_failed       = $hostsFailed
    }
} | ConvertTo-Yaml
```

## Verify
`pwsh -File hermes-wiki-global.ps1 -Subcommand inventory -HostsFile <example> -- --all-workspaces --format toon` -> envelope con items[], hosts_queried, hosts_failed; ok=true incluso si un host falla

## Commit
`feat(hermes): add PowerShell wrapper for cross-host wiki nav orchestration`
