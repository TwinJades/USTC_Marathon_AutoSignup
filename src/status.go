package main

import (
	"fmt"
	"time"
)

func phaseLabel(phase string) (string,error) {
	labels:=map[string]string{"enter_pending":"报名提交结果待核查","reserved":"已取得名额，资料待填写","save_pending":"资料提交结果待核查","complete":"报名和资料已完成","full":"名额已满","not_entered":"未取得名额"}
	label,okay:=labels[phase]
	if !okay{return "",fmt.Errorf("未知的本地报名阶段")}
	return label,nil
}

func (s *Website) status() error {
	if len(s.b.config.Targets)==0{return fmt.Errorf("请先选择目标比赛")}
	account,err:=s.account();if err!=nil{return err}
	rows,err:=s.own();if err!=nil{return err}
	local,err:=s.j.rows();if err!=nil{return err}
	for _,t:=range s.b.config.Targets {
		okay,err:=s.registered(t,rows[t.ID]);if err!=nil{return err}
		label:="未报名"
		if okay {
			saved,err:=s.saved(t,account,rows[t.ID]);if err!=nil{return err}
			label="已报名，参赛资料待填写"
			if saved{label="已报名，参赛资料已保存"}
		}
		fmt.Printf("%s：%s\n",t.Name,label)
		for _,r:=range local {
			if r.Account!=accountKey(account["username"].(string))||r.Item!=t.ID{continue}
			if r.Phase!="enter_pending"&&r.Phase!="reserved"&&r.Phase!="save_pending"{continue}
			phase,err:=phaseLabel(r.Phase);if err!=nil{return err}
			updated,err:=time.Parse(time.RFC3339Nano,r.Updated);if err!=nil{return fmt.Errorf("本地报名阶段时间无效")}
			fmt.Printf("本地待核查阶段：%s；更新时间：%s\n",phase,updated.Local().Format("2006-01-02 15:04:05"))
		}
	}
	fmt.Printf("查询时间：%s（本机时间）\n",time.Now().Format("2006-01-02 15:04:05"))
	return nil
}
