package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// 此项只读真实网站，使用本机配置与凭据，并保存脱敏结果。
func TestLiveBrowsers(t *testing.T) {
	if os.Getenv("USTC_LIVE_VERIFY") != "1" { t.Skip("真实网站验证需要显式启用 USTC_LIVE_VERIFY=1") }
	c,err:=loadConfig()
	if err!=nil {t.Fatal(err)}
	c.Monitor.RequestTimeout=90
	lock,err:=lockInstance(runtimeRoot)
	if err!=nil {t.Fatal(err)}
	defer lock.Close()
	j,err:=openJournal(runtimeRoot)
	if err!=nil {t.Fatal(err)}
	defer j.db.Close()
	for _,installed:=range installedBrowsers() {
		t.Run(installed.Kind,func(t *testing.T){
			current:=c
			current.Browser.Kind=installed.Kind
			current.Browser.Executable=""
			ctx,cancel:=context.WithTimeout(context.Background(),15*time.Minute)
			defer cancel()
			b,err:=newBrowser(ctx,current,j)
			if err!=nil {t.Fatal(err)}
			defer b.close()
			if err:=b.login(false);err!=nil {
				var state map[string]any
				if diagnosis:=b.eval(`() => ({host:location.hostname,path:location.pathname,app:!!document.querySelector('#app'),vue:!!document.querySelector('#app')?.__vue__})`,nil,&state);diagnosis==nil {t.Log("页面状态：",state)}
				t.Fatal(err)
			}
			s:=&Website{b,j}
			account,err:=s.account()
			if err!=nil {t.Fatal(err)}
			races,err:=s.races()
			if err!=nil {t.Fatal(err)}
			own,err:=s.own()
			if err!=nil {t.Fatal(err)}
			paginated,err:=s.pagesSized(listPath,1)
			if err!=nil || len(paginated)!=len(races) {t.Fatal("比赛列表分页查询不一致")}
			paginated,err=s.pagesSized(ownPath,25)
			if err!=nil || len(paginated)!=len(own) {t.Fatal("本人记录分页查询不一致")}
			results:=make([]map[string]any,0,len(c.Targets))
			for _,target:=range c.Targets {
				detail,err:=s.detail(target)
				if err!=nil {t.Fatal(err)}
				keys:=make([]string,0,len(detail))
				for key:=range detail {keys=append(keys,key)}
				sort.Strings(keys)
				a,err:=s.availability(target)
				if err!=nil {t.Fatal(err)}
				options,err:=s.options(target)
				if err!=nil {t.Fatal(err)}
				row:=own[target.ID]
				entry:=map[string]any{"count":a.Count,"capacity":a.Capacity,"phase":a.phase(time.Now()),"prefilled_required":options["院系"]!=""&&options["班级"]!="","own_record":row!=nil}
				entry["detail_fields"]=keys
				entry["application_fields"]=map[string]any{"applyNum":detail["applyNum"],"applyStatus":detail["applyStatus"],"applyRange":detail["applyRange"],"needApply":detail["needApply"],"booleanRegistration":detail["booleanRegistration"],"registrationStatus":detail["registrationStatus"]}
				t.Log("实际报名字段：",entry["application_fields"])
				if row!=nil { entry["applyStatus"]=row["applyStatus"]; entry["marathonDataStatus"]=row["marathonDataStatus"] }
				results=append(results,entry)
				if err:=s.searchRow(target,false);err!=nil {t.Fatal(err)}
				var selector string
				if err:=b.eval(domControl,map[string]any{"op":"rowDetail","id":target.ID},&selector);err!=nil {t.Fatal(err)}
				if err:=b.click(selector);err!=nil {t.Fatal(err)}
				if err:=s.dialog("项目详情");err!=nil {t.Fatal(err)}
				if err:=s.verifyComponent(target,"项目详情");err!=nil {t.Fatal(err)}
				selector,err=b.control("tab","项目详情","评分详情")
				if err!=nil {t.Fatal(err)}
				if err:=b.click(selector);err!=nil {t.Fatal(err)}
				var display struct {Count int; Capacity int; CanApply bool}
				if err:=b.wait(`() => [...document.querySelectorAll('.ant-modal-content')].filter(e=>e.getClientRects().length&&e.innerText.includes('项目详情')).some(e=>/项目参与\s*\d+\s*人/.test(e.textContent)&&/报名人数限制\s*[:：]?\s*\d+/.test(e.textContent))`,nil);err!=nil {t.Fatal(err)}
				if err:=b.eval(`() => {const a=[...document.querySelectorAll('.ant-modal-content')].filter(e=>e.getClientRects().length&&e.innerText.includes('项目详情')); if(a.length!==1)throw Error('详情不唯一'); const text=a[0].textContent; const count=text.match(/项目参与\s*(\d+)\s*人/); const capacity=text.match(/报名人数限制\s*[:：]?\s*(\d+)/); const buttons=[...a[0].querySelectorAll('button')].filter(e=>e.innerText.replace(/\s+/g,'')==='报名'); return {count:count?Number(count[1]):-1,capacity:capacity?Number(capacity[1]):-1,canApply:buttons.length===1&&!buttons[0].disabled};}`,nil,&display);err!=nil {t.Fatal(err)}
				if display.Count!=a.Count || display.Capacity!=a.Capacity {t.Fatal("网页显示人数与接口不一致")}
				evaluation,err:=s.query("/lib/scItemProgramme/selectEvaluationByItemId",map[string]any{"id":target.ID})
				if err!=nil {t.Fatal(err)}
				participants,err:=integer(evaluation,"registrationNum")
				if err!=nil || participants!=a.Count {t.Fatal("报名人数与网站参与人数不一致")}
				entry["registrationNum"]=participants
				entry["display_count"]=display.Count
				entry["display_capacity"]=display.Capacity
				entry["apply_button_available"]=display.CanApply
			}
			if err:=b.launch(true);err!=nil {t.Fatal(err)}
			if err:=b.wait(`() => location.origin==='https://young.ustc.edu.cn' && !!document.querySelector('#app')?.__vue__?.$store?.getters?.token`,nil);err!=nil {t.Fatal(err)}
			if _,err:=s.account();err!=nil {t.Fatal(err)}
			evidence:=map[string]any{"time":time.Now().In(china).Format(time.RFC3339),"browser":installed.Kind,"account_matches":account["username"]==c.Monitor.Username,"race_list_count":len(races),"own_records_count":len(own),"headless_session_reused":true,"targets":results}
			data,err:=json.MarshalIndent(evidence,"","  ")
			if err!=nil {t.Fatal(err)}
			if err:=os.WriteFile(filepath.Join(runtimeRoot,"verification-go-"+installed.Kind+".json"),data,0600);err!=nil {t.Fatal(err)}
			t.Log("实际网站查询及会话验证通过")
			b.close()
			if err:=b.ctx.Err();err==nil {t.Fatal("浏览器上下文没有关闭")}
		})
	}
}
