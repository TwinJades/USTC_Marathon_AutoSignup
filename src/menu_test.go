package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPartialProfileEdits(t *testing.T) {
	previous:=configPath
	defer func(){configPath=previous}()
	configPath=filepath.Join(testRoot(t),"逐项保存.toml")
	if err:=ensureConfig();err!=nil{t.Fatal(err)}
	if err:=updateConfig([]string{"profile","phone"},"13800138000");err!=nil{t.Fatal(err)}
	c,err:=loadConfig();if err!=nil{t.Fatal(err)}
	p:=c.Profile
	p.Phone="错误手机号码"
	p.Size=sizes[2]
	p.Emergency="13900139000"
	if err:=saveProfileEdits(p);err!=nil{t.Fatal(err)}
	c,err=loadConfig();if err!=nil{t.Fatal(err)}
	if c.Profile.Phone!="13800138000"||c.Profile.Size!=sizes[2]||c.Profile.Emergency!="13900139000"{t.Fatal("合法字段保存或非法字段保留失败")}
	if err:=c.Profile.complete();err==nil{t.Fatal("不完整资料允许报名")}
	data,err:=os.ReadFile(configPath);if err!=nil{t.Fatal(err)}
	if !strings.Contains(string(data),"无需本人填写"){t.Fatal("资料保存后注释丢失")}
}

func TestActualMenuSavedConfiguration(t *testing.T) {
	if os.Getenv("USTC_MENU_VERIFY")!="1"{t.Skip("需要先完成真实终端菜单编辑")}
	previous:=configPath
	defer func(){configPath=previous}()
	configPath=filepath.Join(runtimeRoot,"verification","menu-edit","config.toml")
	c,err:=loadConfig();if err!=nil{t.Fatal(err)}
	if c.Profile.Size!=sizes[2]||c.Profile.Emergency!="13900139000"||c.Profile.Phone!=""||c.Profile.IDNumber!=""{t.Fatal("真实菜单资料保存不符")}
	if c.Profile.Department!=""||c.Profile.Sex!=""{t.Fatal("菜单修改了自动读取的字段")}
	if c.Monitor.Interval!=0.75||c.Monitor.RequestTimeout!=12||c.Monitor.LoginTimeout!=60{t.Fatal("真实菜单时间设置保存不符")}
	if err:=c.requireWatch();err==nil{t.Fatal("实际不完整资料允许报名")}
	data,err:=os.ReadFile(configPath);if err!=nil{t.Fatal(err)}
	if !strings.Contains(string(data),"无需本人填写"){t.Fatal("真实菜单编辑丢失注释")}
}
