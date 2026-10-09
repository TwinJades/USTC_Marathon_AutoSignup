param([switch]$Apply, [switch]$Documentation, [switch]$CacheOnly, [switch]$KeepPublishingTools)
$ErrorActionPreference = 'Stop'
$projectRoot = [System.IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$prefix = $projectRoot.TrimEnd('\') + '\'
$relativePaths = if ($Documentation) { @('README.md','VERIFICATION.md') } elseif ($CacheOnly) {
    foreach ($browser in @('chrome','edge')) {
        foreach ($cache in @('Default\Cache','Default\Code Cache','Default\GPUCache','Default\DawnGraphiteCache','Default\DawnWebGPUCache','GrShaderCache','ShaderCache','component_crx_cache','extensions_crx_cache','optimization_guide_model_store','OptimizationGuideModelsManifest','Edge Entity Extraction','EdgeLanguageDetectionModel')) {
            "runtime\sessions\$browser\$cache"
        }
    }
} else {
    @('.venv','ustc_marathon','tests','pyproject.toml','requirements.txt','setup.ps1',
      'tools\inspect_public.py','tools\verify_public.py','tools\install_github_cli.ps1',
      'tools\__pycache__',
      'runtime\browsers','runtime\github-cli','runtime\downloads','runtime\public-verification-profile',
      'runtime\local-verification','runtime\local-go-verification','runtime\browser-profile','runtime\build',
      'runtime\release-verification','runtime\interactive-verification')
}
$targets = foreach ($relativePath in $relativePaths) {
    if ($KeepPublishingTools -and $relativePath -eq 'runtime\github-cli') { continue }
    $absolute = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $relativePath))
    if (-not $absolute.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) { throw '回收目标超出项目目录。' }
    if (Test-Path -LiteralPath $absolute) { Get-Item -Force -LiteralPath $absolute }
}
$targets | Select-Object FullName,PSIsContainer
if (-not $Apply) { return }
if (-not $Documentation -and -not $CacheOnly) {
    foreach ($browser in @('chrome','edge')) {
        $evidencePath = Join-Path $projectRoot ("runtime\verification-go-$browser.json")
        $evidence = Get-Content -Raw -LiteralPath $evidencePath | ConvertFrom-Json
        if (-not $evidence.account_matches -or -not $evidence.headless_session_reused) { throw '新会话尚未完成登录核实，停止回收原会话。' }
    }
}
Add-Type -AssemblyName Microsoft.VisualBasic
foreach ($target in $targets) {
    if ($target.PSIsContainer) {
        [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($target.FullName, 'OnlyErrorDialogs', 'SendToRecycleBin')
    } else {
        [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($target.FullName, 'OnlyErrorDialogs', 'SendToRecycleBin')
    }
}
