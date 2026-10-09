$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$downloadRoot = Join-Path $projectRoot 'runtime\downloads'
New-Item -ItemType Directory -Path $downloadRoot -Force | Out-Null
$releases = Invoke-RestMethod -Uri 'https://go.dev/dl/?mode=json&include=all'
$release = $releases | Where-Object version -EQ 'go1.27.2'
if (-not $release) { throw '官方清单没有 Go 1.27.2。' }
$package = $release.files | Where-Object { $_.os -eq 'windows' -and $_.arch -eq 'amd64' -and $_.kind -eq 'archive' }
if (@($package).Count -ne 1) { throw 'Go 安装包清单不完整。' }
$installer = Join-Path $downloadRoot $package.filename
Invoke-WebRequest -Uri ('https://go.dev/dl/' + $package.filename) -OutFile $installer
if ((Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToLowerInvariant() -ne $package.sha256) { throw 'Go 安装包 SHA256 不一致。' }
$developerRoot = Join-Path $env:LOCALAPPDATA 'Programs\USTC-Go'
if (Test-Path -LiteralPath $developerRoot) { throw 'Go 开发工具目录已经存在，请核查现有版本。' }
Expand-Archive -LiteralPath $installer -DestinationPath $developerRoot
& (Join-Path $developerRoot 'go\bin\go.exe') version
if ($LASTEXITCODE -ne 0) { throw 'Go 版本验证失败。' }
