#Requires -Version 5.1
<#
.SYNOPSIS
  Install the agnostic-ai binary on Windows.

.DESCRIPTION
  Downloads the release zip for this machine's architecture, verifies its
  SHA256 against checksums.txt and, when the GitHub CLI (gh) is installed
  and logged in, its build provenance, extracts it, and puts the install
  directory on the user PATH. No Go toolchain, no package manager.

.EXAMPLE
  irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 | iex

.EXAMPLE
  .\install.ps1 -Version v0.45.0 -InstallDir C:\tools\agnostic-ai

.EXAMPLE
  .\install.ps1 -VerifyAttestation
  Fails unless the GitHub CLI (gh) verifies the zip's build provenance.
  Set AGNOSTIC_AI_VERIFY_ATTESTATION=0 to skip that check instead.
#>
[CmdletBinding()]
param(
    [string]$Version = 'latest',
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\agnostic-ai",
    [switch]$VerifyAttestation = ($env:AGNOSTIC_AI_VERIFY_ATTESTATION -eq '1')
)

$ErrorActionPreference = 'Stop'
# PowerShell 5.1 still negotiates TLS 1.0 by default, which GitHub refuses.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
# Invoke-WebRequest on 5.1 repaints a progress bar per chunk, which dominates
# runtime on a multi-megabyte download.
$ProgressPreference = 'SilentlyContinue'

$repo = 'Chemaclass/agnostic-ai'
$binary = 'agnostic-ai.exe'
# releaseVersionRE in internal/cli/upgrade.go, with \z because $ in .NET
# also matches before a trailing newline.
$versionPattern = '^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?\z'
# Releases before this one were published without build provenance.
$firstAttestedRelease = [version]'0.75.0'

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
    # Resolved from a redirect, not from the API.
    #
    # https://api.github.com/repos/<repo>/releases/latest allows 60
    # unauthenticated requests per hour per IP, which shared egress (CI
    # runners behind NAT, a VPN, an office) exhausts without anyone doing
    # anything wrong; the install then dies on a bare 403 that never says
    # "rate limit". https://github.com/<repo>/releases/latest answers 302
    # to /releases/tag/<tag>, is outside the API, and carries no such limit.
    $url = "https://github.com/$repo/releases/latest"

    # HttpWebRequest, not Invoke-RestMethod. Invoke-RestMethod follows the
    # redirect and discards the Location header we came for, and turning
    # that off with -MaximumRedirection 0 reads differently per host:
    # Windows PowerShell 5.1 raises a terminating WebException on the 3xx,
    # PowerShell 7 hands back the 3xx response instead. This works the same
    # on both, and 5.1 is what ships with Windows.
    $request = [Net.WebRequest]::Create($url)
    $request.AllowAutoRedirect = $false
    $request.UserAgent = 'agnostic-ai-install'
    $request.Timeout = 15000

    try {
        $response = $request.GetResponse()
    } catch [Net.WebException] {
        # 4xx and 5xx arrive as an exception; the response rides along.
        $response = $_.Exception.Response
        if (-not $response) { throw "could not reach ${url}: $($_.Exception.Message)" }
    }

    try {
        $status = [int]$response.StatusCode
        $location = $response.Headers['Location']
    } finally {
        $response.Close()
    }

    if ($status -eq 403 -or $status -eq 429) {
        throw ("github rate limited this network while resolving the latest release. " +
               "Wait a minute and retry, or pass -Version vX.Y.Z")
    }
    if ($status -eq 404) { throw "no published release at $url" }
    if ($status -lt 300 -or $status -gt 399) { throw "unexpected HTTP $status from $url" }

    if ($location -match '/releases/tag/(?<tag>[^/?#]+)/?$') { return $Matches['tag'] }
    $target = if ($location) { $location } else { 'nowhere' }
    throw "no published release: $url redirected to $target"
}

function Get-DownloadUrl($tag, $asset) {
    return "https://github.com/$repo/releases/download/$tag/$asset"
}

function Test-Checksum($archive, $asset, $tag, $workDir) {
    $sums = Join-Path $workDir 'checksums.txt'
    try {
        Invoke-WebRequest -Uri (Get-DownloadUrl $tag 'checksums.txt') -OutFile $sums -UseBasicParsing
    } catch {
        throw "checksums.txt unavailable for $tag, so $asset cannot be verified"
    }

    $line = Select-String -Path $sums -Pattern ('\s' + [regex]::Escape($asset) + '$') | Select-Object -First 1
    if (-not $line) { throw "$asset missing from checksums.txt" }

    $expected = ($line.Line -split '\s+')[0]
    $actual = (Get-FileHash -Path $archive -Algorithm SHA256).Hash
    if ($actual -ne $expected.ToUpper()) { throw "checksum mismatch for $asset" }
    Write-Step 'checksum verified'
}

function Get-ReleaseTag($version) {
    if ($version -cnotmatch $versionPattern) {
        throw "'$version' is not a release version; use one like v0.89.0"
    }
    return 'v' + $version.TrimStart('v')
}

# Test-Attestation checks the zip's build provenance with the GitHub CLI:
# proof that this repository's release workflow built it from the tagged
# commit, which a matching checksum from the same release page cannot give.
# It runs whenever gh is installed and logged in. -VerifyAttestation makes a
# skipped check fail, and AGNOSTIC_AI_VERIFY_ATTESTATION=0 skips it. Only a
# version the user named may predate provenance, since whoever can publish
# release assets could mark such a release latest.
function Test-Attestation($archive, $tag, $pinned) {
    if ($env:AGNOSTIC_AI_VERIFY_ATTESTATION -eq '0' -and -not $VerifyAttestation) {
        Write-Step 'build provenance not checked: AGNOSTIC_AI_VERIFY_ATTESTATION=0'
        return
    }
    $core = [version](($tag.TrimStart('v') -split '-')[0])
    if ($core -lt $firstAttestedRelease) {
        if ($VerifyAttestation) { throw "$tag predates build provenance, which starts at v$firstAttestedRelease, and -VerifyAttestation requires it" }
        if (-not $pinned) { throw "the latest release $tag predates build provenance, which starts at v$firstAttestedRelease; pass -Version to install it on the checksum alone" }
        Write-Step "build provenance not checked: releases before v$firstAttestedRelease do not publish it"
        return
    }
    if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
        if ($VerifyAttestation) { throw '-VerifyAttestation needs the GitHub CLI (gh)' }
        Write-Step 'build provenance not checked: install the GitHub CLI (gh) to verify it'
        return
    }
    # Windows PowerShell 5.1 turns redirected native stderr into errors, which
    # 'Stop' would throw on before the exit code is read.
    $previous = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        # The host is pinned: a gh logged in to GitHub Enterprise only would
        # ask that server, which does not hold these attestations.
        $detail = (& gh attestation verify $archive --hostname github.com --repo $repo `
            --signer-workflow "$repo/.github/workflows/release.yml" `
            --source-ref "refs/tags/$tag" --deny-self-hosted-runners 2>&1 | ForEach-Object { "$_" } | Out-String).Trim()
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    if ($code -ne 0 -and -not $VerifyAttestation) {
        # gh exits 4 when a command needs a login it does not have.
        if ($code -eq 4) {
            Write-Step 'build provenance not checked: gh is not logged in (run gh auth login)'
            return
        }
        if ($detail -match 'unknown (flag|command)') {
            Write-Step 'build provenance not checked: this gh cannot verify build provenance; update gh'
            return
        }
    }
    if ($code -ne 0) { throw "build provenance did not verify for $(Split-Path $archive -Leaf): $detail" }
    Write-Step 'build provenance verified'
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
$tag = Get-ReleaseTag $(if ($Version -eq 'latest') { Get-LatestVersion } else { $Version })
$asset = "agnostic-ai_windows_$arch.zip"

Write-Step "installing agnostic-ai $tag (windows/$arch) into $InstallDir"

$workDir = Join-Path ([IO.Path]::GetTempPath()) "agnostic-ai-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $workDir -Force | Out-Null

try {
    $archive = Join-Path $workDir $asset
    Invoke-WebRequest -Uri (Get-DownloadUrl $tag $asset) -OutFile $archive -UseBasicParsing
    Test-Checksum $archive $asset $tag $workDir
    Test-Attestation $archive $tag ($Version -ne 'latest')

    Expand-Archive -Path $archive -DestinationPath $workDir -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $workDir $binary) -Destination (Join-Path $InstallDir $binary) -Force
} finally {
    Remove-Item -Path $workDir -Recurse -Force -ErrorAction SilentlyContinue
}

Add-ToUserPath $InstallDir

Write-Step "installed: $(Join-Path $InstallDir $binary)"
& (Join-Path $InstallDir $binary) --version
