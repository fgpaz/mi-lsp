#!/usr/bin/env pwsh
# hermes-wiki-global.ps1
#
# Wrapper de orquestación cross-host para nav wiki * --all-workspaces.
# mi-lsp permanece CLI puro per-máquina; este script vive del lado de Hermes
# y NO va dentro de mi-lsp.
#
# Uso:
#   pwsh -File hermes-wiki-global.ps1 -Subcommand inventory
#   pwsh -File hermes-wiki-global.ps1 -Subcommand search -Args2Pass @("--all-workspaces","--format","toon","governance")
#
# Lee ~/.hermes/hosts.yaml (override con -HostsFile) y ejecuta el subcomando
# en cada host (transport=local o transport=tailscale-ssh), mergea TOON,
# anota `host:<name>` por item.

[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateSet('search','inventory','route','trace','pack')]
    [string]$Subcommand,

    [string]$HostsFile = "$HOME/.hermes/hosts.yaml",

    [Parameter(ValueFromRemainingArguments)]
    [string[]]$Args2Pass = @("--all-workspaces", "--format", "toon")
)

function Read-HostsFile([string]$Path) {
    if (-not (Test-Path $Path)) { throw "hosts file not found: $Path" }
    $raw = Get-Content $Path -Raw
    if (Get-Module -ListAvailable powershell-yaml -ErrorAction SilentlyContinue) {
        Import-Module powershell-yaml -ErrorAction SilentlyContinue
        return ConvertFrom-Yaml $raw
    }
    # Fallback minimal parser: solo cubre el shape declarado en hosts.yaml.example
    $hosts = @()
    $defaults = @{ timeout_seconds = 5; semaphore_hosts = 2 }
    $section = $null
    $currentHost = $null
    foreach ($line in ($raw -split "`r?`n")) {
        $trim = $line.Trim()
        if ($trim -eq "" -or $trim.StartsWith("#")) { continue }
        if ($trim -eq "hosts:") { $section = "hosts"; continue }
        if ($trim -eq "defaults:") { $section = "defaults"; continue }
        if ($section -eq "hosts" -and $trim.StartsWith("- name:")) {
            if ($currentHost) { $hosts += $currentHost }
            $currentHost = @{ name = $trim.Substring("- name:".Length).Trim() }
            continue
        }
        if ($section -eq "hosts" -and $currentHost -ne $null) {
            if ($trim -match '^(\w+):\s*(.+)$') {
                $currentHost[$Matches[1]] = $Matches[2].Trim()
            }
            continue
        }
        if ($section -eq "defaults" -and $trim -match '^(\w+):\s*(\d+)') {
            $defaults[$Matches[1]] = [int]$Matches[2]
        }
    }
    if ($currentHost) { $hosts += $currentHost }
    return @{ hosts = $hosts; defaults = $defaults }
}

$cfg = Read-HostsFile $HostsFile
$timeout = [int]($cfg.defaults.timeout_seconds)
$semaphore = [int]($cfg.defaults.semaphore_hosts)

$cliArgs = @("nav", "wiki", $Subcommand) + $Args2Pass

Write-Verbose "Invoking on hosts: $($cfg.hosts.name -join ', ') with args: $($cliArgs -join ' ')"

# Ejecutar en paralelo bounded por semaphore
$jobs = @()
foreach ($h in $cfg.hosts) {
    $jobs += Start-ThreadJob -ThrottleLimit $semaphore -ScriptBlock {
        param($HostInfo, $CliArgs, $TimeoutSec)
        $name = $HostInfo.name
        try {
            $output = $null
            if ($HostInfo.transport -eq "local") {
                $bin = if ($HostInfo.mi_lsp_bin) { $HostInfo.mi_lsp_bin } else { "mi-lsp" }
                $output = & $bin @CliArgs 2>&1
            } elseif ($HostInfo.transport -eq "tailscale-ssh") {
                $bin = if ($HostInfo.mi_lsp_bin) { $HostInfo.mi_lsp_bin } else { "mi-lsp" }
                $target = $HostInfo.ssh_target
                $output = & tailscale ssh $target -- $bin @CliArgs 2>&1
            } else {
                return @{ host = $name; failed = $true; reason = "unknown transport: $($HostInfo.transport)" }
            }
            if ($LASTEXITCODE -ne 0) {
                return @{ host = $name; failed = $true; reason = "exit=$LASTEXITCODE"; output = ($output | Out-String) }
            }
            return @{ host = $name; failed = $false; output = ($output | Out-String) }
        } catch {
            return @{ host = $name; failed = $true; reason = $_.Exception.Message }
        }
    } -ArgumentList $h, $cliArgs, $timeout
}

# Esperar todos con timeout total
$results = $jobs | Wait-Job -Timeout ($timeout * 2 + 5) | Receive-Job

# Merge TOON envelopes
$hostsQueried = @()
$hostsFailed = @()
$mergedLines = @()

foreach ($r in $results) {
    if ($r.failed) {
        $hostsFailed += @{ name = $r.host; reason = $r.reason }
        continue
    }
    $hostsQueried += $r.host
    # Anotación simple: prefijar cada línea TOON con un comentario que indica el host
    # (un parser real haría merge estructurado de items; aquí mantenemos formato legible).
    $mergedLines += "# === host: $($r.host) ==="
    $mergedLines += $r.output.TrimEnd()
    $mergedLines += ""
}

# Emitir TOON unificado (formato simple)
Write-Output "ok: true"
Write-Output "subcommand: $Subcommand"
Write-Output "hosts_queried[$($hostsQueried.Count)]: $($hostsQueried -join ',')"
if ($hostsFailed.Count -gt 0) {
    Write-Output "hosts_failed[$($hostsFailed.Count)]:"
    foreach ($f in $hostsFailed) {
        Write-Output "  - { name: $($f.name), reason: $($f.reason) }"
    }
} else {
    Write-Output "hosts_failed[0]: []"
}
Write-Output "---"
$mergedLines -join "`n" | Write-Output
