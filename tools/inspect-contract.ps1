$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$sourcePath = Join-Path $projectRoot 'runtime\website-contract.js'
Invoke-WebRequest -Uri 'https://young.ustc.edu.cn/login/sc-wisdom-group-learning/static/js/chunk-0d0ec75b.13842608.js' -OutFile $sourcePath
$source = Get-Content -Raw -LiteralPath $sourcePath
if (-not $source.Contains('/item/scItem/queryApplyingMarathonItem')) { throw '前端资源结构发生变化。' }
foreach ($needle in @('"9222":function','showLook:function','title:"项目详情"','staticClass:"project','countdown=10')) {
    $index = $source.IndexOf($needle)
    while ($index -ge 0) {
        $start = [Math]::Max(0,$index - 180)
        $length = [Math]::Min(2500,$source.Length - $start)
        Write-Output $source.Substring($start,$length)
        $index = $source.IndexOf($needle,$index + $needle.Length)
    }
}
