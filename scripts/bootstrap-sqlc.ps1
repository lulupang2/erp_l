$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$root = Split-Path -Parent $PSScriptRoot
$version = '1.31.1'
$name = "sqlc_${version}_windows_amd64.zip"
$tools = Join-Path $root '.tools'
$destination = Join-Path $tools "sqlc-$version"
$executable = Join-Path $destination 'sqlc.exe'
if (-not (Test-Path -LiteralPath $executable)) {
    New-Item -ItemType Directory -Path $destination -Force | Out-Null
    $release = Invoke-RestMethod "https://api.github.com/repos/sqlc-dev/sqlc/releases/tags/v$version"
    $asset = $release.assets | Where-Object { $_.name -eq $name } | Select-Object -First 1
    if (-not $asset -or $asset.digest -notmatch '^sha256:([0-9a-f]{64})$') {
        throw 'Official release SHA-256 metadata is missing; refusing extraction.'
    }
    $expected = $Matches[1]
    $archive = Join-Path $tools $name
    Invoke-WebRequest -UseBasicParsing $asset.browser_download_url -OutFile $archive
    $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw 'sqlc archive checksum mismatch; refusing extraction.' }
    Expand-Archive -LiteralPath $archive -DestinationPath $destination -Force
}
$installed = & $executable version
if ($LASTEXITCODE -ne 0 -or $installed -ne "v$version") { throw 'sqlc binary version verification failed.' }
Write-Output "Verified project-local sqlc $installed; no global installation changed."
