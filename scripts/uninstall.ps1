param([Parameter(Mandatory)][int]$ParentProcessId,[Parameter(Mandatory)][string]$ReleaseRoot,[switch]$NoPause)
$ErrorActionPreference='Stop'
$target=[System.IO.Path]::GetFullPath($ReleaseRoot).TrimEnd('\')
$scriptRoot=[System.IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\')
if ($target -ne $scriptRoot -or $target -eq [System.IO.Path]::GetPathRoot($target).TrimEnd('\')) { throw '卸载目录无效。' }
$item=Get-Item -LiteralPath $target -Force
if ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) { throw '卸载目录包含链接。' }
foreach ($name in @('.git','src')) { if (Test-Path -LiteralPath (Join-Path $target $name)) { throw '卸载目录包含源码或 Git 仓库。' } }
$allowed=@('ustc-marathon.exe','run.cmd','uninstall.ps1','config.toml','config.example.toml','README.md','VERIFICATION.md','LICENSE','THIRD_PARTY_LICENSES.txt','.ustc-marathon.json','runtime','packages')
foreach ($entry in Get-ChildItem -LiteralPath $target -Force) { if ($entry.Name -notin $allowed) { throw '卸载目录包含范围外文件。' } }
$identity=Get-Content -Raw -LiteralPath (Join-Path $target '.ustc-marathon.json') | ConvertFrom-Json
if ($identity.application -ne 'USTC_Marathon_AutoSignup' -or $identity.layout_version -ne 1) { throw '发布目录标识无效。' }
$parent=Get-Process -Id $ParentProcessId -ErrorAction SilentlyContinue
if ($parent) {
    if ($parent.Path -ne (Join-Path $target 'ustc-marathon.exe')) { throw '卸载调用进程无效。' }
    if (-not $parent.WaitForExit(30000)) { throw '程序尚未退出，停止卸载。' }
}
$lockPath=Join-Path $target 'runtime\process.lock'
$runtimePath=Join-Path $target 'runtime'
if (Test-Path -LiteralPath $runtimePath) {
    $uninstallLock=[System.IO.FileStream]::new($lockPath,[System.IO.FileMode]::OpenOrCreate,[System.IO.FileAccess]::ReadWrite,([System.IO.FileShare]::ReadWrite -bor [System.IO.FileShare]::Delete))
    try {
        $uninstallLock.Lock(0,1)
        $linked=Get-ChildItem -LiteralPath $target -Recurse -Force | Where-Object { $_.Attributes -band [System.IO.FileAttributes]::ReparsePoint }
        if ($linked) { throw '卸载目录内部包含链接。' }
        $browserProcesses=@(Get-CimInstance Win32_Process | Where-Object { $_.Name -in @('chrome.exe','msedge.exe') -and $_.CommandLine -and $_.CommandLine.Contains($target+'\runtime\sessions\') })
        if ($browserProcesses.Count -ne 0) { throw '程序浏览器尚未退出，停止卸载。' }
        $bytes=(Get-ChildItem -LiteralPath $target -Recurse -File -Force | Measure-Object Length -Sum).Sum
        $drive=[System.IO.DriveInfo]::new([System.IO.Path]::GetPathRoot($target))
        $freeBefore=$drive.AvailableFreeSpace
        Set-Content -LiteralPath (Join-Path $runtimePath 'uninstalling') -Value 'USTC_Marathon_AutoSignup' -Encoding utf8
        $uninstallLock.Dispose()
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class MarathonRecycle {
    [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)]
    public struct Operation {
        public IntPtr Window;
        public uint Function;
        public IntPtr From;
        public IntPtr To;
        public ushort Flags;
        public int Aborted;
        public IntPtr Mappings;
        public IntPtr Title;
    }
    [DllImport("shell32.dll", CharSet=CharSet.Unicode)]
    public static extern int SHFileOperationW(ref Operation operation);
    public static void Delete(string path) {
        IntPtr source=Marshal.StringToHGlobalUni(path+"\0");
        try {
            Operation operation=new Operation();
            operation.Function=3; operation.From=source; operation.Flags=0x454;
            int result=SHFileOperationW(ref operation);
            if(result!=0 || operation.Aborted!=0) throw new System.IO.IOException("Recycle Bin operation failed: "+result);
        } finally { Marshal.FreeHGlobal(source); }
    }
}
'@
        [MarathonRecycle]::Delete($target)
        $freeAfter=$drive.AvailableFreeSpace
        Write-Output ('已经移入回收站：'+[math]::Round($bytes/1MB,2)+' MiB')
        Write-Output ('磁盘可用空间变化：'+[math]::Round(($freeAfter-$freeBefore)/1MB,2)+' MiB')
        Write-Output '文件进入回收站后仍占用磁盘空间，清空回收站后释放对应空间。'
    } finally { $uninstallLock.Dispose() }
} else { throw '缺少运行目录，无法核查单实例锁。' }
if (-not $NoPause) { Read-Host '卸载完成，按回车关闭窗口' | Out-Null }
