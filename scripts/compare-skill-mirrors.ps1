[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [string]$MirrorRoot = 'C:\repos\buho\assets\skills',
    [string]$AgentsRoot = 'C:\Users\fgpaz\.agents\skills',
    [ValidateSet('Compare', 'SyncAgentsToMirror', 'SyncMirrorToAgents')]
    [string]$Mode = 'Compare',
    [string[]]$Skill,
    [string[]]$ExcludeSkillPattern = @(),
    [string[]]$IgnorePattern = @('*.bak*'),
    [switch]$IncludeIdentical,
    [switch]$AsJson
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Test-IgnoredPath {
    param(
        [Parameter(Mandatory = $true)][string]$RelativePath,
        [Parameter(Mandatory = $true)][string[]]$Patterns
    )

    foreach ($pattern in $Patterns) {
        if ($RelativePath -like $pattern) {
            return $true
        }
    }
    return $false
}

function Test-ExcludedSkill {
    param(
        [Parameter(Mandatory = $true)][string]$SkillName,
        [Parameter(Mandatory = $true)][string[]]$Patterns
    )

    foreach ($pattern in $Patterns) {
        if ($SkillName -like $pattern) {
            return $true
        }
    }
    return $false
}

function Get-RelativeFiles {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string[]]$IgnorePatterns
    )

    if (-not (Test-Path -LiteralPath $Root)) {
        return @()
    }

    $trimChars = [char[]]'\/'
    return Get-ChildItem -LiteralPath $Root -Recurse -File | ForEach-Object {
        $relativePath = $_.FullName.Substring($Root.Length).TrimStart($trimChars)
        if (Test-IgnoredPath -RelativePath $relativePath -Patterns $IgnorePatterns) {
            return
        }

        [pscustomobject]@{
            RelativePath = $relativePath
            FullPath = $_.FullName
            Hash = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash
        }
    }
}

function Get-MapByRelativePath {
    param($Files)

    $map = @{}
    if ($null -eq $Files) {
        return $map
    }
    foreach ($file in $Files) {
        $map[$file.RelativePath] = $file
    }
    return $map
}

function Compare-SkillDirectory {
    param(
        [Parameter(Mandatory = $true)][string]$SkillName,
        [Parameter(Mandatory = $true)][string]$MirrorSkillRoot,
        [Parameter(Mandatory = $true)][string]$AgentsSkillRoot,
        [Parameter(Mandatory = $true)][string[]]$IgnorePatterns
    )

    $mirrorExists = Test-Path -LiteralPath $MirrorSkillRoot
    $agentsExists = Test-Path -LiteralPath $AgentsSkillRoot

    if (-not $mirrorExists -and -not $agentsExists) {
        return [pscustomobject]@{
            Skill = $SkillName
            Status = 'missing_both'
            DiffCount = 0
            MirrorOnly = @()
            AgentsOnly = @()
            ContentDiff = @()
            Sample = ''
        }
    }

    if (-not $mirrorExists) {
        return [pscustomobject]@{
            Skill = $SkillName
            Status = 'missing_in_mirror'
            DiffCount = -1
            MirrorOnly = @()
            AgentsOnly = @('skill directory missing in mirror')
            ContentDiff = @()
            Sample = 'skill directory missing in mirror'
        }
    }

    if (-not $agentsExists) {
        return [pscustomobject]@{
            Skill = $SkillName
            Status = 'missing_in_agents'
            DiffCount = -1
            MirrorOnly = @('skill directory missing in agents')
            AgentsOnly = @()
            ContentDiff = @()
            Sample = 'skill directory missing in agents'
        }
    }

    $mirrorFiles = @(Get-RelativeFiles -Root $MirrorSkillRoot -IgnorePatterns $IgnorePatterns)
    $agentsFiles = @(Get-RelativeFiles -Root $AgentsSkillRoot -IgnorePatterns $IgnorePatterns)
    $mirrorMap = Get-MapByRelativePath -Files $mirrorFiles
    $agentsMap = Get-MapByRelativePath -Files $agentsFiles

    $allPaths = @($mirrorMap.Keys + $agentsMap.Keys | Sort-Object -Unique)
    $mirrorOnly = New-Object System.Collections.Generic.List[string]
    $agentsOnly = New-Object System.Collections.Generic.List[string]
    $contentDiff = New-Object System.Collections.Generic.List[string]

    foreach ($relativePath in $allPaths) {
        $inMirror = $mirrorMap.ContainsKey($relativePath)
        $inAgents = $agentsMap.ContainsKey($relativePath)

        if (-not $inMirror) {
            $agentsOnly.Add($relativePath)
            continue
        }
        if (-not $inAgents) {
            $mirrorOnly.Add($relativePath)
            continue
        }
        if ($mirrorMap[$relativePath].Hash -ne $agentsMap[$relativePath].Hash) {
            $contentDiff.Add($relativePath)
        }
    }

    $diffCount = $mirrorOnly.Count + $agentsOnly.Count + $contentDiff.Count
    $status = if ($diffCount -eq 0) { 'identical' } else { 'outdated' }
    $sample = @(
        $mirrorOnly | Select-Object -First 2 | ForEach-Object { "mirror_only:$_" }
        $agentsOnly | Select-Object -First 2 | ForEach-Object { "agents_only:$_" }
        $contentDiff | Select-Object -First 2 | ForEach-Object { "content_diff:$_" }
    ) -join ' | '

    return [pscustomobject]@{
        Skill = $SkillName
        Status = $status
        DiffCount = $diffCount
        MirrorOnly = @($mirrorOnly)
        AgentsOnly = @($agentsOnly)
        ContentDiff = @($contentDiff)
        Sample = $sample
    }
}

function Copy-FileSet {
    param(
        [Parameter(Mandatory = $true)][string]$SourceRoot,
        [Parameter(Mandatory = $true)][string]$TargetRoot,
        [Parameter(Mandatory = $true)][string[]]$RelativePaths
    )

    foreach ($relativePath in $RelativePaths) {
        $sourcePath = Join-Path $SourceRoot $relativePath
        $targetPath = Join-Path $TargetRoot $relativePath
        $targetDir = Split-Path -Parent $targetPath
        if (-not (Test-Path -LiteralPath $targetDir)) {
            New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
        }
        Copy-Item -LiteralPath $sourcePath -Destination $targetPath -Force
    }
}

if (-not (Test-Path -LiteralPath $MirrorRoot)) {
    throw "Mirror root was not found: $MirrorRoot"
}
if (-not (Test-Path -LiteralPath $AgentsRoot)) {
    throw "Agents root was not found: $AgentsRoot"
}

$skillNames = if ($Skill -and $Skill.Count -gt 0) {
    $Skill
} else {
    Get-ChildItem -LiteralPath $MirrorRoot -Directory | Sort-Object Name | Select-Object -ExpandProperty Name
}

if ($ExcludeSkillPattern.Count -gt 0) {
    $skillNames = $skillNames | Where-Object { -not (Test-ExcludedSkill -SkillName $_ -Patterns $ExcludeSkillPattern) }
}

$results = foreach ($skillName in $skillNames) {
    $mirrorSkillRoot = Join-Path $MirrorRoot $skillName
    $agentsSkillRoot = Join-Path $AgentsRoot $skillName
    $comparison = Compare-SkillDirectory -SkillName $skillName -MirrorSkillRoot $mirrorSkillRoot -AgentsSkillRoot $agentsSkillRoot -IgnorePatterns $IgnorePattern

    switch ($Mode) {
        'SyncAgentsToMirror' {
            if ($comparison.Status -eq 'missing_in_agents') {
                Write-Warning "Skipping '$skillName': missing in agents"
            } elseif ($comparison.Status -ne 'identical') {
                $toCopy = @($comparison.AgentsOnly + $comparison.ContentDiff)
                if ($toCopy.Count -gt 0 -and $PSCmdlet.ShouldProcess($mirrorSkillRoot, "Copy $($toCopy.Count) file(s) from agents to mirror")) {
                    if (-not (Test-Path -LiteralPath $mirrorSkillRoot)) {
                        New-Item -ItemType Directory -Force -Path $mirrorSkillRoot | Out-Null
                    }
                    Copy-FileSet -SourceRoot $agentsSkillRoot -TargetRoot $mirrorSkillRoot -RelativePaths $toCopy
                }
            }
        }
        'SyncMirrorToAgents' {
            if ($comparison.Status -eq 'missing_in_mirror') {
                Write-Warning "Skipping '$skillName': missing in mirror"
            } elseif ($comparison.Status -ne 'identical') {
                $toCopy = @($comparison.MirrorOnly + $comparison.ContentDiff)
                if ($toCopy.Count -gt 0 -and $PSCmdlet.ShouldProcess($agentsSkillRoot, "Copy $($toCopy.Count) file(s) from mirror to agents")) {
                    if (-not (Test-Path -LiteralPath $agentsSkillRoot)) {
                        New-Item -ItemType Directory -Force -Path $agentsSkillRoot | Out-Null
                    }
                    Copy-FileSet -SourceRoot $mirrorSkillRoot -TargetRoot $agentsSkillRoot -RelativePaths $toCopy
                }
            }
        }
    }

    Compare-SkillDirectory -SkillName $skillName -MirrorSkillRoot $mirrorSkillRoot -AgentsSkillRoot $agentsSkillRoot -IgnorePatterns $IgnorePattern
}

$output = if ($IncludeIdentical) {
    $results
} else {
    $results | Where-Object { $_.Status -ne 'identical' }
}

if ($AsJson) {
    $output | ConvertTo-Json -Depth 6
} else {
    $output |
        Select-Object Skill, Status, DiffCount, Sample |
        Sort-Object Skill |
        Format-Table -AutoSize
}
