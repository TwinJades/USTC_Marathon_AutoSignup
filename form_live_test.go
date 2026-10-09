package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
)

func TestLiveFirstForm(t *testing.T) {
	if os.Getenv("USTC_LIVE_FORM_VERIFY")!="1" {t.Skip("真实表单准备验收需要显式启用 USTC_LIVE_FORM_VERIFY=1")}
	c,err:=loadConfig()
	if err!=nil {t.Fatal(err)}
	if err:=c.requireWatch();err!=nil {t.Fatal(err)}
	c.Browser.Kind="chrome"
	c.Monitor.RequestTimeout=90
	lock,err:=lockInstance(runtimeRoot)
	if err!=nil {t.Fatal(err)}
	defer lock.Close()
	j,err:=openJournal(runtimeRoot)
	if err!=nil {t.Fatal(err)}
	defer j.db.Close()
	ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute)
	defer cancel()
	b,err:=newBrowser(ctx,c,j)
	if err!=nil {t.Fatal(err)}
	defer b.close()
	if err:=b.login(false);err!=nil {t.Fatal(err)}
	s:=&Website{b,j}
	target:=c.Targets[0]
	before,err:=s.own()
	if err!=nil {t.Fatal(err)}
	options,err:=s.options(target)
	if err!=nil {t.Fatal(err)}
	var submitted atomic.Bool
	chromedp.ListenTarget(b.ctx,func(event any){
		if e,okay:=event.(*network.EventRequestWillBeSent);okay&&e.Request.Method=="POST" {
			if strings.Contains(e.Request.URL,"/item/scItemRegistration/enter/")||strings.Contains(e.Request.URL,"/item/scItem/marathon/save") {submitted.Store(true)}
		}
	})
	start:=time.Now()
	if err:=s.prepareFirst(target,options);err!=nil {
		t.Fatal(err)
	}
	if time.Since(start)<10*time.Second {t.Fatal("实际准备过程未完整等待十秒")}
	after,err:=s.own()
	if err!=nil {t.Fatal(err)}
	if submitted.Load()||!reflect.DeepEqual(before[target.ID],after[target.ID]) {t.Fatal("表单准备产生了报名提交")}
	data,err:=json.MarshalIndent(map[string]any{"time":time.Now().In(china).Format(time.RFC3339),"notice_wait_at_least_ten_seconds":true,"confirm_button_available":true,"prefilled_department_and_class_preserved":true,"configured_phone_verified":true,"component_target_verified":true,"registration_requests_sent":false,"own_target_record_unchanged":true},"","  ")
	if err!=nil {t.Fatal(err)}
	if err:=os.WriteFile(filepath.Join(runtimeRoot,"verification-go-first-form.json"),data,0600);err!=nil {t.Fatal(err)}
}
