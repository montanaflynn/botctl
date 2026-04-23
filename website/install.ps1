$ErrorActionPreference = 'Stop'

$Repo = "montanaflynn/botctl"
$InstallDir = "$env:LOCALAPPDATA\Programs\botctl"

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
} else {
    Write-Error "Unsupported: 32-bit systems are not supported"
    exit 1
}

# Get latest release tag
$Release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
$Tag = $Release.tag_name
$BaseUrl = "https://github.com/$Repo/releases/download/$Tag"

$Asset = "botctl-$Tag-windows-$Arch.zip"

Write-Host "Downloading botctl $Tag (windows/$Arch)..."
$TmpDir = Join-Path $env:TEMP "botctl-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
try {
    $TmpFile = Join-Path $TmpDir $Asset
    Invoke-WebRequest -Uri "$BaseUrl/$Asset" -OutFile $TmpFile

    # Verify checksum
    Write-Host "Verifying checksum..."
    $Checksums = (Invoke-WebRequest -Uri "$BaseUrl/checksums.txt").Content
    $Expected = ($Checksums -split "`n" | Where-Object { $_ -match "$Asset$" }) -replace '\s+.*', ''
    $Actual = (Get-FileHash $TmpFile -Algorithm SHA256).Hash.ToLower()

    if ($Actual -ne $Expected) {
        Write-Error "Checksum mismatch!`n  expected: $Expected`n  got:      $Actual"
        exit 1
    }

    # Extract
    Expand-Archive -Path $TmpFile -DestinationPath $TmpDir -Force
    $Bin = Join-Path $TmpDir "botctl.exe"
    if (-not (Test-Path $Bin)) {
        Write-Error "botctl.exe not found in archive"
        exit 1
    }

    # Install
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Move-Item -Path $Bin -Destination "$InstallDir\botctl.exe" -Force
} finally {
    Remove-Item -Path $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
}

# Add to user PATH if not already there
$UserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable('PATH', "$UserPath;$InstallDir", 'User')
    Write-Host "Added $InstallDir to PATH (restart your terminal to use)"
}

Write-Host "botctl $Tag installed successfully"
