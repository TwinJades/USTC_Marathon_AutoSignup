package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

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
	interval:=strconv.FormatFloat(c.Monitor.Interval,'g',-1,64)
	form:=huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("统一身份认证账号").Value(&c.Monitor.Username),
		huh.NewInput().Title("学院或部门规范全称").Value(&p.Department),
		huh.NewSelect[string]().Title("用户类型").Options(huh.NewOption("学生","学生"),huh.NewOption("教职工","教职工")).Value(&p.Type),
		huh.NewSelect[string]().Title("性别").Options(huh.NewOption("男","男"),huh.NewOption("女","女")).Value(&p.Sex),
		huh.NewSelect[string]().Title("衣服尺码").Options(huh.NewOptions(sizes...)...).Value(&p.Size),
		huh.NewInput().Title("身份证号").EchoMode(huh.EchoModePassword).Value(&p.IDNumber),
		huh.NewInput().Title("手机号码（两个步骤使用同一号码）").Value(&p.Phone),
		huh.NewInput().Title("紧急联系人电话").Value(&p.Emergency),
		huh.NewInput().Title("检查间隔，单位秒，允许正数小数").Value(&interval),
	))
	if err:=form.Run();err!=nil{return fmt.Errorf("资料编辑已取消：%w",err)}
	p.Department=strings.TrimSpace(p.Department)
	c.Profile=p
	c.Monitor.Interval,err=strconv.ParseFloat(interval,64)
	if err!=nil{return fmt.Errorf("检查间隔必须为正数")}
	c.normalize()
	if c.Monitor.Username==""{return fmt.Errorf("统一身份认证账号不能为空")}
	if err:=c.validate();err!=nil{return err}
	if err:=c.Profile.complete();err!=nil{return err}
	for _,field:=range []struct{name,value string}{{"depart_name",c.Profile.Department},{"user_type",c.Profile.Type},{"sex",c.Profile.Sex},{"size",c.Profile.Size},{"id_number",c.Profile.IDNumber},{"phone",c.Profile.Phone},{"emergency_contact",c.Profile.Emergency}}{if err:=updateConfig([]string{"profile",field.name},field.value);err!=nil{return err}}
	if err:=updateConfig([]string{"monitor","username"},c.Monitor.Username);err!=nil{return err}
	if err:=updateConfig([]string{"monitor","poll_interval_seconds"},c.Monitor.Interval);err!=nil{return err}
	fmt.Println("配置与报名资料已经保存。请通过凭据设置保存密码，再选择目标比赛。")
	return nil
}

func menu(parent context.Context) error {
	fmt.Println("科大马拉松监测与自动报名程序 "+version)
	if _,err:=os.Stat(configPath);os.IsNotExist(err){fmt.Println("尚无私人配置，请先选择“创建或修改配置”。")}else if err!=nil{return err}
	for {
		command:=""
		options:=[]huh.Option[string]{
			huh.NewOption("创建或修改配置","configure"),huh.NewOption("登录并验证账户","login"),
			huh.NewOption("保存登录凭据","credentials set"),huh.NewOption("选择本机浏览器","browser select"),
			huh.NewOption("选择比赛及优先级","select"),huh.NewOption("只读检查比赛与资料","check"),
			huh.NewOption("开始监测与自动报名","watch"),huh.NewOption("查看报名记录","status"),
			huh.NewOption("查看存储空间","storage"),huh.NewOption("清理可重新生成的缓存","clean"),
			huh.NewOption("全部卸载","uninstall"),huh.NewOption("退出程序","exit"),
		}
		if err:=huh.NewSelect[string]().Title("选择操作（方向键选择，回车确认）").Options(options...).Value(&command).Run();err!=nil {return fmt.Errorf("菜单选择已取消：%w",err)}
		if command=="exit"{return nil}
		ctx,stop:=signal.NotifyContext(parent,os.Interrupt)
		var err error
		if command=="configure" {
			lock,lockErr:=lockInstance(runtimeRoot)
			if lockErr!=nil{err=lockErr}else{err=editProfile();lock.Close()}
		}else{
			err=run(ctx,append([]string{"--config",configPath},strings.Fields(command)...))
		}
		interrupted:=ctx.Err()!=nil
		stop()
		if err==nil&&command=="uninstall"{return nil}
		if interrupted{fmt.Println("当前操作已经停止；待核查提交记录已保留。")}else if err!=nil{fmt.Fprintln(os.Stderr,"操作终止："+err.Error())}
		acknowledged:=true
		if err:=huh.NewConfirm().Title("操作已结束，按回车返回菜单").Affirmative("返回菜单").Negative("返回菜单").Value(&acknowledged).Run();err!=nil{return fmt.Errorf("返回菜单已取消：%w",err)}
	}
}
