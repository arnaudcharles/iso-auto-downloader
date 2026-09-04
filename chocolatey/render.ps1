<#
.SYNOPSIS
Renders the Chocolatey package templates into a ready-to-pack directory.

.DESCRIPTION
chocolatey/ holds only templates, so the version lives in the git tag alone
and nothing in the repo can drift out of sync with what was released — which
is exactly what happened before, when the nuspec and install script said
0.1.1 while VERIFICATION.txt still pointed at the v0.1.0 archive.

Shared deliberately by two callers: .github/workflows/chocolatey.yml renders
with the real checksum and pushes, and .github/workflows/ci.yml renders with
a placeholder checksum and only packs, so a broken template fails a pull
request instead of a release. Keeping one script means the thing CI exercises
is the thing that actually publishes.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Version,
    [Parameter(Mandatory)][string]$Sha256,
    [Parameter(Mandatory)][string]$OutDir
)

$ErrorActionPreference = 'Stop'

$here = Split-Path -Parent $MyInvocation.MyCommand.Definition

New-Item -ItemType Directory -Force -Path "$OutDir/tools" | Out-Null
Copy-Item "$here/tools/LICENSE.txt" "$OutDir/tools/" -Force
Copy-Item "$here/tools/chocolateyuninstall.ps1" "$OutDir/tools/" -Force

$renders = @(
    @{ Src = "$here/iso-auto-downloader.nuspec.tmpl";  Dst = "$OutDir/iso-auto-downloader.nuspec" },
    @{ Src = "$here/tools/chocolateyinstall.ps1.tmpl"; Dst = "$OutDir/tools/chocolateyinstall.ps1" },
    @{ Src = "$here/tools/VERIFICATION.txt.tmpl";      Dst = "$OutDir/tools/VERIFICATION.txt" }
)
foreach ($r in $renders) {
    (Get-Content $r.Src -Raw) `
        -replace '__VERSION__', $Version `
        -replace '__SHA256_AMD64__', $Sha256 `
        | Set-Content -Path $r.Dst -NoNewline
}

# A leftover placeholder means a template gained one this script doesn't
# substitute. Catch it here rather than shipping a literal "__VERSION__" to
# the community feed.
$leftovers = Select-String -Path "$OutDir/*.nuspec", "$OutDir/tools/*" -Pattern '__[A-Z0-9_]+__' -ErrorAction SilentlyContinue
if ($leftovers) {
    $leftovers | ForEach-Object { Write-Host $_.Line }
    throw "unsubstituted placeholder left in the rendered package"
}

Write-Host "Rendered Chocolatey package $Version into $OutDir"
