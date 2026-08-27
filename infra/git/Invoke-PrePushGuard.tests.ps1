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

function Invoke-GuardBare {
    param([switch]$Solo)
    $env:AE_SKILL_SOURCE_ROOT = $sourceRoot
    $env:AE_SKILL_INSTALLED_ROOT = $installedRoot
    $env:AE_SKILL_MIRROR_ROOT = $mirrorRoot
    $arguments = @("-DryRun", "-Json")
    if ($Solo) {
        $arguments += @("-CollaborationMode", "solo")
    }
    $output = @(& $pwsh -NoProfile -File $guard @arguments 2>&1)
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
    Set-Content -LiteralPath (Join-Path $repo "README.md") -Value "fixture" -NoNewline -Encoding UTF8
    New-Item -ItemType Directory -Path (Join-Path $repo "skills\fixture") | Out-Null
    Set-Content -LiteralPath (Join-Path $repo "skills\fixture\SKILL.md") -Value "fixture" -NoNewline -Encoding UTF8
    Invoke-GitChecked $repo @("add", ".")
    Invoke-GitChecked $repo @("commit", "-m", "fixture")
    $initialHead = (& git -C $repo rev-parse HEAD).Trim()
    Invoke-GitChecked $repo @("branch", "-M", "main")
    Invoke-GitChecked $repo @("remote", "add", "origin", $remote)
    Invoke-GitChecked $repo @("push", "-u", "origin", "main")

    Push-Location $repo
    try {
        $governedDefault = Invoke-GuardBare
        if ($governedDefault.ExitCode -eq 0 -or
            $governedDefault.Output -notmatch '"collaboration_mode":\s+"governed"' -or
            $governedDefault.Output -notmatch "direct push from local main" -or
            $governedDefault.Output -notmatch "IssueNumber, IssueUrl, or WaiverReason is required" -or
            $governedDefault.Output -notmatch "ExpectedScope is required" -or
            $governedDefault.Output -notmatch "TraceabilityEvidence is required") {
            throw "governed default did not retain direct-main and governed-input blockers"
        }

        Set-Content -LiteralPath (Join-Path $repo "README.md") -Value "fixture`nsafe ahead change" -NoNewline -Encoding UTF8
        Invoke-GitChecked $repo @("add", "README.md")
        Invoke-GitChecked $repo @("commit", "-m", "safe ahead change")
        $soloPositive = Invoke-GuardBare -Solo
        $soloPositiveReport = $soloPositive.Output | ConvertFrom-Json
        if ($soloPositive.ExitCode -ne 0 -or
            $soloPositiveReport.verdict -ne "Approved" -or
            $soloPositiveReport.collaboration_mode -ne "solo" -or
            $soloPositiveReport.ahead_count -ne 1 -or
            $soloPositiveReport.secrets_scan.status -ne "passed") {
            throw "solo local-main safe-ahead case did not pass"
        }

        $secretMarker = "ghp_" + ("Ab3dEf5gH" * 4)
        Set-Content -LiteralPath (Join-Path $repo "secret-marker.txt") -Value "high-confidence marker: $secretMarker" -NoNewline -Encoding UTF8
        Invoke-GitChecked $repo @("add", "secret-marker.txt")
        Invoke-GitChecked $repo @("commit", "-m", "secret marker")
        $soloSecret = Invoke-GuardBare -Solo
        if ($soloSecret.ExitCode -eq 0 -or $soloSecret.Output -match [regex]::Escape($secretMarker)) {
            throw "solo secret case did not block without leaking the marker"
        }
        $soloSecretReport = $soloSecret.Output | ConvertFrom-Json
        if ($soloSecretReport.verdict -ne "Blocked" -or
            $soloSecretReport.secrets_scan.status -ne "blocked" -or
            $soloSecretReport.secrets_scan.rule_counts.github_classic_token -lt 1) {
            throw "solo secret case did not report a sanitized blocked rule count"
        }

        Invoke-GitChecked $repo @("checkout", "-B", "fixture-change", $initialHead)
        Set-Content -LiteralPath (Join-Path $repo "skills\fixture\SKILL.md") -Value "changed" -NoNewline -Encoding UTF8

        foreach ($root in @($sourceRoot, $installedRoot, $mirrorRoot)) {
            New-Item -ItemType Directory -Path (Join-Path $root "fixture") | Out-Null
            Set-Content -LiteralPath (Join-Path $root "fixture\SKILL.md") -Value "same" -NoNewline -Encoding UTF8
        }

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
        governed_default = "PASS"
        solo_local_main = "PASS"
        solo_secret_block = "PASS"
        explicit_roots = "PASS"
        missing_explicit_root = "PASS"
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -LiteralPath $fixture -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item Env:AE_SKILL_SOURCE_ROOT, Env:AE_SKILL_INSTALLED_ROOT, Env:AE_SKILL_MIRROR_ROOT -ErrorAction SilentlyContinue
}
