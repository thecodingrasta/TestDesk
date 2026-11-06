[CmdletBinding()]
param([switch]$Build)
$ErrorActionPreference = "Stop"
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    $unformatted = & gofmt -l cmd internal scripts/sync-agent-assets main.go
    if ($LASTEXITCODE -ne 0 -or $unformatted) { throw "Go formatting failed: $unformatted" }
    & go mod verify
    if ($LASTEXITCODE -ne 0) { throw "Module verification failed." }
    & go test ./... -count=1 -timeout=10m
    if ($LASTEXITCODE -ne 0) { throw "Go tests failed." }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "Go vet failed." }
    & node --check internal/desktop/assets/app.js
    if ($LASTEXITCODE -ne 0) { throw "JavaScript syntax check failed." }
    $tests = Get-ChildItem internal/desktop/testdata/*.test.cjs | ForEach-Object { $_.FullName }
    & node --test @tests
    if ($LASTEXITCODE -ne 0) { throw "JavaScript tests failed." }
    & go run ./scripts/sync-agent-assets -check
    if ($LASTEXITCODE -ne 0) { throw "Agent assets are out of date." }
    & python scripts/check_repository.py
    if ($LASTEXITCODE -ne 0) { throw "Repository checks failed." }
    & python scripts/check_version.py
    if ($LASTEXITCODE -ne 0) { throw "Version strings differ." }
    & python -m unittest discover -s scripts -p 'test_*.py'
    if ($LASTEXITCODE -ne 0) { throw "Script tests failed." }
    if ($Build) { & ./scripts/windows/Build-TestDesk.ps1 }
} finally { Pop-Location }
