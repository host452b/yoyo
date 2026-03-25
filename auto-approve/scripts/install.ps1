# Installation script for aaa (auto-approve-agent) on Windows

param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

# Allow env var override (for iwr | iex usage)
if (-not $Version) {
    $Version = if ($env:AAA_VERSION) { $env:AAA_VERSION } else { "0.0.8" }
}

# Configuration
$InstallDir = Join-Path $env:USERPROFILE ".local\bin"
$PackageName = "aaa"
$BinaryName = "$PackageName.exe"
$RepoUrl = "https://gitlab-master.nvidia.com/api/v4/projects/241133/packages/generic/aaa"

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
}

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
    exit 1
}

function Get-Platform {
    $arch = $env:PROCESSOR_ARCHITECTURE

    switch ($arch) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "aarch64" }
        default { Write-ErrorMsg "Unsupported architecture: $arch" }
    }
}

function Download-Binary {
    param([string]$Arch)

    $downloadUrl = "$RepoUrl/$Version/$PackageName-windows-$Arch.exe"
    $tempFile = Join-Path $env:TEMP "aaa-temp.exe"

    Write-Info "Downloading aaa from $downloadUrl..."

    try {
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tempFile -UseBasicParsing
    } catch {
        Write-ErrorMsg "Failed to download binary: $_"
    }

    return $tempFile
}

function Install-Binary {
    param([string]$TempFile)

    # Create installation directory if it doesn't exist
    if (-not (Test-Path $InstallDir)) {
        Write-Info "Creating installation directory: $InstallDir"
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    $installPath = Join-Path $InstallDir $BinaryName

    # Remove old binary if exists
    if (Test-Path $installPath) {
        Write-Info "Removing old installation..."
        Remove-Item $installPath -Force
    }

    # Move binary to installation directory
    Write-Info "Installing to $installPath..."
    Move-Item $TempFile $installPath -Force

    Write-Info "Installation complete!"
}

function Update-Path {
    # Check if InstallDir is already in PATH
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")

    if ($currentPath -like "*$InstallDir*") {
        Write-Info "$InstallDir is already in PATH"
        return
    }

    Write-Warn "$InstallDir is not in your PATH"
    Write-Info "Adding $InstallDir to user PATH..."

    # Add to user PATH
    $newPath = "$InstallDir;$currentPath"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")

    # Update current session PATH
    $env:Path = "$InstallDir;$env:Path"

    Write-Info "PATH updated successfully"
}

# Main
function Main {
    Write-Info "Installing aaa (auto-approve-agent)..."

    $arch = Get-Platform
    Write-Info "Detected architecture: $arch"

    $tempFile = Download-Binary -Arch $arch
    Install-Binary -TempFile $tempFile
    Update-Path

    Write-Info ""
    Write-Info "aaa has been installed successfully!"
    Write-Info "Run 'aaa --help' to get started"
    Write-Info ""
    Write-Info "Note: You may need to restart your terminal for PATH changes to take effect"
}

Main
