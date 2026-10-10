package main

import (
	"fmt"
	"time"
)

func (s *Website) confirm(t Target) (Object, error) {
	for attempt := 0; attempt < 3; attempt++ {
		rows, err := s.own()
		if err != nil { return nil, err }
		okay, err := registered(rows[t.ID])
		if err != nil { return nil, err }
		if okay { return rows[t.ID], nil }
		if attempt < 2 { if err := sleep(s.b.parent, time.Second); err != nil { return nil, err } }
	}
	return nil, nil
}

func (s *Website) complete(t Target, account Object, row Object, phase string) error {
	if _, err := s.detail(t); err != nil { return err }
	okay, err := s.saved(t, account, row)
	if err != nil { return err }
	username := account["username"].(string)
	if !okay {
		if phase == "save_pending" { return fmt.Errorf("此前资料提交结果仍未确认，请在网站核查；待核查记录已保留") }
		if err := s.j.record(username, t, "reserved"); err != nil { return err }
		result, err := s.save(t, account)
		if err != nil { return err }
		if result == full { return fmt.Errorf("取得名额后资料提交返回异常结果") }
		for attempt := 0; attempt < 3; attempt++ {
			row, err := s.confirm(t)
			if err != nil { return err }
			if row != nil {
				okay, err = s.saved(t, account, row)
				if err != nil { return err }
				if okay { break }
			}
			if attempt < 2 { if err := sleep(s.b.parent, time.Second); err != nil { return err } }
		}
		if !okay { return fmt.Errorf("资料提交结果无法确认，保留记录并停止全部监测") }
	}
	if err := s.j.record(username, t, "complete"); err != nil { return err }
	fmt.Printf("报名及资料保存已核实：%s（%s）。全部监测已经停止。\n", t.Name, t.Campus)
	return nil
}

func (s *Website) check() error {
	c := s.b.config
	if err := c.requireWatch(); err != nil { return err }
	account, err := s.account()
	if err != nil { return err }
	rows, err := s.own()
	if err != nil { return err }
	labels := map[string]string{"ended": "报名已结束", "waiting": "尚未开始报名", "full": "人数已满", "available": "存在空余名额"}
	for index, t := range c.Targets {
		a, err := s.availability(t)
		if err != nil { return err }
		label := labels[a.phase(time.Now())]
		reserved, err := registered(rows[t.ID])
		if err != nil { return err }
		if reserved {
			okay, err := s.saved(t, account, rows[t.ID])
			if err != nil { return err }
			label = "已经取得名额，资料待填写"
			if okay { label = "报名和资料已完成" }
		} else if _, err := s.options(t); err != nil { return err }
		fmt.Printf("%d. %s（%s）：%d/%d，%s\n", index+1, t.Name, t.Campus, a.Count, a.Capacity, label)
		if err := s.j.event("只读检查完成", map[string]any{"item": t.ID, "count": a.Count, "capacity": a.Capacity}); err != nil { return err }
	}
	pending, err := s.j.pending(account["username"].(string))
	if err != nil { return err }
	for _, r := range pending { fmt.Printf("存在待核查记录：%s，阶段 %s。\n", r.Name, r.Phase) }
	return nil
}

func (s *Website) watch() error {
	c := s.b.config
	if err := c.requireWatch(); err != nil { return err }
	account, err := s.account()
	if err != nil { return err }
	username := account["username"].(string)
	rows, err := s.own()
	if err != nil { return err }
	pending, err := s.j.pending(username)
	if err != nil { return err }
	if len(pending) == 1 {
		r := pending[0]
		for _, t := range c.Targets {
			if t.ID != r.Item { continue }
			if t.Name != r.Name || t.Campus != r.Campus { return fmt.Errorf("未完成报名记录与配置中的比赛不一致") }
			row, err := s.confirm(t)
			if err != nil { return err }
			if row == nil { return fmt.Errorf("此前提交结果仍无法确认，停止全部监测并保留待核查记录") }
			return s.complete(t, account, row, r.Phase)
		}
		return fmt.Errorf("未完成报名的比赛已从配置移除，请恢复目标后核查")
	}
	local, err := s.j.rows()
	if err != nil { return err }
	for _, t := range c.Targets {
		okay, err := registered(rows[t.ID])
		if err != nil { return err }
		if okay { return s.complete(t, account, rows[t.ID], "reserved") }
		for _, r := range local {
			if r.Item == t.ID && r.Account == accountKey(username) && r.Phase == "complete" { return fmt.Errorf("本地完成记录与网站报名记录不一致") }
		}
	}
	fmt.Println("开始监测；按 Ctrl+C 停止后续检查和提交。")
	for {
		if err := stableConfig(c); err != nil { return err }
		current, err := loadConfig()
		if err != nil { return err }
		rows, err = s.own()
		if err != nil { return err }
		ended, fullCount := 0, 0
		for _, t := range c.Targets {
			okay, err := registered(rows[t.ID])
			if err != nil { return err }
			if okay { return s.complete(t, account, rows[t.ID], "reserved") }
			a, err := s.availability(t)
			if err != nil { return err }
			phase := a.phase(time.Now())
			fmt.Printf("%s %s：%d/%d，%s\n", time.Now().In(china).Format("15:04:05"), t.Name, a.Count, a.Capacity, map[string]string{"ended":"报名结束","waiting":"等待开始","full":"人数已满","available":"发现空余名额"}[phase])
			if phase == "ended" { ended++; continue }
			if phase != "available" { if phase == "full" { fullCount++ }; continue }
			result, err := s.enter(t, username)
			if err != nil { return err }
			if result == full { if err := s.j.record(username, t, "full"); err != nil { return err }; continue }
			row, err := s.confirm(t)
			if err != nil { return err }
			if row == nil { return fmt.Errorf("报名提交结果无法确认，保留记录并停止全部监测") }
			return s.complete(t, account, row, "reserved")
		}
		if ended == len(c.Targets) { fmt.Println("全部目标比赛已经结束报名，监测已停止。"); return nil }
		if err := s.j.event("监测轮次完成", map[string]any{"targets": len(c.Targets), "ended": ended, "full": fullCount}); err != nil { return err }
		if err := waitSeconds(s.b.parent, current.Monitor.Interval); err != nil { return err }
	}
}
