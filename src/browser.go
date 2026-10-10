package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"golang.org/x/sys/windows/registry"
)

const siteURL = "https://young.ustc.edu.cn"
const appPath = "/login/sc-wisdom-group-learning"
const marathonURL = siteURL + appPath + "/myproject/marathonProject"

type InstalledBrowser struct { Kind, Path string }
type Browser struct {
	ctx context.Context
	parent context.Context
	cancel context.CancelFunc
	allocatorCancel context.CancelFunc
	config Config
	path string
	headless bool
	journal *Journal
}

func installedBrowsers() []InstalledBrowser {
	var result []InstalledBrowser
	for _, kind := range []string{"chrome", "edge"} {
		file := "chrome.exe"
		folder := filepath.Join("Google", "Chrome", "Application")
		if kind == "edge" { file = "msedge.exe"; folder = filepath.Join("Microsoft", "Edge", "Application") }
		var paths []string
		for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
			key, err := registry.OpenKey(hive, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+file, registry.QUERY_VALUE)
			if err == nil {
				p, _, readErr := key.GetStringValue("")
				key.Close()
				if readErr == nil { paths = append(paths, strings.Trim(p, "\"")) }
			}
		}
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if root != "" { paths = append(paths, filepath.Join(root, folder, file)) }
		}
		for _, p := range paths {
			info, err := os.Stat(p)
			if err == nil && !info.IsDir() { result = append(result, InstalledBrowser{kind, p}); break }
		}
	}
	return result
}

func browserPath(c Config) (string, error) {
	if c.Browser.Kind == "" { return "", fmt.Errorf("尚未选择浏览器，请运行 browser select 或填写 browser.kind") }
	if c.Browser.Executable != "" {
		p, err := filepath.Abs(c.Browser.Executable)
		if err != nil { return "", err }
		info, err := os.Stat(p)
		if err != nil || info.IsDir() { return "", fmt.Errorf("指定的浏览器文件不存在") }
		return p, nil
	}
	for _, b := range installedBrowsers() { if b.Kind == c.Browser.Kind { return b.Path, nil } }
	return "", fmt.Errorf("本机没有找到配置指定的浏览器：%s", c.Browser.Kind)
}

func newBrowser(parent context.Context, c Config, j *Journal) (*Browser, error) {
	p, err := browserPath(c)
	if err != nil { return nil, err }
	return &Browser{parent: parent, config: c, path: p, journal: j}, nil
}

func seconds(value float64) time.Duration {
	if value >= float64(time.Duration(1<<63-1))/float64(time.Second) { return time.Duration(1<<63-1) }
	d := time.Duration(value*float64(time.Second))
	if d < time.Nanosecond { return time.Nanosecond }
	return d
}

func waitSeconds(ctx context.Context, value float64) error {
	limit := float64(time.Duration(1<<63-1))/float64(time.Second)
	for value > limit {
		if err := sleep(ctx, time.Duration(1<<63-1)); err != nil { return err }
		value -= limit
	}
	return sleep(ctx, seconds(value))
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select { case <-ctx.Done(): return ctx.Err(); case <-t.C: return nil }
}

func (b *Browser) close() error {
	started:=b.allocatorCancel!=nil
	if b.ctx != nil && b.ctx.Err() == nil {
		ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
		chromedp.Cancel(ctx)
		cancel()
	}
	if b.cancel != nil { b.cancel() }
	if b.allocatorCancel != nil { b.allocatorCancel() }
	b.ctx,b.cancel,b.allocatorCancel=nil,nil,nil
	if started{return cleanCaches(runtimeRoot)}
	return nil
}

func (b *Browser) launch(headless bool) error {
	if err:=b.close();err!=nil{return err}
	profile, err := filepath.Abs(filepath.Join(runtimeRoot, "sessions", b.config.Browser.Kind))
	if err != nil { return err }
	opts := []chromedp.ExecAllocatorOption{chromedp.ExecPath(b.path), chromedp.UserDataDir(profile), chromedp.NoFirstRun, chromedp.NoDefaultBrowserCheck, chromedp.Flag("headless", headless), chromedp.Flag("disable-gpu", headless), chromedp.Flag("lang", "zh-CN"), chromedp.Flag("disk-cache-size", "1048576"), chromedp.Flag("media-cache-size", "1048576"), chromedp.Flag("disable-background-mode", true)}
	opts = append(opts, chromedp.Flag("disable-background-networking", true), chromedp.Flag("disable-component-update", true), chromedp.Flag("disable-background-timer-throttling", true))
	opts = append(opts,chromedp.Flag("disable-extensions",true),chromedp.Flag("disable-sync",true),chromedp.Flag("metrics-recording-only",true),chromedp.Flag("disable-features","OptimizationGuideModelDownloading,OptimizationGuidePredictionModelDownloads,OptimizationHintsFetching,OptimizationGuideOnDeviceModel,OptimizationTargetPrediction,OptimizationHints,PageContentAnnotations,TextSafetyClassifier,HistoryEmbeddings"))
	if headless { opts = append(opts, chromedp.Flag("headless", "new")) }
	if b.config.Browser.Kind == "edge" { opts = append(opts, chromedp.Flag("edge-skip-compat-layer-relaunch", true)) }
	alloc, cancel := chromedp.NewExecAllocator(b.parent, opts...)
	b.allocatorCancel = cancel
	b.ctx, b.cancel = chromedp.NewContext(alloc)
	b.headless = headless
	// 浏览器生命周期使用会话上下文，单次查询的超时不终止整个进程。
	if err := chromedp.Run(b.ctx); err != nil { return fmt.Errorf("浏览器启动失败：%w", err) }
	ctx, stop := context.WithTimeout(b.ctx, seconds(b.config.Monitor.RequestTimeout))
	defer stop()
	if err := chromedp.Run(ctx, network.Enable(), network.SetCacheDisabled(true), chromedp.Navigate(marathonURL)); err != nil { return fmt.Errorf("浏览器启动或页面加载失败") }
	return nil
}

func (b *Browser) eval(script string, params any, output any) error {
	encoded, err := json.Marshal(params)
	if err != nil { return err }
	expression := "(" + script + ")(" + string(encoded) + ")"
	ctx, cancel := context.WithTimeout(b.ctx, seconds(b.config.Monitor.RequestTimeout))
	defer cancel()
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		result, exception, err := runtime.Evaluate(expression).WithAwaitPromise(true).WithReturnByValue(true).Do(ctx)
		if err != nil { return fmt.Errorf("浏览器查询失败：%w", err) }
		if exception != nil {
			if exception.Exception != nil {
				for _, reason := range []string{"路由结构变化", "路由不唯一", "页面字段不唯一", "按钮不可用", "查询路径无效", "网站查询机制变化", "网站查询失败", "字段缺失"} {
					if strings.Contains(exception.Exception.Description, reason) { return fmt.Errorf("页面执行失败：%s", reason) }
				}
			}
			return fmt.Errorf("页面执行失败，请核查网站结构和响应")
		}
		if output != nil {
			if err := json.Unmarshal(result.Value, output); err != nil { return fmt.Errorf("页面返回结构发生变化") }
		}
		return nil
	}))
}

func (b *Browser) ready() (bool, error) {
	var ready bool
	err := b.eval(`() => location.origin === 'https://young.ustc.edu.cn' && !!document.querySelector('#app')?.__vue__?.$http?.get && !!document.querySelector('#app')?.__vue__?.$store?.getters?.token`, nil, &ready)
	return ready, err
}

func (b *Browser) wait(script string, params any) error {
	deadline := time.Now().Add(seconds(b.config.Monitor.RequestTimeout))
	for time.Now().Before(deadline) {
		var okay bool
		if err := b.eval(script, params, &okay); err != nil { return err }
		if okay { return nil }
		if err := sleep(b.parent, 100*time.Millisecond); err != nil { return err }
	}
	return fmt.Errorf("等待页面状态超时")
}

func (b *Browser) navigate(url string) error {
	ctx, cancel := context.WithTimeout(b.ctx, seconds(b.config.Monitor.RequestTimeout))
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil { return fmt.Errorf("页面跳转失败") }
	return nil
}

func (b *Browser) click(selector string) error {
	var point struct { X, Y float64 }
	if err := b.eval(`async selector => {const e=document.querySelector(selector); if(!e)throw Error('字段缺失'); e.scrollIntoView({block:'center',inline:'center'}); await new Promise(resolve=>setTimeout(resolve,50)); const r=e.getBoundingClientRect(); let l=Math.max(0,r.left),t=Math.max(0,r.top),right=Math.min(innerWidth,r.right),bottom=Math.min(innerHeight,r.bottom); for(let p=e.parentElement;p;p=p.parentElement){const s=getComputedStyle(p),a=p.getBoundingClientRect(); if(/auto|scroll|hidden|clip/.test(s.overflowX)){l=Math.max(l,a.left);right=Math.min(right,a.right);} if(/auto|scroll|hidden|clip/.test(s.overflowY)){t=Math.max(t,a.top);bottom=Math.min(bottom,a.bottom);}} if(right<=l||bottom<=t)throw Error('按钮不可见'); const x=(l+right)/2,y=(t+bottom)/2; const hit=document.elementFromPoint(x,y); if(!hit||!e.contains(hit))throw Error('按钮被遮挡'); return {x,y};}`, selector, &point); err != nil { return err }
	ctx, cancel := context.WithTimeout(b.ctx, seconds(b.config.Monitor.RequestTimeout))
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.MouseClickXY(point.X, point.Y)); err != nil { return fmt.Errorf("页面按钮点击失败") }
	return nil
}

func (b *Browser) queryClick(path string, action func() error) error {
	ready := make(chan int64, 1)
	ctx, cancel := context.WithCancel(b.ctx)
	defer cancel()
	var request network.RequestID
	var status int64
	chromedp.ListenTarget(ctx, func(event any) {
		switch e := event.(type) {
		case *network.EventRequestWillBeSent:
			if e.Request.Method == "GET" && strings.HasPrefix(e.Request.URL, siteURL+"/login/wisdom-group-learning-bg"+path) { request = e.RequestID }
		case *network.EventResponseReceived:
			if request != "" && e.RequestID == request { status = e.Response.Status }
		case *network.EventLoadingFinished:
			if request != "" && e.RequestID == request { select {case ready <- status: default:} }
		}
	})
	if err := action(); err != nil { return err }
	timer := time.NewTimer(seconds(b.config.Monitor.RequestTimeout))
	defer timer.Stop()
	select {
	case <-b.parent.Done(): return b.parent.Err()
	case <-timer.C: return fmt.Errorf("页面查询请求超时")
	case status := <-ready:
		if status != 200 { return fmt.Errorf("页面查询请求失败") }
		return b.eval(`async () => {await document.querySelector('#app').__vue__.$nextTick();await new Promise(resolve=>setTimeout(resolve,50));}`, nil, nil)
	}
}

func (b *Browser) login(visible bool) error {
	if err := b.launch(!visible && b.config.Monitor.Headless); err != nil { return err }
	password, err := readPassword(b.config.Monitor.Username)
	if err != nil { return err }
	submitted, notified := false, false
	deadline := time.Now().Add(seconds(b.config.Monitor.LoginTimeout))
	for time.Now().Before(deadline) {
		ready, err := b.ready()
		if err != nil { return err }
		if ready {
			var path string
			if err := b.eval(`() => location.pathname`, nil, &path); err != nil { return err }
			if path != appPath+"/myproject/marathonProject" { if err := b.navigate(marathonURL); err != nil { return err } }
			if err := b.wait(`() => !!document.querySelector('#app')?.__vue__?.$http?.get`, nil); err != nil { return err }
			fmt.Println("统一身份认证成功。")
			return b.journal.event("登录验证完成", map[string]any{"browser": b.config.Browser.Kind})
		}
		var location string
		if err := b.eval(`() => location.href`, nil, &location); err != nil { return err }
		u, err := url.Parse(location)
		if err != nil { return fmt.Errorf("认证地址无效") }
		host := u.Hostname()
		if host != "young.ustc.edu.cn" && host != "id.ustc.edu.cn" && host != "passport.ustc.edu.cn" { return fmt.Errorf("认证跳转进入未知域名") }
		if host == "young.ustc.edu.cn" {
			if err := b.wait(`() => location.hostname !== 'young.ustc.edu.cn' || !!document.querySelector('#app')?.__vue__`, nil); err != nil { return fmt.Errorf("网站页面初始化失败：%w", err) }
		}
		if host != "young.ustc.edu.cn" && b.headless { if err := b.launch(false); err != nil { return err }; continue }
		var notice string
		if err := b.eval(`() => { const a=[...document.querySelectorAll('button')].filter(e=>e.getClientRects().length && e.innerText.trim()==='知道了'); if(a.length>1) throw Error('重复按钮'); if(!a.length)return ''; a[0].setAttribute('data-marathon-notice','1'); return '[data-marathon-notice="1"]'; }`, nil, &notice); err != nil { return err }
		if notice != "" { if err := b.click(notice); err != nil { return err } }
		var probe struct { Kind string; AccountIndex, PasswordIndex int }
		if err := b.eval(loginProbe, nil, &probe); err != nil { return err }
		interactive := probe.Kind == "verification" || password == ""
		if interactive && b.headless { if err := b.launch(false); err != nil { return err }; continue }
		if interactive && !notified { fmt.Println("请在登录窗口完成本人验证，程序等待正常认证结果。"); notified = true }
		if probe.Kind == "login" && host != "young.ustc.edu.cn" && password != "" && !submitted {
			if err := b.fillIndex(probe.AccountIndex, b.config.Monitor.Username); err != nil { return err }
			if err := b.fillIndex(probe.PasswordIndex, password); err != nil { return err }
			selector, err := b.control("button", "", "立即登录")
			if err != nil { return err }
			if err := b.click(selector); err != nil { return err }
			submitted = true
		}
		if submitted {
			var rejected bool
			if err := b.eval(`() => /密码错误|密码不正确|用户名或密码错误|账号被锁定/.test(document.body.innerText)`, nil, &rejected); err != nil { return err }
			if rejected { return fmt.Errorf("统一身份认证拒绝登录，请核查保存的凭据") }
		}
		if err := sleep(b.parent, 400*time.Millisecond); err != nil { return err }
	}
	return fmt.Errorf("等待统一身份认证超时")
}

func (b *Browser) fillIndex(index int, value string) error {
	return b.eval(`p => { const e=document.querySelectorAll('input')[p.index]; if(!e)throw Error('字段缺失'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(e,p.value); e.dispatchEvent(new Event('input',{bubbles:true})); e.dispatchEvent(new Event('change',{bubbles:true})); }`, map[string]any{"index": index, "value": value}, nil)
}

func (b *Browser) get(path string, params map[string]any) (json.RawMessage, error) {
	for attempt := 0; attempt < 2; attempt++ {
		ready, err := b.ready()
		if err != nil { return nil, err }
		if !ready { if err := b.login(true); err != nil { return nil, err } }
		var username string
		if err := b.eval(`() => document.querySelector('#app').__vue__.$store.getters.userInfo.username`, nil, &username); err != nil { return nil, err }
		if b.config.Monitor.Username != "" && username != b.config.Monitor.Username { return nil, fmt.Errorf("当前账户与配置不一致") }
		var response struct { Kind string; Data struct { Success *bool; Code any; Message string; Result json.RawMessage } }
		if err := b.eval(pageBridge, map[string]any{"path": path, "params": params, "timeout": b.config.Monitor.RequestTimeout*1000}, &response); err != nil { return nil, err }
		if response.Kind == "authentication" || response.Data.Message == "Token失效，请重新登录" || fmt.Sprint(response.Data.Code) == "401" {
			if attempt == 1 { return nil, fmt.Errorf("重新认证后查询仍然失效") }
			if err := b.login(true); err != nil { return nil, err }
			continue
		}
		if response.Kind != "response" || (response.Data.Success != nil && !*response.Data.Success) || (response.Data.Success == nil && fmt.Sprint(response.Data.Code) != "200") { return nil, fmt.Errorf("网站未成功完成查询") }
		if len(response.Data.Result) == 0 || string(response.Data.Result) == "null" { return nil, fmt.Errorf("网站查询缺少有效 result 字段") }
		return response.Data.Result, nil
	}
	return nil, fmt.Errorf("网站认证未完成")
}

func (b *Browser) route(suffix string) error {
	var path string
	if err := b.eval(`suffix => { const vm=document.querySelector('#app').__vue__; const routes=vm.$store.state.permission.routers; if(!Array.isArray(routes))throw Error('路由结构变化'); const paths=[]; function visit(items,prefix){for(const r of items){const p=r.path.startsWith('/')?r.path:prefix.replace(/\/$/,'')+'/'+r.path; paths.push(p); if(r.children)visit(r.children,p);}} visit(routes,''); const a=paths.filter(p=>p.toLowerCase().endsWith(suffix.toLowerCase())); if(a.length!==1)throw Error('路由不唯一'); return a[0]; }`, suffix, &path); err != nil { return err }
	if err := b.eval(`async path => {const vm=document.querySelector('#app').__vue__; if(vm.$route.path!==path)await vm.$router.push(path); await vm.$nextTick();}`, path, nil); err != nil { return err }
	return b.wait(`path => {const vm=document.querySelector('#app')?.__vue__;return !!vm?.$store?.getters?.token && vm.$route.path===path;}`, path)
}
