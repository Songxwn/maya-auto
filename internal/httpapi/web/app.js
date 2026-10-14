const titles = {
  overview: "总览", connect: "连接", model: "建模", fx: "特效", sim: "解算",
  light: "灯光", render: "渲染", agent: "指令", newapi: "NewAPI", mcp: "MCP", logs: "日志"
};
const state = { page: "overview", config: {}, health: {}, models: [], probes: [], script: "", running: null };

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

function toast(text, bad) {
  const el = document.createElement("div");
  el.className = "toast" + (bad ? " bad" : "");
  el.textContent = text;
  $("toasts").appendChild(el);
  setTimeout(() => el.remove(), 3600);
}

async function api(path, opt) {
  const res = await fetch(path, Object.assign({ headers: { "Content-Type": "application/json" } }, opt || {}));
  const text = await res.text();
  let data = {};
  try { data = text ? JSON.parse(text) : {}; } catch { data = { error: text }; }
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function val(id) { return ($(id) && $(id).value || "").trim(); }
function num(id) {
  const el = $(id);
  if (!el || el.value === "") return undefined;
  const n = Number(el.value);
  if (!Number.isFinite(n)) throw new Error("请填写数字");
  return n;
}
function vec(id) {
  const raw = val(id);
  if (!raw) return undefined;
  const parts = raw.split(/[,\s]+/).filter(Boolean).map(Number);
  if (parts.length < 3 || parts.some((n) => !Number.isFinite(n))) throw new Error("坐标用三个数字，逗号分隔");
  return parts.slice(0, 3);
}
function names(id) {
  const raw = val(id);
  if (!raw) return undefined;
  return raw.split(",").map((s) => s.trim()).filter(Boolean);
}
function rgb(id) {
  const hex = ($(id).value || "#cccccc").slice(1);
  const n = parseInt(hex, 16);
  return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255].map((x) => Math.round(x * 1000) / 1000);
}
function compact(obj) {
  const out = {};
  Object.keys(obj).forEach((k) => { if (obj[k] !== undefined && obj[k] !== "") out[k] = obj[k]; });
  return out;
}

async function callTool(name, args) {
  const data = await api("/api/maya/call", { method: "POST", body: JSON.stringify({ name, args: compact(args || {}) }) });
  const box = $("result");
  if (box) box.textContent = JSON.stringify(data.result, null, 2);
  toast(name + " 完成");
  refreshPill();
  return data.result;
}

async function sse(path, body, onEvent, signal) {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal
  });
  if (!res.ok) {
    const text = await res.text();
    let msg = text;
    try { msg = JSON.parse(text).error || text; } catch { /* keep */ }
    throw new Error(msg);
  }
  const reader = res.body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    let idx;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const block = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      let event = "message";
      let data = "";
      block.split("\n").forEach((line) => {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) data += line.slice(5).trim();
      });
      if (data) onEvent(event, JSON.parse(data));
    }
  }
}

function field(label, id, extra) {
  const wide = extra && extra.includes("wide") ? "wide" : "";
  return `<label class="${wide}">${esc(label)}<input id="${id}" ${extra || ""}></label>`;
}
function select(label, id, options) {
  return `<label>${esc(label)}<select id="${id}">${options.map((o) => `<option value="${esc(o[0])}">${esc(o[1])}</option>`).join("")}</select></label>`;
}

const pages = {
  overview() {
    return `<div class="grid">
      <div class="card span2"><h2>会话</h2><div id="live" class="muted">正在读取…</div></div>
      <div class="card"><h2>快捷建模</h2>
        <div class="row">
          <button class="primary" data-action="quick" data-kind="cube">立方体</button>
          <button data-action="quick" data-kind="sphere">球体</button>
          <button data-action="quick" data-kind="cylinder">圆柱</button>
          <button data-action="quick" data-kind="plane">平面</button>
        </div>
      </div>
      <div class="card"><h2>说明</h2>
        <p class="muted">先在 Maya 2024 及以上的脚本编辑器运行连接页里的启动脚本，再点连接。建模、特效、解算、灯光和渲染走同一套 MCP 工具。自然语言指令使用 NewAPI 的工具调用。</p>
      </div>
    </div>`;
  },
  connect() {
    const c = state.config;
    return `<div class="grid">
      <div class="card"><h2>命令端口</h2>
        <div class="form">
          ${field("主机", "mayaHost", `value="${esc(c.mayaHost || "127.0.0.1")}"`)}
          ${field("端口", "mayaPort", `type="number" value="${esc(c.mayaPort || 7001)}"`)}
        </div>
        <div class="row" style="margin-top:12px">
          <button class="primary" data-action="connect">连接</button>
          <button data-action="disconnect">断开</button>
          <button data-action="save-maya">保存</button>
        </div>
      </div>
      <div class="card"><h2>在 Maya 里运行一次</h2>
        <p class="muted">打开脚本编辑器，切到 Python，粘贴后执行。只监听本机。</p>
        <pre class="script" id="scriptBox">${esc(state.script || "加载中…")}</pre>
        <button data-action="copy-script">复制脚本</button>
      </div>
    </div>`;
  },
  model() {
    return `<div class="grid">
      <div class="card"><h2>基本体</h2><div class="form">
        ${select("类型", "kind", [["cube", "立方体"], ["sphere", "球体"], ["cylinder", "圆柱"], ["plane", "平面"], ["torus", "圆环"], ["cone", "圆锥"], ["pipe", "管体"], ["prism", "棱柱"]])}
        ${field("名称", "primName", 'placeholder="pCube1"')}
        ${field("宽 / 半径", "width", 'type="number" step="0.1" placeholder="1"')}
        ${field("高", "height", 'type="number" step="0.1"')}
        ${field("深 / 截面半径", "depth", 'type="number" step="0.1"')}
        ${field("细分 X Y Z", "subdiv", 'placeholder="1, 1, 1"')}
        ${field("位移", "translate", 'placeholder="0, 0, 0" class="wide"')}
        <div class="wide"><button class="primary" data-action="primitive">创建</button></div>
      </div></div>
      <div class="card"><h2>编辑</h2><div class="form">
        ${field("对象", "editNames", 'placeholder="pCube1，空则用当前选择" class="wide"')}
        ${select("操作", "editAction", [["extrude", "挤出"], ["bevel", "倒角"], ["smooth", "平滑"], ["duplicate", "复制"], ["combine", "合并"], ["separate", "分离"], ["freeze", "冻结变换"], ["center_pivot", "居中枢轴"], ["history", "删除历史"], ["delete", "删除"], ["rename", "重命名"], ["parent", "设置父级"]])}
        ${field("距离 / 比例 / 段数", "editAmount", 'placeholder="挤出距离或倒角比例"')}
        ${field("新名称或父级", "editExtra", 'placeholder="rename 或 parent"')}
        <div class="wide row">
          <button class="primary" data-action="edit">应用</button>
          <button data-action="move">按位移移动</button>
        </div>
        ${field("移动到", "moveTo", 'placeholder="0, 1, 0" class="wide"')}
      </div></div>
      <div class="card span2"><h2>材质</h2><div class="form">
        ${field("名称", "matName", 'value="mayaAutoMat"')}
        ${select("类型", "matType", [["standardSurface", "Standard Surface"], ["lambert", "Lambert"], ["aiStandardSurface", "Arnold Standard Surface"]])}
        <label>颜色<input id="matColor" type="color" value="#d7a15a"></label>
        ${field("指定给", "matAssign", 'placeholder="pCube1, pSphere1"')}
        <div class="wide"><button class="primary" data-action="material">创建并指定</button></div>
      </div></div>
      <pre class="result span2" id="result"></pre>
    </div>`;
  },
  fx() {
    return `<div class="grid">
      <div class="card"><h2>nParticle</h2><div class="form">
        ${field("名称", "fxName", 'value="spray"')}
        ${field("速率", "fxRate", 'type="number" value="80"')}
        ${field("位置", "fxPos", 'value="0, 2, 0" class="wide"')}
        <div class="wide"><button class="primary" data-action="nparticle">创建发射器</button></div>
      </div></div>
      <div class="card"><h2>nCloth</h2><div class="form">
        ${field("网格", "clothMesh", 'placeholder="pPlane1" class="wide"')}
        <div class="wide"><button class="primary" data-action="ncloth">转为布料</button></div>
      </div></div>
      <div class="card"><h2>力场</h2><div class="form">
        ${select("类型", "fieldKind", [["gravity", "重力"], ["air", "风"], ["turbulence", "湍流"], ["vortex", "漩涡"], ["radial", "径向"], ["drag", "阻力"]])}
        ${field("强度", "fieldMag", 'type="number" value="9.8"')}
        ${field("位置", "fieldPos", 'value="0, 5, 0"')}
        ${field("影响对象", "fieldAffect", 'placeholder="nParticle1"')}
        <div class="wide"><button class="primary" data-action="field">创建力场</button></div>
      </div></div>
      <div class="card"><h2>Nucleus</h2><div class="form">
        ${field("节点", "nucName", 'value="nucleus1"')}
        ${field("重力", "nucG", 'type="number" value="9.8"')}
        ${field("重力方向", "nucDir", 'value="0, -1, 0"')}
        <div><button class="primary" data-action="nucleus">写入</button></div>
      </div></div>
      <pre class="result span2" id="result"></pre>
    </div>`;
  },
  sim() {
    return `<div class="card"><h2>时间与缓存</h2><div class="form">
      ${field("起始帧", "simStart", 'type="number" value="1"')}
      ${field("结束帧", "simEnd", 'type="number" value="48"')}
      ${field("步长", "simStep", 'type="number" value="1"')}
      ${field("对象", "simNames", 'placeholder="空则使用当前选择"')}
      ${field("输出路径", "simPath", 'placeholder="playblast 或缓存路径" class="wide"')}
      <div class="wide row">
        <button data-action="sim" data-sim="range">设置范围</button>
        <button class="primary" data-action="sim" data-sim="evaluate">逐帧解算</button>
        <button data-action="sim" data-sim="bake">烘焙</button>
        <button data-action="sim" data-sim="geometry_cache">几何缓存</button>
        <button data-action="sim" data-sim="playblast">拍屏</button>
      </div>
    </div><pre class="result" id="result"></pre></div>`;
  },
  light() {
    return `<div class="card"><h2>灯光</h2><div class="form">
      ${select("类型", "lightKind", [["point", "点光"], ["spot", "聚光"], ["directional", "平行光"], ["area", "面光"], ["ambient", "环境光"], ["aiSkyDome", "Arnold 天球"], ["aiArea", "Arnold 面光"]])}
      ${field("名称", "lightName", 'placeholder="keyLight"')}
      ${field("强度", "lightIntensity", 'type="number" value="1" step="0.1"')}
      <label>颜色<input id="lightColor" type="color" value="#fff2d8"></label>
      ${field("位移", "lightPos", 'placeholder="0, 5, 2"')}
      ${field("旋转", "lightRot", 'placeholder="-40, 30, 0"')}
      ${field("聚光锥角", "cone", 'type="number" placeholder="40"')}
      <div><button class="primary" data-action="light">创建</button></div>
    </div><pre class="result" id="result"></pre></div>`;
  },
  render() {
    return `<div class="grid">
      <div class="card"><h2>渲染设置</h2><div class="form">
        ${select("渲染器", "renderer", [["arnold", "Arnold"], ["mayaSoftware", "Maya Software"], ["mayaHardware2", "Maya Hardware 2.0"]])}
        ${field("宽", "rw", 'type="number" value="1280"')}
        ${field("高", "rh", 'type="number" value="720"')}
        ${field("Arnold 采样", "samples", 'type="number" value="3"')}
        ${field("图像前缀", "prefix", 'placeholder="maya_auto"')}
        <div><button class="primary" data-action="render-settings">应用</button></div>
      </div></div>
      <div class="card"><h2>渲染当前画面</h2><div class="form">
        ${field("摄影机", "camera", 'value="persp"')}
        ${field("帧", "frame", 'type="number" placeholder="留空为当前帧"')}
        <div class="wide row">
          <button class="primary" data-action="render">渲染</button>
          <button data-action="camera">创建摄影机</button>
        </div>
        ${field("摄影机名", "camName", 'value="shotCam"')}
        ${field("焦距", "focal", 'type="number" value="35"')}
      </div></div>
      <pre class="result span2" id="result"></pre>
    </div>`;
  },
  agent() {
    const model = state.config.newAPIModel || "";
    const opts = state.models.map((m) => `<option ${m.id === model ? "selected" : ""}>${esc(m.id)}</option>`).join("");
    return `<div class="card"><h2>用 NewAPI 模型操作 Maya</h2>
      <div class="form">
        <label class="wide">模型 ${state.models.length ? `<select id="agentModel"><option value="">沿用已保存模型</option>${opts}</select>` : `<input id="agentModel" value="${esc(model)}" placeholder="模型 ID">`}</label>
        <label class="wide">指令<textarea id="prompt" placeholder="例如：建一个 4×1×2 的立方体，指定暖色材质，再加一盏主光"></textarea></label>
      </div>
      <div class="row" style="margin:12px 0">
        <button class="primary" data-action="agent">执行</button>
        <button data-action="agent-stop">停止</button>
      </div>
      <div id="trace" class="trace"></div>
    </div>`;
  },
  newapi() {
    const c = state.config;
    const rows = state.probes.map((p) => `<tr>
      <td>${esc(p.model)}</td><td class="${p.ok ? "ok" : "bad"}">${esc(p.class || "")}</td>
      <td>${esc(p.latencyMs)} ms</td><td>${esc(p.reply || p.error || "")}</td></tr>`).join("");
    const checks = state.models.map((m) => `<label><input type="checkbox" class="model-check" value="${esc(m.id)}"> ${esc(m.id)} <span class="muted">${esc(m.ownedBy || "")}</span></label>`).join("");
    return `<div class="grid">
      <div class="card"><h2>网关</h2>
        <div class="banner">地址可以是站点根或带 /v1。密钥只保存在本机配置里。</div>
        <div class="form">
          ${field("NewAPI 地址", "apiBase", `value="${esc(c.newAPIBase || "")}" placeholder="https://example.com/v1" class="wide"`)}
          ${field("密钥", "apiKey", `type="password" placeholder="${c.hasKey ? "已保存 " + esc(c.keyMask) + "，留空不修改" : "sk-..."}" class="wide"`)}
          ${field("默认模型", "apiModel", `value="${esc(c.newAPIModel || "")}" placeholder="gpt-4.1" class="wide"`)}
        </div>
        <div class="row" style="margin-top:12px">
          <button data-action="save-api">保存</button>
          <button class="primary" data-action="list-models">拉取模型</button>
        </div>
      </div>
      <div class="card"><h2>探测</h2>
        <p class="muted">每个模型发一条最短对话，记录是否可用、分类和延迟。单次最多 60 个，并发 3。</p>
        <div class="checks" id="modelChecks">${checks || '<span class="muted">还没有模型列表</span>'}</div>
        <div class="row" style="margin-top:12px">
          <button class="primary" data-action="probe-selected">探测选中</button>
          <button data-action="probe-ten">探测前 10 个</button>
        </div>
      </div>
      <div class="card span2"><h2>结果</h2>
        <table><thead><tr><th>模型</th><th>分类</th><th>延迟</th><th>回复 / 错误</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="4" class="muted">尚未探测</td></tr>'}</tbody></table>
      </div>
    </div>`;
  },
  mcp() {
    const origin = location.origin;
    const exe = state.health.mcpExe || "maya-auto-mcp.exe";
    const httpCfg = JSON.stringify({ mcpServers: { "maya-auto": { url: origin + "/mcp" } } }, null, 2);
    const stdioCfg = JSON.stringify({ mcpServers: { "maya-auto": { command: exe } } }, null, 2);
    const tools = (state.tools || []).map((t) => `<li><b>${esc(t.name)}</b> <span class="muted">${esc(t.description)}</span></li>`).join("");
    return `<div class="grid">
      <div class="card"><h2>HTTP</h2>
        <p class="muted">界面开着时，Cursor 走这个地址。请求由当前进程转发到 Maya，避免占住第二个命令端口连接。</p>
        <pre id="httpCfg">${esc(httpCfg)}</pre>
        <button data-action="copy" data-copy="httpCfg">复制</button>
      </div>
      <div class="card"><h2>stdio</h2>
        <p class="muted">界面没开时，Cursor 启动 maya-auto-mcp.exe。若界面已开，它会自动转发到 HTTP。</p>
        <pre id="stdioCfg">${esc(stdioCfg)}</pre>
        <button data-action="copy" data-copy="stdioCfg">复制</button>
      </div>
      <div class="card span2"><h2>工具</h2><ul>${tools}</ul></div>
    </div>`;
  },
  logs() {
    const lines = (state.logLines || []).map((l) => `<div class="${l.level === "error" ? "bad" : ""}">${esc(l.time)} ${esc(l.level)} ${esc(l.text)}</div>`).join("");
    return `<div class="card"><div class="row"><h2 style="margin:0">运行日志</h2><button data-action="reload-logs">刷新</button></div>
      <pre class="result">${lines || "暂无"}</pre>
      <p class="muted">${esc(state.health.logPath || "")}</p></div>`;
  }
};

function render() {
  document.querySelectorAll("#nav button").forEach((btn) => btn.classList.toggle("active", btn.dataset.page === state.page));
  $("title").textContent = titles[state.page];
  $("content").innerHTML = pages[state.page]();
  if (state.page === "overview") fillLive();
}

async function fillLive() {
  const live = $("live");
  if (!live) return;
  try {
    const st = await api("/api/maya/status");
    if (!st.connected) {
      live.innerHTML = `<p class="bad">未连接${st.error ? "：" + esc(st.error) : ""}</p>`;
      return;
    }
    const d = st.detail || {};
    const counts = d.counts || {};
    live.innerHTML = `<div class="stats">
      <div class="stat"><b>${esc(d.version || "-")}</b><span>Maya</span></div>
      <div class="stat"><b>${esc(d.time ?? "-")}</b><span>当前帧 ${esc(d.min)}–${esc(d.max)}</span></div>
      <div class="stat"><b>${esc(counts.mesh ?? 0)}</b><span>网格</span></div>
      <div class="stat"><b>${esc(counts.light ?? 0)}</b><span>灯光</span></div>
    </div>
    <p>文件：${esc(d.file || "未保存")}</p>
    <p>渲染器：${esc(d.renderer || "-")} ／ 单位：${esc(d.units || "-")}</p>
    <p>选择：${esc((d.selection || []).join(", ") || "无")}</p>`;
  } catch (err) {
    live.textContent = err.message;
  }
}

async function refreshPill() {
  try {
    const st = await api("/api/maya/status");
    const pill = $("connPill");
    pill.classList.toggle("on", !!st.connected);
    if (!st.connected) pill.textContent = "未连接";
    else {
      const d = st.detail || {};
      pill.textContent = "Maya " + (d.version || "已连接");
    }
    if (state.page === "overview") {
      const live = $("live");
      if (live) await fillLive();
    }
  } catch { /* ignore polling errors */ }
}

async function saveConfig(partial) {
  state.config = await api("/api/config", { method: "POST", body: JSON.stringify(partial) });
  toast("配置已保存");
}

function selectedModels() {
  return Array.from(document.querySelectorAll(".model-check:checked")).map((el) => el.value);
}

async function probe(models) {
  if (!models.length) { toast("先选择模型", true); return; }
  if (models.length > 20 && !confirm("将探测 " + models.length + " 个模型，可能产生费用。继续？")) return;
  state.probes = [];
  render();
  try {
    await sse("/api/newapi/probe", { models }, (event, data) => {
      if (event === "probe") {
        state.probes.push(data);
        const tbody = document.querySelector("tbody");
        if (tbody) {
          const tr = document.createElement("tr");
          tr.innerHTML = `<td>${esc(data.model)}</td><td class="${data.ok ? "ok" : "bad"}">${esc(data.class || "")}</td><td>${esc(data.latencyMs)} ms</td><td>${esc(data.reply || data.error || "")}</td>`;
          if (tbody.children.length === 1 && tbody.textContent.includes("尚未探测")) tbody.innerHTML = "";
          tbody.appendChild(tr);
        }
      }
    });
    toast("探测完成");
  } catch (err) {
    toast(err.message, true);
  }
}

function addTrace(className, html) {
  const trace = $("trace");
  if (!trace) return;
  const el = document.createElement("div");
  el.className = "bubble " + className;
  el.innerHTML = html;
  trace.appendChild(el);
  el.scrollIntoView({ block: "nearest" });
}

async function onClick(event) {
  const btn = event.target.closest("[data-action]");
  if (!btn) return;
  const action = btn.dataset.action;
  try {
    if (action === "connect") {
      await api("/api/maya/connect", { method: "POST", body: JSON.stringify({ host: val("mayaHost"), port: Number(val("mayaPort")) }) });
      toast("已连接");
      refreshPill();
    } else if (action === "disconnect") {
      await api("/api/maya/disconnect", { method: "POST", body: "{}" });
      toast("已断开");
      refreshPill();
    } else if (action === "save-maya") {
      await saveConfig({ mayaHost: val("mayaHost"), mayaPort: Number(val("mayaPort")) });
    } else if (action === "copy-script") {
      await navigator.clipboard.writeText(state.script);
      toast("已复制");
    } else if (action === "quick") {
      await callTool("maya_primitive", { kind: btn.dataset.kind });
    } else if (action === "primitive") {
      const sub = val("subdiv");
      const parts = sub ? sub.split(/[,\s]+/).map(Number) : [];
      const width = num("width");
      const kind = val("kind");
      const args = {
        kind, name: val("primName") || undefined,
        height: num("height"), depth: num("depth"), translate: vec("translate"),
        subdivisionsX: parts[0], subdivisionsY: parts[1], subdivisionsZ: parts[2]
      };
      if (kind === "cube" || kind === "plane") args.width = width;
      else args.radius = width;
      if (kind === "torus") { args.radius = width; args.sectionRadius = num("depth"); }
      await callTool("maya_primitive", args);
    } else if (action === "edit") {
      const act = val("editAction");
      const args = { action: act, names: names("editNames") };
      const amount = num("editAmount");
      if (act === "extrude") args.distance = amount;
      if (act === "bevel") { args.fraction = amount; args.segments = 1; }
      if (act === "smooth") args.divisions = amount;
      if (act === "rename") args.name = val("editExtra");
      if (act === "parent") args.parent = val("editExtra") || "world";
      await callTool("maya_edit", args);
    } else if (action === "move") {
      await callTool("maya_transform", { names: names("editNames"), translate: vec("moveTo") });
    } else if (action === "material") {
      await callTool("maya_material", { name: val("matName"), type: val("matType"), color: rgb("matColor"), assignTo: names("matAssign") });
    } else if (action === "nparticle") {
      await callTool("maya_fx", { kind: "nparticle", name: val("fxName"), rate: num("fxRate"), position: vec("fxPos") });
    } else if (action === "ncloth") {
      await callTool("maya_fx", { kind: "ncloth", mesh: val("clothMesh") || undefined });
    } else if (action === "field") {
      await callTool("maya_fx", { kind: "field", field: val("fieldKind"), magnitude: num("fieldMag"), position: vec("fieldPos"), affect: names("fieldAffect") });
    } else if (action === "nucleus") {
      await callTool("maya_fx", { kind: "nucleus", name: val("nucName"), gravity: num("nucG"), gravityDirection: vec("nucDir") });
    } else if (action === "sim") {
      await callTool("maya_simulate", {
        action: btn.dataset.sim, start: num("simStart"), end: num("simEnd"), step: num("simStep"),
        names: names("simNames"), path: val("simPath") || undefined, width: 1280, height: 720
      });
    } else if (action === "light") {
      await callTool("maya_light", {
        kind: val("lightKind"), name: val("lightName") || undefined, intensity: num("lightIntensity"),
        color: rgb("lightColor"), translate: vec("lightPos"), rotate: vec("lightRot"), coneAngle: num("cone")
      });
    } else if (action === "render-settings") {
      await callTool("maya_render_settings", {
        renderer: val("renderer"), width: num("rw"), height: num("rh"), samples: num("samples"), prefix: val("prefix") || undefined
      });
    } else if (action === "render") {
      await callTool("maya_render", { camera: val("camera") || "persp", frame: num("frame"), width: num("rw") , height: num("rh") });
    } else if (action === "camera") {
      await callTool("maya_camera", { name: val("camName"), focalLength: num("focal"), translate: [0, 2, 8], rotate: [-10, 0, 0] });
    } else if (action === "save-api") {
      const body = { newAPIBase: val("apiBase"), newAPIModel: val("apiModel") };
      const key = val("apiKey");
      if (key) body.newAPIKey = key;
      await saveConfig(body);
    } else if (action === "list-models") {
      const key = val("apiKey");
      const body = { newAPIBase: val("apiBase"), newAPIModel: val("apiModel") };
      if (key) body.newAPIKey = key;
      await saveConfig(body);
      const listed = await api("/api/newapi/models", { method: "POST", body: "{}" });
      state.models = listed.models || [];
      render();
      toast("拉取到 " + state.models.length + " 个模型");
    } else if (action === "probe-selected") {
      await probe(selectedModels());
    } else if (action === "probe-ten") {
      await probe(state.models.slice(0, 10).map((m) => m.id));
    } else if (action === "agent") {
      if (state.running) state.running.abort();
      const ctrl = new AbortController();
      state.running = ctrl;
      $("trace").innerHTML = "";
      await sse("/api/agent/run", { prompt: val("prompt"), model: val("agentModel") }, (event, data) => {
        if (event === "status") addTrace("", esc(data.text));
        else if (event === "assistant") addTrace("", esc(data.text));
        else if (event === "tool_call") addTrace("tool", `<b>${esc(data.name)}</b><pre>${esc(JSON.stringify(data.args))}</pre>`);
        else if (event === "tool_result") addTrace("tool", `<b>${esc(data.name)} 结果</b><pre>${esc(JSON.stringify(data.result))}</pre>`);
        else if (event === "tool_error" || event === "error") addTrace("err", esc(data.error || data.text));
        else if (event === "done") addTrace("", "完成 " + esc(data.text || ""));
      }, ctrl.signal);
      toast("指令结束");
    } else if (action === "agent-stop") {
      if (state.running) state.running.abort();
    } else if (action === "copy") {
      await navigator.clipboard.writeText($(btn.dataset.copy).textContent);
      toast("已复制");
    } else if (action === "reload-logs") {
      const data = await api("/api/logs");
      state.logLines = data.lines || [];
      render();
    }
  } catch (err) {
    if (err.name === "AbortError") toast("已停止");
    else toast(err.message, true);
  }
}

$("nav").addEventListener("click", async (event) => {
  const btn = event.target.closest("[data-page]");
  if (!btn) return;
  state.page = btn.dataset.page;
  if (state.page === "logs") {
    const data = await api("/api/logs");
    state.logLines = data.lines || [];
  }
  if (state.page === "mcp" && !state.tools) {
    const data = await api("/api/tools");
    state.tools = data.tools || [];
  }
  if (state.page === "connect") {
    const boot = await api("/api/maya/bootstrap");
    state.script = boot.script;
  }
  render();
});
$("content").addEventListener("click", onClick);
$("quit").addEventListener("click", async () => {
  if (!confirm("退出 Maya Auto？")) return;
  await api("/api/shutdown", { method: "POST", body: "{}" });
});

(async function init() {
  state.health = await api("/api/health");
  state.config = await api("/api/config");
  $("ver").textContent = state.health.version || "";
  const boot = await api("/api/maya/bootstrap");
  state.script = boot.script;
  render();
  refreshPill();
  setInterval(refreshPill, 5000);
})().catch((err) => toast(err.message, true));
