$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
$releaseRoot=Join-Path $projectRoot 'release'
$evidenceRoot=Join-Path $releaseRoot 'runtime\verification'
$destination=Join-Path $evidenceRoot '中文解压验收'
if (Test-Path -LiteralPath $destination) { throw '解压验收目录必须为空。' }
$archive=Join-Path $releaseRoot 'packages\USTC_Marathon_AutoSignup_v2.0.2_windows_amd64.zip'
Expand-Archive -LiteralPath $archive -DestinationPath $destination
$expected=@('ustc-marathon.exe','config.example.toml','README.md','docs\LICENSE','run.cmd','uninstall.ps1','docs\THIRD_PARTY_LICENSES.txt','.ustc-marathon.json')
$actual=@(Get-ChildItem -LiteralPath $destination -File -Force -Recurse | ForEach-Object {[System.IO.Path]::GetRelativePath($destination,$_.FullName)})
if (Compare-Object $expected $actual) { throw '发布包内容不符合要求。' }
$executable=Join-Path $destination 'ustc-marathon.exe'
if ((Get-FileHash -LiteralPath $executable).Hash -ne (Get-FileHash -LiteralPath (Join-Path $releaseRoot 'ustc-marathon.exe')).Hash) { throw '解压后的 EXE 校验失败。' }
Push-Location -LiteralPath $evidenceRoot
try {
    foreach ($arguments in @(@('--version'),@('--help'),@('browser','list'),@('storage'),@('status'))) {
        & $executable @arguments
        if ($LASTEXITCODE -ne 0) { throw '干净目录命令验证失败。' }
    }
    Copy-Item -LiteralPath (Join-Path $destination 'config.example.toml') -Destination (Join-Path $destination 'config.toml')
    & $executable validate
    if ($LASTEXITCODE -eq 0) { throw '空白配置未被拒绝。' }
    $outsideFile=Join-Path $destination '其他文件.txt'
    Set-Content -LiteralPath $outsideFile -Value '独立目录范围保护验证' -Encoding utf8
    & $executable uninstall
    if ($LASTEXITCODE -eq 0 -or -not (Test-Path -LiteralPath $destination)) { throw '卸载范围保护失效。' }
    Add-Type -AssemblyName Microsoft.VisualBasic
    [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($outsideFile,'OnlyErrorDialogs','SendToRecycleBin')
    & $executable uninstall
    if ($LASTEXITCODE -ne 0) { throw '干净发布包卸载失败。' }
    $deadline=[DateTime]::UtcNow.AddSeconds(30)
    while (Test-Path -LiteralPath $destination) {
        if ([DateTime]::UtcNow -gt $deadline) { throw '干净发布包卸载尚未完成。' }
        Start-Sleep -Milliseconds 250
    }
} finally { Pop-Location }
@{time=[DateTimeOffset]::Now.ToString('o');members=$actual.Count;extracted_executable_hash_matches=$true;different_working_directory=$true;blank_config_rejected=$true;unrelated_file_guard=$true;actual_uninstall_completed=$true} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $evidenceRoot 'release-result.json') -Encoding utf8NoBOM
Write-Output '干净中文目录的 EXE、内容、工作目录和实际卸载验收通过。'
