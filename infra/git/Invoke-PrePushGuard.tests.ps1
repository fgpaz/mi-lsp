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
    param([switch]$Solo, [switch]$Text)
    $env:AE_SKILL_SOURCE_ROOT = $sourceRoot
    $env:AE_SKILL_INSTALLED_ROOT = $installedRoot
    $env:AE_SKILL_MIRROR_ROOT = $mirrorRoot
    $arguments = @("-DryRun")
    if (-not $Text) {
        $arguments += "-Json"
    }
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
    $readmeFixtureContent = "README rename source: unique fixture content 41f2e9"
    $skillFixtureContent = "Skill rename source: unique fixture content 8c7a31"
    Set-Content -LiteralPath (Join-Path $repo "README.md") -Value $readmeFixtureContent -NoNewline -Encoding UTF8
    New-Item -ItemType Directory -Path (Join-Path $repo "skills\fixture") | Out-Null
    Set-Content -LiteralPath (Join-Path $repo "skills\fixture\SKILL.md") -Value $skillFixtureContent -NoNewline -Encoding UTF8
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

        function Invoke-GuardWithScope([string[]]$Scope) {
            $env:AE_SKILL_SOURCE_ROOT = $sourceRoot
            $env:AE_SKILL_INSTALLED_ROOT = $installedRoot
            $env:AE_SKILL_MIRROR_ROOT = $mirrorRoot
            $arguments = @("-IssueNumber", "1", "-ExpectedScope") + $Scope + @("-TraceabilityEvidence", "README.md", "-DryRun", "-Json")
            $output = @(& $pwsh -NoProfile -File $guard @arguments 2>&1)
            [pscustomobject]@{
                ExitCode = [int]$LASTEXITCODE
                Output = $output -join [Environment]::NewLine
            }
        }

        function Reset-FixtureBranch([string]$Name) {
            Invoke-GitChecked $repo @("checkout", "-f", "-B", $Name, $initialHead)
            Invoke-GitChecked $repo @("clean", "-fd")
            Invoke-GitChecked $repo @("restore", "--worktree", ".")
        }

        $guardSource = Get-Content -LiteralPath $guard -Raw
        foreach ($contractText in @(
            'merge-base --is-ancestor $baseSha $headSha',
            'Get-SecretScan -Range $snapshotRange',
            'HEAD changed during guard; captured snapshot cannot be approved',
            '$snapshotStable = $snapshotAvailable -and $headAtApprovalResult.ExitCode -eq 0 -and',
            'base_sha = $baseSha',
            'head_sha = $headSha',
            'Convert-StatusPorcelainZ $pendingStatus.OutputText',
            'Convert-NameStatusZ $outgoingDiff.OutputText',
            'Test-BranchIdentityStable',
            'governed mode requires an attached symbolic branch',
            'branch_identity_stable = $branchIdentityStable'
        )) {
            if (-not $guardSource.Contains($contractText)) {
                throw "guard snapshot/NUL inventory contract omitted: $contractText"
            }
        }
        if ($guardSource.Contains("origin/main..HEAD") -or $guardSource.Contains("origin/main...HEAD")) {
            throw "guard still uses a live symbolic ref range after snapshot capture"
        }

        $branchIdentityFunction = [regex]::Match($guardSource, '(?s)function Test-BranchIdentityStable \{.*?\n\}').Value
        if (-not $branchIdentityFunction) {
            throw "guard branch identity comparator is missing"
        }
        Invoke-Expression $branchIdentityFunction
        $capturedBranchRef = (& git -C $repo symbolic-ref --quiet HEAD).Trim()
        $capturedBranchSha = (& git -C $repo rev-parse --verify 'HEAD^{commit}').Trim()
        Invoke-GitChecked $repo @("checkout", "-B", "same-sha-new-branch", $capturedBranchSha)
        $currentBranchRef = (& git -C $repo symbolic-ref --quiet HEAD).Trim()
        $currentBranchSha = (& git -C $repo rev-parse --verify 'HEAD^{commit}').Trim()
        if ($capturedBranchSha -ne $currentBranchSha -or $capturedBranchRef -eq $currentBranchRef -or
            (Test-BranchIdentityStable 0 $capturedBranchRef 0 $currentBranchRef)) {
            throw "branch identity comparator accepted a branch switch at the same SHA"
        }
        if (-not (Test-BranchIdentityStable 1 "" 1 "")) {
            throw "stable detached identity comparison should be representable for non-governed policy"
        }
        if (Test-BranchIdentityStable 128 "" 128 "") {
            throw "branch identity query failure was treated as a valid stable identity"
        }
        $branchIdentitySameSha = "PASS"

        Reset-FixtureBranch "guard-governed-detached"
        Invoke-GitChecked $repo @("checkout", "--detach", $initialHead)
        $governedDetached = Invoke-GuardWithScope @("backend")
        $governedDetachedReport = $governedDetached.Output | ConvertFrom-Json
        if ($governedDetached.ExitCode -eq 0 -or
            ($governedDetachedReport.blockers -join " ") -notmatch "governed mode requires an attached symbolic branch") {
            throw "governed detached HEAD was not explicitly blocked"
        }
        $governedDetachedBlock = "PASS"

        $defaultSnapshotReport = $governedDefault.Output | ConvertFrom-Json
        if ($defaultSnapshotReport.base_sha -ne $initialHead -or
            $defaultSnapshotReport.head_sha -ne $initialHead -or
            $defaultSnapshotReport.snapshot_stable -ne $true -or
            $defaultSnapshotReport.branch_ref -ne "refs/heads/main" -or
            $defaultSnapshotReport.branch_identity_stable -ne $true) {
            throw "guard report did not retain the full stable branch/commit snapshot"
        }
        Reset-FixtureBranch "main"
        $governedText = Invoke-GuardBare -Text
        if ($governedText.Output -notmatch "branch_ref: refs/heads/main" -or
            $governedText.Output -notmatch "branch_identity_stable: True" -or
            $governedText.Output -notmatch "snapshot_stable: True" -or
            $governedText.Output -notmatch "head_sha: $initialHead" -or
            $governedText.Output -notmatch "base_sha: $initialHead") {
            throw "human-readable report omitted the minimal branch/commit snapshot"
        }

        Reset-FixtureBranch "guard-clean-outgoing-out-of-scope"
        Invoke-GitChecked $repo @("restore", "--worktree", "skills/fixture/SKILL.md")
        New-Item -ItemType Directory -Path (Join-Path $repo "internal") | Out-Null
        Set-Content -LiteralPath (Join-Path $repo "internal\outside-scope.cs") -Value "class OutsideScope {}" -NoNewline -Encoding UTF8
        Invoke-GitChecked $repo @("add", "internal/outside-scope.cs")
        Invoke-GitChecked $repo @("commit", "-m", "clean outgoing path outside scope")
        $outOfScope = Invoke-GuardWithScope @("canon-docs")
        $outOfScopeReport = $outOfScope.Output | ConvertFrom-Json
        if ($outOfScope.ExitCode -eq 0 -or
            @($outOfScopeReport.changed_paths | Where-Object { $_.Path -eq "internal/outside-scope.cs" }).Count -ne 1 -or
            $outOfScopeReport.detected_surfaces -notcontains "backend" -or
            ($outOfScopeReport.blockers -join " ") -notmatch "backend.*ExpectedScope") {
            throw "clean outgoing out-of-scope path was omitted or not blocked: $($outOfScope.Output)"
        }

        Reset-FixtureBranch "guard-clean-outgoing-covered"
        New-Item -ItemType Directory -Path (Join-Path $repo "internal") -Force | Out-Null
        Set-Content -LiteralPath (Join-Path $repo "internal\covered.cs") -Value "class Covered {}" -NoNewline -Encoding UTF8
        Invoke-GitChecked $repo @("add", "internal/covered.cs")
        Invoke-GitChecked $repo @("commit", "-m", "clean outgoing covered path")
        $covered = Invoke-GuardWithScope @("backend")
        $coveredReport = $covered.Output | ConvertFrom-Json
        if ($covered.ExitCode -ne 0 -or
            @($coveredReport.changed_paths | Where-Object { $_.Path -eq "internal/covered.cs" }).Count -ne 1 -or
            $coveredReport.detected_surfaces -notcontains "backend" -or
            @($coveredReport.pending_changes.staged).Count -ne 0 -or
            @($coveredReport.pending_changes.unstaged).Count -ne 0 -or
            @($coveredReport.pending_changes.untracked).Count -ne 0) {
            throw "clean outgoing covered path or empty pending inventory was wrong: $($covered.Output)"
        }

        Reset-FixtureBranch "guard-no-diff"
        $noDiff = Invoke-GuardWithScope @("backend")
        $noDiffReport = $noDiff.Output | ConvertFrom-Json
        if ($noDiff.ExitCode -ne 0 -or
            @($noDiffReport.changed_paths).Count -ne 0 -or
            @($noDiffReport.pending_changes.staged).Count -ne 0 -or
            @($noDiffReport.pending_changes.unstaged).Count -ne 0 -or
            @($noDiffReport.pending_changes.untracked).Count -ne 0) {
            throw "no-diff checkout reported omitted or invented paths: $($noDiff.Output)"
        }

        Reset-FixtureBranch "guard-dirty-mixed"
        New-Item -ItemType Directory -Path (Join-Path $repo "internal") -Force | Out-Null
        Set-Content -LiteralPath (Join-Path $repo "internal\old name.txt") -Value "base" -NoNewline -Encoding UTF8
        Invoke-GitChecked $repo @("add", "internal/old name.txt")
        Invoke-GitChecked $repo @("commit", "-m", "add rename fixture")
        Invoke-GitChecked $repo @("mv", "internal/old name.txt", "internal/new name.txt")
        Set-Content -LiteralPath (Join-Path $repo "internal\new name.txt") -Value "renamed and modified" -NoNewline -Encoding UTF8
        Invoke-GitChecked $repo @("rm", "README.md")
        Set-Content -LiteralPath (Join-Path $repo "skills\fixture\SKILL.md") -Value "unstaged outside scope" -NoNewline -Encoding UTF8
        Set-Content -LiteralPath (Join-Path $repo "internal\untracked name.txt") -Value "untracked covered" -NoNewline -Encoding UTF8
        $dirtyMixed = Invoke-GuardWithScope @("backend")
        $dirtyMixedReport = $dirtyMixed.Output | ConvertFrom-Json
        $stagedText = $dirtyMixedReport.pending_changes.staged | ConvertTo-Json -Depth 8 -Compress
        $unstagedText = $dirtyMixedReport.pending_changes.unstaged | ConvertTo-Json -Depth 8 -Compress
        $untrackedText = $dirtyMixedReport.pending_changes.untracked | ConvertTo-Json -Depth 8 -Compress
        if ($dirtyMixed.ExitCode -eq 0 -or
            @($dirtyMixedReport.pending_changes.staged).Count -eq 0 -or
            @($dirtyMixedReport.pending_changes.unstaged).Count -eq 0 -or
            @($dirtyMixedReport.pending_changes.untracked).Count -eq 0 -or
            $stagedText -notmatch "new name\.txt" -or
            $stagedText -notmatch "README\.md" -or
            $unstagedText -notmatch "new name\.txt|SKILL\.md" -or
            $untrackedText -notmatch "untracked name\.txt" -or
            ($dirtyMixedReport.blockers -join " ") -notmatch "canon-docs|shared-skill") {
            throw "dirty mixed paths were omitted, merged, or not scope-blocked: $($dirtyMixed.Output)"
        }

        Reset-FixtureBranch "guard-clean-unknown"
        Set-Content -LiteralPath (Join-Path $repo "unknown-root.txt") -Value "unknown surface" -NoNewline -Encoding UTF8
        Invoke-GitChecked $repo @("add", "unknown-root.txt")
        Invoke-GitChecked $repo @("commit", "-m", "clean outgoing unknown surface")
        $unknown = Invoke-GuardWithScope @("backend")
        $unknownReport = $unknown.Output | ConvertFrom-Json
        if ($unknown.ExitCode -eq 0 -or
            $unknownReport.detected_surfaces -notcontains "unknown" -or
            ($unknownReport.blockers -join " ") -notmatch "unknown") {
            throw "clean outgoing unknown surface did not fail closed: $($unknown.Output)"
        }

        Reset-FixtureBranch "guard-outgoing-renames-both-boundaries"
        New-Item -ItemType Directory -Path (Join-Path $repo "internal"), (Join-Path $repo ".docs\wiki") -Force | Out-Null
        Invoke-GitChecked $repo @("mv", "README.md", "internal/readme-moved.cs")
        Invoke-GitChecked $repo @("mv", "skills/fixture/SKILL.md", ".docs/wiki/skill-moved.md")
        Invoke-GitChecked $repo @("commit", "-m", "outgoing renames cross both surface boundaries")
        $renames = Invoke-GuardWithScope @("backend")
        $renamesReport = $renames.Output | ConvertFrom-Json
        $readmeRename = @($renamesReport.changed_paths | Where-Object {
            $_.Path -eq "internal/readme-moved.cs" -and $_.PreviousPath -eq "README.md"
        })
        $skillRename = @($renamesReport.changed_paths | Where-Object {
            $_.Path -eq ".docs/wiki/skill-moved.md" -and $_.PreviousPath -eq "skills/fixture/SKILL.md"
        })
        if ($renames.ExitCode -eq 0 -or $readmeRename.Count -ne 1 -or $skillRename.Count -ne 1 -or
            $renamesReport.detected_surfaces -notcontains "canon-docs" -or
            $renamesReport.detected_surfaces -notcontains "shared-skill" -or
            $renamesReport.detected_surfaces -notcontains "backend" -or
            ($renamesReport.blockers -join " ") -notmatch "canon-docs|shared-skill") {
            throw "outgoing renames did not preserve and scope-check both endpoints: $($renames.Output)"
        }

        Reset-FixtureBranch "guard-outgoing-deletion"
        Invoke-GitChecked $repo @("rm", "README.md")
        Invoke-GitChecked $repo @("commit", "-m", "outgoing deletion")
        $deletion = Invoke-GuardWithScope @("backend")
        $deletionReport = $deletion.Output | ConvertFrom-Json
        if ($deletion.ExitCode -eq 0 -or
            @($deletionReport.changed_paths | Where-Object { $_.Path -eq "README.md" -and $_.Status -eq "D" }).Count -ne 1 -or
            $deletionReport.detected_surfaces -notcontains "canon-docs" -or
            ($deletionReport.blockers -join " ") -notmatch "canon-docs") {
            throw "outgoing deletion was omitted or not scope-checked: $($deletion.Output)"
        }

        Reset-FixtureBranch "guard-pending-rename-canon-to-backend"
        New-Item -ItemType Directory -Path (Join-Path $repo "internal") -Force | Out-Null
        Invoke-GitChecked $repo @("mv", "README.md", "internal/readme-moved.cs")
        $pendingReadmeRename = Invoke-GuardWithScope @("backend")
        $pendingReadmeReport = $pendingReadmeRename.Output | ConvertFrom-Json
        if ($pendingReadmeRename.ExitCode -eq 0 -or
            @($pendingReadmeReport.pending_changes.staged | Where-Object {
                $_.Path -eq "internal/readme-moved.cs" -and $_.PreviousPath -eq "README.md"
            }).Count -ne 1 -or
            $pendingReadmeReport.detected_surfaces -notcontains "canon-docs" -or
            $pendingReadmeReport.detected_surfaces -notcontains "backend") {
            throw "pending canon-to-backend rename omitted an endpoint or escaped scope"
        }

        Reset-FixtureBranch "guard-pending-rename-skill-to-canon"
        New-Item -ItemType Directory -Path (Join-Path $repo ".docs\wiki") -Force | Out-Null
        Invoke-GitChecked $repo @("mv", "skills/fixture/SKILL.md", ".docs/wiki/skill-moved.md")
        $pendingSkillRename = Invoke-GuardWithScope @("canon-docs")
        $pendingSkillReport = $pendingSkillRename.Output | ConvertFrom-Json
        if ($pendingSkillRename.ExitCode -eq 0 -or
            @($pendingSkillReport.pending_changes.staged | Where-Object {
                $_.Path -eq ".docs/wiki/skill-moved.md" -and $_.PreviousPath -eq "skills/fixture/SKILL.md"
            }).Count -ne 1 -or
            $pendingSkillReport.detected_surfaces -notcontains "shared-skill" -or
            $pendingSkillReport.detected_surfaces -notcontains "canon-docs") {
            throw "pending shared-skill-to-canon rename omitted an endpoint or escaped scope"
        }

        Reset-FixtureBranch "guard-outgoing-copy"
        New-Item -ItemType Directory -Path (Join-Path $repo "internal") | Out-Null
        Set-Content -LiteralPath (Join-Path $repo "internal\copied-from-readme.cs") -Value $readmeFixtureContent -NoNewline -Encoding UTF8
        Invoke-GitChecked $repo @("add", "--", "internal/copied-from-readme.cs")
        Invoke-GitChecked $repo @("commit", "-m", "outgoing copy from readme")
        $copy = Invoke-GuardWithScope @("backend")
        $copyReport = $copy.Output | ConvertFrom-Json
        $copyEntries = @($copyReport.changed_paths | Where-Object {
            $_.Status -match '^C' -and $_.Path -eq "internal/copied-from-readme.cs"
        })
        $copySourceSurface = if ($copyEntries.Count -eq 1 -and $copyEntries[0].PreviousPath -eq "README.md") {
            "canon-docs"
        } else {
            "shared-skill"
        }
        if ($copy.ExitCode -eq 0 -or $copyEntries.Count -ne 1 -or
            $copyEntries[0].PreviousPath -notin @("README.md", "skills/fixture/SKILL.md") -or
            $copyReport.detected_surfaces -notcontains $copySourceSurface -or
            ($copyReport.blockers -join " ") -notmatch "canon-docs|shared-skill") {
            throw "outgoing copy did not consume and validate both NUL paths: $($copy.Output)"
        }

        Reset-FixtureBranch "guard-arrow-unicode-path"
        New-Item -ItemType Directory -Path (Join-Path $repo "internal") | Out-Null
        $literalArrowPath = "internal/literal -> ü path.txt"
        Set-Content -LiteralPath (Join-Path $repo $literalArrowPath) -Value "literal path" -NoNewline -Encoding UTF8
        $literalPending = Invoke-GuardWithScope @("backend")
        $literalPendingReport = $literalPending.Output | ConvertFrom-Json
        if ($literalPending.ExitCode -ne 0 -or
            @($literalPendingReport.pending_changes.untracked | Where-Object { $_.Path -ceq $literalArrowPath }).Count -ne 1) {
            throw "pending filename containing literal arrow or Unicode was reinterpreted: $($literalPending.Output)"
        }
        Invoke-GitChecked $repo @("add", "--", $literalArrowPath)
        Invoke-GitChecked $repo @("commit", "-m", "outgoing literal arrow unicode path")
        $literalOutgoing = Invoke-GuardWithScope @("backend")
        $literalOutgoingReport = $literalOutgoing.Output | ConvertFrom-Json
        if ($literalOutgoing.ExitCode -ne 0 -or
            @($literalOutgoingReport.changed_paths | Where-Object { $_.Path -ceq $literalArrowPath }).Count -ne 1) {
            throw "outgoing filename containing literal arrow or Unicode was not preserved: $($literalOutgoing.Output)"
        }

        $nulControlPaths = "SKIPPED_PLATFORM"
        if ([IO.Path]::DirectorySeparatorChar -eq '/') {
            Reset-FixtureBranch "guard-tab-newline-paths"
            New-Item -ItemType Directory -Path (Join-Path $repo "internal") | Out-Null
            $controlPath = "internal/tab" + [char]9 + "line" + [char]10 + " -> ñ.txt"
            Set-Content -LiteralPath (Join-Path $repo $controlPath) -Value "control path" -NoNewline -Encoding UTF8
            $controlPending = Invoke-GuardWithScope @("backend")
            $controlPendingReport = $controlPending.Output | ConvertFrom-Json
            if ($controlPending.ExitCode -ne 0 -or
                @($controlPendingReport.pending_changes.untracked | Where-Object { $_.Path -ceq $controlPath }).Count -ne 1) {
                throw "pending NUL parser did not preserve tab/newline/Unicode filename"
            }
            Invoke-GitChecked $repo @("add", "--", $controlPath)
            Invoke-GitChecked $repo @("commit", "-m", "outgoing control character path")
            $controlOutgoing = Invoke-GuardWithScope @("backend")
            $controlOutgoingReport = $controlOutgoing.Output | ConvertFrom-Json
            if ($controlOutgoing.ExitCode -ne 0 -or
                @($controlOutgoingReport.changed_paths | Where-Object { $_.Path -ceq $controlPath }).Count -ne 1) {
                throw "outgoing NUL parser did not preserve tab/newline/Unicode filename"
            }
            $nulControlPaths = "PASS"
        }

        $outgoingScope = "PASS"
        $coveredNoDiff = "PASS"
        $dirtyMixedResult = "PASS"
        $unknownSurface = "PASS"
        $renameBothBoundaries = "PASS"
        $deletionScope = "PASS"
        $pendingRenameBothPaths = "PASS"
        $copyBothPaths = "PASS"
        $literalArrowUnicode = "PASS"
        $snapshotContract = "PASS"

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
        clean_outgoing_out_of_scope = $outgoingScope
        clean_outgoing_covered_and_empty_pending = $coveredNoDiff
        dirty_mixed_pending_changes = $dirtyMixedResult
        clean_unknown_surface_block = $unknownSurface
        outgoing_rename_both_boundaries = $renameBothBoundaries
        outgoing_deletion_scope = $deletionScope
        pending_rename_both_endpoints = $pendingRenameBothPaths
        outgoing_copy_both_endpoints = $copyBothPaths
        literal_arrow_unicode_paths = $literalArrowUnicode
        tab_newline_unicode_paths = $nulControlPaths
        snapshot_pair_contract = $snapshotContract
        branch_identity_same_sha = $branchIdentitySameSha
        governed_detached_head_block = $governedDetachedBlock
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -LiteralPath $fixture -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item Env:AE_SKILL_SOURCE_ROOT, Env:AE_SKILL_INSTALLED_ROOT, Env:AE_SKILL_MIRROR_ROOT -ErrorAction SilentlyContinue
}
