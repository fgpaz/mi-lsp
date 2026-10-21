[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Binary
)

$ErrorActionPreference = "Stop"
$resolvedBinary = (Resolve-Path -LiteralPath $Binary).Path
$runRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("mi-lsp-setup-wiki-smoke-" + [guid]::NewGuid().ToString("N"))
$fixtureHome = Join-Path $runRoot "home"
$repo = Join-Path $runRoot "repo"
$wiki = Join-Path $runRoot "external-wiki"
$alias = "setup-wiki-gate-smoke"
$oldHome = $env:HOME
$oldUserProfile = $env:USERPROFILE
$oldNoAutoRegister = $env:MI_LSP_NO_AUTO_REGISTER

function Invoke-MiLsp {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)

    $output = & $resolvedBinary @Arguments
    $exitCode = $LASTEXITCODE
    $text = ($output | Out-String).Trim()
    if ($exitCode -ne 0) {
        throw "mi-lsp exited ${exitCode}: $($Arguments -join ' ')`n$text"
    }
    try {
        $data = $text | ConvertFrom-Json -ErrorAction Stop
    }
    catch {
        throw "mi-lsp did not return JSON for '$($Arguments -join ' ')':`n$text"
    }
    if ($data.ok -eq $false) {
        throw "mi-lsp returned ok=false for '$($Arguments -join ' ')':`n$text"
    }
    return [pscustomobject]@{ Text = $text; Data = $data }
}

function Assert-Contains {
    param([string]$Value, [string]$Needle, [string]$Label)
    if (-not $Value.Contains($Needle)) {
        throw "$Label did not contain '$Needle'. Output:`n$Value"
    }
}

try {
    New-Item -ItemType Directory -Path $fixtureHome, $repo, $wiki -Force | Out-Null
    $env:HOME = $fixtureHome
    $env:USERPROFILE = $fixtureHome
    $env:MI_LSP_NO_AUTO_REGISTER = "1"

    if (Test-Path -LiteralPath (Join-Path $repo ".docs/wiki/00_governance.md")) {
        throw "Smoke fixture must start without a local legacy governance file"
    }
    New-Item -ItemType Directory -Path (Join-Path $repo ".sandinas") -Force | Out-Null
    @'
schema: sandinas-project/v2
project:
  name: setup-wiki-gate-smoke
  stack: [go]
wiki:
  setup_source: azure_wiki
'@ | Set-Content -LiteralPath (Join-Path $repo ".sandinas/project.yaml") -Encoding utf8
    @"
paths:
  workspace_root: '$runRoot'
  wiki_checkout: '$wiki'
"@ | Set-Content -LiteralPath (Join-Path $repo ".sandinas/project.local.yaml") -Encoding utf8

    @'
# External wiki governance

```yaml
wiki_contract: WIKI-CONTRACT-v1
version: 1
```
'@ | Set-Content -LiteralPath (Join-Path $wiki "00_governance.md") -Encoding utf8

    @'
---
wiki_contract: WIKI-CONTRACT-v1
id: EXT-GATE-SMOKE
kind: technical
status: active
title: External search marker
---

This document contains the searchable phrase external setup wiki gate marker.
'@ | Set-Content -LiteralPath (Join-Path $wiki "EXT-GATE-SMOKE.md") -Encoding utf8

    @'
# Smoke harness

```yaml
harness_protocol: SDD-HARNESS-v1
id: EXT-GATE-HARNESS
kind: support-doc
audience: llm-first
imports:
  - EXT-GATE-SOURCE.md
exports:
  - EXT-GATE-HARNESS
agent_must_read:
  - EXT-GATE-SOURCE.md
agent_may_edit:
  - EXT-GATE-HARNESS.md
agent_must_not_edit:
  - 00_governance.md
verify:
  - run the smoke script
stop_if:
  - governance_blocked=true
evidence:
  - EXT-GATE-HARNESS.md
```
'@ | Set-Content -LiteralPath (Join-Path $wiki "EXT-GATE-HARNESS.md") -Encoding utf8

    @'
# Smoke source

```yaml
source_protocol: SDD-WIKI-SOURCE-v1
harness_protocol: SDD-HARNESS-v1
doc_id: EXT-GATE-SOURCE
id: EXT-GATE-SOURCE
kind: support-doc
audience: llm-first
imports:
  - EXT-GATE-HARNESS.md
exports:
  - EXT-GATE-SOURCE
```

```toon
doc_id: EXT-GATE-SOURCE
block_id: EXT-GATE-SOURCE.smoke
kind: policy
source_of_truth: this
verify:
  - run the smoke script
evidence:
  - EXT-GATE-SOURCE.md
```
'@ | Set-Content -LiteralPath (Join-Path $wiki "EXT-GATE-SOURCE.md") -Encoding utf8

    foreach ($path in @($repo, $wiki)) {
        & git -C $path init --quiet
        if ($LASTEXITCODE -ne 0) { throw "git init failed in $path" }
        & git -C $path config user.name "mi-lsp smoke"
        & git -C $path config user.email "mi-lsp-smoke@example.invalid"
        & git -C $path add .
        & git -C $path commit --quiet -m "isolated Setup wiki gate smoke fixture"
        if ($LASTEXITCODE -ne 0) { throw "fixture commit failed in $path" }
    }

    Push-Location $repo
    try {
        $baseArgs = @("--no-daemon", "--no-auto-register", "--format", "json")
        Invoke-MiLsp ($baseArgs + @("workspace", "add", $repo, "--name", $alias, "--no-index")) | Out-Null

        $root = Invoke-MiLsp ($baseArgs + @("--workspace", $alias, "nav", "wiki", "root"))
        Assert-Contains $root.Text "external-wiki" "configured wiki root"
        Assert-Contains $root.Text "00_governance.md" "configured governance path"

        $governance = Invoke-MiLsp ($baseArgs + @("--workspace", $alias, "nav", "governance"))
        $governanceStatus = @($governance.Data.items)[0]
        if ($governanceStatus.state -ne "resolved" -or $governanceStatus.blocked -ne $false) {
            throw "External Setup governance did not resolve cleanly: $($governance.Text)"
        }
        $index = Invoke-MiLsp ($baseArgs + @("--workspace", $alias, "index", $repo, "--docs-only"))

        $searchByID = Invoke-MiLsp ($baseArgs + @("--workspace", $alias, "nav", "wiki", "search", "EXT-GATE-SMOKE", "--include-content"))
        Assert-Contains $searchByID.Text "EXT-GATE-SMOKE" "search by document ID"
        Assert-Contains $searchByID.Text "external-wiki" "search result source path"

        $searchByText = Invoke-MiLsp ($baseArgs + @("--workspace", $alias, "nav", "wiki", "search", "external setup wiki gate marker", "--include-content"))
        Assert-Contains $searchByText.Text "external setup wiki gate marker" "search by indexed text"

        $harness = Invoke-MiLsp ($baseArgs + @("--workspace", $alias, "nav", "wiki", "validate-harness", "--paths", "EXT-GATE-HARNESS.md"))
        $harnessItem = @($harness.Data.items)[0]
        if ($harnessItem.harness_verdict -ne "PASS") {
            throw "External-root harness validation did not PASS: $($harness.Text)"
        }
        Assert-Contains $harness.Text "EXT-GATE-HARNESS" "external-root harness validation"

        $source = Invoke-MiLsp ($baseArgs + @("--workspace", $alias, "nav", "wiki", "validate-source", "--paths", "EXT-GATE-SOURCE.md"))
        $sourceItem = @($source.Data.items)[0]
        if ($sourceItem.wiki_source_verdict -ne "PASS") {
            throw "External-root source validation did not PASS: $($source.Text)"
        }
        Assert-Contains $source.Text "EXT-GATE-SOURCE" "external-root source validation"

        if (Test-Path -LiteralPath (Join-Path $repo ".docs/wiki/00_governance.md")) {
            throw "Smoke fixture unexpectedly contains a local legacy governance file"
        }
    }
    finally {
        Pop-Location
    }

    Write-Output "PASS: Setup external governance gate; docs-only index, search by ID/text, and validators all used the configured wiki root."
}
finally {
    $env:HOME = $oldHome
    $env:USERPROFILE = $oldUserProfile
    $env:MI_LSP_NO_AUTO_REGISTER = $oldNoAutoRegister
    if (Test-Path -LiteralPath $runRoot) {
        Remove-Item -LiteralPath $runRoot -Recurse -Force
    }
}
