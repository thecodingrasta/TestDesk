[CmdletBinding()]
param(
    [string]$Source = "",
    [switch]$DesktopShortcut,
    [string]$InstallRoot = "",
    [switch]$NoStartMenuShortcut,
    [switch]$NoLaunch
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$source = if ($Source) { $Source } else { Join-Path $root "dist\TestDesk.exe" }
if (-not (Test-Path -LiteralPath $source)) {
    throw "TestDesk.exe is missing. Build with scripts/windows/Build-TestDesk.ps1 first, or use -Source for a downloaded desktop executable."
}

$installDir = if ($InstallRoot -ne "") { $InstallRoot } else { Join-Path $env:LOCALAPPDATA "Programs\TestDesk" }
$target = Join-Path $installDir "TestDesk.exe"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Copy-Item -LiteralPath $source -Destination $target -Force
$packageDir = Split-Path -Parent (Resolve-Path -LiteralPath $source).Path
foreach ($name in @('LICENSE', 'THIRD_PARTY_NOTICES.md', 'testdesk-cli.exe')) {
    $asset = Join-Path $packageDir $name
    if (Test-Path -LiteralPath $asset) { Copy-Item -LiteralPath $asset -Destination $installDir -Force }
}
$notices = Join-Path $packageDir 'third_party'
if (Test-Path -LiteralPath $notices) { Copy-Item -LiteralPath $notices -Destination $installDir -Recurse -Force }

$shell = $null
if (-not $NoStartMenuShortcut -or $DesktopShortcut) {
    $shell = New-Object -ComObject WScript.Shell
}
if (-not $NoStartMenuShortcut) {
    $startMenu = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
    $shortcut = $shell.CreateShortcut((Join-Path $startMenu "TestDesk.lnk"))
    $shortcut.TargetPath = $target
    $shortcut.WorkingDirectory = $installDir
    $shortcut.Description = "TestDesk"
    $shortcut.Save()
}

if ($DesktopShortcut) {
    $desktop = [Environment]::GetFolderPath("Desktop")
    $desktopLink = $shell.CreateShortcut((Join-Path $desktop "TestDesk.lnk"))
    $desktopLink.TargetPath = $target
    $desktopLink.WorkingDirectory = $installDir
    $desktopLink.Description = "TestDesk"
    $desktopLink.Save()
}

Write-Host "TestDesk installed to $target" -ForegroundColor Green
if (-not $NoLaunch) {
    Start-Process -FilePath $target -WindowStyle Hidden
}
