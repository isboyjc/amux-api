@echo off
setlocal

cd /d "%~dp0"

set "GO_EXE="
where go.exe >nul 2>nul
if not errorlevel 1 set "GO_EXE=go.exe"

if not defined GO_EXE if exist "%ProgramFiles%\Go\bin\go.exe" (
  set "GO_EXE=%ProgramFiles%\Go\bin\go.exe"
)

if not defined GO_EXE (
  echo [dev] Go was not found. Install 64-bit Go and reopen the terminal.
  exit /b 1
)

"%GO_EXE%" run .\scripts\devwindows
exit /b %errorlevel%
