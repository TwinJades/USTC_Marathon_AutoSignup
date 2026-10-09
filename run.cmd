@echo off
cd /d "%~dp0"
if not exist "ustc-marathon.exe" (
  echo Please download the Windows release or run build.ps1.
  exit /b 1
)
"ustc-marathon.exe" %*
exit /b %ERRORLEVEL%
