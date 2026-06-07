# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Apache License, Version 2.0.
#
# Elastic Docs Harness installer shim for Windows (PowerShell).
# Usage:
#   irm https://github.com/elastic/docs-harness/releases/latest/download/install.ps1 | iex

$ErrorActionPreference = 'Stop'

$Repo = "elastic/docs-harness"
$BinaryName = "docs-harness"
$InstallDir = "$env:LOCALAPPDATA\docs-harness"

function Write-Info  { Write-Host "ℹ $args" -ForegroundColor Cyan }
function Write-OK    { Write-Host "✓ $args" -ForegroundColor Green }
function Write-Warn  { Write-Host "⚠ $args" -ForegroundColor Yellow }
function Write-Err   { Write-Host "✗ $args" -ForegroundColor Red }

# Fetch latest release version from GitHub.
function Get-LatestVersion {
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    return $release.tag_name
}

$Version = Get-LatestVersion
Write-Info "Latest release: $Version"

$Archive = "${BinaryName}_windows_amd64.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$Archive"
$ChecksumUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt"

$TempDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }

try {
    $ArchivePath = Join-Path $TempDir $Archive

    Write-Info "Downloading $Archive..."
    Invoke-WebRequest $DownloadUrl -OutFile $ArchivePath

    # Verify checksum.
    Write-Info "Verifying checksum..."
    $Checksums = (Invoke-WebRequest $ChecksumUrl).Content
    $Expected = ($Checksums -split "`n" | Where-Object { $_ -match $Archive } | Select-Object -First 1) -split '\s+' | Select-Object -First 1
    if ($Expected) {
        $Actual = (Get-FileHash -Algorithm SHA256 $ArchivePath).Hash.ToLower()
        $Expected = $Expected.ToLower()
        if ($Actual -ne $Expected) {
            Write-Err "Checksum mismatch! Expected $Expected, got $Actual"
            exit 1
        }
        Write-OK "Checksum verified"
    } else {
        Write-Warn "Could not find checksum for $Archive — skipping verification"
    }

    # Extract and install.
    Expand-Archive $ArchivePath -DestinationPath $TempDir
    $BinaryPath = Join-Path $TempDir "$BinaryName.exe"

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $InstallPath = Join-Path $InstallDir "$BinaryName.exe"
    Copy-Item $BinaryPath $InstallPath -Force
    Write-OK "Installed docs-harness $Version → $InstallPath"

    # Ensure install dir is on PATH.
    $UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
        Write-Info "Added $InstallDir to user PATH (restart your shell to pick it up)"
    }

    Write-Host ""
    & $InstallPath @args

} finally {
    Remove-Item $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
