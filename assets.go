package main

import "fmt"

const loginProbe = `() => {
 const visible=e=>!!e.getClientRects().length && getComputedStyle(e).visibility!=='hidden';
 const all=[...document.querySelectorAll('input')], inputs=all.filter(visible);
 const meta=e=>[e.id,e.name,e.placeholder,e.autocomplete].filter(Boolean).join(' ');
 if(inputs.some(e=>['text','tel','number'].includes(e.type)&&/captcha|otp|sms|verify|验证码|短信|动态码/i.test(meta(e))) || [...document.querySelectorAll('iframe')].some(e=>visible(e)&&/captcha|recaptcha|hcaptcha/i.test(e.src)))return {kind:'verification'};
 const passwords=inputs.filter(e=>e.type==='password'), accounts=inputs.filter(e=>e.type!=='password'&&/username|account|login|学号|账号|工号|gid/i.test(meta(e)));
 if(passwords.length===1&&accounts.length===1)return {kind:'login',accountIndex:all.indexOf(accounts[0]),passwordIndex:all.indexOf(passwords[0])};
 if(!passwords.length&&/短信验证|新设备|二次认证|动态码|安全验证/.test(document.body?.innerText||''))return {kind:'verification'};
 return {kind:'waiting'};
}`

const pageBridge = `async ({path,params,timeout}) => {
 const allowed=new Set(['/item/scItem/queryApplyingMarathonItem','/item/scItem/queryById','/lib/scItemProgramme/selectEvaluationByItemId','/item/scItem/marathon/myInfo','/item/scItemRegistration/getOptions','/item/scParticipateItem/list?queryCatelog=0']);
 if(!allowed.has(path)&&!/^\/item\/scItem\/marathon\/my\/[A-Za-z0-9_-]+$/.test(path))throw Error('查询路径无效');
 if(location.origin!=='https://young.ustc.edu.cn')return {kind:'authentication'};
 const vm=document.querySelector('#app')?.__vue__; if(!vm?.$http?.get)throw Error('网站查询机制变化');
 return vm.$http.get(path,{params:params||{},timeout}).then(data=>({kind:'response',data}),error=>{if(error.response&&(error.response.status===401||error.response.data?.message==='Token失效，请重新登录'))return {kind:'authentication'}; throw Error('网站查询失败');});
}`

const domControl = `p => {
 const visible=e=>!!e.getClientRects().length&&getComputedStyle(e).visibility!=='hidden';
 const unique=(a)=>{if(a.length!==1)throw Error('页面字段不唯一'); return a[0];};
 let root=document;
 if(p.dialog)root=unique([...document.querySelectorAll('.ant-modal-content')].filter(visible).filter(e=>[...e.querySelectorAll('.ant-modal-title')].some(t=>t.innerText.trim().split(/\n/)[0].trim()===p.dialog)));
 let e;
 if(p.op==='button')e=unique([...root.querySelectorAll('button')].filter(visible).filter(e=>e.innerText.replace(/\s+/g,'')===p.label));
 else if(p.op==='input'||p.op==='value'||p.op==='fill'||p.op==='selected'||p.op==='select'){
  const item=unique([...root.querySelectorAll('.ant-form-item')].filter(e=>[...e.querySelectorAll('label')].some(l=>l.innerText.trim().replace(/[:：]$/,'')===p.label)));
  if(p.op==='selected')return unique([...item.querySelectorAll('.ant-select-selection-selected-value')]).innerText.trim();
  if(p.op==='select')e=unique([...item.querySelectorAll('.ant-select-selection')]);
  else e=unique([...item.querySelectorAll('input:not([type="hidden"])')]);
 } else if(p.op==='placeholder')e=unique([...document.querySelectorAll('input')].filter(visible).filter(e=>e.placeholder===p.label));
 else if(p.op==='row')e=unique([...document.querySelectorAll('tr.ant-table-row')].filter(e=>e.getAttribute('data-row-key')===p.id));
 else if(p.op==='rowDetail'){const row=unique([...document.querySelectorAll('tr.ant-table-row')].filter(e=>e.getAttribute('data-row-key')===p.id)); e=unique([...row.querySelectorAll('.pjList')]);}
 else if(p.op==='rowInfo'){const row=unique([...document.querySelectorAll('tr.ant-table-row')].filter(e=>e.getAttribute('data-row-key')===p.id)); e=unique([...row.querySelectorAll('a,button,span')].filter(visible).filter(e=>e.innerText.trim()==='填写信息').filter(e=>![...e.children].some(c=>c.innerText.trim()==='填写信息')));}
 else if(p.op==='option')e=unique([...document.querySelectorAll('.ant-select-dropdown')].filter(visible).flatMap(d=>[...d.querySelectorAll('.ant-select-dropdown-menu-item')]).filter(e=>e.innerText.trim()===p.label));
 else if(p.op==='tab')e=unique([...root.querySelectorAll('[role="tab"]')].filter(visible).filter(e=>e.innerText.trim()===p.label));
 else if(p.op==='popover')e=unique([...document.querySelectorAll('.ant-popover')].filter(visible).filter(e=>e.innerText.includes('确定报名')).flatMap(e=>[...e.querySelectorAll('button')]).filter(e=>e.innerText.replace(/\s+/g,'')==='确定'));
 else throw Error('操作无效');
 if(p.op==='value')return e.value;
 if(p.op==='fill'){Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(e,p.value); e.dispatchEvent(new Event('input',{bubbles:true})); e.dispatchEvent(new Event('change',{bubbles:true})); return e.value;}
 if(p.op==='button' && e.disabled)throw Error('按钮不可用');
 const id=String((window.__marathonControl=(window.__marathonControl||0)+1)); e.setAttribute('data-marathon-control',id); return '[data-marathon-control="'+id+'"]';
}`

func (b *Browser) control(op, dialog, label string) (string, error) {
	var value string
	err := b.eval(domControl, map[string]any{"op": op, "dialog": dialog, "label": label}, &value)
	return value, err
}

func (b *Browser) input(dialog, label, value string) error {
	var actual string
	if err := b.eval(domControl, map[string]any{"op": "fill", "dialog": dialog, "label": label, "value": value}, &actual); err != nil { return err }
	if actual != value { return fmt.Errorf("页面填写内容不一致") }
	return nil
}

func (b *Browser) press(dialog, label string) error {
	selector, err := b.control("button", dialog, label)
	if err != nil { return err }
	return b.click(selector)
}
