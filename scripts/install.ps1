#Requires -Version 5.1
<#
.SYNOPSIS
  Install the agnostic-ai binary on Windows.

.DESCRIPTION
  Downloads the release zip for this machine's architecture, verifies its
  SHA256 against checksums.txt, extracts it, and puts the install directory
  on the user PATH. No Go toolchain, no package manager.

.EXAMPLE
  irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 | iex

.EXAMPLE
  .\install.ps1 -Version v0.45.0 -InstallDir C:\tools\agnostic-ai
#>
[CmdletBinding()]
param(
    [string]$Version = 'latest',
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\agnostic-ai"
)

$ErrorActionPreference = 'Stop'
# PowerShell 5.1 still negotiates TLS 1.0 by default, which GitHub refuses.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
# Invoke-WebRequest on 5.1 repaints a progress bar per chunk, which dominates
# runtime on a multi-megabyte download.
$ProgressPreference = 'SilentlyContinue'

$repo = 'Chemaclass/agnostic-ai'
$binary = 'agnostic-ai.exe'

function Write-Step($message) { Write-Host "-> $message" }

function Get-Architecture {
    # PROCESSOR_ARCHITECTURE reports the *process* arch, so a 32-bit shell on a
    # 64-bit machine reads x86; PROCESSOR_ARCHITEW6432 carries the real one.
    $arch = $env:PROCESSOR_ARCHITEW6432
    if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }

    switch ($arch) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        default { throw "unsupported architecture: $arch" }
    }
}

function Get-LatestVersion {
    $api = "https://api.github.com/repos/$repo/releases/latest"
    $tag = (Invoke-RestMethod -Uri $api -UseBasicParsing).tag_name
    if (-not $tag) { throw "could not resolve the latest release from $api" }
    return $tag
}

function Get-DownloadUrl($tag, $asset) {
    return "https://github.com/$repo/releases/download/$tag/$asset"
}

function Test-Checksum($archive, $asset, $tag, $workDir) {
    $sums = Join-Path $workDir 'checksums.txt'
    try {
        Invoke-WebRequest -Uri (Get-DownloadUrl $tag 'checksums.txt') -OutFile $sums -UseBasicParsing
    } catch {
        Write-Step "checksums.txt unavailable for $tag, skipping verification"
        return
    }

    $line = Select-String -Path $sums -Pattern ([regex]::Escape($asset)) | Select-Object -First 1
    if (-not $line) { throw "$asset missing from checksums.txt" }

    $expected = ($line.Line -split '\s+')[0]
    $actual = (Get-FileHash -Path $archive -Algorithm SHA256).Hash
    if ($actual -ne $expected.ToUpper()) { throw "checksum mismatch for $asset" }
    Write-Step 'checksum verified'
}

function Add-ToUserPath($directory) {
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @()
    if ($current) { $entries = $current -split ';' | Where-Object { $_ } }

    if ($entries -contains $directory) {
        Write-Step "PATH already contains $directory"
    } else {
        [Environment]::SetEnvironmentVariable('Path', (@($entries + $directory) -join ';'), 'User')
        Write-Step "added $directory to the user PATH (new terminals pick it up)"
    }

    if (($env:Path -split ';') -notcontains $directory) { $env:Path = "$env:Path;$directory" }
}

$arch = Get-Architecture
$tag = if ($Version -eq 'latest') { Get-LatestVersion } else { $Version }
$asset = "agnostic-ai_windows_$arch.zip"

Write-Step "installing agnostic-ai $tag (windows/$arch) into $InstallDir"

$workDir = Join-Path ([IO.Path]::GetTempPath()) "agnostic-ai-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $workDir -Force | Out-Null

try {
    $archive = Join-Path $workDir $asset
    Invoke-WebRequest -Uri (Get-DownloadUrl $tag $asset) -OutFile $archive -UseBasicParsing
    Test-Checksum $archive $asset $tag $workDir

    Expand-Archive -Path $archive -DestinationPath $workDir -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $workDir $binary) -Destination (Join-Path $InstallDir $binary) -Force
} finally {
    Remove-Item -Path $workDir -Recurse -Force -ErrorAction SilentlyContinue
}

Add-ToUserPath $InstallDir

Write-Step "installed: $(Join-Path $InstallDir $binary)"
& (Join-Path $InstallDir $binary) --version
