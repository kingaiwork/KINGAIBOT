package platform

import (
	"net/http"
	"strings"
)

func (m *Manager) UIHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ui/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/" {
			http.NotFound(w, r)
			return
		}
		uiHeaders(w, "text/html; charset=utf-8")
		_, _ = w.Write([]byte(controlCenterHTML))
	})
	mux.HandleFunc("GET /ui/app.css", func(w http.ResponseWriter, _ *http.Request) {
		uiHeaders(w, "text/css; charset=utf-8")
		_, _ = w.Write([]byte(controlCenterCSS))
	})
	mux.HandleFunc("GET /ui/app.js", func(w http.ResponseWriter, _ *http.Request) {
		uiHeaders(w, "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(controlCenterJS))
	})
	return mux
}

func uiHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	if strings.Contains(contentType, "text/html") {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	}
}

const controlCenterHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="color-scheme" content="dark light">
<title>KINGAIBOT Control Center</title>
<link rel="stylesheet" href="/ui/app.css">
</head>
<body>
<header class="topbar">
  <div>
    <div class="eyebrow">KING AI · EXECUTION PLATFORM</div>
    <h1>KINGAIBOT Control Center</h1>
  </div>
  <div class="connect">
    <input id="token" type="password" autocomplete="off" spellcheck="false" placeholder="Admin / Scoped API Token">
    <button id="connectBtn">连接</button>
    <span id="connection" class="pill">未连接</span>
  </div>
</header>
<main>
  <section class="hero">
    <div><span class="dot"></span><span id="healthText">等待连接</span></div>
    <div class="muted">Token 仅保存在当前页面内存，刷新后自动清除。</div>
  </section>
  <section id="metrics" class="metrics"></section>
  <nav id="nav" class="nav" aria-label="Control Center modules"></nav>
  <section class="workspace">
    <div class="workspace-head">
      <div><div class="eyebrow" id="moduleEyebrow">PLATFORM</div><h2 id="moduleTitle">概览</h2></div>
      <button id="refreshBtn" class="secondary">刷新</button>
    </div>
    <div id="actions" class="actions"></div>
    <div id="content" class="content"><div class="empty">输入 Token 并连接以读取平台状态。</div></div>
  </section>
</main>
<footer>KINGAIBOT 1.3 · policy → exact approval → audit → execution</footer>
<script src="/ui/app.js" defer></script>
</body>
</html>`

const controlCenterCSS = `:root{font-family:Inter,ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0b0d10;color:#f5f7fa;font-synthesis:none}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:radial-gradient(circle at 15% 0%,#1a2230 0,transparent 32rem),#0b0d10;color:#f5f7fa}.topbar{position:sticky;top:0;z-index:10;display:flex;justify-content:space-between;gap:2rem;align-items:center;padding:1.2rem clamp(1rem,4vw,4rem);background:rgba(11,13,16,.84);backdrop-filter:blur(24px);border-bottom:1px solid rgba(255,255,255,.08)}h1,h2{margin:.15rem 0 0;letter-spacing:-.035em}h1{font-size:clamp(1.35rem,3vw,2rem)}h2{font-size:1.45rem}.eyebrow{font-size:.68rem;letter-spacing:.18em;color:#9aa4b2;font-weight:700}.connect{display:flex;gap:.55rem;align-items:center;flex-wrap:wrap;justify-content:flex-end}input,textarea,select{width:100%;background:#11151b;color:#f8fafc;border:1px solid #2b3340;border-radius:12px;padding:.72rem .82rem;outline:none}input:focus,textarea:focus,select:focus{border-color:#7d9cff;box-shadow:0 0 0 3px rgba(125,156,255,.12)}#token{width:min(42vw,360px)}button{border:0;border-radius:12px;padding:.72rem 1rem;font-weight:700;cursor:pointer;background:#f4f7fb;color:#11151a}button:hover{filter:brightness(.92)}button.secondary{background:#1b212b;color:#e8edf5;border:1px solid #303947}.pill{display:inline-flex;padding:.45rem .7rem;border-radius:999px;background:#20252d;color:#b7c0cc;font-size:.78rem}.pill.ok{background:#163523;color:#8ce3a9}.pill.bad{background:#3c1d21;color:#ffadb4}main{width:min(1400px,94vw);margin:1.5rem auto 3rem}.hero{display:flex;justify-content:space-between;gap:1rem;align-items:center;padding:.85rem 1rem;margin-bottom:1rem;border:1px solid rgba(255,255,255,.08);border-radius:16px;background:rgba(17,21,27,.68)}.dot{display:inline-block;width:.55rem;height:.55rem;border-radius:50%;background:#7d9cff;margin-right:.55rem;box-shadow:0 0 18px #7d9cff}.muted{color:#8e99a8;font-size:.82rem}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:.75rem;margin:1rem 0}.metric{padding:1rem;border-radius:18px;background:linear-gradient(145deg,#151a22,#10141a);border:1px solid rgba(255,255,255,.07)}.metric strong{display:block;font-size:1.7rem;letter-spacing:-.04em}.metric span{font-size:.78rem;color:#96a0af}.nav{display:flex;gap:.45rem;overflow:auto;padding:.25rem 0 1rem;scrollbar-width:none}.nav button{white-space:nowrap;background:#141920;color:#aeb8c5;border:1px solid #252d39;padding:.55rem .8rem}.nav button.active{background:#eaf0ff;color:#121722}.workspace{border-radius:22px;background:rgba(15,18,24,.88);border:1px solid rgba(255,255,255,.08);padding:clamp(1rem,3vw,1.6rem);box-shadow:0 25px 80px rgba(0,0,0,.2)}.workspace-head{display:flex;justify-content:space-between;align-items:center;gap:1rem}.actions{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:.75rem;margin:1rem 0}.action-card{background:#11161d;border:1px solid #252d38;border-radius:16px;padding:.9rem}.action-card h3{margin:.1rem 0 .7rem;font-size:.92rem}.action-card label{display:block;color:#9da8b5;font-size:.73rem;margin:.55rem 0 .25rem}.action-card textarea{min-height:88px;resize:vertical}.action-card button{margin-top:.7rem;width:100%}.content{min-height:280px}.list{display:grid;gap:.65rem}.item{background:#0d1117;border:1px solid #222a35;border-radius:15px;padding:.9rem 1rem;display:grid;grid-template-columns:minmax(120px,.6fr) minmax(0,2fr);gap:1rem}.item-title{font-weight:750;word-break:break-word}.item-meta{font-size:.75rem;color:#8290a0;margin-top:.3rem}.item pre{margin:0;white-space:pre-wrap;word-break:break-word;color:#bcc6d3;font:12px/1.55 ui-monospace,SFMono-Regular,Menlo,monospace;max-height:260px;overflow:auto}.empty,.error{padding:3rem 1rem;text-align:center;color:#8994a3}.error{color:#ff9ea7}.toast{position:fixed;right:1rem;bottom:1rem;max-width:min(420px,90vw);padding:.9rem 1rem;border-radius:14px;background:#eaf0ff;color:#10151c;box-shadow:0 16px 50px rgba(0,0,0,.35);z-index:20}footer{text-align:center;color:#697584;font-size:.75rem;padding:2rem}@media(max-width:760px){.topbar{align-items:flex-start;flex-direction:column;gap:.8rem}.connect{justify-content:flex-start;width:100%}#token{width:100%;flex:1}.hero{align-items:flex-start;flex-direction:column}.item{grid-template-columns:1fr}.workspace-head{align-items:flex-start}.muted{line-height:1.45}}`

const controlCenterJS = `(() => {
'use strict';
let token = '';
let active = 'agents';
const modules = [
  ['agents','Agents','/v1/platform/agents'],['sessions','Sessions','/v1/platform/sessions'],['schedules','Schedules','/v1/platform/schedules'],
  ['workflows','Workflows','/v1/platform/workflows'],['missions','Missions','/v1/platform/missions'],['nodes','Nodes','/v1/platform/nodes'],
  ['plugins','Plugins','/v1/platform/plugins'],['channels','Channels','/v1/platform/channels'],['skills','Skills','/v1/platform/skills'],
  ['knowledge','Knowledge','/v1/knowledge/items'],['workers','Workers','/v1/cluster/workers'],['jobs','Cluster Jobs','/v1/cluster/jobs'],
  ['evolution','Evolution','/v1/evolution/control/proposals'],['identities','Identities','/v1/platform/identities']
];
const $ = (id) => document.getElementById(id);
const headers = () => ({'Authorization':'Bearer '+token,'Content-Type':'application/json','Accept':'application/json'});
async function api(path, options){
  if(!token) throw new Error('请先输入 Token 并连接');
  const opt = options || {};
  opt.headers = Object.assign({}, headers(), opt.headers || {});
  const res = await fetch(path,opt);
  if(res.status === 204) return null;
  const text = await res.text();
  let body = text; try{ body = text ? JSON.parse(text) : null; }catch(_e){}
  if(!res.ok){ const detail = body && (body.detail || body.error) ? (body.detail || body.error) : text; throw new Error(res.status+' '+detail); }
  return body;
}
function toast(message){ const old=document.querySelector('.toast'); if(old)old.remove(); const n=document.createElement('div'); n.className='toast'; n.textContent=message; document.body.appendChild(n); setTimeout(()=>n.remove(),3600); }
function safeLabel(k){ return String(k).replaceAll('_',' '); }
function renderMetrics(status){ const box=$('metrics'); box.replaceChildren(); const counts=(status&&status.counts)||{}; Object.keys(counts).sort().slice(0,16).forEach(k=>{ const c=document.createElement('div'); c.className='metric'; const strong=document.createElement('strong'); strong.textContent=counts[k]; const span=document.createElement('span'); span.textContent=safeLabel(k); c.append(strong,span); box.appendChild(c); }); }
function summary(obj){ return obj.name || obj.title || obj.kind || obj.status || obj.id || 'item'; }
function meta(obj){ const parts=[]; if(obj.status)parts.push(obj.status); if(obj.id)parts.push(obj.id); if(obj.created_at)parts.push(new Date(obj.created_at).toLocaleString()); return parts.join(' · '); }
function renderList(data){
  const content=$('content'); content.replaceChildren();
  let rows=data; if(data && !Array.isArray(data)){ const candidates=['agents','sessions','schedules','workflows','missions','nodes','plugins','channels','skills','workers','jobs','proposals','identities','access_keys']; for(const k of candidates){ if(Array.isArray(data[k])){rows=data[k];break;} } }
  if(!Array.isArray(rows)) rows = rows ? [rows] : [];
  if(!rows.length){ const e=document.createElement('div'); e.className='empty'; e.textContent='暂无记录'; content.appendChild(e); return; }
  const list=document.createElement('div'); list.className='list';
  rows.forEach(obj=>{ const item=document.createElement('article'); item.className='item'; const left=document.createElement('div'); const title=document.createElement('div'); title.className='item-title'; title.textContent=summary(obj); const m=document.createElement('div'); m.className='item-meta'; m.textContent=meta(obj); left.append(title,m); const pre=document.createElement('pre'); pre.textContent=JSON.stringify(obj,null,2); item.append(left,pre); list.appendChild(item); });
  content.appendChild(list);
}
function errorView(err){ const c=$('content'); c.replaceChildren(); const e=document.createElement('div'); e.className='error'; e.textContent=err.message||String(err); c.appendChild(e); }
function field(card,label,name,type,value){ const l=document.createElement('label'); l.textContent=label; let input; if(type==='textarea'){ input=document.createElement('textarea'); }else{ input=document.createElement('input'); input.type=type||'text'; } input.name=name; if(value!==undefined)input.value=value; card.append(l,input); return input; }
function actionCard(title){ const c=document.createElement('form'); c.className='action-card'; const h=document.createElement('h3'); h.textContent=title; c.appendChild(h); return c; }
function submitButton(card,label){ const b=document.createElement('button'); b.type='submit'; b.textContent=label; card.appendChild(b); }
function actionsFor(name){
  const box=$('actions'); box.replaceChildren();
  if(name==='agents'){
    const f=actionCard('创建 Agent'); const n=field(f,'名称','name'); const p=field(f,'System Prompt','prompt','textarea'); submitButton(f,'创建'); f.onsubmit=async e=>{e.preventDefault(); await api('/v1/platform/agents',{method:'POST',body:JSON.stringify({name:n.value,system_prompt:p.value})}); toast('Agent 已创建'); load();}; box.appendChild(f);
  } else if(name==='sessions'){
    const f=actionCard('新建会话'); const a=field(f,'Agent ID（可选）','agent'); submitButton(f,'创建 Session'); f.onsubmit=async e=>{e.preventDefault(); const body={}; if(a.value.trim())body.agent_id=a.value.trim(); const s=await api('/v1/platform/sessions',{method:'POST',body:JSON.stringify(body)}); toast('Session '+s.id+' 已创建'); load();};
    const g=actionCard('发送消息'); const sid=field(g,'Session ID','sid'); const text=field(g,'消息','text','textarea'); submitButton(g,'发送'); g.onsubmit=async e=>{e.preventDefault(); const t=await api('/v1/platform/sessions/'+encodeURIComponent(sid.value)+'/messages',{method:'POST',body:JSON.stringify({message:text.value})}); toast('任务 '+t.id+' 已进入 Runtime'); load();}; box.append(f,g);
  } else if(name==='schedules'){
    const f=actionCard('创建定时任务'); const n=field(f,'名称','name'); const p=field(f,'Prompt','prompt','textarea'); const i=field(f,'间隔秒（>=60）','interval','number','3600'); submitButton(f,'创建 Schedule'); f.onsubmit=async e=>{e.preventDefault(); await api('/v1/platform/schedules',{method:'POST',body:JSON.stringify({name:n.value,prompt:p.value,interval_seconds:Number(i.value)})}); toast('Schedule 已创建'); load();}; box.appendChild(f);
  } else if(name==='missions'){
    const f=actionCard('并行 Mission'); const o=field(f,'目标','objective','textarea'); const ids=field(f,'Agent IDs（逗号分隔，可空）','ids'); submitButton(f,'派发'); f.onsubmit=async e=>{e.preventDefault(); const agent_ids=ids.value.split(',').map(x=>x.trim()).filter(Boolean); await api('/v1/platform/missions',{method:'POST',body:JSON.stringify({objective:o.value,agent_ids:agent_ids,mode:'parallel'})}); toast('Mission 已派发'); load();}; box.appendChild(f);
  } else if(name==='knowledge'){
    const f=actionCard('提交知识提案（Admin）'); const kind=field(f,'类型','kind','text','note'); const content=field(f,'内容','content','textarea'); const source=field(f,'来源','source'); submitButton(f,'提交 Proposal'); f.onsubmit=async e=>{e.preventDefault(); await api('/v1/knowledge/admin/items',{method:'POST',body:JSON.stringify({item:{kind:kind.value,content:content.value,source:source.value,confidence:.7},approved:false})}); toast('知识提案已提交，尚未进入可信检索'); load();}; box.appendChild(f);
  } else if(name==='workers'){
    const f=actionCard('注册 Worker（Admin）'); const n=field(f,'名称','name'); const caps=field(f,'Capabilities（逗号分隔）','caps'); submitButton(f,'注册 Worker'); f.onsubmit=async e=>{e.preventDefault(); const v=await api('/v1/cluster/workers',{method:'POST',body:JSON.stringify({name:n.value,capabilities:caps.value.split(',').map(x=>x.trim()).filter(Boolean)})}); toast('Worker token（仅本次显示）: '+v.token); load();}; box.appendChild(f);
  } else if(name==='jobs'){
    const f=actionCard('提交 Cluster Job（Admin）'); const kind=field(f,'Kind','kind'); const caps=field(f,'Capabilities','caps'); const payload=field(f,'Payload JSON','payload','textarea','{}'); submitButton(f,'提交 Job'); f.onsubmit=async e=>{e.preventDefault(); let p={}; try{p=JSON.parse(payload.value||'{}');}catch(_e){throw new Error('Payload 不是合法 JSON');} await api('/v1/cluster/jobs',{method:'POST',body:JSON.stringify({kind:kind.value,payload:p,required_capabilities:caps.value.split(',').map(x=>x.trim()).filter(Boolean),replay_policy:'manual'})}); toast('Job 已进入 Durable Queue'); load();}; box.appendChild(f);
  } else if(name==='evolution'){
    const f=actionCard('创建改进提案（Admin）'); const title=field(f,'标题','title'); const rationale=field(f,'理由','rationale','textarea'); submitButton(f,'创建 Proposal'); f.onsubmit=async e=>{e.preventDefault(); await api('/v1/evolution/control/proposals',{method:'POST',body:JSON.stringify({kind:'improvement',title:title.value,rationale:rationale.value,risk:'medium'})}); toast('Evolution proposal 已创建'); load();}; box.appendChild(f);
  } else if(name==='identities'){
    const f=actionCard('创建 Scoped Identity（Admin）'); const n=field(f,'名称','name'); const role=field(f,'角色 viewer/operator/automation/admin','role','text','viewer'); submitButton(f,'创建 Identity'); f.onsubmit=async e=>{e.preventDefault(); await api('/v1/platform/identities',{method:'POST',body:JSON.stringify({name:n.value,roles:[role.value]})}); toast('Identity 已创建'); load();}; box.appendChild(f);
  }
}
async function loadStatus(){ const s=await api('/v1/platform/status'); renderMetrics(s); $('healthText').textContent='Platform healthy · '+new Date(s.time).toLocaleTimeString(); }
async function load(){
  actionsFor(active); const mod=modules.find(x=>x[0]===active); $('moduleTitle').textContent=mod[1]; $('moduleEyebrow').textContent=active.toUpperCase();
  try{ const data=await api(mod[2]); renderList(data); }catch(err){ errorView(err); }
}
function buildNav(){ const nav=$('nav'); modules.forEach(m=>{ const b=document.createElement('button'); b.textContent=m[1]; b.dataset.key=m[0]; if(m[0]===active)b.className='active'; b.onclick=()=>{active=m[0]; nav.querySelectorAll('button').forEach(x=>x.classList.toggle('active',x.dataset.key===active)); load();}; nav.appendChild(b); }); }
$('connectBtn').onclick=async()=>{ token=$('token').value.trim(); if(!token){toast('请输入 Token');return;} try{ await loadStatus(); $('connection').textContent='已连接'; $('connection').className='pill ok'; await load(); }catch(err){ $('connection').textContent='连接失败'; $('connection').className='pill bad'; errorView(err); } };
$('refreshBtn').onclick=async()=>{ try{await loadStatus();await load();}catch(err){errorView(err);} };
buildNav(); actionsFor(active);
})();`
