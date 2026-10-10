$ErrorActionPreference='Stop'
$developerRoot=Join-Path $env:LOCALAPPDATA 'Programs\GitHub-CLI'
if (Test-Path -LiteralPath $developerRoot) { throw 'GitHub CLI 开发工具目录已经存在。' }
$projectRoot=Split-Path -Parent $PSScriptRoot
$downloadRoot=Join-Path $projectRoot 'release\runtime\verification\publisher-download'
New-Item -ItemType Directory -Force -Path $downloadRoot | Out-Null
$metadata=Invoke-RestMethod -Uri 'https://api.github.com/repos/cli/cli/releases/latest'
$asset=@($metadata.assets | Where-Object name -Match '^gh_[0-9.]+_windows_amd64.zip$')
if ($asset.Count -ne 1 -or -not $asset[0].digest) { throw 'GitHub CLI 官方发布资源不完整。' }
$archive=Join-Path $downloadRoot $asset[0].name
Invoke-WebRequest -Uri $asset[0].browser_download_url -OutFile $archive
if ($asset[0].digest -ne ('sha256:'+(Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant())) { throw 'GitHub CLI 校验失败。' }
Expand-Archive -LiteralPath $archive -DestinationPath $developerRoot
& (Join-Path $developerRoot 'bin\gh.exe') version
if ($LASTEXITCODE -ne 0) { throw 'GitHub CLI 验证失败。' }
