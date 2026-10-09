@echo off
cd /d "%~dp0"
if not exist ".venv\Scripts\python.exe" (
  echo Please run setup.ps1 first.
  exit /b 1
)
set "PLAYWRIGHT_BROWSERS_PATH=%~dp0runtime\browsers"
set "PYTHONUTF8=1"
".venv\Scripts\python.exe" -m ustc_marathon %*
exit /b %ERRORLEVEL%
