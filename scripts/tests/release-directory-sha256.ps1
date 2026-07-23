[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$releaseScript = Join-Path $PSScriptRoot '..\release\ae-release-binaries.ps1'
$tokens = $null
$parseErrors = $null
$releaseAst = [System.Management.Automation.Language.Parser]::ParseFile(
    $releaseScript,
    [ref]$tokens,
    [ref]$parseErrors
)
if ($parseErrors.Count -gt 0) {
    throw "Could not parse $releaseScript."
}

foreach ($functionName in @('Get-FileSha256', 'Get-DirectorySha256')) {
    $functionAst = $releaseAst.Find(
        {
            param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
                $node.Name -eq $functionName
        },
        $true
    )
    if ($null -eq $functionAst) {
        throw "Could not locate $functionName in $releaseScript."
    }
    . ([scriptblock]::Create($functionAst.Extent.Text))
}

$root = Join-Path ([System.IO.Path]::GetTempPath()) ('mi-lsp-directory-sha256-' + [guid]::NewGuid().ToString('N'))
try {
    $nested = Join-Path $root 'nested'
    New-Item -ItemType Directory -Force -Path $nested | Out-Null
    $rootFile = Join-Path $root 'root.txt'
    $nestedFile = Join-Path $nested 'child.txt'
    Set-Content -LiteralPath $rootFile -Value 'root' -NoNewline
    Set-Content -LiteralPath $nestedFile -Value 'child' -NoNewline

    $actual = Get-DirectorySha256 -Path $root
    $separator = [string][System.IO.Path]::DirectorySeparatorChar
    $records = @(
        "nested${separator}child.txt`t$((Get-FileHash -Algorithm SHA256 -Path $nestedFile).Hash.ToLowerInvariant())"
        "root.txt`t$((Get-FileHash -Algorithm SHA256 -Path $rootFile).Hash.ToLowerInvariant())"
    )
    $bytes = [System.Text.Encoding]::UTF8.GetBytes(($records -join "`n"))
    $expected = -join ([System.Security.Cryptography.SHA256]::Create().ComputeHash($bytes) | ForEach-Object { $_.ToString('x2') })
    if ($actual -ne $expected) {
        throw "Get-DirectorySha256 returned '$actual'; expected '$expected'."
    }

    Write-Output 'PASS: Get-DirectorySha256 handles Windows PowerShell TrimStart compatibility and preserves the digest'
}
finally {
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}
