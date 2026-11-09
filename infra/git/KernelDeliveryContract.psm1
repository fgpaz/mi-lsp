Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

function Unquote-AeDeliveryScalar {
    param([string]$Value)
    $text = ([string]$Value).Trim()
    if ($text.Length -ge 2) {
        $first = $text[0]
        $last = $text[$text.Length - 1]
        if (($first -eq [char]39 -and $last -eq [char]39) -or ($first -eq [char]34 -and $last -eq [char]34)) {
            return $text.Substring(1, $text.Length - 2)
        }
    }
    return $text
}

function Get-AeDeliveryIndent {
    param([string]$Line)
    return ([regex]::Match($Line, '^(\s*)')).Groups[1].Value.Length
}

function Get-AeDeliveryScalar {
    param([string[]]$Lines, [string]$Path)
    $stack = New-Object System.Collections.ArrayList
    foreach ($line in @($Lines)) {
        if (-not $line.Trim() -or $line -match '^\s*#') { continue }
        $match = [regex]::Match($line, '^(\s*)([A-Za-z_][A-Za-z0-9_-]*):(?:\s*(.*?)\s*)?$')
        if (-not $match.Success) { continue }
        $indent = $match.Groups[1].Value.Length
        while ($stack.Count -gt 0 -and [int]$stack[$stack.Count - 1].Indent -ge $indent) { $stack.RemoveAt($stack.Count - 1) }
        $key = $match.Groups[2].Value
        $prefix = if ($stack.Count -gt 0) { (($stack | ForEach-Object { $_.Key }) -join '.') + '.' } else { '' }
        $current = $prefix + $key
        if ($current -eq $Path) {
            $raw = $match.Groups[3].Value
            return [pscustomobject]@{ Present = $true; Value = if ($null -eq $raw) { '' } else { Unquote-AeDeliveryScalar $raw } }
        }
        [void]$stack.Add([pscustomobject]@{ Indent = $indent; Key = $key })
    }
    return [pscustomobject]@{ Present = $false; Value = '' }
}

function Test-AeDeliveryPathDeclared {
    param([string[]]$Lines, [string]$Path)
    return [bool](Get-AeDeliveryScalar $Lines $Path).Present
}

function Get-AeDeliveryDestinationRecords {
    param([string[]]$Lines, [string]$Path)
    $records = New-Object System.Collections.ArrayList
    $section = $null
    for ($index = 0; $index -lt $Lines.Count; $index++) {
        $line = [string]$Lines[$index]
        if (-not $line.Trim() -or $line -match '^\s*#') { continue }
        $match = [regex]::Match($line, '^(\s*)' + [regex]::Escape(($Path -split '\.')[-1]) + ':\s*(.*?)\s*$')
        if ($match.Success) {
            $prefix = ($Path -split '\.')
            $stack = New-Object System.Collections.ArrayList
            foreach ($candidate in @($Lines[0..$index])) {
                if (-not ([string]$candidate).Trim() -or ([string]$candidate) -match '^\s*#') { continue }
                $keyMatch = [regex]::Match(([string]$candidate), '^(\s*)([A-Za-z_][A-Za-z0-9_-]*):')
                if (-not $keyMatch.Success) { continue }
                $indent = $keyMatch.Groups[1].Value.Length
                while ($stack.Count -gt 0 -and [int]$stack[$stack.Count - 1].Indent -ge $indent) { $stack.RemoveAt($stack.Count - 1) }
                [void]$stack.Add([pscustomobject]@{ Indent = $indent; Key = $keyMatch.Groups[2].Value })
            }
            $computed = if ($stack.Count -gt 0) { (($stack | ForEach-Object { $_.Key }) -join '.') } else { '' }
            if ($computed -eq $Path) { $section = [pscustomobject]@{ Index = $index; Indent = $match.Groups[1].Value.Length }; break }
        }
    }
    if ($null -eq $section) { return @($records) }
    $index = [int]$section.Index + 1
    while ($index -lt $Lines.Count) {
        $line = [string]$Lines[$index]
        if (-not $line.Trim() -or $line -match '^\s*#') { $index++; continue }
        $indent = Get-AeDeliveryIndent $line
        if ($indent -le [int]$section.Indent) { break }
        $item = [regex]::Match($line, '^\s*-\s*(.*?)\s*$')
        if (-not $item.Success) { $index++; continue }
        $itemIndent = $indent
        $record = [ordered]@{}
        $duplicateFields = New-Object System.Collections.ArrayList
        $addField = {
            param([string]$Name, [string]$Value)
            if ($record.Contains($Name)) { [void]$duplicateFields.Add($Name) }
            else { $record[$Name] = Unquote-AeDeliveryScalar $Value }
        }
        $inline = [regex]::Match($item.Groups[1].Value, '^([A-Za-z_][A-Za-z0-9_-]*):\s*(.*?)\s*$')
        if ($inline.Success) { & $addField $inline.Groups[1].Value $inline.Groups[2].Value }
        $index++
        while ($index -lt $Lines.Count) {
            $child = [string]$Lines[$index]
            if (-not $child.Trim() -or $child -match '^\s*#') { $index++; continue }
            $childIndent = Get-AeDeliveryIndent $child
            if ($childIndent -le $itemIndent) { break }
            $childMatch = [regex]::Match($child, '^\s*([A-Za-z_][A-Za-z0-9_-]*):\s*(.*?)\s*$')
            if ($childMatch.Success) { & $addField $childMatch.Groups[1].Value $childMatch.Groups[2].Value }
            $index++
        }
        $record['__duplicate_fields'] = @($duplicateFields | Sort-Object -Unique)
        [void]$records.Add([pscustomobject]$record)
    }
    return @($records)
}

function Resolve-AeDeliveryPath {
    param([string]$RepositoryRoot, [string]$Path)
    if ([System.IO.Path]::IsPathRooted($Path)) { return [System.IO.Path]::GetFullPath($Path) }
    return [System.IO.Path]::GetFullPath((Join-Path $RepositoryRoot ($Path.Replace('/', [IO.Path]::DirectorySeparatorChar))))
}

function Test-AeDeliverySamePath {
    param([string]$Left, [string]$Right)
    return [string]::Equals(
        ([System.IO.Path]::GetFullPath($Left)).TrimEnd([char[]]@('\', '/')),
        ([System.IO.Path]::GetFullPath($Right)).TrimEnd([char[]]@('\', '/')),
        [System.StringComparison]::OrdinalIgnoreCase)
}

function Test-AeDeliveryPathHasReparsePoint {
    param([string]$Path)
    $full = [System.IO.Path]::GetFullPath($Path)
    $root = [System.IO.Path]::GetPathRoot($full)
    if (-not $root) { return $false }
    $current = $root
    $relative = $full.Substring($root.Length).TrimStart([char[]]@('\', '/'))
    foreach ($part in @($relative -split '[\\/]+' | Where-Object { $_ })) {
        $current = Join-Path $current $part
        if (Test-Path -LiteralPath $current) {
            $item = Get-Item -LiteralPath $current -Force -ErrorAction Stop
            if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { return $true }
        }
    }
    return $false
}

function Read-AeDeliveryContract {
    param([string]$Path, [string]$RepositoryRoot)
    $errors = New-Object System.Collections.ArrayList
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        [void]$errors.Add('delivery contract not found: ' + $Path)
        return [pscustomobject]@{ Valid = $false; Errors = @($errors); Path = $Path }
    }
    $text = [System.IO.File]::ReadAllText($Path)
    $lines = @($text -split "`r?`n")
    if ($text -match "`t") { [void]$errors.Add('delivery contract contains tabs; use spaces only') }

    # Validate the small delivery-contract grammar without accepting a known key at
    # an arbitrary level. The scalar readers below are intentionally permissive for
    # legacy syntax, so this structural pass is the fail-closed authority for paths,
    # scalar-vs-mapping shape, and duplicate mappings.
    $childrenByParent = @{
        '' = @('schema','product','repository','delivery')
        'delivery' = @('mode','remote','base','branch','source_root','source_product','same_product_only','publication','installation')
        'delivery.publication' = @('stage','mirror_policy','distribution_mirrors')
        'delivery.installation' = @('stage','target_policy','installation_targets')
        'delivery.publication.distribution_mirrors[]' = @('name','product','root')
        'delivery.installation.installation_targets[]' = @('name','product','root')
    }
    $listPaths = @('delivery.publication.distribution_mirrors','delivery.installation.installation_targets')
    $scalarPaths = @('schema','product','repository','delivery.mode','delivery.remote','delivery.base','delivery.branch','delivery.source_root','delivery.source_product','delivery.same_product_only','delivery.publication.stage','delivery.publication.mirror_policy','delivery.installation.stage','delivery.installation.target_policy')
    $knownKeys = @($childrenByParent.Values | ForEach-Object { $_ } | Sort-Object -Unique)
    $seenMappings = @{}
    $pathStack = New-Object System.Collections.ArrayList
    $nodes = New-Object System.Collections.ArrayList
    foreach ($line in @($lines)) {
        if (-not $line.Trim() -or $line -match '^\s*#' -or $line -match '^\s*---\s*$') { continue }
        $itemMatch = [regex]::Match($line, '^(\s*)-\s*(.*?)\s*$')
        if ($itemMatch.Success) {
            $indent = $itemMatch.Groups[1].Value.Length
            while ($pathStack.Count -gt 0 -and [int]$pathStack[$pathStack.Count - 1].Indent -ge $indent) { $pathStack.RemoveAt($pathStack.Count - 1) }
            $parent = if ($pathStack.Count -gt 0) { [string]$pathStack[$pathStack.Count - 1].Path } else { '' }
            if ($listPaths -notcontains $parent) { [void]$errors.Add('delivery contract list item is misplaced: ' + $parent); continue }
            $itemPath = $parent + '[]'
            [void]$nodes.Add([pscustomobject]@{ Path=$itemPath; Indent=$indent; Raw=''; IsListItem=$true })
            [void]$pathStack.Add([pscustomobject]@{ Indent=$indent; Path=$itemPath; IsListItem=$true; Seen=@{} })
            $inline = [regex]::Match($itemMatch.Groups[2].Value, '^([A-Za-z_][A-Za-z0-9_-]*):\s*(.*?)\s*$')
            if ($inline.Success) {
                $inlineKey = $inline.Groups[1].Value
                if ($childrenByParent[$itemPath] -notcontains $inlineKey) {
                    if ($knownKeys -contains $inlineKey) { [void]$errors.Add('delivery contract field is misplaced: ' + $inlineKey + ' (under ' + $itemPath + ')') }
                    else { [void]$errors.Add('delivery contract has unknown field: ' + $inlineKey) }
                } elseif ($pathStack[$pathStack.Count - 1].Seen.ContainsKey($inlineKey)) {
                    [void]$errors.Add('delivery contract has duplicate/ambiguous field: ' + $itemPath + '.' + $inlineKey)
                } else {
                    $pathStack[$pathStack.Count - 1].Seen[$inlineKey] = $true
                    [void]$nodes.Add([pscustomobject]@{ Path=$itemPath + '.' + $inlineKey; Indent=$indent; Raw=$inline.Groups[2].Value })
                }
            }
            continue
        }
        $keyMatch = [regex]::Match($line, '^(\s*)([A-Za-z_][A-Za-z0-9_-]*):(?:\s*(.*?)\s*)?$')
        if (-not $keyMatch.Success) { [void]$errors.Add('delivery contract has malformed YAML line: ' + $line.Trim()); continue }
        $indent = $keyMatch.Groups[1].Value.Length
        while ($pathStack.Count -gt 0 -and [int]$pathStack[$pathStack.Count - 1].Indent -ge $indent) { $pathStack.RemoveAt($pathStack.Count - 1) }
        $parent = if ($pathStack.Count -gt 0) { [string]$pathStack[$pathStack.Count - 1].Path } else { '' }
        $key = $keyMatch.Groups[2].Value
        $raw = if ($null -eq $keyMatch.Groups[3].Value) { '' } else { $keyMatch.Groups[3].Value }
        $allowedChildren = if ($childrenByParent.ContainsKey($parent)) { @($childrenByParent[$parent]) } else { @() }
        if ($allowedChildren -notcontains $key) {
            $location = if ($parent) { ' under ' + $parent } else { ' at root' }
            if ($knownKeys -contains $key) { [void]$errors.Add('delivery contract field is misplaced: ' + $key + $location) }
            else { [void]$errors.Add('delivery contract has unknown field: ' + $key) }
        }
        $path = if ($parent) { $parent + '.' + $key } else { $key }
        if ($parent -match '\[\]$') {
            $item = $pathStack[$pathStack.Count - 1]
            if ($item.Seen.ContainsKey($key)) { [void]$errors.Add('delivery contract has duplicate/ambiguous field: ' + $path) }
            else { $item.Seen[$key] = $true }
        } elseif ($seenMappings.ContainsKey($path)) {
            [void]$errors.Add('delivery contract has duplicate/ambiguous field: ' + $path)
        } else {
            $seenMappings[$path] = $true
        }
        [void]$nodes.Add([pscustomobject]@{ Path=$path; Indent=$indent; Raw=$raw })
        [void]$pathStack.Add([pscustomobject]@{ Indent=$indent; Path=$path; IsListItem=$false; Seen=@{} })
    }
    foreach ($node in @($nodes | Where-Object { $scalarPaths -contains $_.Path -and [string]::IsNullOrWhiteSpace([string]$_.Raw) })) {
        foreach ($child in @($nodes | Where-Object { $_.Indent -gt $node.Indent -and $_.Path.StartsWith($node.Path + '.', [System.StringComparison]::Ordinal) })) {
            [void]$errors.Add('delivery contract scalar field cannot have children: ' + $node.Path)
            break
        }
    }
    foreach ($node in @($nodes | Where-Object { @('delivery','delivery.publication','delivery.installation') -contains $_.Path -and -not [string]::IsNullOrWhiteSpace([string]$_.Raw) })) {
        [void]$errors.Add('delivery contract mapping field must be empty: ' + $node.Path)
    }
    foreach ($node in @($nodes | Where-Object { @('delivery.publication.distribution_mirrors','delivery.installation.installation_targets') -contains $_.Path -and ([string]$_.Raw).Trim() -eq '[]' })) {
        foreach ($child in @($nodes | Where-Object { $_.Indent -gt $node.Indent -and $_.Path.StartsWith($node.Path + '[]', [System.StringComparison]::Ordinal) })) {
            [void]$errors.Add('delivery contract empty list cannot have children or items: ' + $node.Path)
            break
        }
    }

    $schema = (Get-AeDeliveryScalar $lines 'schema').Value
    $product = (Get-AeDeliveryScalar $lines 'product').Value
    $repository = (Get-AeDeliveryScalar $lines 'repository').Value
    $mode = (Get-AeDeliveryScalar $lines 'delivery.mode').Value
    $remote = (Get-AeDeliveryScalar $lines 'delivery.remote').Value
    $base = (Get-AeDeliveryScalar $lines 'delivery.base').Value
    $branch = (Get-AeDeliveryScalar $lines 'delivery.branch').Value
    $sourceRoot = (Get-AeDeliveryScalar $lines 'delivery.source_root').Value
    $sourceProduct = (Get-AeDeliveryScalar $lines 'delivery.source_product').Value
    $sameProductOnly = (Get-AeDeliveryScalar $lines 'delivery.same_product_only').Value
    $publicationStage = (Get-AeDeliveryScalar $lines 'delivery.publication.stage').Value
    $mirrorPolicy = (Get-AeDeliveryScalar $lines 'delivery.publication.mirror_policy').Value
    $installationStage = (Get-AeDeliveryScalar $lines 'delivery.installation.stage').Value
    $targetPolicy = (Get-AeDeliveryScalar $lines 'delivery.installation.target_policy').Value
    $mirrorsDeclaration = Get-AeDeliveryScalar $lines 'delivery.publication.distribution_mirrors'
    $targetsDeclaration = Get-AeDeliveryScalar $lines 'delivery.installation.installation_targets'
    if (-not $mirrorsDeclaration.Present) { [void]$errors.Add('delivery.publication.distribution_mirrors must be explicitly declared') }
    if (-not $targetsDeclaration.Present) { [void]$errors.Add('delivery.installation.installation_targets must be explicitly declared') }
    $mirrors = @(Get-AeDeliveryDestinationRecords $lines 'delivery.publication.distribution_mirrors')
    $targets = @(Get-AeDeliveryDestinationRecords $lines 'delivery.installation.installation_targets')
    if ($mirrorsDeclaration.Present -and $mirrorsDeclaration.Value -and $mirrorsDeclaration.Value -ne '[]') { [void]$errors.Add('delivery.publication.distribution_mirrors must be a YAML list') }
    if ($targetsDeclaration.Present -and $targetsDeclaration.Value -and $targetsDeclaration.Value -ne '[]') { [void]$errors.Add('delivery.installation.installation_targets must be a YAML list') }

    $required = @(
        @{ Name = 'schema'; Value = $schema }, @{ Name = 'product'; Value = $product }, @{ Name = 'repository'; Value = $repository },
        @{ Name = 'delivery.mode'; Value = $mode }, @{ Name = 'delivery.remote'; Value = $remote }, @{ Name = 'delivery.base'; Value = $base },
        @{ Name = 'delivery.branch'; Value = $branch }, @{ Name = 'delivery.source_root'; Value = $sourceRoot },
        @{ Name = 'delivery.source_product'; Value = $sourceProduct }, @{ Name = 'delivery.same_product_only'; Value = $sameProductOnly },
        @{ Name = 'delivery.publication.stage'; Value = $publicationStage }, @{ Name = 'delivery.publication.mirror_policy'; Value = $mirrorPolicy },
        @{ Name = 'delivery.installation.stage'; Value = $installationStage }, @{ Name = 'delivery.installation.target_policy'; Value = $targetPolicy }
    )
    foreach ($field in $required) { if ([string]::IsNullOrWhiteSpace([string]$field.Value)) { [void]$errors.Add('delivery contract required field is missing or empty: ' + $field.Name) } }
    if ($schema -ne 'ae-delivery-contract/v1') { [void]$errors.Add('delivery contract schema must be exactly ae-delivery-contract/v1') }
    if ($mode -notin @('mainline','pull_request')) { [void]$errors.Add('delivery.mode must be mainline or pull_request') }
    if ($remote -notmatch '^[A-Za-z0-9._-]+$') { [void]$errors.Add('delivery.remote is malformed') }
    if ($base -notmatch '^[A-Za-z0-9._-]+$') { [void]$errors.Add('delivery.base must be a branch name without a remote prefix') }
    if ($branch -notin @('main','current')) { [void]$errors.Add('delivery.branch must be main for mainline or current for pull_request') }
    if ($mode -eq 'mainline' -and $branch -ne 'main') { [void]$errors.Add('mainline delivery requires delivery.branch: main') }
    if ($mode -eq 'pull_request' -and $branch -ne 'current') { [void]$errors.Add('pull_request delivery requires delivery.branch: current') }
    if ($sourceProduct -and $product -and $sourceProduct -ne $product) { [void]$errors.Add('delivery.source_product must match product') }
    if ($sameProductOnly -ine 'true') { [void]$errors.Add('delivery.same_product_only must be true') }
    if ($publicationStage -ne 'publication') { [void]$errors.Add('delivery.publication.stage must be publication') }
    if ($installationStage -ne 'installation') { [void]$errors.Add('delivery.installation.stage must be installation') }
    if ($mirrors.Count -eq 0 -and $mirrorPolicy -ne 'none_declared') { [void]$errors.Add('empty distribution_mirrors requires mirror_policy: none_declared') }
    if ($mirrors.Count -gt 0 -and $mirrorPolicy -ne 'required_declared') { [void]$errors.Add('declared distribution_mirrors require mirror_policy: required_declared') }
    if ($targets.Count -eq 0 -and $targetPolicy -ne 'manual_only') { [void]$errors.Add('empty installation_targets requires target_policy: manual_only') }
    if ($targets.Count -gt 0 -and $targetPolicy -ne 'declared') { [void]$errors.Add('declared installation_targets require target_policy: declared') }
    if ([System.IO.Path]::IsPathRooted($sourceRoot) -or $sourceRoot -match '(^|[\\/])\.\.([\\/]|$)' -or $sourceRoot -notmatch '[^\\/]') { [void]$errors.Add('delivery.source_root must be a repository-relative non-empty path') }
    $sourcePath = Resolve-AeDeliveryPath $RepositoryRoot $sourceRoot
    if (Test-AeDeliveryPathHasReparsePoint $sourcePath) { [void]$errors.Add('delivery source root contains a symlink or reparse point: ' + $sourceRoot) }
    $destinationPaths = New-Object System.Collections.ArrayList
    foreach ($destination in @($mirrors + $targets)) {
        $destinationName = if ($destination.PSObject.Properties.Name -contains 'name') { [string]$destination.name } else { '<unnamed>' }
        if ($destination.PSObject.Properties.Name -contains '__duplicate_fields') {
            foreach ($field in @($destination.__duplicate_fields)) { [void]$errors.Add('delivery destination has duplicate field: ' + $field + ' (' + $destinationName + ')') }
        }
        foreach ($field in @('name','product','root')) {
            if (-not ($destination.PSObject.Properties.Name -contains $field) -or [string]::IsNullOrWhiteSpace([string]$destination.$field)) { [void]$errors.Add('delivery destination requires name, product, and root') }
        }
        if (($destination.PSObject.Properties.Name -contains 'product') -and $destination.product -ne $product) { [void]$errors.Add('delivery destination product must match the contract product: ' + $destinationName) }
        if (($destination.PSObject.Properties.Name -contains 'root') -and $destination.root) {
            $destinationPath = Resolve-AeDeliveryPath $RepositoryRoot ([string]$destination.root)
            if (Test-AeDeliveryPathHasReparsePoint $destinationPath) { [void]$errors.Add('delivery destination contains a symlink or reparse point: ' + [string]$destination.root) }
            if (Test-AeDeliverySamePath $sourcePath $destinationPath) { [void]$errors.Add('delivery destination must not alias the source root: ' + [string]$destination.root) }
            foreach ($prior in @($destinationPaths)) { if (Test-AeDeliverySamePath $prior $destinationPath) { [void]$errors.Add('delivery destinations must not alias one another: ' + [string]$destination.root) } }
            [void]$destinationPaths.Add($destinationPath)
        }
    }
    $mirrorFields = @('name','product','root')
    foreach ($destination in @($mirrors + $targets)) {
        foreach ($fieldName in @($destination.PSObject.Properties.Name)) {
            if ($fieldName -eq '__duplicate_fields') { continue }
            if ($mirrorFields -notcontains $fieldName) { [void]$errors.Add('delivery destination has unknown field: ' + $fieldName) }
        }
    }
    return [pscustomobject]@{
        Valid = ($errors.Count -eq 0)
        Errors = @($errors)
        Path = $Path
        Schema = $schema
        Product = $product
        Repository = $repository
        Mode = $mode
        Remote = $remote
        Base = $base
        Branch = $branch
        SourceRoot = $sourceRoot.TrimEnd('/', '\')
        SourceProduct = $sourceProduct
        SameProductOnly = ($sameProductOnly -ieq 'true')
        PublicationStage = $publicationStage
        MirrorPolicy = $mirrorPolicy
        DistributionMirrors = @($mirrors)
        InstallationStage = $installationStage
        TargetPolicy = $targetPolicy
        InstallationTargets = @($targets)
        SourcePath = $sourcePath
    }
}

Export-ModuleMember -Function Read-AeDeliveryContract,Resolve-AeDeliveryPath,Test-AeDeliverySamePath,Test-AeDeliveryPathHasReparsePoint
