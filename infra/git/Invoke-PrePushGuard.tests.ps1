$ErrorActionPreference = "Stop"

$guard = Join-Path $PSScriptRoot "Invoke-PrePushGuard.ps1"
$pwsh = (Get-Command pwsh).Source
$fixture = Join-Path ([IO.Path]::GetTempPath()) ("mi-lsp-pre-push-" + [Guid]::NewGuid().ToString("N"))
$remote = Join-Path $fixture "remote.git"
$repo = Join-Path $fixture "repo"
$sourceRoot = Join-Path $fixture "source"
$installedRoot = Join-Path $fixture "installed"
$mirrorRoot = Join-Path $fixture "mirror"

function Invoke-GitChecked {
    param([string]$WorkingDirectory, [string[]]$Arguments)
    & git -C $WorkingDirectory @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "git command failed: git -C $WorkingDirectory $($Arguments -join ' ')"
    }
}

function Invoke-Guard {
    param([string]$MirrorRootValue)
    $env:AE_SKILL_SOURCE_ROOT = $sourceRoot
    $env:AE_SKILL_INSTALLED_ROOT = $installedRoot
    $env:AE_SKILL_MIRROR_ROOT = $MirrorRootValue
    $output = @(& $pwsh -NoProfile -File $guard -IssueNumber 1 -ExpectedScope shared-skill -TraceabilityEvidence README.md -SharedSkillName fixture -DryRun -Json 2>&1)
    [pscustomobject]@{
        ExitCode = [int]$LASTEXITCODE
        Output = $output -join [Environment]::NewLine
    }
}

try {
    New-Item -ItemType Directory -Path $fixture, $sourceRoot, $installedRoot, $mirrorRoot | Out-Null
    Invoke-GitChecked $fixture @("init", "--bare", $remote)
    Invoke-GitChecked $fixture @("init", $repo)
    Invoke-GitChecked $repo @("config", "user.email", "test@example.invalid")
    Invoke-GitChecked $repo @("config", "user.name", "PrePushGuard Test")
    Set-Content -LiteralPath (Join-Path $repo "README.md") -Value "fixture" -NoNewline
    New-Item -ItemType Directory -Path (Join-Path $repo "skills\fixture") | Out-Null
    Set-Content -LiteralPath (Join-Path $repo "skills\fixture\SKILL.md") -Value "fixture" -NoNewline
    Invoke-GitChecked $repo @("add", ".")
    Invoke-GitChecked $repo @("commit", "-m", "fixture")
    Invoke-GitChecked $repo @("branch", "-M", "main")
    Invoke-GitChecked $repo @("remote", "add", "origin", $remote)
    Invoke-GitChecked $repo @("push", "-u", "origin", "main")
    Invoke-GitChecked $repo @("checkout", "-b", "fixture-change")
    Set-Content -LiteralPath (Join-Path $repo "skills\fixture\SKILL.md") -Value "changed" -NoNewline

    foreach ($root in @($sourceRoot, $installedRoot, $mirrorRoot)) {
        New-Item -ItemType Directory -Path (Join-Path $root "fixture") | Out-Null
        Set-Content -LiteralPath (Join-Path $root "fixture\SKILL.md") -Value "same" -NoNewline
    }

    Push-Location $repo
    try {
        $positive = Invoke-Guard $mirrorRoot
        if ($positive.ExitCode -ne 0) {
            throw "explicit-root parity case failed: $($positive.Output)"
        }

        $missingRoot = Join-Path $fixture "missing-mirror"
        $negative = Invoke-Guard $missingRoot
        if ($negative.ExitCode -eq 0 -or $negative.Output -notmatch "AE_SKILL_MIRROR_ROOT.*not found") {
            throw "missing explicit root case did not fail closed: $($negative.Output)"
        }
    }
    finally {
        Pop-Location
    }

    [pscustomobject]@{
        explicit_roots = "PASS"
        missing_explicit_root = "PASS"
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -LiteralPath $fixture -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item Env:AE_SKILL_SOURCE_ROOT, Env:AE_SKILL_INSTALLED_ROOT, Env:AE_SKILL_MIRROR_ROOT -ErrorAction SilentlyContinue
}
