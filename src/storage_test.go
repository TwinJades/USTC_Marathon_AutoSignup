package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRecyclePreservesAuthentication(t *testing.T){
	root:=testRoot(t)
	cache:=filepath.Join(root,"sessions","chrome","Default","Cache")
	cookies:=filepath.Join(root,"sessions","chrome","Default","Network","Cookies")
	for _,path:=range []string{cache,filepath.Dir(cookies)}{if err:=os.MkdirAll(path,0700);err!=nil{t.Fatal(err)}}
	if err:=os.WriteFile(filepath.Join(cache,"本地文件验证"),make([]byte,1024),0600);err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(cookies,[]byte("本地保留文件验证"),0600);err!=nil{t.Fatal(err)}
	if err:=cleanCaches(root);err!=nil{t.Fatal(err)}
	if _,err:=os.Stat(cache);!os.IsNotExist(err){t.Fatal("缓存未移入回收站")}
	if _,err:=os.Stat(cookies);err!=nil{t.Fatal("保留文件发生变化",err)}
	if err:=recycle(root,filepath.Dir(root));err==nil{t.Fatal("范围外路径未被拒绝")}
}

func TestReleaseUninstall(t *testing.T){
	if os.Getenv("USTC_RELEASE_VERIFY")!="1"{t.Skip("解压与卸载验收需要构建完成后启用 USTC_RELEASE_VERIFY=1")}
	root:=filepath.Join(testRoot(t),"中文卸载验收")
	if err:=os.MkdirAll(root,0700);err!=nil{t.Fatal(err)}
	for _,name:=range []string{"ustc-marathon.exe","uninstall.ps1",".ustc-marathon.json","config.example.toml"}{
		data,err:=os.ReadFile(filepath.Join("..","release",name));if err!=nil{t.Fatal(err)}
		if name=="config.example.toml"{name="config.toml"}
		if err:=os.WriteFile(filepath.Join(root,name),data,0600);err!=nil{t.Fatal(err)}
	}
	username:="local-uninstall-verification-"+time.Now().Format("20060102150405.000000000")
	oldConfig:=configPath
	configPath=filepath.Join(root,"config.toml")
	err:=updateConfig([]string{"monitor","username"},username)
	configPath=oldConfig
	if err!=nil{t.Fatal(err)}
	if err:=writePassword(username,"本地卸载凭据验证");err!=nil{t.Fatal(err)}
	defer func(){if err:=deletePassword(username);err!=nil{t.Error(err)}}()
	executable:=filepath.Join(root,"ustc-marathon.exe")
	version,err:=exec.Command(executable,"--version").CombinedOutput()
	if err!=nil||string(version)!="2.0.2\n"{t.Fatal("中文目录 EXE 版本验证失败",err)}
	lock,err:=lockInstance(filepath.Join(root,"runtime"));if err!=nil{t.Fatal(err)}
	if err:=exec.Command(executable,"uninstall").Run();err==nil{lock.Close();t.Fatal("卸载未拒绝其他运行实例")}
	lock.Close()
	output,err:=exec.Command(executable,"uninstall").CombinedOutput()
	if err!=nil{t.Fatal("实际卸载启动失败",err,string(output))}
	deadline:=time.Now().Add(40*time.Second)
	for {
		_,err:=os.Stat(root)
		if os.IsNotExist(err){break}
		if err!=nil{t.Fatal(err)}
		if time.Now().After(deadline){t.Fatal("实际卸载未完成",string(output))}
		time.Sleep(250*time.Millisecond)
	}
	if password,err:=readPassword(username);err!=nil||password!=""{t.Fatal("真实卸载凭据未清除",err)}
	t.Log("实际 EXE、发布目录与测试凭据卸载通过，其他实例保护通过")
}
