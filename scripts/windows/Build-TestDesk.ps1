[CmdletBinding()]
param([switch]$Launch, [string]$Project = "", [string]$Platform = "windows/amd64", [string]$OutputDirectory = "dist")
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Push-Location $root
try {
    if ($env:OS -ne "Windows_NT" -or $Platform -notmatch '^windows/(amd64|arm64)$') {
        throw "Use this script on Windows for windows/amd64 or windows/arm64. See scripts/build.sh for Linux/macOS."
    }
    $goVersion = & go version
    if ($LASTEXITCODE -ne 0) { throw "Go is required. See docs/INSTALL.md." }
    Write-Host $goVersion
    $expected = (& go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)
    if ($LASTEXITCODE -ne 0 -or -not $expected) { throw "Could not read the Wails version from go.mod." }
    $wailsVersion = & wails version
    if ($LASTEXITCODE -ne 0 -or ($wailsVersion -join " ") -notmatch [regex]::Escape($expected)) {
        throw "Install Wails ${expected}: go install github.com/wailsapp/wails/v2/cmd/wails@$expected"
    }
    # A runner's transient network error should cost a retry, not the job.
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        & go mod download
        if ($LASTEXITCODE -eq 0) { break }
        if ($attempt -eq 3) { throw "Go module download failed after 3 attempts." }
        Write-Warning "Go module download failed (attempt $attempt of 3)."
        Start-Sleep -Seconds ($attempt * 5)
    }
    # All embedded assets and bindings are checked in. Skip Wails' legacy
    # package loader, which cannot read the patched Go toolchain's export data.
    & wails build -clean -platform $Platform -webview2 error -skipbindings -skipembedcreate -m -nosyncgomod -o TestDesk.exe -trimpath
    if ($LASTEXITCODE -ne 0) { throw "Wails build failed." }
    New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
    $output = (Resolve-Path -LiteralPath $OutputDirectory).Path
    Copy-Item -LiteralPath build/bin/TestDesk.exe -Destination (Join-Path $output 'TestDesk.exe') -Force
    $previousGOOS, $previousGOARCH = $env:GOOS, $env:GOARCH
    try {
        $env:GOOS = 'windows'
        $env:GOARCH = $Platform.Split('/')[1]
        & go build -trimpath -o (Join-Path $output 'testdesk-cli.exe') ./cmd/testdesk
        if ($LASTEXITCODE -ne 0) { throw "CLI build failed." }
    } finally {
        $env:GOOS, $env:GOARCH = $previousGOOS, $previousGOARCH
    }
    Copy-Item -LiteralPath LICENSE,THIRD_PARTY_NOTICES.md -Destination $output -Force
    $thirdParty = Join-Path $output 'third_party'
    New-Item -ItemType Directory -Force -Path $thirdParty | Out-Null
    Copy-Item -LiteralPath third_party/licenses -Destination $thirdParty -Recurse -Force
    Write-Host "Built desktop and CLI in $output"
    if ($Launch) {
        $exe = Join-Path $output 'TestDesk.exe'
        if ($Project) {
            Start-Process -FilePath $exe -ArgumentList @('--project', ('"{0}"' -f $Project)) -WindowStyle Hidden
        } else {
            Start-Process -FilePath $exe -WindowStyle Hidden
        }
    }
} finally {
    Pop-Location
}
