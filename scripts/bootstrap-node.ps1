$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$root = Split-Path -Parent $PSScriptRoot
$version = '26.8.2'
$name = "node-v$version-win-x64"
$tools = Join-Path $root '.tools'
$destination = Join-Path $tools $name
$node = Join-Path $destination 'node.exe'

if (-not (Test-Path -LiteralPath $node)) {
    New-Item -ItemType Directory -Path $tools -Force | Out-Null
    $archive = Join-Path $tools "$name.zip"
    $base = "https://nodejs.org/dist/v$version"
    $sums = (Invoke-WebRequest -UseBasicParsing "$base/SHASUMS256.txt").Content
    $escaped = [regex]::Escape("$name.zip")
    $match = [regex]::Match($sums, "(?m)^([0-9a-f]{64})\s+$escaped\r?$")
    if (-not $match.Success) { throw 'Official Node checksum not found; refusing install.' }
    Invoke-WebRequest -UseBasicParsing "$base/$name.zip" -OutFile $archive
    $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $match.Groups[1].Value) { throw 'Node archive checksum mismatch; refusing extraction.' }
    Expand-Archive -LiteralPath $archive -DestinationPath $tools -Force
}

$installed = & $node --version
if ($LASTEXITCODE -ne 0 -or $installed -ne "v$version") { throw 'Portable Node verification failed.' }
Write-Output "Verified project-local Node $installed. No system PATH or global Node installation was changed."
Write-Output 'For this PowerShell session, run:'
Write-Output ('$env:PATH = "' + $destination + ';$env:PATH"')
