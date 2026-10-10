package main

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"math"

	"github.com/charmbracelet/huh"
)

//go:embed config.example.toml
var exampleConfig []byte

func ensureConfig() error {
	if _,err:=os.Stat(configPath);err==nil {return nil}else if !os.IsNotExist(err){return err}
	f,err:=os.OpenFile(configPath,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
	if err!=nil{return err}
	if _,err=f.Write(exampleConfig);err!=nil{f.Close();return err}
	return f.Close()
}

func editProfile() error {
	if err:=ensureConfig();err!=nil{return err}
	c,err:=loadConfig()
	if err!=nil{return err}
	p:=c.Profile
	form:=huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("用户类型").Options(huh.NewOption("学生","学生"),huh.NewOption("教职工","教职工")).Value(&p.Type),
		huh.NewSelect[string]().Title("衣服尺码").Options(huh.NewOptions(sizes...)...).Value(&p.Size),
		huh.NewInput().Title("身份证号").EchoMode(huh.EchoModePassword).Value(&p.IDNumber),
		huh.NewInput().Title("手机号码（两个步骤使用同一号码）").Value(&p.Phone),
		huh.NewInput().Title("紧急联系人电话").Value(&p.Emergency),
	))
	if err:=form.Run();err!=nil{return fmt.Errorf("资料编辑已取消：%w",err)}
	return saveProfileEdits(p)
}

func saveProfileEdits(p Profile) error {
	for _,field:=range []struct{name,value string}{{"user_type",p.Type},{"size",p.Size},{"id_number",p.IDNumber},{"phone",p.Phone},{"emergency_contact",p.Emergency}} {
		value:=strings.TrimSpace(field.value)
		if err:=validateProfileField(field.name,value);err!=nil{fmt.Printf("%s 保留原值：%s\n",field.name,err);continue}
		if err:=updateConfig([]string{"profile",field.name},value);err!=nil{return err}
	}
	c,err:=loadConfig();if err!=nil{return err}
	fmt.Println("合法资料已经保存；学院和性别在登录后读取网站预填值。")
	if err:=c.Profile.complete();err!=nil{fmt.Println("资料尚未通过完整验证，禁止报名：",err)}
	return nil
}

func editTimes() error {
	c,err:=loadConfig();if err!=nil{return err}
	for _,field:=range []struct{key,title string;value float64}{{"poll_interval_seconds","每轮全部目标检查完成后的等待秒数",c.Monitor.Interval},{"request_timeout_seconds","请求与页面操作超时秒数",c.Monitor.RequestTimeout},{"login_timeout_seconds","登录等待超时秒数",c.Monitor.LoginTimeout}} {
		value:=strconv.FormatFloat(field.value,'g',-1,64)
		if err:=huh.NewInput().Title(field.title).Value(&value).Run();err!=nil{return err}
		n,err:=strconv.ParseFloat(strings.TrimSpace(value),64)
		if err!=nil||n<=0||math.IsInf(n,0)||math.IsNaN(n){fmt.Println(field.title+"必须为有限正数，保留原值。");continue}
		if err:=updateConfig([]string{"monitor",field.key},n);err!=nil{return err}
	}
	return nil
}

func menuOptions(section string) []huh.Option[string] {
	switch section {
	case "settings":return []huh.Option[string]{huh.NewOption("登录","login"),huh.NewOption("选择比赛及优先级","select"),huh.NewOption("填写信息","configure"),huh.NewOption("时间设置","times"),huh.NewOption("只读验证","check"),huh.NewOption("返回主菜单","back")}
	case "maintenance":return []huh.Option[string]{huh.NewOption("空间查看","storage"),huh.NewOption("缓存清理","clean"),huh.NewOption("全部卸载","uninstall"),huh.NewOption("返回主菜单","back")}
	default:return []huh.Option[string]{huh.NewOption("设置","settings"),huh.NewOption("开始监测与自动报名","watch"),huh.NewOption("查看报名记录","status"),huh.NewOption("存储与卸载","maintenance"),huh.NewOption("退出程序","exit")}
	}
}

func menu(parent context.Context) error {
	fmt.Println("科大马拉松监测与自动报名程序 "+version)
	if _,err:=os.Stat(configPath);os.IsNotExist(err){fmt.Println("首次使用请进入“设置”，依次完成登录、比赛选择和资料填写。")}else if err!=nil{return err}
	section:=""
	for {
		command:=""
		options:=menuOptions(section)
		if err:=huh.NewSelect[string]().Title("选择操作（方向键选择，回车确认）").Options(options...).Value(&command).Run();err!=nil {return fmt.Errorf("菜单选择已取消：%w",err)}
		if command=="exit"{return nil}
		if command=="settings"||command=="maintenance"{section=command;continue}
		if command=="back"{section="";continue}
		ctx,stop:=signal.NotifyContext(parent,os.Interrupt)
		var err error
		if command=="configure"||command=="times" {
			lock,lockErr:=lockInstance(runtimeRoot)
			if lockErr!=nil{err=lockErr}else{if err=ensureConfig();err==nil{if command=="configure"{err=editProfile()}else{err=editTimes()}};lock.Close()}
		}else{
			err=run(ctx,append([]string{"--config",configPath},strings.Fields(command)...))
		}
		interrupted:=ctx.Err()!=nil
		stop()
		if err==nil&&command=="uninstall"{return nil}
		if interrupted{fmt.Println("当前操作已经停止；待核查提交记录已保留。")}else if err!=nil{fmt.Fprintln(os.Stderr,"操作终止："+err.Error())}
		fmt.Print("按回车返回菜单")
		if _,err:=bufio.NewReader(os.Stdin).ReadString('\n');err!=nil{return err}
	}
}
