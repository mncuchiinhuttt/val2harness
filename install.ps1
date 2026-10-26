# val2harness Windows Installer
# Usage in PowerShell:
#   irm https://raw.githubusercontent.com/mncuchiinhuttt/val2harness/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$repo = "mncuchiinhuttt/val2harness"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }

Write-Host "Detecting latest release for val2harness (Windows $arch)..." -ForegroundColor Cyan

$tag = "v1.0.0"
try {
    $releaseInfo = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ "User-Agent" = "PowerShell" }
    if ($releaseInfo.tag_name) {
        $tag = $releaseInfo.tag_name
    }
} catch {
    # Fallback to default tag if API rate limited
}

$zipName = "val2harness-windows-$arch.zip"
$downloadUrl = "https://github.com/$repo/releases/download/$tag/$zipName"

$installDir = Join-Path $HOME ".val2harness\bin"
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

$tempZip = Join-Path ([System.IO.Path]::GetTempPath()) "val2harness.zip"

Write-Host "Downloading $tag from $downloadUrl..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $downloadUrl -OutFile $tempZip -UseBasicParsing

Write-Host "Extracting val2harness.exe to $installDir..." -ForegroundColor Cyan
Expand-Archive -Path $tempZip -DestinationPath $installDir -Force
Remove-Item -Path $tempZip -Force

# Add to user PATH if not present
$userPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    $newPath = "$userPath;$installDir"
    [System.Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "Added $installDir to User PATH." -ForegroundColor Green
}

$env:Path = "$env:Path;$installDir"

Write-Host ""
Write-Host "[OK] val2harness $tag installed successfully!" -ForegroundColor Green
Write-Host "Location: $installDir\val2harness.exe"
Write-Host "Run 'val2harness' to launch the interactive TUI." -ForegroundColor Cyan
