package internal

const adminHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Qianwen Creator Account Pool</title>
<script src="https://unpkg.com/vue@3/dist/vue.global.prod.js"></script>
<style>
:root{
  color:#f4f8ff;background:#020409;color-scheme:dark;
  font-family:Inter,"Segoe UI","Microsoft YaHei",Arial,sans-serif;
  --bg:#020409;--panel:#08111b;--panel-2:#0d1825;--panel-3:#060d16;
  --line:#182638;--line-strong:#294057;--text:#f4f8ff;--muted:#9cafc7;--dim:#667891;
  --cyan:#36e7ff;--blue:#3e7dff;--violet:#9a66ff;--green:#3fe59a;--red:#ff6675;--amber:#ffd166;
  --grad:linear-gradient(135deg,#36e7ff 0%,#3e7dff 48%,#9a66ff 100%);
  --grad-soft:linear-gradient(135deg,rgba(54,231,255,.15),rgba(62,125,255,.1) 46%,rgba(154,102,255,.13));
  --shadow:0 22px 56px rgba(0,0,0,.44);
  --glow:0 0 26px rgba(54,231,255,.16),0 0 42px rgba(154,102,255,.08);
  --radius-sm:14px;--radius-md:18px;--radius-lg:24px;--radius-xl:30px;--ease:cubic-bezier(.2,.8,.2,1);
}
*{box-sizing:border-box}*{scrollbar-color:rgba(54,231,255,.32) rgba(6,12,21,.8);scrollbar-width:thin}
*::-webkit-scrollbar{width:10px;height:10px}*::-webkit-scrollbar-track{background:rgba(6,12,21,.8)}
*::-webkit-scrollbar-thumb{background:linear-gradient(180deg,rgba(54,231,255,.5),rgba(154,102,255,.45));border:2px solid rgba(6,12,21,.9);border-radius:999px}
body{
  margin:0;min-width:320px;min-height:100vh;
  background:
    linear-gradient(90deg,rgba(144,189,255,.018) 1px,transparent 1px),
    linear-gradient(rgba(144,189,255,.014) 1px,transparent 1px),
    linear-gradient(135deg,#010208 0%,#040813 48%,#080815 100%);
  background-size:34px 34px,34px 34px,auto;
}
button,input,select,textarea{font:inherit}button{cursor:pointer}[v-cloak]{display:none}
.shell{display:grid;grid-template-columns:268px 1fr;min-height:100vh}
.side{
  background:linear-gradient(180deg,rgba(9,17,28,.99),rgba(2,4,9,.99)),var(--bg);
  border-right:1px solid var(--line);padding:22px 16px;display:flex;flex-direction:column;gap:22px;
  box-shadow:18px 0 48px rgba(0,0,0,.26);
}
.brand{display:flex;align-items:center;gap:12px;min-height:50px}
.brand-mark{width:44px;height:44px;border-radius:var(--radius-md);display:grid;place-items:center;color:#00131a;font-weight:900;background:var(--grad);box-shadow:var(--glow);border:1px solid rgba(54,231,255,.24)}
.brand-title{font-size:20px;font-weight:900;letter-spacing:0}.brand-sub,.muted,.metric-label,.eyebrow{color:var(--muted);font-size:12px}
.nav{display:grid;gap:8px}
.nav button{
  min-height:42px;border:1px solid transparent;color:#c9d6e6;background:transparent;
  display:flex;align-items:center;gap:10px;padding:10px 12px;border-radius:var(--radius-md);
  transition:color 180ms var(--ease),border-color 180ms var(--ease),background 180ms var(--ease),transform 180ms var(--ease),box-shadow 180ms var(--ease);
}
.nav button:hover,.nav button.active{color:var(--text);background:rgba(54,231,255,.065);border-color:rgba(54,231,255,.22);box-shadow:inset 0 1px 0 rgba(255,255,255,.045)}
.nav button.active{background:linear-gradient(90deg,rgba(54,231,255,.15),rgba(62,125,255,.09) 46%,rgba(154,102,255,.08)),rgba(255,255,255,.018);border-color:rgba(54,231,255,.34);box-shadow:var(--glow),inset 0 1px 0 rgba(255,255,255,.06)}
.nav button:hover{transform:translateX(2px)}.side-foot{margin-top:auto;border:1px solid var(--line);border-radius:var(--radius-lg);background:rgba(8,17,27,.72);padding:14px}
.main{min-width:0;padding:24px 28px 42px}.topbar{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:22px}
h1,h2,h3,p{margin:0}h1{font-size:28px;line-height:1.12}h2{font-size:18px}h3{font-size:15px}.subline{margin-top:8px;color:var(--muted);font-size:13px}
.toolbar{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.tab-panel{animation:tabReveal 360ms var(--ease)}@keyframes tabReveal{from{opacity:0;transform:translateY(10px) scale(.995)}to{opacity:1;transform:none}}
.grid{display:grid;gap:16px}.stats{grid-template-columns:repeat(4,minmax(0,1fr));margin-bottom:18px}.two{grid-template-columns:minmax(360px,1.1fr) minmax(340px,.9fr)}
.card,.table-wrap{background:linear-gradient(180deg,rgba(12,24,38,.94),rgba(7,15,25,.94)),var(--panel);border:1px solid var(--line);border-radius:var(--radius-lg);box-shadow:var(--shadow);transition:border-color 220ms var(--ease),box-shadow 220ms var(--ease),transform 220ms var(--ease)}
.card{padding:18px}.card:hover,.table-wrap:hover{border-color:rgba(54,231,255,.24);box-shadow:var(--shadow),0 0 36px rgba(54,231,255,.06)}
.metric{min-height:110px;display:flex;flex-direction:column;justify-content:space-between;background:linear-gradient(135deg,rgba(54,231,255,.08),rgba(154,102,255,.06)),rgba(8,17,27,.92)}
.metric-value{font-size:26px;font-weight:900}.metric-meta{font-size:12px;color:var(--dim)}
.account-list{display:grid;gap:12px;max-height:calc(100vh - 255px);overflow:auto;padding-right:3px}
.account-card{border:1px solid var(--line);background:rgba(255,255,255,.026);border-radius:var(--radius-md);padding:14px;display:grid;gap:11px;transition:transform 180ms var(--ease),border-color 180ms var(--ease),background 180ms var(--ease),box-shadow 180ms var(--ease)}
.account-card:hover{transform:translateY(-1px);border-color:rgba(54,231,255,.24)}.account-card.active{border-color:rgba(54,231,255,.48);background:var(--grad-soft);box-shadow:var(--glow)}
.account-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}.account-name{font-weight:800}.account-id{font-size:12px;color:var(--dim);font-family:"SFMono-Regular",Consolas,monospace;margin-top:3px}
.badges{display:flex;gap:6px;flex-wrap:wrap}.badge{display:inline-flex;align-items:center;gap:6px;min-height:24px;padding:3px 9px;border-radius:999px;font-size:12px;border:1px solid transparent;background:rgba(142,160,182,.12);color:var(--muted)}
.badge.valid,.badge.ready,.badge.healthy,.badge.captured{background:rgba(63,229,154,.14);color:var(--green)}.badge.hot,.badge.waiting_scan,.badge.opening{background:rgba(54,231,255,.12);color:var(--cyan)}
.badge.invalid,.badge.failed,.badge.capture_failed,.badge.error{background:rgba(255,102,117,.14);color:var(--red)}.badge.unknown,.badge.expired,.badge.starting,.badge.not_logged_in{background:rgba(255,209,102,.13);color:var(--amber)}
.detail-head{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:16px}.detail-title{display:flex;align-items:center;gap:12px}
.avatar{width:46px;height:46px;border-radius:var(--radius-md);display:grid;place-items:center;background:var(--grad);color:#00131a;font-weight:900}
.actions{display:flex;gap:8px;flex-wrap:wrap}.btn{border:1px solid var(--line-strong);background:rgba(11,19,30,.94);color:var(--text);border-radius:var(--radius-sm);min-height:38px;padding:8px 13px;display:inline-flex;align-items:center;justify-content:center;gap:8px;transition:transform 180ms var(--ease),border-color 180ms var(--ease),color 180ms var(--ease),background 180ms var(--ease),box-shadow 180ms var(--ease)}
.btn:hover{transform:translateY(-1px);border-color:var(--cyan);color:var(--cyan);box-shadow:var(--glow)}.btn.primary{border:0;background:var(--grad);color:#00131a;font-weight:900;box-shadow:0 14px 34px rgba(54,231,255,.18)}
.btn.danger{color:var(--red);border-color:rgba(255,102,117,.35)}.btn.ghost{background:transparent}.btn:disabled{opacity:.48;cursor:not-allowed;transform:none;box-shadow:none}
.form-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}label{display:grid;gap:7px;color:var(--muted);font-size:12px}
input,select,textarea{width:100%;border:1px solid var(--line-strong);border-radius:var(--radius-sm);background:rgba(5,11,18,.92);color:var(--text);padding:10px 12px;outline:none;transition:border-color 160ms var(--ease),box-shadow 160ms var(--ease),background 160ms var(--ease)}
textarea{resize:vertical;min-height:96px}input:focus,select:focus,textarea:focus{border-color:var(--cyan);box-shadow:0 0 0 3px rgba(54,231,255,.12);background:#07111d}
.kv{display:grid;grid-template-columns:130px 1fr;gap:10px;padding:9px 0;border-bottom:1px solid rgba(41,64,87,.42);font-size:13px}.kv:last-child{border-bottom:0}.kv span:first-child{color:var(--muted)}
.mono{font-family:"SFMono-Regular",Consolas,monospace;font-size:12px;word-break:break-all}.table-wrap{overflow:hidden}table{width:100%;border-collapse:collapse;font-size:13px}
th,td{padding:12px 14px;border-bottom:1px solid var(--line);text-align:left;vertical-align:top}th{color:var(--muted);font-size:12px;background:#070f19}tr:hover td{background:rgba(54,231,255,.025)}
pre.out{max-height:420px;overflow:auto;white-space:pre-wrap;word-break:break-word;background:#050b12;border:1px solid var(--line);border-radius:var(--radius-md);padding:14px;color:#adf5ff}
.screenshot{width:100%;border-radius:var(--radius-md);border:1px solid var(--line-strong);background:#03070c;max-height:66vh;object-fit:contain}.qr-wrap{display:grid;gap:14px;place-items:center}.qr-img{width:100%;max-height:58vh;background:#050b12;border-radius:18px;border:1px solid var(--line-strong);object-fit:contain}.qr-img.interactive{cursor:crosshair}.login-input-row{width:100%;display:grid;grid-template-columns:1fr auto;gap:10px}.key-row{display:flex;gap:8px;flex-wrap:wrap;justify-content:center}
.overlay{position:fixed;inset:0;z-index:60;background:rgba(2,4,9,.72);backdrop-filter:blur(18px);display:grid;place-items:center;padding:24px;animation:fadeIn 180ms var(--ease)}
.modal{width:min(840px,100%);background:linear-gradient(180deg,rgba(12,24,38,.98),rgba(5,11,18,.98));border:1px solid rgba(54,231,255,.24);border-radius:var(--radius-xl);box-shadow:var(--shadow),var(--glow);padding:22px;animation:modalUp 240ms var(--ease)}
.modal-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;margin-bottom:16px}@keyframes fadeIn{from{opacity:0}to{opacity:1}}@keyframes modalUp{from{opacity:0;transform:translateY(12px) scale(.98)}to{opacity:1;transform:none}}
.empty{padding:38px;text-align:center;color:var(--muted)}.split{display:flex;align-items:center;justify-content:space-between;gap:12px}.hint{font-size:12px;color:var(--dim);line-height:1.6}
.loading-ribbon{position:fixed;inset:0 0 auto;z-index:80;height:3px;overflow:hidden;background:rgba(54,231,255,.08)}.loading-ribbon span{display:block;width:42%;height:100%;background:linear-gradient(90deg,transparent,var(--cyan),var(--blue),var(--violet),transparent);box-shadow:0 0 18px rgba(54,231,255,.62);animation:loadingSweep 1.1s var(--ease) infinite}
@keyframes loadingSweep{from{transform:translateX(-100%)}to{transform:translateX(240%)}}
@media (max-width:1100px){.shell{grid-template-columns:1fr}.side{position:sticky;top:0;z-index:20;display:block;padding:14px}.brand{margin-bottom:12px}.nav{grid-template-columns:repeat(4,minmax(0,1fr))}.side-foot{display:none}.stats,.two{grid-template-columns:1fr}}
@media (max-width:720px){.main{padding:18px 14px 32px}.topbar,.detail-head,.split{display:grid}.stats{grid-template-columns:repeat(2,minmax(0,1fr))}.form-grid{grid-template-columns:1fr}.nav{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
</head>
<body>
<div id="app" v-cloak>
  <div v-if="busy" class="loading-ribbon"><span></span></div>
  <div class="shell">
    <aside class="side">
      <div class="brand"><div class="brand-mark">Q</div><div><div class="brand-title">Qianwen Creator</div><div class="brand-sub">gen2api style console</div></div></div>
      <nav class="nav"><button v-for="item in tabs" :key="item.key" :class="{active:tab===item.key}" @click="tab=item.key"><span>{{item.icon}}</span><span>{{item.name}}</span></button></nav>
      <div class="side-foot"><div class="split"><span class="muted">默认视频模型</span><span class="badge hot">wan25-t2v</span></div><p class="hint" style="margin-top:10px">只有测活为 valid 的账号会参与接口调度。</p></div>
    </aside>
    <main class="main">
      <header class="topbar">
        <div><div class="eyebrow">QIANWEN CREATOR WEB REVERSE PROXY</div><h1>{{title}}</h1><p class="subline">Creator / AI Studio 账号池、扫码登录、素材视频任务与逐账号测活。</p></div>
        <div class="toolbar"><button class="btn" @click="refreshAll">刷新</button><button class="btn primary" @click="openAdd">新增账号</button></div>
      </header>

      <section v-show="tab==='accounts'" class="tab-panel">
        <div class="grid stats">
          <div class="card metric"><div class="metric-label">账号总数</div><div class="metric-value">{{accounts.length}}</div><div class="metric-meta">SQLite account pool</div></div>
          <div class="card metric"><div class="metric-label">可调度</div><div class="metric-value">{{validCount}}</div><div class="metric-meta">status = valid</div></div>
          <div class="card metric"><div class="metric-label">登录会话</div><div class="metric-value">{{sessions.length}}</div><div class="metric-meta">Chromium QR sessions</div></div>
          <div class="card metric"><div class="metric-label">任务数</div><div class="metric-value">{{tasks.length}}</div><div class="metric-meta">recent media tasks</div></div>
        </div>
        <div class="grid two">
          <div class="card">
            <div class="split" style="margin-bottom:14px"><h2>账号池</h2><button class="btn ghost" @click="loadAccounts">同步状态</button></div>
            <div class="account-list">
              <div v-for="account in accounts" :key="account.id" :class="['account-card',{active:selectedId===account.id}]" @click="selectAccount(account.id)">
                <div class="account-head"><div><div class="account-name">{{account.name}}</div><div class="account-id">{{account.id}}</div></div><span :class="['badge',account.status]">{{statusText(account.status)}}</span></div>
                <div class="badges"><span v-if="account.enabled" class="badge valid">启用</span><span v-else class="badge unknown">禁用</span><span class="badge hot">{{account.type}}</span></div>
                <div class="hint mono">{{account.last_error || account.cookie_string || '等待扫码登录'}}</div>
              </div>
              <div v-if="!accounts.length" class="empty">暂无账号，点击“新增账号”开始扫码登录。</div>
            </div>
          </div>
          <div class="card" v-if="selectedAccount">
            <div class="detail-head"><div class="detail-title"><div class="avatar">{{selectedAccount.name.slice(0,1).toUpperCase()}}</div><div><h2>{{selectedAccount.name}}</h2><div class="account-id">{{selectedAccount.id}}</div></div></div><span :class="['badge',selectedAccount.status]">{{statusText(selectedAccount.status)}}</span></div>
            <div class="actions" style="margin-bottom:16px">
              <button class="btn primary" @click="testAccount(selectedAccount.id)" :disabled="probe.loading">测活</button>
              <button class="btn" @click="deleteAccount(selectedAccount.id)">删除</button>
            </div>
            <div class="grid" style="gap:0">
              <div class="kv"><span>类型</span><span>{{selectedAccount.type}}</span></div>
              <div class="kv"><span>状态</span><strong>{{statusText(selectedAccount.status)}}</strong></div>
              <div class="kv"><span>最后测试</span><span>{{selectedAccount.last_test_at || '-'}}</span></div>
              <div class="kv"><span>最后成功</span><span>{{selectedAccount.last_success_at || '-'}}</span></div>
              <div class="kv"><span>错误信息</span><span>{{selectedAccount.last_error || '-'}}</span></div>
            </div>
            <div v-if="probe.message" style="margin-top:14px"><span :class="['badge',probe.status]">{{probe.status}}</span><span class="hint" style="margin-left:8px">{{probe.message}}</span></div>
          </div>
          <div class="card" v-else><div class="empty">请选择一个账号查看详情。</div></div>
        </div>
      </section>

      <section v-show="tab==='sessions'" class="tab-panel">
        <div class="table-wrap">
          <table><thead><tr><th>会话</th><th>状态</th><th>Cookies</th><th>消息</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="s in sessions" :key="s.id"><td><div class="mono">{{s.name}}</div><div class="hint mono">{{s.id}}</div></td><td><span :class="['badge',s.status]">{{statusText(s.status)}}</span></td><td>{{s.cookie_count}}</td><td>{{s.message}}</td><td><div class="actions"><button class="btn" @click="showSession(s.id)">打开</button><button class="btn" @click="refreshSession(s.id)">刷新</button><button class="btn primary" @click="captureSession(s.id)">捕获并测活</button><button class="btn danger" @click="deleteSession(s.id)">删除</button></div></td></tr>
            <tr v-if="!sessions.length"><td colspan="5" class="empty">暂无登录会话。</td></tr>
          </tbody></table>
        </div>
      </section>

      <section v-show="tab==='tasks'" class="tab-panel">
        <div class="table-wrap">
          <table><thead><tr><th>ID</th><th>模型</th><th>状态</th><th>账号</th><th>错误</th><th>创建时间</th></tr></thead>
          <tbody><tr v-for="task in tasks" :key="task.id"><td class="mono">{{task.id}}</td><td>{{task.model}}</td><td><span :class="['badge',task.status]">{{task.status}}</span></td><td class="mono">{{task.provider_account_id || '-'}}</td><td>{{task.error_message || '-'}}</td><td>{{task.created_at}}</td></tr><tr v-if="!tasks.length"><td colspan="6" class="empty">暂无任务。</td></tr></tbody></table>
        </div>
      </section>

      <section v-show="tab==='system'" class="tab-panel">
        <div class="grid two">
          <div class="card"><h2 style="margin-bottom:12px">运行信息</h2><div class="kv"><span>服务</span><span>{{summary.service?.name}}</span></div><div class="kv"><span>监听</span><span class="mono">{{summary.service?.host}}:{{summary.service?.port}}</span></div><div class="kv"><span>公开地址</span><span class="mono">{{summary.service?.public_base_url}}</span></div><div class="kv"><span>数据库</span><span class="mono">{{summary.service?.database_path}}</span></div><div class="kv"><span>数据目录</span><span class="mono">{{summary.service?.data_dir}}</span></div></div>
          <div class="card"><h2 style="margin-bottom:12px">模型</h2><div class="badges"><span v-for="m in models" :key="m.id || m.name" class="badge hot">{{m.id || m.name}}</span></div><pre v-if="systemNote" class="out" style="margin-top:14px">{{systemNote}}</pre></div>
        </div>
      </section>
    </main>
  </div>

  <div v-if="addModal" class="overlay" @click.self="addModal=false">
    <div class="modal">
      <div class="modal-head"><div><h2>新增千问 Creator 账号</h2><p class="subline">截图登录用于服务器 Chromium；Cookie 导入用于从已登录浏览器复制请求 Cookie。</p></div><button class="btn ghost" @click="addModal=false">关闭</button></div>
      <div class="form-grid">
        <label>账号名称<input v-model="newAccount.name" placeholder="例如：Creator 视频号 01"></label>
        <label>导入方式<select v-model="newAccount.mode"><option value="browser">截图登录</option><option value="cookie">Cookie 导入</option></select></label>
      </div>
      <div v-if="newAccount.mode==='cookie'" style="margin-top:14px">
        <label>Cookie Header<textarea v-model="newAccount.cookie_string" placeholder="粘贴 Request Headers 里的 Cookie: a=b; c=d"></textarea></label>
        <label style="margin-top:12px">Cookie JSON<textarea v-model="newAccount.cookie_json" placeholder="可选。粘贴浏览器导出的 [{name,value,domain,path}]"></textarea></label>
        <label style="margin-top:12px">LocalStorage JSON<textarea v-model="newAccount.local_storage_json" placeholder="可选。粘贴 create.qianwen.com localStorage JSON"></textarea></label>
        <label style="margin-top:12px">User-Agent<input v-model="newAccount.user_agent" placeholder="可选，不填则自动生成"></label>
      </div>
      <p class="hint" style="margin-top:12px">Cookie 导入保存后会自动测活；只有 valid 账号会参与接口调度。</p>
      <div class="actions" style="margin-top:16px"><button class="btn primary" @click="createAccount">{{newAccount.mode==='cookie'?'保存并测活':'创建并打开登录截图'}}</button></div>
    </div>
  </div>

  <div v-if="qr.open" class="overlay" @click.self="closeQr">
    <div class="modal">
      <div class="modal-head"><div><h2>扫码登录</h2><p class="subline">{{qr.name}} / {{qr.text}}</p></div><button class="btn ghost" @click="closeQr">关闭</button></div>
      <div class="qr-wrap">
        <img v-if="qr.session_id" class="qr-img interactive" :src="screenshotUrl(qr.session_id)" alt="qianwen login screenshot" @click="clickSessionImage">
        <div v-else class="empty">正在启动登录浏览器</div>
        <span :class="['badge',qr.status]">{{statusText(qr.status || 'starting')}}</span>
        <p class="hint">如果页面没有二维码，可以直接点击上方截图中的输入框/按钮，再用下面的输入框把文字发送到服务器侧 Chromium。登录完成后点击“捕获并测活”。</p>
        <div class="login-input-row">
          <input v-model="qr.input" type="password" placeholder="输入到当前焦点。适合手机号、密码、短信验证码；刷新后不保留。">
          <button class="btn primary" @click="typeIntoSession(qr.session_id)">输入</button>
        </div>
        <div class="key-row">
          <button class="btn" @click="pressSessionKey(qr.session_id,'Tab')">Tab</button>
          <button class="btn" @click="pressSessionKey(qr.session_id,'Enter')">Enter</button>
          <button class="btn" @click="pressSessionKey(qr.session_id,'Backspace')">Backspace</button>
          <button class="btn" @click="pressSessionKey(qr.session_id,'Escape')">Esc</button>
        </div>
        <div class="actions"><button class="btn" @click="clickLoginEntry(qr.session_id)">点击登录入口</button><button class="btn" @click="refreshSession(qr.session_id)">刷新会话</button><button class="btn primary" @click="captureSession(qr.session_id)">捕获并测活</button><button class="btn danger" @click="deleteSession(qr.session_id)">删除会话</button></div>
      </div>
    </div>
  </div>
</div>
<script>
const {createApp,ref,reactive,computed,onMounted}=Vue;
createApp({
  setup(){
    const API=window.location.origin;
    const adminKey=new URLSearchParams(window.location.search).get("key")||"";
    const tabs=[{key:"accounts",name:"账号池",icon:"◎"},{key:"sessions",name:"扫码会话",icon:"▣"},{key:"tasks",name:"任务",icon:"≡"},{key:"system",name:"系统",icon:"⚙"}];
    const tab=ref("accounts"),busy=ref(false),accounts=ref([]),sessions=ref([]),tasks=ref([]),models=ref([]),selectedId=ref("");
    const summary=reactive({service:{},accounts:{},tasks:{}});
    const addModal=ref(false),newAccount=reactive({name:"",mode:"browser",cookie_string:"",cookie_json:"",local_storage_json:"",user_agent:""});
    const probe=reactive({loading:false,status:"",message:""});
    const qr=reactive({open:false,session_id:"",name:"",status:"",text:"",input:"",timer:null});
    const systemNote=ref("公网图片 URL 自动转素材当前不计入稳定能力；稳定图生视频路径是 first_frame_material_id / last_frame_material_id。");
    const title=computed(()=>tabs.find(t=>t.key===tab.value)?.name||"账号池");
    const selectedAccount=computed(()=>accounts.value.find(a=>a.id===selectedId.value)||null);
    const validCount=computed(()=>accounts.value.filter(a=>a.status==="valid").length);
    function adminPath(path){return path+(path.includes("?")?"&":"?")+"key="+encodeURIComponent(adminKey)}
    function headers(){return {"Content-Type":"application/json","X-Admin-Key":adminKey}}
    async function api(path,opts={}){
      busy.value=true;
      try{
        const resp=await fetch(adminPath(path),{...opts,headers:{...headers(),...(opts.headers||{})}});
        const text=await resp.text();let data={};
        try{data=text?JSON.parse(text):{}}catch{data={raw:text}}
        if(!resp.ok){throw new Error(data.error?.message||data.message||text||("HTTP "+resp.status))}
        return data;
      }finally{busy.value=false}
    }
    function screenshotUrl(id){return adminPath("/api/login-sessions/"+encodeURIComponent(id)+"/screenshot")+"&t="+Date.now()}
    async function loadSummary(){Object.assign(summary,await api("/api/admin/summary"))}
    async function loadAccounts(){const data=await api("/api/accounts");accounts.value=data.data||[];if(!selectedId.value&&accounts.value.length)selectedId.value=accounts.value[0].id;if(selectedId.value&&!accounts.value.some(a=>a.id===selectedId.value))selectedId.value=accounts.value[0]?.id||""}
    async function loadSessions(){const data=await api("/api/login-sessions");sessions.value=data.data||[]}
    async function loadTasks(){const data=await api("/api/tasks?limit=100");tasks.value=data.data||[]}
    async function loadModels(){try{models.value=(await api("/api/models")).data||[]}catch(e){models.value=[]}}
    async function refreshAll(){await Promise.all([loadSummary(),loadAccounts(),loadSessions(),loadTasks(),loadModels()])}
    function selectAccount(id){selectedId.value=id;probe.message=""}
    function openAdd(){newAccount.name="";newAccount.mode="browser";newAccount.cookie_string="";newAccount.cookie_json="";newAccount.local_storage_json="";newAccount.user_agent="";addModal.value=true}
    async function createAccount(){
      const name=(newAccount.name||"").trim();
      if(!name){probe.status="unknown";probe.message="请先填写账号名称";return}
      if(newAccount.mode==="cookie"){
        const data=await api("/api/accounts",{method:"POST",body:JSON.stringify({
          name:name,
          cookie_string:newAccount.cookie_string,
          cookie_json:newAccount.cookie_json,
          local_storage_json:newAccount.local_storage_json,
          user_agent:newAccount.user_agent
        })});
        addModal.value=false;
        selectedId.value=data.data.id;
        await loadAccounts();
        await testAccount(data.data.id);
        return;
      }
      const data=await api("/api/accounts",{method:"POST",body:JSON.stringify({name:name})});
      addModal.value=false;openQr(data.data);await refreshAll();
    }
    function openQr(session){qr.open=true;qr.session_id=session.id;qr.name=session.name;qr.status=session.status;qr.text=session.message||"";startQrPolling()}
    async function showSession(id){const data=await api("/api/login-sessions/"+encodeURIComponent(id));openQr(data.data)}
    function startQrPolling(){
      if(qr.timer)clearInterval(qr.timer);
      qr.timer=setInterval(async()=>{
        if(!qr.open||!qr.session_id)return;
        try{
          const data=await api("/api/login-sessions/"+encodeURIComponent(qr.session_id));
          qr.status=data.data.status;qr.text=data.data.message||qr.text;
          if(data.data.account_id){selectedId.value=data.data.account_id;await loadAccounts()}
        }catch(e){qr.text=e.message}
      },2500);
    }
    function closeQr(){if(qr.timer)clearInterval(qr.timer);qr.timer=null;qr.open=false;qr.session_id="";qr.text="";qr.input=""}
    async function clickLoginEntry(id){if(!id)return;const data=await api("/api/login-sessions/"+encodeURIComponent(id)+"/click-login",{method:"POST"});openQr(data.data)}
    async function clickSessionImage(event){
      if(!qr.session_id)return;
      const img=event.currentTarget;
      const rect=img.getBoundingClientRect();
      const scaleX=img.naturalWidth/rect.width;
      const scaleY=img.naturalHeight/rect.height;
      const x=Math.round((event.clientX-rect.left)*scaleX);
      const y=Math.round((event.clientY-rect.top)*scaleY);
      const data=await api("/api/login-sessions/"+encodeURIComponent(qr.session_id)+"/click",{method:"POST",body:JSON.stringify({x:x,y:y})});
      openQr(data.data);
    }
    async function typeIntoSession(id){
      if(!id||!qr.input)return;
      const data=await api("/api/login-sessions/"+encodeURIComponent(id)+"/type",{method:"POST",body:JSON.stringify({text:qr.input})});
      qr.input="";
      openQr(data.data);
    }
    async function pressSessionKey(id,key){
      if(!id)return;
      const data=await api("/api/login-sessions/"+encodeURIComponent(id)+"/key",{method:"POST",body:JSON.stringify({key:key})});
      openQr(data.data);
    }
    async function refreshSession(id){if(!id)return;const data=await api("/api/login-sessions/"+encodeURIComponent(id)+"/refresh",{method:"POST"});openQr(data.data);await loadSessions()}
    async function captureSession(id){
      if(!id)return;
      try{
        const data=await api("/api/login-sessions/"+encodeURIComponent(id)+"/capture",{method:"POST"});
        const account=data.data;
        selectedId.value=account.id;
        probe.status="unknown";probe.message="已捕获登录 cookie，正在测活";
        await loadAccounts();
        await testAccount(account.id);
        await loadSessions();
      }catch(e){probe.status="error";probe.message=e.message}
    }
    async function deleteSession(id){if(!id)return;await api("/api/login-sessions/"+encodeURIComponent(id),{method:"DELETE"});if(qr.session_id===id)closeQr();await loadSessions()}
    async function testAccount(id){
      probe.loading=true;probe.status="";probe.message="";
      try{
        const result=await api("/api/accounts/"+encodeURIComponent(id)+"/test",{method:"POST",body:JSON.stringify({capability:"chat"})});
        probe.status=result.status;probe.message=(result.message||"")+" "+(result.response_text||"");
      }catch(e){probe.status="error";probe.message=e.message}
      finally{probe.loading=false;await loadAccounts()}
    }
    async function deleteAccount(id){if(!confirm("删除这个账号？"))return;await api("/api/accounts/"+encodeURIComponent(id),{method:"DELETE"});selectedId.value="";await loadAccounts()}
    function statusText(v){const map={valid:"可用",unknown:"未测活",invalid:"不可用",starting:"启动中",opening:"打开中",waiting_scan:"等待扫码",login_detected:"检测到登录",captured:"已捕获",capture_failed:"捕获失败",failed:"失败",expired:"已过期"};return map[v]||v||"未知"}
    onMounted(()=>{refreshAll();setInterval(()=>{if(tab.value==="tasks")loadTasks();if(tab.value==="sessions")loadSessions()},5000);setInterval(()=>loadAccounts(),15000)});
    return{tabs,tab,title,busy,accounts,sessions,tasks,models,selectedId,selectedAccount,validCount,summary,addModal,newAccount,probe,qr,systemNote,refreshAll,loadAccounts,selectAccount,openAdd,createAccount,showSession,clickLoginEntry,clickSessionImage,typeIntoSession,pressSessionKey,refreshSession,captureSession,deleteSession,testAccount,deleteAccount,closeQr,screenshotUrl,statusText};
  }
}).mount("#app");
</script>
</body>
</html>`
