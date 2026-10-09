package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
)

const listPath = "/item/scItem/queryApplyingMarathonItem"
const ownPath = "/item/scParticipateItem/list?queryCatelog=0"
var china = time.FixedZone("Asia/Shanghai", 8*3600)
var fullMessage = regexp.MustCompile(`名额已满|人数已满|报名已满|人数已达上限|报名人数超过|报名人数已达`)
type Object map[string]any
type Website struct { b *Browser; j *Journal }
type Availability struct { Count, Capacity int; Start, End time.Time }
type Outcome int
const (unknown Outcome = iota; accepted; full)

func object(raw json.RawMessage) (Object, error) {
	var o Object
	d := json.NewDecoder(bytes.NewReader(raw)); d.UseNumber()
	if err := d.Decode(&o); err != nil || o == nil { return nil, fmt.Errorf("网站响应结构发生变化") }
	return o, nil
}

func text(o Object, key string) (string, error) {
	v, ok := o[key].(string)
	if !ok || strings.TrimSpace(v) == "" { return "", fmt.Errorf("网站缺少有效文本字段：%s", key) }
	return v, nil
}

func integer(o Object, key string) (int, error) {
	v := ""
	switch n := o[key].(type) { case string: v = n; case json.Number: v = string(n); default: return 0, fmt.Errorf("网站缺少有效整数字段：%s", key) }
	if !regexp.MustCompile(`^[0-9]+$`).MatchString(v) { return 0, fmt.Errorf("网站整数字段无效：%s", key) }
	n, err := strconv.Atoi(v)
	if err != nil { return 0, fmt.Errorf("网站整数字段超出范围：%s", key) }
	return n, nil
}

func (s *Website) query(path string, params map[string]any) (Object, error) {
	r, err := s.b.get(path, params)
	if err != nil { return nil, err }
	return object(r)
}

func (s *Website) account() (Object, error) {
	o, err := s.query("/item/scItem/marathon/myInfo", nil)
	if err != nil { return nil, err }
	u, err := text(o, "username")
	if err != nil { return nil, err }
	if _, err := text(o, "realname"); err != nil { return nil, err }
	if s.b.config.Monitor.Username != "" && u != s.b.config.Monitor.Username { return nil, fmt.Errorf("网站账户与配置账号不一致") }
	return o, nil
}

func (s *Website) pages(path string) ([]Object, error) {
	return s.pagesSized(path, 100)
}

func (s *Website) pagesSized(path string, size int) ([]Object, error) {
	var rows []Object
	seen := map[string]bool{}
	for page := 1; ; page++ {
		o, err := s.query(path, map[string]any{"pageNo": page, "pageSize": size})
		if err != nil { return nil, err }
		total, err := integer(o, "total")
		if err != nil { return nil, err }
		batch, ok := o["records"].([]any)
		if !ok { return nil, fmt.Errorf("网站分页缺少 records") }
		for _, raw := range batch {
			m, ok := raw.(map[string]any)
			if !ok { return nil, fmt.Errorf("网站分页记录无效") }
			id, err := text(Object(m), "id")
			if err != nil { return nil, err }
			if seen[id] { return nil, fmt.Errorf("网站分页出现重复记录") }
			seen[id] = true
			rows = append(rows, Object(m))
		}
		if len(rows) == total { return rows, nil }
		if len(rows) > total || len(batch) == 0 { return nil, fmt.Errorf("网站分页数量不一致") }
	}
}

func targetFrom(o Object) (Target, error) {
	var t Target
	var err error
	if t.ID, err = text(o, "id"); err != nil { return t, err }
	if t.Name, err = text(o, "itemName"); err != nil { return t, err }
	if t.Campus, err = text(o, "marathonCampus"); err != nil { return t, err }
	if !validID.MatchString(t.ID) || (t.Campus != "西校区" && t.Campus != "高新校区") { return t, fmt.Errorf("比赛编号或校区无效") }
	if fmt.Sprint(o["marathon"]) != "1" { return t, fmt.Errorf("马拉松列表出现其他类型比赛") }
	return t, nil
}

func (s *Website) races() ([]Target, error) {
	rows, err := s.pages(listPath)
	if err != nil { return nil, err }
	var targets []Target
	for _, row := range rows {
		id, err := text(row, "id")
		if err != nil { return nil, err }
		o, err := s.query("/item/scItem/queryById", map[string]any{"id": id})
		if err != nil { return nil, err }
		t, err := targetFrom(o)
		if err != nil { return nil, err }
		targets = append(targets, t)
	}
	return targets, nil
}

func (s *Website) own() (map[string]Object, error) {
	rows, err := s.pages(ownPath)
	if err != nil { return nil, err }
	result := map[string]Object{}
	for _, row := range rows { result[row["id"].(string)] = row }
	return result, nil
}

func (s *Website) detail(t Target) (Object, error) {
	o, err := s.query("/item/scItem/queryById", map[string]any{"id": t.ID})
	if err != nil { return nil, err }
	actual, err := targetFrom(o)
	if err != nil { return nil, err }
	if actual != t || fmt.Sprint(o["itemCategory"]) != "0" || fmt.Sprint(o["form"]) != "0" { return nil, fmt.Errorf("目标比赛名称、校区、编号或报名形式变化，请重新选择") }
	return o, nil
}

func parseTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999"} {
		if t, err := time.ParseInLocation(layout, value, china); err == nil { return t, nil }
	}
	return time.Time{}, fmt.Errorf("网站报名时间格式无效")
}

func (s *Website) availability(t Target) (Availability, error) {
	a := Availability{}
	o, err := s.detail(t)
	if err != nil { return a, err }
	if a.Capacity, err = integer(o, "peopleNum"); err != nil { return a, err }
	if a.Capacity <= 0 { return a, fmt.Errorf("缺少可靠的报名人数上限") }
	if a.Count, err = integer(o, "applyNum"); err != nil { return a, err }
	start, err := text(o, "applySt")
	if err != nil { return a, err }
	end, err := text(o, "applyEt")
	if err != nil { return a, err }
	if a.Start, err = parseTime(start); err != nil { return a, err }
	if a.End, err = parseTime(end); err != nil { return a, err }
	if !a.Start.Before(a.End) { return a, fmt.Errorf("网站报名起止时间无效") }
	if a.phase(time.Now()) != "ended" {
		status, err := integer(o, "applyStatus")
		if err != nil { return a, err }
		if status != 26 && status != 28 { return a, fmt.Errorf("当前账户的报名资格或报名状态需要核查") }
		rows, err := s.pages(listPath)
		if err != nil { return a, err }
		eligible := false
		for _, row := range rows { if row["id"] == t.ID { eligible = true } }
		if !eligible { return a, fmt.Errorf("目标比赛未出现在当前账户可报名列表，报名资格无法确认") }
	}
	return a, nil
}

func (a Availability) phase(now time.Time) string {
	if !now.Before(a.End) { return "ended" }
	if now.Before(a.Start) { return "waiting" }
	if a.Count >= a.Capacity { return "full" }
	return "available"
}

func (s *Website) options(t Target) (map[string]string, error) {
	r, err := s.b.get("/item/scItemRegistration/getOptions", map[string]any{"itemId": t.ID})
	if err != nil { return nil, err }
	var entries []Object
	if err := json.Unmarshal(r, &entries); err != nil { return nil, fmt.Errorf("第一阶段表单结构变化") }
	result := map[string]string{}
	for _, e := range entries {
		if e["value"] == "picUrls" { continue }
		label, err := text(e, "text")
		if err != nil { return nil, err }
		if _, exists := result[label]; exists { return nil, fmt.Errorf("第一阶段表单字段重复") }
		if label != "院系" && label != "班级" && label != "手机" { return nil, fmt.Errorf("第一阶段报名字段变化") }
		if label == "手机" { if e["value"] != "phone" { return nil, fmt.Errorf("手机字段结构变化") }; result[label] = "" } else {
			value, err := text(e, "text_value")
			if err != nil { return nil, fmt.Errorf("系统预填的院系或班级为空") }
			result[label] = value
		}
	}
	if len(result) != 3 { return nil, fmt.Errorf("第一阶段表单必填字段不完整") }
	return result, nil
}

func registered(row Object) (bool, error) {
	if row == nil { return false, nil }
	if fmt.Sprint(row["examineStatus"]) == "-3" { return false, fmt.Errorf("目标比赛已被网站终止") }
	status, err := integer(row, "applyStatus")
	if err != nil { return false, err }
	if status == 28 { return true, nil }
	if status == 26 { return false, nil }
	return false, fmt.Errorf("本人报名状态未知，需要核查")
}

func (s *Website) saved(t Target, account Object, row Object) (bool, error) {
	status, err := integer(row, "marathonDataStatus")
	if err != nil { return false, err }
	if status == 0 { return false, nil }
	if status != 1 && status != 2 { return false, fmt.Errorf("参赛资料状态未知") }
	o, err := s.query("/item/scItem/marathon/my/"+t.ID, nil)
	if err != nil { return false, err }
	p := s.b.config.Profile
	expected := Object{"username": account["username"], "realname": account["realname"], "itemId": t.ID, "campus": t.Campus, "type": p.Type, "departName": p.Department, "sex": p.Sex, "size": p.Size, "idNumber": p.IDNumber, "phone": p.Phone, "emergencyContact": p.Emergency}
	for key, value := range expected { if o[key] != value { return false, fmt.Errorf("网站保存的资料与当前配置不一致：%s", key) } }
	return true, nil
}

func (s *Website) searchRow(t Target, own bool) error {
	var opened bool
	if err := s.b.eval(`() => [...document.querySelectorAll('.ant-modal-content')].filter(e=>e.getClientRects().length).some(e=>[...e.querySelectorAll('.ant-modal-title')].some(t=>t.innerText.trim().split(/\n/)[0].trim()==='项目详情'))`, nil, &opened); err != nil { return err }
	if opened {
		if err := s.b.press("项目详情", "关闭"); err != nil { return err }
		if err := s.b.wait(`() => ![...document.querySelectorAll('.ant-modal-content')].filter(e=>e.getClientRects().length).some(e=>[...e.querySelectorAll('.ant-modal-title')].some(t=>t.innerText.trim().split(/\n/)[0].trim()==='项目详情'))`, nil); err != nil { return err }
	}
	suffix, placeholder := "/myproject/marathonProject", "项目名称"
	if own { suffix, placeholder = "/myproject/HaveSign", "请输入项目名称" }
	if err := s.b.route(suffix); err != nil { return fmt.Errorf("项目页面路由选择失败：%w", err) }
	if err := s.b.wait(`p => [...document.querySelectorAll('input')].some(e=>e.getClientRects().length&&e.placeholder===p)`, placeholder); err != nil { return err }
	selector, err := s.b.control("placeholder", "", placeholder)
	if err != nil { return err }
	if err := s.b.eval(`p => {const e=document.querySelector(p.selector); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(e,p.value); e.dispatchEvent(new Event('input',{bubbles:true})); e.dispatchEvent(new Event('change',{bubbles:true}));}`, map[string]any{"selector": selector, "value": t.Name}, nil); err != nil { return err }
	path := listPath
	if own { path = ownPath }
	if err := s.b.queryClick(path, func() error { return s.b.press("", "查询") }); err != nil { return err }
	return s.b.wait(`id => ![...document.querySelectorAll('.ant-spin-spinning')].some(e=>e.getClientRects().length) && [...document.querySelectorAll('tr.ant-table-row')].filter(e=>e.getAttribute('data-row-key')===id).length===1`, t.ID)
}

func (s *Website) dialog(title string) error {
	return s.b.wait(`title => [...document.querySelectorAll('.ant-modal-content')].filter(e=>e.getClientRects().length).some(e=>[...e.querySelectorAll('.ant-modal-title')].some(t=>t.innerText.trim().split(/\n/)[0].trim()===title)) && ![...document.querySelectorAll('.ant-spin-spinning')].some(e=>e.getClientRects().length)`, title)
}

func (s *Website) verifyComponent(t Target, title string) error {
	return s.b.wait(`p => {const root=document.querySelector('#app')?.__vue__;if(!root)throw Error('字段缺失');const found=[];function visit(vm){const matches=p.title==='报名信息'?Array.isArray(vm.regOptions)&&Array.isArray(vm.options)&&typeof vm.getmoduel==='function':vm.title===p.title;if(vm.visible&&matches&&String(vm.itemId)===p.id)found.push(vm);for(const child of vm.$children||[])visit(child);}visit(root);if(found.length!==1)return false;const vm=found[0];if(p.title==='项目详情')return vm.data?.id===p.id&&vm.data?.itemName===p.name&&vm.data?.marathonCampus===p.campus;return true;}`, map[string]any{"title": title, "id": t.ID, "name": t.Name, "campus": t.Campus})
}

func (s *Website) submission(t Target, username, stage, label, dialog, path string) (Outcome, error) {
	if err := stableConfig(s.b.config); err != nil { return unknown, err }
	account, err := s.account()
	if err != nil { return unknown, err }
	if account["username"] != username { return unknown, fmt.Errorf("提交前账户发生变化") }
	if err := s.verifyComponent(t, dialog); err != nil { return unknown, err }
	selector, err := s.b.control("button", dialog, label)
	if err != nil { return unknown, err }
	if err := s.j.record(username, t, stage); err != nil { return unknown, err }
	type reply struct { ID network.RequestID; Status int64; Failed bool }
	responses := make(chan reply, 1)
	listenCtx, cancel := context.WithCancel(s.b.ctx)
	defer cancel()
	var request network.RequestID
	var status int64
	chromedp.ListenTarget(listenCtx, func(event any) {
		switch e := event.(type) {
		case *network.EventRequestWillBeSent:
			if e.Request.Method == "POST" && e.Request.URL == siteURL+"/login/wisdom-group-learning-bg"+path { request = e.RequestID }
		case *network.EventResponseReceived:
			if request != "" && e.RequestID == request { status = e.Response.Status }
		case *network.EventLoadingFinished:
			if request != "" && e.RequestID == request { select { case responses <- reply{request, status, false}: default: } }
		case *network.EventLoadingFailed:
			if request != "" && e.RequestID == request { select { case responses <- reply{request, status, true}: default: } }
		}
	})
	if err := s.b.click(selector); err != nil { return unknown, nil }
	timer := time.NewTimer(seconds(s.b.config.Monitor.RequestTimeout))
	defer timer.Stop()
	select {
	case <-s.b.parent.Done(): return unknown, s.b.parent.Err()
	case <-timer.C: return unknown, nil
	case r := <-responses:
		if r.Failed || r.Status != 200 { return unknown, nil }
		var body []byte
		if err := chromedp.Run(s.b.ctx, chromedp.ActionFunc(func(ctx context.Context) error { var err error; body, err = network.GetResponseBody(r.ID).Do(ctx); return err })); err != nil { return unknown, nil }
		o, err := object(body)
		if err != nil { return unknown, nil }
		if o["success"] == true { return accepted, nil }
		message, ok := o["message"].(string)
		if ok && fullMessage.MatchString(message) { return full, nil }
		return unknown, nil
	}
}

func (s *Website) enter(t Target, username string) (Outcome, error) {
	rows, err := s.own()
	if err != nil { return unknown, err }
	already, err := registered(rows[t.ID])
	if err != nil { return unknown, err }
	if already { return accepted, nil }
	options, err := s.options(t)
	if err != nil { return unknown, err }
	a, err := s.availability(t)
	if err != nil { return unknown, err }
	if a.phase(time.Now()) != "available" { return full, nil }
	if err := s.searchRow(t, false); err != nil { return unknown, err }
	var selector string
	if err := s.b.eval(domControl, map[string]any{"op": "rowDetail", "id": t.ID}, &selector); err != nil { return unknown, err }
	if err := s.b.click(selector); err != nil { return unknown, err }
	if err := s.dialog("项目详情"); err != nil { return unknown, err }
	if err := s.verifyComponent(t, "项目详情"); err != nil { return unknown, err }
	if err := s.b.press("项目详情", "报名"); err != nil { return unknown, err }
	if err := s.b.wait(`() => [...document.querySelectorAll('.ant-modal-title,.ant-popover')].some(e=>e.getClientRects().length&&(e.innerText.trim()==='报名须知'||e.innerText.includes('确定报名')))`, nil); err != nil { return unknown, err }
	var popover bool
	if err := s.b.eval(`() => [...document.querySelectorAll('.ant-popover')].some(e=>e.getClientRects().length)`, nil, &popover); err != nil { return unknown, err }
	if popover { selector, err = s.b.control("popover", "", ""); if err != nil { return unknown, err }; if err := s.b.click(selector); err != nil { return unknown, err } }
	if err := s.dialog("报名须知"); err != nil { return unknown, err }
	visibleAt := time.Now()
	if err := sleep(s.b.parent, 10*time.Second); err != nil { return unknown, err }
	if err := s.b.wait(`() => [...document.querySelectorAll('.ant-modal-content')].filter(e=>e.getClientRects().length&&e.innerText.includes('报名须知')).some(e=>[...e.querySelectorAll('button')].some(b=>b.innerText.trim()==='确认报名'&&!b.disabled))`, nil); err != nil { return unknown, err }
	if time.Since(visibleAt) < 10*time.Second { return unknown, fmt.Errorf("报名须知等待不足十秒") }
	if err := s.j.event("报名须知等待完成", map[string]any{"item": t.ID, "seconds": time.Since(visibleAt).Seconds()}); err != nil { return unknown, err }
	if err := s.b.press("报名须知", "确认报名"); err != nil { return unknown, err }
	if err := s.dialog("报名信息"); err != nil { return unknown, err }
	for _, label := range []string{"院系", "班级"} {
		actual, err := s.b.control("value", "报名信息", label)
		if err != nil { return unknown, err }
		if actual != options[label] { return unknown, fmt.Errorf("系统预填的院系或班级无效") }
	}
	if err := s.b.input("报名信息", "手机", s.b.config.Profile.Phone); err != nil { return unknown, err }
	for _, label := range []string{"院系", "班级"} {
		actual, err := s.b.control("value", "报名信息", label)
		if err != nil { return unknown, err }
		if actual != options[label] { return unknown, fmt.Errorf("系统预填内容发生变化") }
	}
	a, err = s.availability(t)
	if err != nil { return unknown, err }
	if a.phase(time.Now()) != "available" { return full, nil }
	rows, err = s.own()
	if err != nil { return unknown, err }
	already, err = registered(rows[t.ID])
	if err != nil { return unknown, err }
	if already { return accepted, nil }
	return s.submission(t, username, "enter_pending", "确定", "报名信息", "/item/scItemRegistration/enter/"+t.ID)
}

func (s *Website) save(t Target, account Object) (Outcome, error) {
	var visible bool
	if err := s.b.eval(`() => [...document.querySelectorAll('.ant-modal-title')].some(e=>e.getClientRects().length&&e.innerText.trim()==='新增')`, nil, &visible); err != nil { return unknown, err }
	if !visible {
		if err := s.searchRow(t, true); err != nil { return unknown, err }
		var selector string
		if err := s.b.eval(domControl, map[string]any{"op": "rowInfo", "id": t.ID}, &selector); err != nil { return unknown, err }
		if err := s.b.click(selector); err != nil { return unknown, err }
	}
	if err := s.dialog("新增"); err != nil { return unknown, err }
	for label, value := range map[string]any{"姓名": account["realname"], "学/工号": account["username"]} {
		actual, err := s.b.control("value", "新增", label)
		if err != nil { return unknown, err }
		if actual != value { return unknown, fmt.Errorf("第二阶段预填账户资料不一致：%s", label) }
	}
	campus, err := s.b.control("selected", "新增", "参赛校区")
	if err != nil { return unknown, err }
	if campus != t.Campus { return unknown, fmt.Errorf("第二阶段校区与目标比赛不一致") }
	p := s.b.config.Profile
	for label, value := range map[string]string{"用户类型": p.Type, "性别": p.Sex, "衣服尺码": p.Size} {
		selector, err := s.b.control("select", "新增", label)
		if err != nil { return unknown, err }
		if err := s.b.click(selector); err != nil { return unknown, err }
		if err := s.b.wait(`() => [...document.querySelectorAll('.ant-select-dropdown')].some(e=>e.getClientRects().length)`, nil); err != nil { return unknown, err }
		selector, err = s.b.control("option", "", value)
		if err != nil { return unknown, err }
		if err := s.b.click(selector); err != nil { return unknown, err }
		actual, err := s.b.control("selected", "新增", label)
		if err != nil { return unknown, err }
		if actual != value { return unknown, fmt.Errorf("第二阶段选项填写不一致：%s", label) }
	}
	for label, value := range map[string]string{"学院/部门": p.Department, "身份证号": p.IDNumber, "手机号码": p.Phone, "紧急联系人联系方式": p.Emergency} {
		if err := s.b.input("新增", label, value); err != nil { return unknown, err }
	}
	return s.submission(t, account["username"].(string), "save_pending", "提交", "新增", "/item/scItem/marathon/save")
}
