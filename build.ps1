$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$goCommand = Get-Command go -ErrorAction SilentlyContinue
$goExecutable = if ($goCommand) { $goCommand.Source } else { Join-Path $env:LOCALAPPDATA 'Programs\USTC-Go\go\bin\go.exe' }
if (-not (Test-Path -LiteralPath $goExecutable)) { throw '需要安装 Go 1.27.2。' }
if ((& $goExecutable version) -notmatch 'go1\.27\.2 ') { throw '构建需要 Go 1.27.2。' }
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
& $goExecutable mod verify
if ($LASTEXITCODE -ne 0) { throw '依赖校验失败。' }
& $goExecutable test ./...
if ($LASTEXITCODE -ne 0) { throw '本地测试失败。' }
& $goExecutable vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Go 静态检查失败。' }
& $goExecutable build -trimpath -ldflags='-s -w' -o ustc-marathon.exe .
if ($LASTEXITCODE -ne 0) { throw '构建失败。' }
$releaseRoot = Join-Path $PSScriptRoot 'dist'
New-Item -ItemType Directory -Force -Path $releaseRoot | Out-Null
$moduleNames = @(& $goExecutable list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}@{{.Version}}{{end}}{{end}}' . | Where-Object { $_ } | Sort-Object -Unique)
if ($LASTEXITCODE -ne 0) { throw '读取依赖清单失败。' }
$licenseFile = Join-Path $releaseRoot 'THIRD_PARTY_LICENSES.txt'
$licenseText = [System.Collections.Generic.List[string]]::new()
$licenseText.Add('Go:')
$licenseText.Add((Get-Content -Raw -LiteralPath (Join-Path (& $goExecutable env GOROOT) 'LICENSE')))
for ($index = 0; $index -lt $moduleNames.Count; $index++) {
    $moduleDirectory = & $goExecutable list -m -f '{{.Dir}}' $moduleNames[$index]
    if ($LASTEXITCODE -ne 0 -or -not $moduleDirectory) { throw '读取依赖目录失败。' }
    $license = Get-ChildItem -LiteralPath $moduleDirectory -File | Where-Object { $_.Name -match '^(LICENSE|COPYING)(\..*)?$' } | Select-Object -First 1
    if ($moduleNames[$index] -eq 'github.com/mattn/go-localereader@v0.0.1') { $license = Get-Item -LiteralPath (Join-Path $moduleDirectory 'README.md') }
    if (-not $license) { throw "依赖缺少许可证：$($moduleNames[$index])" }
    $licenseText.Add("`n$($moduleNames[$index]):")
    $licenseText.Add((Get-Content -Raw -LiteralPath $license.FullName))
}
$licenseText -join "`n" | Set-Content -LiteralPath $licenseFile -Encoding utf8NoBOM
$archive = Join-Path $releaseRoot 'USTC_Marathon_AutoSignup_v2.0.0_windows_amd64.zip'
if (Test-Path -LiteralPath $archive) { throw '发布压缩包已经存在，请先将旧压缩包移入回收站。' }
Compress-Archive -LiteralPath @('ustc-marathon.exe','config.example.toml','README.md','VERIFICATION.md','LICENSE','run.cmd',$licenseFile) -DestinationPath $archive -CompressionLevel Optimal
$hashFile = Join-Path $releaseRoot 'SHA256.txt'
$hashes = @('ustc-marathon.exe', $archive) | ForEach-Object { $entry = Get-FileHash -LiteralPath $_ -Algorithm SHA256; $entry.Hash.ToLowerInvariant() + '  ' + (Split-Path -Leaf $_) }
$hashes | Set-Content -LiteralPath $hashFile -Encoding utf8NoBOM
Get-Item -LiteralPath 'ustc-marathon.exe', $archive | Select-Object Name,@{Name='MiB';Expression={[math]::Round($_.Length / 1MB, 2)}},Length
