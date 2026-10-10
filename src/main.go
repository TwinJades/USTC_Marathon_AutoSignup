package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

const version = "2.0.2"

func selectBrowser(c *Config) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) { return fmt.Errorf("浏览器选择需要交互终端；也可以在配置中填写 browser.kind = \"chrome\" 或 \"edge\"") }
	installed := installedBrowsers()
	if len(installed) == 0 { return fmt.Errorf("本机没有检测到 Chrome 或 Edge，请安装浏览器后再次运行") }
	options := make([]huh.Option[string], 0, len(installed))
	for _, b := range installed { options = append(options, huh.NewOption(b.Kind+"："+b.Path, b.Kind)) }
	kind := c.Browser.Kind
	if err := huh.NewSelect[string]().Title("选择本机浏览器").Options(options...).Value(&kind).Run(); err != nil { return fmt.Errorf("浏览器选择已取消") }
	if err := updateConfig([]string{"browser", "kind"}, kind); err != nil { return err }
	if err := updateConfig([]string{"browser", "executable_path"}, ""); err != nil { return err }
	c.Browser.Kind, c.Browser.Executable = kind, ""
	fmt.Println("浏览器选择已保存：" + kind)
	return nil
}

func selectRaces(s *Website) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) { return fmt.Errorf("比赛选择需要交互终端") }
	account, err := s.account()
	if err != nil { return err }
	if s.b.config.Monitor.Username == "" { if err := updateConfig([]string{"monitor", "username"}, account["username"]); err != nil { return err } }
	races, err := s.races()
	if err != nil { return err }
	if len(races) == 0 { return fmt.Errorf("网站当前没有可选择的马拉松比赛") }
	options := make([]huh.Option[string], 0, len(races))
	var selected []string
	for _, t := range s.b.config.Targets { selected = append(selected, t.ID) }
	for _, t := range races { options = append(options, huh.NewOption(t.Name+"（"+t.Campus+"）", t.ID)) }
	if err := huh.NewMultiSelect[string]().Title("选择目标比赛（空格勾选，回车确认）").Options(options...).Value(&selected).Run(); err != nil { return fmt.Errorf("比赛选择已取消") }
	var ordered []Target
	for len(selected) > 0 {
		id := selected[0]
		if len(selected) > 1 {
			var priority []huh.Option[string]
			for _, r := range races { for _, candidate := range selected { if candidate == r.ID { priority = append(priority, huh.NewOption(r.Name+"（"+r.Campus+"）", r.ID)) } } }
			if err := huh.NewSelect[string]().Title("选择下一优先级比赛").Options(priority...).Value(&id).Run(); err != nil { return fmt.Errorf("优先级选择已取消") }
		}
		for _, r := range races { if r.ID == id { ordered = append(ordered, r) } }
		for i, candidate := range selected { if candidate == id { selected = append(selected[:i], selected[i+1:]...); break } }
	}
	values := make([]map[string]any, 0, len(ordered))
	for _, t := range ordered { values = append(values, map[string]any{"id": t.ID, "name": t.Name, "campus": t.Campus}) }
	if err := updateConfig([]string{"targets"}, values); err != nil { return err }
	fmt.Println("目标比赛及优先级已保存。")
	return nil
}

func usage() {
	fmt.Println("科大马拉松监测与自动报名程序 " + version)
	fmt.Println("用法：ustc-marathon [--config config.toml] <命令>")
	fmt.Println("login                  登录并验证账户")
	fmt.Println("select                 勾选比赛并设置优先级")
	fmt.Println("check                  只读检查比赛与资料")
	fmt.Println("watch                  监测并自动完成一场报名")
	fmt.Println("status                 查看本地报名阶段")
	fmt.Println("validate               验证本地配置")
	fmt.Println("credentials set/status 保存或检查 Windows 登录凭据")
	fmt.Println("browser list/select    查看或选择本机 Chrome、Edge")
	fmt.Println("storage / clean        查看空间占用或清理缓存")
	fmt.Println("uninstall              清除本项目凭据并将运行目录移入回收站")
	fmt.Println("无参数运行进入中文终端菜单。")
}

func run(ctx context.Context, args []string) (result error) {
	flags := flag.NewFlagSet("ustc-marathon", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	path := flags.String("config", "", "TOML 配置文件路径；默认使用 EXE 所在目录")
	ver := flags.Bool("version", false, "显示程序版本")
	flags.Usage = usage
	if err := flags.Parse(args); err != nil { if errors.Is(err, flag.ErrHelp) { return nil }; return fmt.Errorf("命令参数无效") }
	if *ver { fmt.Println(version); return nil }
	if *path == "" { executable, err := os.Executable(); if err != nil { return err }; *path = filepath.Join(filepath.Dir(executable), "config.toml") }
	if err := setRoot(*path); err != nil { return err }
	commands := flags.Args()
	if len(commands) == 0 { if !term.IsTerminal(int(os.Stdin.Fd())) { usage(); return nil }; return menu(ctx) }
	ctx, stop := signal.NotifyContext(ctx,os.Interrupt)
	defer stop()
	command := commands[0]
	valid := map[string]bool{"login":true,"select":true,"check":true,"watch":true,"status":true,"validate":true,"credentials":true,"browser":true,"storage":true,"clean":true,"uninstall":true}
	if !valid[command] { return fmt.Errorf("未知命令：%s", command) }
	if command == "storage" { if len(commands)!=1 { return fmt.Errorf("storage 不接受额外参数") }; return showStorage() }
	if command == "browser" && len(commands) == 2 && commands[1] == "list" {
		for _, b := range installedBrowsers() { fmt.Println(b.Kind + "：" + b.Path) }
		return nil
	}
	lock, err := acquireInstance(runtimeRoot,command=="uninstall")
	if err != nil { return err }
	defer lock.Close()
	if command == "clean" { if len(commands)!=1 { return fmt.Errorf("clean 不接受额外参数") }; return cleanCaches(runtimeRoot) }
	if command == "uninstall" { if len(commands)!=1 { return fmt.Errorf("uninstall 不接受额外参数") }; return uninstall() }
	if command == "status" {
		if len(commands)!=1{return fmt.Errorf("status 不接受额外参数")}
		j, err := openJournal(runtimeRoot)
		if err != nil { return err }
		defer j.db.Close()
		rows, err := j.rows()
		if err != nil { return err }
		if len(rows) == 0 { fmt.Println("尚无报名提交记录。") }
		for _, r := range rows { fmt.Printf("%s（%s）：%s；%s\n", r.Name, r.Campus, r.Phase, r.Updated) }
		return nil
	}
	if err:=ensureConfig();err!=nil{return err}
	c, err := loadConfig()
	if err != nil { return err }
	if command == "validate" { if len(commands)!=1{return fmt.Errorf("validate 不接受额外参数")}; if err := c.requireWatch(); err != nil { return err }; fmt.Println("配置格式、目标比赛及报名资料检查通过。"); return nil }
	if command == "browser" {
		if len(commands) != 2 || commands[1] != "select" { return fmt.Errorf("用法：browser list 或 browser select") }
		return selectBrowser(&c)
	}
	if command == "credentials" {
		if len(commands) != 2 { return fmt.Errorf("用法：credentials set 或 credentials status") }
		if commands[1] == "status" {
			password, err := readPassword(c.Monitor.Username)
			if err != nil { return err }
			if password == "" { fmt.Println("尚未保存登录密码。") } else { fmt.Println("登录密码已保存。") }
			return nil
		}
		if commands[1] != "set" { return fmt.Errorf("用法：credentials set 或 credentials status") }
		command="login"
		commands=[]string{"login"}
	}
	if len(commands) != 1 { return fmt.Errorf("命令不接受额外参数") }
	if c.Browser.Kind == "" { if err := selectBrowser(&c); err != nil { return err } }
	j, err := openJournal(runtimeRoot)
	if err != nil { return err }
	defer j.db.Close()
	b, err := newBrowser(ctx, c, j)
	if err != nil { return err }
	defer func(){ if err:=b.close();err!=nil {result=errors.Join(result,err)} }()
	if err := b.login(command == "login"); err != nil { return err }
	for _,field:=range []struct{key,value string}{{"depart_name",b.config.Profile.Department},{"sex",b.config.Profile.Sex}}{if err:=updateConfig([]string{"profile",field.key},field.value);err!=nil{return err}}
	if command=="watch"||command=="check"{if err:=b.config.requireWatch();err!=nil{return err}}
	s := &Website{b,j}
	switch command {
	case "login": return nil
	case "select": return selectRaces(s)
	case "check": return s.check()
	case "watch": return s.watch()
	}
	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:]); err != nil {
		if errors.Is(err,context.Canceled) { fmt.Fprintln(os.Stderr, "运行已停止；待核查的提交记录已保留。") } else { fmt.Fprintln(os.Stderr, "运行终止："+err.Error()) }
		os.Exit(1)
	}
}
