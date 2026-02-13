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

$Asset = "botctl-windows-$Arch"

# Get latest release tag
$Release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
$Tag = $Release.tag_name
$BaseUrl = "https://github.com/$Repo/releases/download/$Tag"

Write-Host "Downloading botctl $Tag (windows/$Arch)..."
$TmpFile = Join-Path $env:TEMP "botctl-download.exe"
Invoke-WebRequest -Uri "$BaseUrl/$Asset" -OutFile $TmpFile

# Verify checksum
Write-Host "Verifying checksum..."
$Checksums = (Invoke-WebRequest -Uri "$BaseUrl/checksums.txt").Content
$Expected = ($Checksums -split "`n" | Where-Object { $_ -match "$Asset$" }) -replace '\s+.*', ''
$Actual = (Get-FileHash $TmpFile -Algorithm SHA256).Hash.ToLower()

if ($Actual -ne $Expected) {
    Remove-Item $TmpFile -Force
    Write-Error "Checksum mismatch!`n  expected: $Expected`n  got:      $Actual"
    exit 1
}

# Install
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
Move-Item -Path $TmpFile -Destination "$InstallDir\botctl.exe" -Force

# Add to user PATH if not already there
$UserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable('PATH', "$UserPath;$InstallDir", 'User')
    Write-Host "Added $InstallDir to PATH (restart your terminal to use)"
}

Write-Host "botctl $Tag installed successfully"
