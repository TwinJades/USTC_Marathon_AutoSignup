package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
)

func TestLiveRegisteredAccount(t *testing.T) {
	if os.Getenv("USTC_LIVE_REGISTERED")!="1"{t.Skip("需要显式启用真实已报名账户验收")}
	c,err:=loadConfig();if err!=nil{t.Fatal(err)}
	lock,err:=lockInstance(runtimeRoot);if err!=nil{t.Fatal(err)};defer lock.Close()
	j,err:=openJournal(runtimeRoot);if err!=nil{t.Fatal(err)};defer j.db.Close()
	ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute);defer cancel()
	b,err:=newBrowser(ctx,c,j);if err!=nil{t.Fatal(err)};defer b.close()
	if err:=b.login(false);err!=nil{t.Fatal(err)}
	// 验收禁止真实提交；任何提交尝试都会导致验收失败。
	if err:=chromedp.Run(b.ctx,network.SetBlockedURLs([]string{"*scItemRegistration/enter/*","*scItem/marathon/save*"}));err!=nil{t.Fatal(err)}
	s:=&Website{b,j}
	account,err:=s.account();if err!=nil{t.Fatal(err)}
	rows,err:=s.own();if err!=nil{t.Fatal(err)}
	var submissions atomic.Int64
	chromedp.ListenTarget(b.ctx,func(event any){
		if e,okay:=event.(*network.EventRequestWillBeSent);okay&&e.Request.Method=="POST"&&(strings.Contains(e.Request.URL,"/scItemRegistration/enter/")||strings.Contains(e.Request.URL,"/scItem/marathon/save")){submissions.Add(1)}
	})
	var west Target
	for _,target:=range c.Targets {
		registered,err:=s.registered(target,rows[target.ID]);if err!=nil{t.Fatal(err)}
		if target.Campus=="西校区"{
			if !registered{t.Fatal("真实西校区报名未识别")}
			saved,err:=s.saved(target,account,rows[target.ID]);if err!=nil{t.Fatal(err)}
			if !saved{t.Fatal("当前账户参赛资料尚未保存，禁止执行监测验收")}
			west=target
		}else if registered{t.Fatal("当前验收要求高新校区尚未报名")}
	}
	if west.ID==""||len(c.Targets)!=2{t.Fatal("当前验收需要真实西校区及高新校区两场目标")}
	if err:=s.status();err!=nil{t.Fatal(err)}
	if err:=s.check();err!=nil{t.Fatal(err)}
	for run:=0;run<2;run++{
		if err:=s.watch();err!=nil{t.Fatal(err)}
		local,err:=j.rows();if err!=nil{t.Fatal(err)}
		complete:=false
		for _,r:=range local{if r.Account==accountKey(account["username"].(string))&&r.Item==west.ID&&r.Phase=="complete"{complete=true}}
		if !complete{t.Fatal("真实网站核实结果未持久保存")}
	}
	if submissions.Load()!=0{t.Fatal("已报名验收期间产生提交请求")}
	evidence:=map[string]any{"time":time.Now().Format("2006-01-02 15:04:05"),"version":version,"west_registered":true,"west_profile_matches":true,"gaoxin_registered":false,"watch_stops_on_complete":true,"repeat_watch_verified":true,"submission_requests":submissions.Load()}
	data,err:=json.MarshalIndent(evidence,"","  ");if err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(filepath.Join(runtimeRoot,"verification-registered-2.0.3.json"),data,0600);err!=nil{t.Fatal(err)}
}
