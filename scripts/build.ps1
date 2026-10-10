$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath (Join-Path $projectRoot 'src')
try {
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
$releaseRoot = Join-Path $projectRoot 'release'
New-Item -ItemType Directory -Force -Path $releaseRoot | Out-Null
$executable = Join-Path $releaseRoot 'ustc-marathon.exe'
$pendingExecutable = Join-Path $releaseRoot 'ustc-marathon.exe.pending'
& $goExecutable build -trimpath -ldflags='-s -w' -o $pendingExecutable .
if ($LASTEXITCODE -ne 0) { throw '构建失败。' }
Add-Type -AssemblyName Microsoft.VisualBasic
if (Test-Path -LiteralPath $executable) { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($executable,'OnlyErrorDialogs','SendToRecycleBin') }
Move-Item -LiteralPath $pendingExecutable -Destination $executable
Copy-Item -LiteralPath (Join-Path $projectRoot 'src\config.example.toml') -Destination (Join-Path $releaseRoot 'config.example.toml')
foreach ($name in @('README.md','LICENSE')) { Copy-Item -LiteralPath (Join-Path $projectRoot $name) -Destination (Join-Path $releaseRoot $name) }
Copy-Item -LiteralPath (Join-Path $projectRoot 'docs\VERIFICATION.md') -Destination (Join-Path $releaseRoot 'VERIFICATION.md')
foreach ($name in @('run.cmd','uninstall.ps1')) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $releaseRoot $name) }
@{application='USTC_Marathon_AutoSignup';layout_version=1} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $releaseRoot '.ustc-marathon.json') -Encoding utf8NoBOM
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
$packagesRoot = Join-Path $releaseRoot 'packages'
New-Item -ItemType Directory -Force -Path $packagesRoot | Out-Null
$archive = Join-Path $packagesRoot 'USTC_Marathon_AutoSignup_v2.0.1_windows_amd64.zip'
if (Test-Path -LiteralPath $archive) { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($archive,'OnlyErrorDialogs','SendToRecycleBin') }
$releaseFiles = @('ustc-marathon.exe','config.example.toml','README.md','VERIFICATION.md','LICENSE','run.cmd','uninstall.ps1','THIRD_PARTY_LICENSES.txt','.ustc-marathon.json') | ForEach-Object { Join-Path $releaseRoot $_ }
Add-Type -AssemblyName System.IO.Compression
$stream=[System.IO.FileStream]::new($archive,[System.IO.FileMode]::CreateNew)
$zip=[System.IO.Compression.ZipArchive]::new($stream,[System.IO.Compression.ZipArchiveMode]::Create)
try { foreach ($path in $releaseFiles) { [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip,$path,(Split-Path -Leaf $path),[System.IO.Compression.CompressionLevel]::Optimal) | Out-Null } } finally { $zip.Dispose(); $stream.Dispose() }
$hashFile = Join-Path $packagesRoot 'SHA256.txt'
$hashes = @($executable, $archive) | ForEach-Object { $entry = Get-FileHash -LiteralPath $_ -Algorithm SHA256; $entry.Hash.ToLowerInvariant() + '  ' + (Split-Path -Leaf $_) }
$hashes | Set-Content -LiteralPath $hashFile -Encoding utf8NoBOM
Get-Item -LiteralPath $executable, $archive | Select-Object Name,@{Name='MiB';Expression={[math]::Round($_.Length / 1MB, 2)}},Length
} finally { Pop-Location }
