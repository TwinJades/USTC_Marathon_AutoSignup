package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 独立空白浏览器目录实际执行统一认证，保留用户正在使用的会话。
func TestLiveFreshLogin(t *testing.T) {
	if os.Getenv("USTC_LIVE_LOGIN_VERIFY")!="1"{t.Skip("真实新会话登录需要显式启用")}
	c,err:=loadConfig();if err!=nil{t.Fatal(err)}
	password,err:=readPassword(c.Monitor.Username);if err!=nil||password==""{t.Fatal("缺少本机真实凭据")}
	root:=testRoot(t)
	oldRoot,oldConfig:=runtimeRoot,configPath
	defer func(){runtimeRoot=oldRoot;configPath=oldConfig}()
	data,err:=os.ReadFile(configPath);if err!=nil{t.Fatal(err)}
	configPath=filepath.Join(root,"config.toml")
	if err:=os.WriteFile(configPath,data,0600);err!=nil{t.Fatal(err)}
	runtimeRoot=filepath.Join(root,"runtime")
	c.Monitor.RequestTimeout=90
	c.Monitor.LoginTimeout=120
	j,err:=openJournal(runtimeRoot);if err!=nil{t.Fatal(err)};defer j.db.Close()
	ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
	b,err:=newBrowser(ctx,c,j);if err!=nil{t.Fatal(err)};defer b.close()
	if err:=b.login(true);err!=nil{t.Fatal(err)}
	s:=&Website{b,j}
	account,err:=s.account();if err!=nil{t.Fatal(err)}
	if account["username"]!=c.Monitor.Username{t.Fatal("新会话登录账户不匹配")}
	if actual,err:=readPassword(c.Monitor.Username);err!=nil||actual!=password{t.Fatal("验证过程改变了原有凭据")}
	t.Log("空白独立浏览器目录通过真实统一认证，账户及预填资料已核实，原凭据与会话保留")
}
