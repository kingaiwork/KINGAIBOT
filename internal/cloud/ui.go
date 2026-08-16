package cloud

import (
	"net/http"
	"strings"
)

func ControlCenterHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ui/cloud/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/cloud/" {
			http.NotFound(w, r)
			return
		}
		cloudUIHeaders(w, "text/html; charset=utf-8")
		_, _ = w.Write([]byte(cloudControlHTML))
	})
	mux.HandleFunc("GET /ui/cloud/app.css", func(w http.ResponseWriter, _ *http.Request) {
		cloudUIHeaders(w, "text/css; charset=utf-8")
		_, _ = w.Write([]byte(cloudControlCSS))
	})
	mux.HandleFunc("GET /ui/cloud/app.js", func(w http.ResponseWriter, _ *http.Request) {
		cloudUIHeaders(w, "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(cloudControlJS))
	})
	return mux
}

func cloudUIHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	if strings.Contains(contentType, "text/html") {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	}
}

const cloudControlHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="color-scheme" content="light dark">
<title>KING AI Cloud & Fleet</title>
<link rel="stylesheet" href="/ui/cloud/app.css">
</head>
<body>
<header>
  <a class="back" href="/ui/">← Control Center</a>
  <div class="brand"><span>KING AI</span><strong>Cloud & Fleet</strong></div>
  <div class="connect"><input id="token" type="password" autocomplete="off" spellcheck="false" placeholder="Admin Token"><button id="connect">连接</button></div>
</header>
<main>
  <section class="hero">
    <div><div class="eyebrow">DEVICE CONTINUITY</div><h1>这台设备，现在属于谁、由谁管理，一眼看清。</h1><p>云端可以收紧策略、汇总健康状态和保存端到端加密连续性快照，但不能扩大本机权限，也不能替本机批准高风险操作。</p></div>
    <div id="statePill" class="pill">等待连接</div>
  </section>
  <section id="restartBox" class="notice hidden"></section>
  <section id="cards" class="cards"></section>
  <section class="panel">
    <div class="panelHead"><div><div class="eyebrow">GOVERNANCE</div><h2>云策略</h2><p>Channel 禁用会立即审计并收紧；Provider、Runtime 上限与 Tool Policy 的变化会明确标记“需要重启”，避免假装已热应用。</p></div><button id="pull" class="secondary">立即拉取策略</button></div>
    <pre id="policy">连接后显示当前 restriction-only policy。</pre>
  </section>
  <section class="panel split">
    <div><div class="eyebrow">E2EE MEMORY</div><h2>连续性同步</h2><p id="syncText">同步密钥只存在于受管设备，不上传到 KING AI Cloud。</p></div>
    <button id="sync">立即同步</button>
  </section>
  <section class="panel split">
    <div><div class="eyebrow">DEVICE IDENTITY</div><h2>Ed25519 设备密钥</h2><p id="rotationText">两阶段轮换：旧密钥和新密钥都必须证明所有权；网络结果不确定时保留本机 pending key，下一次重试可幂等恢复。</p></div>
    <button id="rotate" class="danger">轮换设备密钥</button>
  </section>
  <section id="errorBox" class="error hidden"></section>
</main>
<footer>KINGAIBOT 1.8 · local authority remains final · ciphertext-only cloud continuity</footer>
<script src="/ui/cloud/app.js" defer></script>
</body>
</html>`

const cloudControlCSS = `:root{font-family:Inter,ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#17202a;background:#f5f1ea;font-synthesis:none}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:radial-gradient(circle at 85% 0,#e7edf9 0,transparent 34rem),linear-gradient(180deg,#faf8f3,#f0ece4);color:#17202a}header{display:grid;grid-template-columns:1fr auto 1fr;align-items:center;gap:1rem;padding:1rem clamp(1rem,4vw,4rem);position:sticky;top:0;background:rgba(250,248,243,.86);backdrop-filter:blur(22px);border-bottom:1px solid rgba(23,32,42,.08);z-index:10}.back{color:#526170;text-decoration:none;font-size:.86rem}.brand{text-align:center}.brand span{display:block;font-size:.65rem;letter-spacing:.2em;color:#76818d}.brand strong{font-size:1.05rem;letter-spacing:-.02em}.connect{display:flex;justify-content:flex-end;gap:.5rem}.connect input{width:min(260px,32vw)}input{border:1px solid #d6d0c6;border-radius:12px;background:rgba(255,255,255,.75);color:#17202a;padding:.7rem .8rem;outline:none}input:focus{border-color:#7b91bd;box-shadow:0 0 0 3px rgba(89,116,170,.12)}button{border:0;border-radius:12px;background:#17202a;color:#fff;padding:.7rem 1rem;font-weight:700;cursor:pointer}button.secondary{background:#e8e2d8;color:#283442}button.danger{background:#842f31}button:hover{filter:brightness(.96)}main{width:min(1180px,92vw);margin:2.2rem auto 4rem}.hero{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:2rem;align-items:center;padding:clamp(1.3rem,4vw,2.5rem);border:1px solid rgba(23,32,42,.08);border-radius:28px;background:rgba(255,255,255,.58);box-shadow:0 28px 90px rgba(44,51,57,.08)}.eyebrow{font-size:.68rem;letter-spacing:.18em;font-weight:800;color:#7b8590}h1{max-width:760px;font-size:clamp(2rem,5vw,4.2rem);line-height:1.02;letter-spacing:-.055em;margin:.6rem 0 1rem}h2{font-size:1.45rem;letter-spacing:-.035em;margin:.25rem 0}p{color:#65717d;line-height:1.7;margin:.4rem 0}.pill{border-radius:999px;padding:.65rem .9rem;background:#e8e2d8;color:#65717d;font-size:.82rem;white-space:nowrap}.pill.ok{background:#dbeedd;color:#235e36}.pill.warn{background:#f4e7ca;color:#815e17}.notice{margin-top:1rem;padding:1rem 1.1rem;border-radius:16px;background:#f4e7ca;color:#6f5115;border:1px solid rgba(129,94,23,.16)}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:.8rem;margin:1.2rem 0}.card{padding:1.15rem;border-radius:20px;background:rgba(255,255,255,.72);border:1px solid rgba(23,32,42,.08)}.card span{display:block;color:#77828d;font-size:.72rem;text-transform:uppercase;letter-spacing:.08em}.card strong{display:block;margin-top:.45rem;font-size:1.1rem;word-break:break-word}.panel{margin-top:1rem;padding:1.3rem;border-radius:22px;border:1px solid rgba(23,32,42,.08);background:rgba(255,255,255,.68)}.panelHead,.split{display:flex;align-items:center;justify-content:space-between;gap:1.2rem}.panelHead>div,.split>div{min-width:0}.panel pre{margin:1rem 0 0;padding:1rem;border-radius:15px;background:#18212b;color:#dfe6ee;white-space:pre-wrap;word-break:break-word;max-height:420px;overflow:auto;font:12px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace}.error{margin-top:1rem;padding:1rem;border-radius:16px;background:#fde2e1;color:#8a3030}.hidden{display:none}footer{text-align:center;color:#84909b;font-size:.72rem;padding:2rem}@media(prefers-color-scheme:dark){:root{background:#0c1015;color:#f3f0ea}body{background:radial-gradient(circle at 85% 0,#172033 0,transparent 32rem),#0c1015;color:#f3f0ea}header{background:rgba(12,16,21,.87);border-color:rgba(255,255,255,.08)}.back,p{color:#9ca8b4}.card,.panel,.hero{background:rgba(20,25,32,.78);border-color:rgba(255,255,255,.08)}input{background:#111720;border-color:#313a45;color:#f4f6f8}.pill{background:#272e36}.pill.ok{background:#173522;color:#9bdfad}.pill.warn,.notice{background:#3d321c;color:#f1d18b}button.secondary{background:#252b33;color:#e9edf2}}@media(max-width:720px){header{grid-template-columns:1fr;align-items:start}.brand{text-align:left}.connect{justify-content:stretch}.connect input{width:100%;flex:1}.hero{grid-template-columns:1fr}.panelHead,.split{align-items:flex-start;flex-direction:column}.panelHead button,.split button{width:100%}}`

const cloudControlJS = `(() => {
'use strict';
let token='';
const $=id=>document.getElementById(id);
const auth=()=>({'Authorization':'Bearer '+token,'Accept':'application/json','Content-Type':'application/json'});
function setError(message){const box=$('errorBox');if(!message){box.classList.add('hidden');box.textContent='';return;}box.textContent=message;box.classList.remove('hidden');}
async function api(path,method='GET'){
  if(!token)throw new Error('请先输入 Admin Token');
  const res=await fetch(path,{method,headers:auth()});
  const text=await res.text();let body=text;try{body=text?JSON.parse(text):null}catch(_e){}
  if(!res.ok)throw new Error((body&&body.error?body.error:text)||('HTTP '+res.status));
  return body;
}
function value(v){if(v===undefined||v===null||v==='')return '—';if(typeof v==='boolean')return v?'是':'否';return String(v)}
function card(label,v){const e=document.createElement('div');e.className='card';const a=document.createElement('span');a.textContent=label;const b=document.createElement('strong');b.textContent=value(v);e.append(a,b);return e}
function time(v){if(!v||String(v).startsWith('0001-'))return '—';const d=new Date(v);return Number.isNaN(d.valueOf())?String(v):d.toLocaleString()}
function render(data){
  const s=(data&&data.state)||{},p=(data&&data.policy)||{},r=(data&&data.key_rotation)||{};const cards=$('cards');cards.replaceChildren();
  [['设备注册',s.enrolled?'已注册':'未注册'],['Node ID',s.node_id],['Workspace',s.workspace_id],['Key ID',s.key_id],['策略版本',s.policy_version],['最后心跳',time(s.last_heartbeat_at)],['最后同步',time(s.last_sync_at)],['最后轮换',time(s.last_key_rotation_at)],['E2EE Sync',s.memory_sync_enabled?'已启用':'未启用'],['Cloud',s.cloud_enabled?'已启用':'本地模式']].forEach(x=>cards.appendChild(card(x[0],x[1])));
  $('policy').textContent=JSON.stringify(p,null,2);
  $('syncText').textContent=s.memory_sync_enabled?'已启用端到端加密连续性同步；Cloud 仅保存 nonce、ciphertext、digest 和 key id。':'当前未启用 Memory Sync。设置 KINGAI_MEMORY_SYNC=1 与 32-byte KINGAI_SYNC_KEY 后重启服务即可启用。';
  $('rotationText').textContent=r.status==='prepared'?'存在未完成的两阶段轮换；再次点击“轮换设备密钥”会优先幂等完成现有事务，不会生成第二套身份。':'两阶段轮换：旧密钥和新密钥都必须证明所有权；网络结果不确定时保留本机 pending key，下一次重试可幂等恢复。';
  const restart=$('restartBox');if(s.policy_restart_required){restart.textContent='已收到新的更严格 Runtime / Provider / Tool Policy。Channel 收紧已即时生效；其余静态组件需要重启 KINGAIBOT 才会按新上限重新构造。';restart.classList.remove('hidden')}else{restart.classList.add('hidden');restart.textContent=''}
  const pill=$('statePill');pill.textContent=s.enrolled?'Cloud Managed':'Local First';pill.className='pill '+(s.enrolled?'ok':'warn');
  if(s.last_error)setError('最近云端错误：'+s.last_error);else setError('');
}
async function load(){try{render(await api('/v1/cloud/status'));}catch(e){setError(e.message);throw e}}
$('connect').onclick=async()=>{token=$('token').value.trim();try{await load()}catch(_e){}};
$('token').addEventListener('keydown',e=>{if(e.key==='Enter')$('connect').click()});
$('pull').onclick=async()=>{try{await api('/v1/cloud/policy/pull','POST');await load()}catch(e){setError(e.message)}};
$('sync').onclick=async()=>{try{await api('/v1/cloud/sync','POST');await load()}catch(e){setError(e.message)}};
$('rotate').onclick=async()=>{if(!confirm('确认轮换这台设备的 Ed25519 身份密钥？系统将使用两阶段协议并保留崩溃恢复状态。'))return;try{await api('/v1/cloud/key/rotate','POST');await load()}catch(e){setError(e.message);await load().catch(()=>{})}};
})();`
