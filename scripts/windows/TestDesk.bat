@echo off
setlocal
set "ROOT=%~dp0..\..\"
if not exist "%ROOT%dist\TestDesk.exe" (
  echo Build TestDesk first: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\windows\Build-TestDesk.ps1
  exit /b 1
)
start "" "%ROOT%dist\TestDesk.exe" %*
