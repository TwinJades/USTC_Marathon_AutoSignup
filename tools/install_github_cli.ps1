$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$toolRoot = Join-Path $projectRoot 'runtime\github-cli'
New-Item -ItemType Directory -Force -Path $toolRoot | Out-Null
$release = Invoke-RestMethod -Uri 'https://api.github.com/repos/cli/cli/releases/latest'
$asset = @($release.assets | Where-Object { $_.name -match '^gh_.*_windows_amd64.zip$' })
if ($asset.Count -ne 1) { throw 'GitHub CLI 官方发布资源未能唯一确定。' }
$archive = Join-Path $toolRoot $asset[0].name
Invoke-WebRequest -Uri $asset[0].browser_download_url -OutFile $archive
Expand-Archive -LiteralPath $archive -DestinationPath $toolRoot -Force
$executable = Join-Path $toolRoot 'bin\gh.exe'
if (-not (Test-Path -LiteralPath $executable)) { throw 'GitHub CLI 解压目录缺少执行文件。' }
& $executable --version
if ($LASTEXITCODE -ne 0) { throw 'GitHub CLI 版本检查失败。' }
