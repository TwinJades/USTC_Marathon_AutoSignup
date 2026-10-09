$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
function Invoke-CheckedPython {
    param([string]$Executable, [string[]]$Arguments)
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Python 命令执行失败，退出代码：$LASTEXITCODE" }
}
Invoke-CheckedPython -Executable 'python' -Arguments @('-c', 'import sys; assert sys.version_info[:2] == (3, 12), "需要 Python 3.12"')
Invoke-CheckedPython -Executable 'python' -Arguments @('-m', 'venv', '.venv')
$projectPython = Join-Path $PSScriptRoot '.venv\Scripts\python.exe'
Invoke-CheckedPython -Executable $projectPython -Arguments @('-m', 'pip', 'install', '--disable-pip-version-check', '-r', 'requirements.txt')
$env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $PSScriptRoot 'runtime\browsers'
Invoke-CheckedPython -Executable $projectPython -Arguments @('-m', 'playwright', 'install', 'chromium')
if (-not (Test-Path -LiteralPath 'config.toml')) { Copy-Item -LiteralPath 'config.example.toml' -Destination 'config.toml' }
Write-Host '安装完成。请填写 config.toml，然后运行 .\run.cmd --help。'
