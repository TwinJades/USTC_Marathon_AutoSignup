package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/UserExistsError/conpty"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/sys/windows"
)

// 使用真实 Windows 终端输入本机凭据，输出只留在内存中。
func TestLiveTerminalLogin(t *testing.T) {
	if os.Getenv("USTC_LIVE_TERMINAL_VERIFY")!="1"{t.Skip("真实终端认证需要显式启用")}
	c,err:=loadConfig();if err!=nil{t.Fatal(err)}
	username:=c.Monitor.Username
	password,err:=readPassword(username);if err!=nil||password==""{t.Fatal("缺少真实登录凭据")}
	root:=testRoot(t)
	private:=filepath.Join(root,"config.toml")
	if err:=os.WriteFile(private,exampleConfig,0600);err!=nil{t.Fatal(err)}
	previous:=configPath
	browserIndex:=-1
	for index,browser:=range installedBrowsers(){if browser.Kind==c.Browser.Kind{browserIndex=index}}
	if browserIndex<0{t.Fatal("原配置浏览器没有安装")}
	executable,err:=filepath.Abs(filepath.Join("..","release","ustc-marathon.exe"));if err!=nil{t.Fatal(err)}
	terminal,err:=conpty.Start(windows.ComposeCommandLine([]string{executable,"--config",private,"login"}),conpty.ConPtyDimensions(120,40));if err!=nil{t.Fatal(err)}
	ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
	defer func(){if err:=terminal.Close();err!=nil{t.Error(err)};cancel()}()
	chunks:=make(chan string,64)
	go func(){data:=make([]byte,4096);for{n,err:=terminal.Read(data);if n>0{select{case chunks<-string(data[:n]):case <-ctx.Done():return}};if err!=nil{return}}}()
	type processResult struct{exit uint32;err error}
	finished:=make(chan processResult,1)
	go func(){exit,err:=terminal.Wait(context.Background());finished<-processResult{exit,err}}()
	var output strings.Builder
	browserSent,accountSent,passwordSent:=false,false,false
	for {
		select {
		case <-ctx.Done():
			if _,err:=terminal.Write([]byte{3});err!=nil{t.Fatal(err)}
			select{case <-finished:case <-time.After(10*time.Second):t.Fatal("终端登录未响应停止信号")}
			t.Fatal("真实终端认证等待超时，认证输出保持隐藏")
		case result:=<-finished:
			if result.err!=nil{t.Fatal(result.err)}
			for len(chunks)>0{output.WriteString(<-chunks)}
			text:=output.String()
			if result.exit!=0||!browserSent||!accountSent||!passwordSent||!strings.Contains(text,"账户验证成功，登录凭据已保存"){
				for _,line:=range strings.Split(ansi.Strip(text),"\n"){if index:=strings.Index(line,"运行终止：");index>=0{t.Log(strings.ReplaceAll(strings.ReplaceAll(line[index:],username,"[账户已隐藏]"),password,"[密码已隐藏]"))}}
				for _,reason:=range []string{"TypeError","ReferenceError","Inspected target navigated or closed","浏览器查询失败","登录账号和密码不能为空","浏览器启动失败","账户验证成功，登录凭据已保存","统一身份认证拒绝登录"}{if strings.Contains(text,reason){t.Log("脱敏状态：",reason)}}
				t.Log("退出码和已发送输入：",result.exit,browserSent,accountSent,passwordSent)
				t.Fatal("真实终端认证未完成，认证输出保持隐藏")
			}
			if strings.Count(text,"统一身份认证账号")!=1||strings.Count(text,"统一身份认证密码")!=1{t.Fatal("账号或密码出现重复输入")}
			if strings.Contains(text,password){t.Fatal("终端输出泄露密码，认证输出保持隐藏")}
			if actual,err:=readPassword(username);err!=nil||actual!=password{t.Fatal("成功后保存的真实凭据不符")}
			configPath=private
			verified,err:=loadConfig()
			configPath=previous
			if err!=nil||verified.Browser.Kind!=c.Browser.Kind||verified.Monitor.Username!=username||verified.Profile.Department==""||(verified.Profile.Sex!="男"&&verified.Profile.Sex!="女"){t.Fatal("成功后浏览器、账户及预填资料保存不符")}
			t.Log("真实终端首次浏览器选择、各输入一次账号与密码、自动认证、成功后凭据保存、预填资料保存通过；密码输出保持隐藏")
			return
		case chunk:=<-chunks:
			output.WriteString(chunk)
			text:=output.String()
			if !browserSent&&strings.Contains(text,"选择本机浏览器"){if _,err:=terminal.Write([]byte(strconv.Itoa(browserIndex+1)+"\r"));err!=nil{t.Fatal(err)};browserSent=true}
			if !accountSent&&strings.Contains(text,"统一身份认证账号"){if _,err:=terminal.Write([]byte(username+"\r"));err!=nil{t.Fatal(err)};accountSent=true}
			if !passwordSent&&strings.Contains(text,"统一身份认证密码"){if _,err:=terminal.Write([]byte(password+"\r"));err!=nil{t.Fatal(err)};passwordSent=true}
		}
	}
}
