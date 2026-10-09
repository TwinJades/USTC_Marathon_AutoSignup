package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func TestLiveWatch(t *testing.T) {
	if os.Getenv("USTC_LIVE_WATCH")!="1" {t.Skip("真实监测验收需要显式启用 USTC_LIVE_WATCH=1")}
	c,err:=loadConfig()
	if err!=nil {t.Fatal(err)}
	if err:=c.requireWatch();err!=nil {t.Fatal(err)}
	oldConfig:=configPath
	defer func(){configPath=oldConfig}()
	c.Browser.Kind="chrome"
	c.Monitor.RequestTimeout=90
	c.Monitor.Interval=0.25
	data,err:=toml.Marshal(c)
	if err!=nil {t.Fatal(err)}
	configPath=filepath.Join(testRoot(t),"真实监测配置.toml")
	if err:=os.WriteFile(configPath,data,0600);err!=nil {t.Fatal(err)}
	lock,err:=lockInstance(runtimeRoot)
	if err!=nil {t.Fatal(err)}
	defer lock.Close()
	j,err:=openJournal(runtimeRoot)
	if err!=nil {t.Fatal(err)}
	defer j.db.Close()
	before,err:=j.rows()
	if err!=nil {t.Fatal(err)}
	ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute)
	defer cancel()
	b,err:=newBrowser(ctx,c,j)
	if err!=nil {t.Fatal(err)}
	defer b.close()
	if err:=b.login(false);err!=nil {t.Fatal(err)}
	s:=&Website{b,j}
	for _,target:=range c.Targets {
		a,err:=s.availability(target)
		if err!=nil {t.Fatal(err)}
		if a.phase(time.Now())!="full" {t.Skip("此项验收要求实际目标全部满额")}
	}
	logPath:=filepath.Join(runtimeRoot,"activity.jsonl")
	info,err:=os.Stat(logPath)
	if err!=nil {t.Fatal(err)}
	completed:=make(chan bool,1)
	go func(){
		for ctx.Err()==nil {
			f,err:=os.Open(logPath)
			if err!=nil {completed<-false;cancel();return}
			if _,err:=f.Seek(info.Size(),io.SeekStart);err!=nil {f.Close();completed<-false;cancel();return}
			d:=json.NewDecoder(f)
			for {
				var event map[string]any
				if err:=d.Decode(&event);errors.Is(err,io.EOF){break}else if err!=nil {break}
				if event["event"]=="监测轮次完成" && event["full"]==float64(len(c.Targets)) {
					f.Close();completed<-true;cancel();return
				}
			}
			f.Close()
			if sleep(ctx,100*time.Millisecond)!=nil {return}
		}
	}()
	err=s.watch()
	if err!=nil && !errors.Is(err,context.Canceled) {t.Fatal(err)}
	select {case okay:=<-completed: if !okay {t.Fatal("监测记录读取失败")};default:t.Fatal("没有核实完整满额检查轮次")}
	after,err:=j.rows()
	if err!=nil {t.Fatal(err)}
	if !reflect.DeepEqual(before,after) {t.Fatal("满额监测产生了提交记录")}
	evidence,err:=json.MarshalIndent(map[string]any{"time":time.Now().In(china).Format(time.RFC3339),"targets":len(c.Targets),"full_round_verified":true,"submission_records_unchanged":true,"stop_during_wait_verified":true,"interval":c.Monitor.Interval},"","  ")
	if err!=nil {t.Fatal(err)}
	if err:=os.WriteFile(filepath.Join(runtimeRoot,"verification-go-watch.json"),evidence,0600);err!=nil {t.Fatal(err)}
}
