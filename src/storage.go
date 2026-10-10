package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

var cachePaths=[]string{
	"Default/Cache","Default/Code Cache","Default/GPUCache","Default/DawnGraphiteCache","Default/DawnWebGPUCache",
	"Default/History","Default/History-journal","Default/Extensions","Default/EdgeCoupons","Default/Web Applications",
	"Default/settings_diagnostic.log",
	"GrShaderCache","ShaderCache","component_crx_cache","extensions_crx_cache","optimization_guide_model_store",
	"OptimizationGuideModelsManifest","EdgeOptimizationGuideModelsManifest","Edge Entity Extraction","EdgeLanguageDetectionModel",
	"Edge Sidebar","BrowserMetrics","BrowserMetrics-spare.pma","Crashpad","CrashpadMetrics-active.pma",
}

func within(root,path string) error {
	r,err:=filepath.Abs(root);if err!=nil{return err}
	p,err:=filepath.Abs(path);if err!=nil{return err}
	rel,err:=filepath.Rel(r,p);if err!=nil{return err}
	if rel=="."||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator)){return fmt.Errorf("操作目标超出允许目录")}
	for cursor:=p;cursor!=filepath.Dir(r);cursor=filepath.Dir(cursor){
		info,err:=os.Lstat(cursor)
		if err!=nil{if os.IsNotExist(err){continue};return err}
		if info.Mode()&os.ModeSymlink!=0{return fmt.Errorf("操作路径包含链接，停止清理")}
		if strings.EqualFold(cursor,r){break}
	}
	return nil
}

func treeSize(path string)(int64,error){
	var size int64
	err:=filepath.WalkDir(path,func(p string,d fs.DirEntry,err error)error{
		if err!=nil{return err}
		if d.Type()&os.ModeSymlink!=0{return fmt.Errorf("存储目录包含链接")}
		if !d.IsDir(){info,err:=d.Info();if err!=nil{return err};size+=info.Size()}
		return nil
	})
	if os.IsNotExist(err){return 0,nil}
	return size,err
}

type shellFileOperation struct{
	Window windows.Handle
	Operation uint32
	From *uint16
	To *uint16
	Flags uint16
	Aborted int32
	Mappings uintptr
	Title *uint16
}

func recycle(root,path string)error{
	if err:=within(root,path);err!=nil{return err}
	if _,err:=os.Lstat(path);os.IsNotExist(err){return nil}else if err!=nil{return err}
	abs,err:=filepath.Abs(path);if err!=nil{return err}
	volume,err:=windows.UTF16PtrFromString(filepath.VolumeName(abs)+`\`);if err!=nil{return err}
	if windows.GetDriveType(volume)!=windows.DRIVE_FIXED{return fmt.Errorf("回收站操作仅支持本机固定磁盘")}
	from,err:=windows.UTF16FromString(abs);if err!=nil{return err};from=append(from,0)
	op:=shellFileOperation{Operation:3,From:&from[0],Flags:0x40|0x10|0x400|0x4}
	code,_,_:=windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW").Call(uintptr(unsafe.Pointer(&op)))
	if code!=0||op.Aborted!=0{return fmt.Errorf("移入回收站失败：%s，Windows 返回 %d",filepath.Base(path),code)}
	return nil
}

func cacheTargets(root string)([]string,error){
	var targets []string
	for _,kind:=range []string{"chrome","edge"}{for _,relative:=range cachePaths{
		path:=filepath.Join(root,"sessions",kind,filepath.FromSlash(relative))
		if err:=within(root,path);err!=nil{return nil,err}
		if _,err:=os.Stat(path);err==nil{targets=append(targets,path)}else if !os.IsNotExist(err){return nil,err}
	}}
	return targets,nil
}

func cleanCaches(root string)error{
	targets,err:=cacheTargets(root);if err!=nil{return err}
	var total int64
	for _,path:=range targets{size,err:=treeSize(path);if err!=nil{return err};if err:=recycle(root,path);err!=nil{return err};total+=size}
	fmt.Printf("已将 %.2f MiB 的缓存移入回收站；认证数据与报名记录保留。\n",float64(total)/(1<<20))
	return nil
}

func showStorage()error{
	root:=filepath.Dir(configPath)
	total,err:=treeSize(runtimeRoot);if err!=nil{return err}
	sessions,err:=treeSize(filepath.Join(runtimeRoot,"sessions"));if err!=nil{return err}
	targets,err:=cacheTargets(runtimeRoot);if err!=nil{return err}
	var caches int64
	for _,path:=range targets{size,err:=treeSize(path);if err!=nil{return err};caches+=size}
	logs,err:=treeSize(filepath.Join(runtimeRoot,"activity.jsonl"));if err!=nil{return err}
	tests,err:=treeSize(filepath.Join(runtimeRoot,"verification"));if err!=nil{return err}
	program,err:=treeSize(filepath.Join(root,"ustc-marathon.exe"));if err!=nil{return err}
	fmt.Printf("程序：%.2f MiB\n运行数据总计：%.2f MiB\n浏览器可清理缓存：%.2f MiB\n浏览器认证与必要设置：%.2f MiB\n活动日志：%.2f MiB\n测试文件：%.2f MiB\n",float64(program)/(1<<20),float64(total)/(1<<20),float64(caches)/(1<<20),float64(sessions-caches)/(1<<20),float64(logs)/(1<<20),float64(tests)/(1<<20))
	fmt.Println("移入回收站的文件仍占用磁盘空间；清空回收站后释放对应空间。")
	return nil
}

func uninstall()error{
	executable,err:=os.Executable();if err!=nil{return err}
	root:=filepath.Dir(executable)
	if err:=within(filepath.Dir(root),root);err!=nil{return err}
	allowed:=map[string]bool{"ustc-marathon.exe":true,"run.cmd":true,"uninstall.ps1":true,"config.toml":true,"config.example.toml":true,"README.md":true,"docs":true,".ustc-marathon.json":true,"runtime":true,"packages":true}
	entries,err:=os.ReadDir(root);if err!=nil{return err}
	for _,entry:=range entries{if !allowed[entry.Name()]{return fmt.Errorf("发布目录包含范围外文件 %s，禁止卸载",entry.Name())}}
	if !strings.EqualFold(filepath.Dir(configPath),root){return fmt.Errorf("卸载要求配置与 EXE 位于同一发布目录")}
	for _,name:=range []string{".git","src"}{if _,err:=os.Stat(filepath.Join(root,name));err==nil{return fmt.Errorf("卸载目录包含源码或 Git 仓库，禁止清除")}else if !os.IsNotExist(err){return err}}
	marker,err:=os.ReadFile(filepath.Join(root,".ustc-marathon.json"));if err!=nil{return fmt.Errorf("缺少发布目录标识，禁止卸载")}
	var identity struct{Application string `json:"application"`;Layout int `json:"layout_version"`}
	if err:=json.Unmarshal(marker,&identity);err!=nil{return err}
	if identity.Application!="USTC_Marathon_AutoSignup"||identity.Layout!=1{return fmt.Errorf("发布目录标识无效")}
	var c Config
	if _,err:=os.Stat(configPath);err==nil{c,err=loadConfig();if err!=nil{return err}}else if !os.IsNotExist(err){return err}
	size,err:=treeSize(root);if err!=nil{return err}
	fmt.Printf("卸载目录：%s\n将移入回收站：%.2f MiB\n凭据范围：当前账户的本项目凭据（兼容两种名称）。\n",root,float64(size)/(1<<20))
	script:=filepath.Join(root,"uninstall.ps1")
	if _,err:=os.Stat(script);err!=nil{return err}
	powershell,err:=exec.LookPath("powershell.exe");if err!=nil{return err}
	arguments:=[]string{"-NoProfile","-ExecutionPolicy","Bypass","-File",script,"-ParentProcessId",fmt.Sprint(os.Getpid()),"-ReleaseRoot",root}
	if !term.IsTerminal(int(os.Stdin.Fd())){arguments=append(arguments,"-NoPause")}
	for i:=range arguments{arguments[i]=syscall.EscapeArg(arguments[i])}
	verb,err:=windows.UTF16PtrFromString("open");if err!=nil{return err}
	file,err:=windows.UTF16PtrFromString(powershell);if err!=nil{return err}
	params,err:=windows.UTF16PtrFromString(strings.Join(arguments," "));if err!=nil{return err}
	cwd,err:=windows.UTF16PtrFromString(filepath.Dir(root));if err!=nil{return err}
	if err:=deletePassword(c.Monitor.Username);err!=nil{return err}
	if err:=windows.ShellExecute(0,verb,file,params,cwd,windows.SW_SHOW);err!=nil{return fmt.Errorf("无法启动卸载脚本：%w",err)}
	fmt.Println("卸载脚本已经启动，等待当前程序退出后清除运行目录。")
	return nil
}
