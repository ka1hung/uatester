import { Call, Events } from "/wails/runtime.js";
import { t, applyText, setLang, getLang } from "./i18n.js";

// ───────────── helpers ─────────────

const call = (service, method, ...args) => Call.ByName(`main.${service}.${method}`, ...args);
const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

function h(tag, props = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (k === "class") e.className = v;
    else if (k === "i18n") e.dataset.i18n = v;
    else if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else if (v !== undefined && v !== null && v !== false) e.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    e.append(c instanceof Node ? c : String(c));
  }
  return e;
}

function errText(err) {
  const m = err?.message ?? String(err);
  try { return JSON.parse(m).message ?? m; } catch { return m; }
}

function showError(box, err) {
  if (!err) { box.hidden = true; box.textContent = ""; return; }
  box.textContent = t("err.prefix") + errText(err);
  box.hidden = false;
}

function fmtTime(iso, withDate = false) {
  if (!iso || iso.startsWith("0001-")) return "";
  const d = new Date(iso);
  const p = (n, w = 2) => String(n).padStart(w, "0");
  const time = `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${p(d.getMilliseconds(), 3)}`;
  return withDate ? `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${time}` : time;
}

function fmtValue(v) {
  if (v === null || v === undefined) return "";
  if (typeof v === "number") return Number.isInteger(v) ? String(v) : String(Math.round(v * 1e6) / 1e6);
  return typeof v === "object" ? JSON.stringify(v) : String(v);
}

function formData(form) {
  const out = {};
  for (const el of form.elements) {
    if (!el.name) continue;
    out[el.name] = el.type === "checkbox" ? el.checked : el.type === "number" ? Number(el.value) : el.value;
  }
  return out;
}

// Re-renders a table body only when its data changed, so buttons in it
// are not replaced between a press and its click.
function renderRows(tbody, data, row, empty) {
  const key = JSON.stringify(data);
  if (tbody.dataset.key === key) return;
  tbody.dataset.key = key;
  tbody.replaceChildren(...(data.length ? data.map(row)
    : [h("tr", {}, h("td", { colspan: 9, class: "muted" }, t(empty)))]));
}

// ───────────── tabs and language ─────────────

const tabHooks = {};

function selectTab(group, name) {
  $$(`.tabs[data-group="${group}"] button`).forEach(b => b.classList.toggle("active", b.dataset.tab === name));
  $$(`.tab[data-group="${group}"]`).forEach(tab => { tab.hidden = tab.dataset.tab !== name; });
  tabHooks[`${group}.${name}`]?.();
}

$$(".tabs").forEach(nav => nav.addEventListener("click", e => {
  const b = e.target.closest("button[data-tab]");
  if (b) selectTab(nav.dataset.group, b.dataset.tab);
}));
const activeTab = group => $(`.tabs[data-group="${group}"] button.active`)?.dataset.tab;

$("#lang").addEventListener("click", () => {
  setLang(getLang() === "zh" ? "en" : "zh");
  relabel();
});

// relabel refreshes text set by code rather than by data-i18n.
function relabel() {
  applyText();
  fillSecurities();
  showServerState();
  showClientState();
  $$("tbody").forEach(tb => { delete tb.dataset.key; });
  ptKey = "";
  renderWatch();
  if (lastHistory) renderHistory(...lastHistory);
  showStress(lastStress);
  renderEvents();
  showStatus();
  showEventsState();
  if (selected) showDetail();
  $$("#log-lines .side").forEach(s => { s.textContent = t("log." + s.parentElement.dataset.side); });
}

// ───────────── server ─────────────

const srvForm = $("#srv-form");
const srvErr = $("#srv-error");
let srv = { running: false };

async function startStopServer() {
  const btn = $("#srv-toggle");
  btn.disabled = true;
  showError(srvErr, null);
  try {
    if (srv.running) await call("ServerService", "Stop");
    else {
      await call("ServerService", "Start", formData(srvForm));
      loadPointNames();
    }
  } catch (err) {
    showError(srvErr, err);
  }
  btn.disabled = false;
  await refreshServer();
}
$("#srv-toggle").addEventListener("click", startStopServer);
srvForm.addEventListener("submit", e => { e.preventDefault(); startStopServer(); });

function showServerState() {
  const chip = $("#srv-state");
  chip.className = "chip " + (srv.running ? "on" : "off");
  chip.textContent = t(srv.running ? "server.running" : "server.stopped");
  $("#srv-toggle").textContent = t(srv.running ? "server.stop" : "server.start");
  srvForm.hidden = srv.running;
  $("#srv-live").hidden = !srv.running;
  const sum = $("#srv-summary");
  sum.hidden = !srv.running;
  if (srv.running) {
    const f = formData(srvForm);
    sum.textContent = `${f.addr} · ${t("server.points")} ${f.points} · ${t("server.interval")} ${f.intervalMs} · ${t("server.users")}`;
  }
}

async function refreshServer() {
  try {
    srv = await call("ServerService", "Status");
  } catch (err) {
    return;
  }
  showServerState();
  if (!srv.running) return;
  const l = srv.load;
  $("#st-conn").textContent = l.connections;
  $("#st-sess").textContent = (l.sessions || []).length;
  $("#st-subs").textContent = l.subscriptions;
  $("#st-mem").textContent = `${l.heapMB} / ${l.sysMB} MB`;
  $("#pt-sim").checked = srv.simulating;

  const eps = srv.endpoints || [];
  $("#ep-count").textContent = eps.length;
  renderRows($("#ep-rows"), eps, ep => h("tr", {},
    h("td", {}, h("code", {}, ep.url)), h("td", {}, ep.policy), h("td", {}, ep.mode),
    h("td", {}, h("button", { class: "small ghost", onclick: () => useInClient(ep) }, t("server.useInClient")))), "server.endpoints");

  switch (activeTab("srv")) {
    case "points": await refreshPoints(); break;
    case "alarms": await refreshAlarms(); break;
    case "sessions": refreshSessions(l.sessions || []); break;
  }
}

function useInClient(ep) {
  const f = $("#cl-form");
  f.endpoint.value = ep.url.replace(/\/\/[^:/]+/, "//localhost");
  f.security.value = ep.policy === "None" ? "none" : `${ep.policy}:${ep.mode}`;
}

// points

let ptKey = "";
const ptCells = new Map();

async function refreshPoints() {
  let page;
  try {
    page = await call("ServerService", "Points", $("#pt-filter").value, 300);
  } catch { return; }
  const pts = page.points || [];
  $("#pt-count").textContent = t("points.shown", pts.length, page.total);
  const key = pts.map(p => p.name).join(",");
  if (key !== ptKey) {
    ptKey = key;
    ptCells.clear();
    $("#pt-rows").replaceChildren(...pts.map(pointRow));
  }
  for (const p of pts) {
    const c = ptCells.get(p.name);
    if (!c) continue;
    c.value.textContent = fmtValue(p.value);
    c.status.textContent = p.status;
    c.status.className = p.status === "Good" ? "" : "status-bad";
    c.time.textContent = fmtTime(p.time);
    c.manual.hidden = !p.manual || p.type !== "Double";
  }
}

function pointRow(p) {
  const input = h("input", { placeholder: t("points.newValue.ph") });
  const set = async () => {
    try {
      await call("ServerService", "SetPoint", p.name, input.value);
      input.value = "";
      showError(srvErr, null);
      refreshPoints();
    } catch (err) { showError(srvErr, err); }
  };
  input.addEventListener("keydown", e => { if (e.key === "Enter") set(); });
  const manual = h("span", { class: "row-actions" },
    h("span", { class: "chip" }, t("points.manual")),
    h("button", { class: "small ghost", onclick: async () => { await call("ServerService", "ResumePoint", p.name); refreshPoints(); } }, t("points.resume")));
  const cells = { value: h("td", { class: "num" }), status: h("td"), time: h("td"), manual };
  ptCells.set(p.name, cells);
  return h("tr", {}, h("td", {}, p.name), cells.value, cells.status, cells.time,
    h("td", {}, h("div", { class: "row-actions" }, manual, input, h("button", { class: "small", onclick: set }, t("points.set")))));
}

$("#pt-filter").addEventListener("input", refreshPoints);
$("#pt-sim").addEventListener("change", e => call("ServerService", "SetSimulating", e.target.checked));
tabHooks["srv.points"] = refreshPoints;

async function loadPointNames() {
  try {
    const page = await call("ServerService", "Points", "", 100000);
    $("#pt-names").replaceChildren(...(page.points || []).map(p => h("option", { value: p.name })));
  } catch { /* not running */ }
}

// alarms and events

async function refreshAlarms() {
  let alarms;
  try { alarms = await call("ServerService", "Alarms"); } catch { return; }
  renderRows($("#al-rows"), alarms || [], a => h("tr", {},
    h("td", {}, a.source), h("td", {}, a.name),
    h("td", {}, h("span", { class: "chip " + (a.active ? "bad" : "off") }, t(a.active ? "alarms.active" : "alarms.inactive")),
      a.acked ? " ✓ " + t("alarms.acked") : ""),
    h("td", { class: "num" }, a.severity), h("td", { class: "wrap" }, a.message), h("td", {}, fmtTime(a.time)),
    h("td", {}, a.acked ? "" : h("button", {
      class: "small", onclick: async () => {
        try { await call("ServerService", "Acknowledge", a.source, a.name); refreshAlarms(); } catch (err) { showError(srvErr, err); }
      },
    }, t("alarms.ack")))), "alarms.none");
}
tabHooks["srv.alarms"] = refreshAlarms;

async function setAlarm(active) {
  const f = formData($("#al-form"));
  try {
    await call("ServerService", "SetAlarm", f.point, active, f.severity, f.message);
    showError(srvErr, null);
    refreshAlarms();
  } catch (err) { showError(srvErr, err); }
}
$("#al-raise").addEventListener("click", () => setAlarm(true));
$("#al-clear").addEventListener("click", () => setAlarm(false));
$("#ev-form").addEventListener("submit", async e => {
  e.preventDefault();
  try {
    await call("ServerService", "FireEvent", e.target.message.value || "uatester event");
    e.target.message.value = "";
  } catch (err) { showError(srvErr, err); }
});

// sessions and certificates

function refreshSessions(sessions) {
  renderRows($("#se-rows"), sessions, s => h("tr", {},
    h("td", {}, s.client || s.name), h("td", {}, s.user), h("td", { class: "num" }, s.subscriptions), h("td", {}, fmtTime(s.lastActivity))),
  "sessions.none");
}

async function refreshCerts() {
  let certs;
  try { certs = await call("ServerService", "Rejected"); } catch (err) { showError(srvErr, err); return; }
  renderRows($("#ce-rows"), certs || [], c => h("tr", {},
    h("td", {}, c.subject), h("td", {}, h("code", {}, c.uri)), h("td", {}, (c.notAfter || "").slice(0, 10)),
    h("td", {}, h("button", {
      class: "small", onclick: async () => {
        try { await call("ServerService", "Trust", c.thumbprint); refreshCerts(); } catch (err) { showError(srvErr, err); }
      },
    }, t("certs.trust")))), "certs.none");
}
tabHooks["srv.certs"] = refreshCerts;
$("#ce-refresh").addEventListener("click", refreshCerts);

// ───────────── client ─────────────

const clForm = $("#cl-form");
const clErr = $("#cl-error");
let securities = [];
let clState = "Closed";
let connected = null; // Connection while connected

function fillSecurities() {
  const sel = clForm.security;
  const cur = sel.value || "none";
  sel.replaceChildren(...securities.map(s => h("option", { value: s },
    s === "none" ? t("client.sec.none") : s === "auto" ? t("client.sec.auto") : s)));
  sel.value = cur;
}

function clientParams() {
  return { ...formData(clForm), timeoutMs: 5000 };
}

function showClientState() {
  const on = connected !== null;
  const chip = $("#cl-state");
  chip.className = "chip " + (clState === "Connected" ? "on" : on ? "bad" : "off");
  chip.textContent = clState === "Connected" ? t("client.connected") : on ? clState : t("client.disconnected");
  $("#cl-conn").textContent = !on ? "" : connected.policy === "None" ? "None" : `${connected.policy} ${connected.mode}`;
  $("#cl-toggle").textContent = t(on ? "client.disconnect" : "client.connect");
  // While connected the settings collapse into the status line.
  clForm.hidden = on;
  if (on) $("#cl-eplist").hidden = true;
  showStatus();
}

async function connectToggle() {
  const btn = $("#cl-toggle");
  btn.disabled = true;
  showError(clErr, null);
  try {
    if (connected) {
      await call("ClientService", "Disconnect");
      connected = null;
      clState = "Closed";
      status = null;
      eventsOn = false;
      showStatus();
      showEventsState();
    } else {
      connected = await call("ClientService", "Connect", clientParams());
      clState = "Connected";
      $("#cl-eplist").hidden = true;
      loadTree();
      loadStatus();
      if (watchIds.length) applyWatch();
    }
  } catch (err) {
    showError(clErr, err);
  }
  btn.disabled = false;
  showClientState();
}
$("#cl-toggle").addEventListener("click", connectToggle);
clForm.addEventListener("submit", e => { e.preventDefault(); connectToggle(); });

Events.On("client:state", ev => {
  clState = ev.data;
  if (clState === "Closed" && connected) connected = null;
  showClientState();
});

// browse

let selected = null;

function loadTree() {
  const root = h("ul");
  $("#tree").replaceChildren(root);
  expand(root, "i=84", ["i=85"]);
}

async function expand(ul, nodeId, open = []) {
  const parent = nodeId;
  let items;
  try { items = await call("ClientService", "Browse", nodeId); } catch (err) { showError(clErr, err); return; }
  ul.replaceChildren(...(items || []).map(it => {
    const children = h("ul", { hidden: true });
    const twist = h("span", { class: "twist" }, "▸");
    let loaded = false;
    const toggle = async () => {
      if (!loaded) { loaded = true; await expand(children, it.nodeId); if (!children.childElementCount) twist.textContent = "·"; }
      children.hidden = !children.hidden;
      if (children.childElementCount) twist.textContent = children.hidden ? "▸" : "▾";
    };
    twist.addEventListener("click", e => { e.stopPropagation(); toggle(); });
    const row = h("div", { class: "node", onclick: () => select({ ...it, parent }, row), ondblclick: toggle },
      twist, h("span", {}, it.displayName || it.browseName), h("span", { class: "cls" }, it.nodeClass));
    if (open.includes(it.nodeId)) toggle();
    return h("li", {}, row, children);
  }));
}

function select(item, row) {
  $$(".tree .node.sel").forEach(n => n.classList.remove("sel"));
  row.classList.add("sel");
  selected = item;
  $("#nd-empty").hidden = true;
  $("#nd").hidden = false;
  $("#nd-name").textContent = item.displayName || item.browseName;
  $("#nd-id").textContent = item.nodeId;
  $("#nd-class").textContent = item.nodeClass;
  $("#nd-value").replaceChildren();
  $("#nd-out").replaceChildren();
  $("#nd-msg").textContent = "";
  selectedAttrs = null;
  selectedArgs = null;
  showDetail();
  loadDetail(item);
  if (item.nodeClass === "Variable") readSelected();
}

let selectedAttrs = null;
let selectedArgs = null;

async function loadDetail(item) {
  try {
    const a = await call("ClientService", "Attributes", item.nodeId);
    if (selected !== item) return;
    selectedAttrs = a;
    if (item.nodeClass === "Method") {
      selectedArgs = await call("ClientService", "MethodArgs", item.nodeId);
      if (selected !== item) return;
    }
    showDetail();
  } catch (err) { showError(clErr, err); }
}

function showDetail() {
  const item = selected, a = selectedAttrs;
  $("#nd-var").hidden = item.nodeClass !== "Variable";
  $("#nd-method").hidden = item.nodeClass !== "Method";
  $("#nd-obj").hidden = !(a && a.events);
  const rows = [];
  if (a) {
    if (a.description) rows.push([t("attr.description"), a.description]);
    if (a.dataType) rows.push([t("attr.datatype"), a.dataType + (a.valueRank >= 0 ? " []" : "")]);
    const yes = [], no = [];
    if (item.nodeClass === "Variable") {
      (a.canRead ? yes : no).push(t("attr.read"));
      (a.canWrite ? yes : no).push(t("attr.write"));
      if (a.history) yes.push(t("attr.history"));
    }
    if (a.events) yes.push(t("attr.events"));
    if (a.history && item.nodeClass !== "Variable") yes.push(t("attr.history"));
    if (item.nodeClass === "Method") (a.userExecutable ? yes : no).push(t("attr.exec"));
    if (yes.length || no.length) {
      rows.push([t("attr.access"), yes.join("、") + (no.length ? `（${t("attr.denied")} ${no.join("、")}）` : "")]);
    }
  }
  $("#nd-attrs").replaceChildren(...rows.flatMap(([k, v]) => [h("dt", {}, k), h("dd", {}, v)]));
  if (item.nodeClass === "Method") {
    const args = selectedArgs?.in || [];
    $("#nd-args").replaceChildren(...(args.length ? args.map((x, i) => h("label", {},
      h("span", {}, `${x.name} (${x.dataType})`), h("input", { name: "arg" + i, title: x.description || "" })))
      : [h("p", { class: "muted" }, t("browse.noargs"))]));
  }
}

$("#nd-method").addEventListener("submit", async e => {
  e.preventDefault();
  const n = selectedArgs?.in?.length || 0;
  const args = Array.from({ length: n }, (_, i) => e.target.elements["arg" + i].value);
  try {
    const out = await call("ClientService", "Call", selected.parent, selected.nodeId, args);
    showError(clErr, null);
    const names = selectedArgs?.out || [];
    $("#nd-out").replaceChildren(h("dt", {}, "✓"), h("dd", {}, t("browse.called")),
      ...(out || []).flatMap((v, i) => [h("dt", {}, names[i]?.name || "out " + (i + 1)), h("dd", {}, fmtValue(v))]));
  } catch (err) {
    showError(clErr, err);
    $("#nd-out").replaceChildren();
  }
});

$("#nd-events").addEventListener("click", () => {
  $("#ev-sub").notifier.value = selected.nodeId;
  selectTab("cl", "events");
  subscribeEvents();
});
$("#nd-evhist").addEventListener("click", () => {
  const hf = $("#hi-form");
  hf.node.value = selected.nodeId;
  hf.agg.value = "events";
  selectTab("cl", "history");
  readHistory();
});

async function readSelected() {
  if (!selected) return;
  try {
    const [v] = await call("ClientService", "Read", [selected.nodeId]);
    $("#nd-value").replaceChildren(
      h("dt", {}, t("points.value")), h("dd", {}, fmtValue(v.value)),
      h("dt", {}, "Type"), h("dd", {}, v.type || ""),
      h("dt", {}, t("points.status")), h("dd", { class: v.good ? "" : "status-bad" }, v.error || v.status),
      h("dt", {}, t("points.time")), h("dd", {}, fmtTime(v.time, true)));
  } catch (err) { showError(clErr, err); }
}
$("#nd-read").addEventListener("click", readSelected);
$("#nd-write").addEventListener("submit", async e => {
  e.preventDefault();
  try {
    await call("ClientService", "Write", selected.nodeId, e.target.value.value);
    $("#nd-msg").textContent = t("browse.written");
    showError(clErr, null);
    readSelected();
  } catch (err) { showError(clErr, err); }
});
$("#nd-watch").addEventListener("click", () => { addWatch(selected.nodeId); });
$("#nd-hist").addEventListener("click", () => {
  $("#hi-form").node.value = selected.nodeId;
  selectTab("cl", "history");
  readHistory();
});

// watch

let watchIds = [];
const watchVals = new Map();

function addWatch(id) {
  id = id.trim();
  if (!id || watchIds.includes(id)) return;
  watchIds.push(id);
  applyWatch();
}

async function applyWatch() {
  renderWatch();
  if (!connected) return;
  try {
    const wf = $("#wa-form");
    const vals = await call("ClientService", "Watch", watchIds, Number(wf.interval.value) || 500, wf.dbkind.value, Number(wf.dbvalue.value) || 0);
    for (const v of vals || []) watchVals.set(v.nodeId, v);
    showError(clErr, null);
  } catch (err) { showError(clErr, err); }
  renderWatch();
}

const watchCells = new Map();

function renderWatch() {
  const tb = $("#wa-rows");
  if (!watchIds.length) {
    tb.replaceChildren(h("tr", {}, h("td", { colspan: 5, class: "muted" }, t("watch.none"))));
    watchCells.clear();
    return;
  }
  watchCells.clear();
  tb.replaceChildren(...watchIds.map(id => {
    const cells = { value: h("td", { class: "num" }), status: h("td"), time: h("td") };
    watchCells.set(id, cells);
    return h("tr", {}, h("td", {}, h("code", {}, id)), cells.value, cells.status, cells.time,
      h("td", {}, h("button", {
        class: "small ghost", onclick: () => { watchIds = watchIds.filter(x => x !== id); watchVals.delete(id); applyWatch(); },
      }, t("watch.remove"))));
  }));
  watchIds.forEach(id => showWatch(watchVals.get(id)));
}

function showWatch(v) {
  if (!v) return;
  const c = watchCells.get(v.nodeId);
  if (!c) return;
  c.value.textContent = fmtValue(v.value);
  c.status.textContent = v.error || v.status;
  c.status.className = v.good ? "" : "status-bad";
  c.time.textContent = fmtTime(v.time);
}

Events.On("client:value", ev => { watchVals.set(ev.data.nodeId, ev.data); showWatch(ev.data); });
$("#wa-form").addEventListener("submit", e => { e.preventDefault(); addWatch(e.target.node.value); e.target.node.value = ""; });
["interval", "dbkind", "dbvalue"].forEach(n => $("#wa-form")[n].addEventListener("change", () => { if (watchIds.length) applyWatch(); }));

// history

let lastHistory = null;

async function readHistory() {
  const f = formData($("#hi-form"));
  const out = $("#hi-out");
  if (f.agg === "events") {
    try {
      const evs = await call("ClientService", "HistoryEvents", f.node, f.from);
      showError(clErr, null);
      lastHistory = null;
      out.replaceChildren(h("p", { class: "chart-title" }, `${f.node} · ${t("history.agg.events")} · ${t("history.count", (evs || []).length)}`),
        (evs || []).length ? eventTable(evs) : h("p", { class: "muted" }, t("history.none")));
    } catch (err) { showError(clErr, err); out.replaceChildren(); }
    return;
  }
  try {
    const vals = await call("ClientService", "History", f.node, f.agg, f.from, f.agg === "raw" ? 0 : f.interval);
    showError(clErr, null);
    lastHistory = [f, vals || []];
    renderHistory(f, vals || []);
  } catch (err) {
    showError(clErr, err);
    out.replaceChildren();
    lastHistory = null;
  }
}
$("#hi-form").addEventListener("submit", e => { e.preventDefault(); readHistory(); });

function renderHistory(f, vals) {
  const out = $("#hi-out");
  const title = h("p", { class: "chart-title" },
    `${f.node} · ${t("history.agg." + f.agg)} · ${t("history.count", vals.length)}`);
  if (!vals.length) { out.replaceChildren(title, h("p", { class: "muted" }, t("history.none"))); return; }
  const pts = vals
    .map(v => ({ x: new Date(v.time).getTime(), y: typeof v.value === "boolean" ? Number(v.value) : v.value, v }))
    .filter(p => typeof p.y === "number" && Number.isFinite(p.y))
    .sort((a, b) => a.x - b.x);
  const table = h("details", {}, h("summary", {}, t("history.table")),
    h("div", { class: "scroll", style: "max-height: 220px" }, h("table", { class: "grid" },
      h("thead", {}, h("tr", {}, h("th", {}, t("points.time")), h("th", { class: "num" }, t("points.value")), h("th", {}, t("points.status")))),
      h("tbody", {}, vals.map(v => h("tr", {}, h("td", {}, fmtTime(v.time, true)), h("td", { class: "num" }, fmtValue(v.value)),
        h("td", { class: v.status === "Good" ? "" : "status-bad" }, v.status)))))));
  if (pts.length < 2) {
    out.replaceChildren(title, pts.length ? "" : h("p", { class: "muted" }, t("history.noChart")), table);
    if (pts.length < 2) table.open = true;
    return;
  }
  out.replaceChildren(title, lineChart(pts), table);
}

// niceStep rounds a tick step to 1, 2 or 5 times a power of ten.
function niceStep(raw) {
  const p = 10 ** Math.floor(Math.log10(raw));
  const f = raw / p;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * p;
}

// lineChart draws one series: 2px line, hairline grid, crosshair tooltip.
function lineChart(pts) {
  const NS = "http://www.w3.org/2000/svg";
  const s = (tag, attrs = {}) => {
    const e = document.createElementNS(NS, tag);
    for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
    return e;
  };
  const W = Math.max(400, $("#hi-out").clientWidth || 600), H = 240;
  const m = { l: 56, r: 12, t: 10, b: 24 };
  const x0 = pts[0].x, x1 = pts[pts.length - 1].x;
  let y0 = Math.min(...pts.map(p => p.y)), y1 = Math.max(...pts.map(p => p.y));
  if (y0 === y1) { y0 -= 1; y1 += 1; }
  const step = niceStep((y1 - y0) / 4);
  y0 = Math.floor(y0 / step) * step;
  y1 = Math.ceil(y1 / step) * step;
  const ticks = [];
  for (let y = y0; y <= y1 + step / 2; y += step) ticks.push(Math.round(y / step) * step);
  const X = x => m.l + (x - x0) / (x1 - x0 || 1) * (W - m.l - m.r);
  const Y = y => H - m.b - (y - y0) / (y1 - y0) * (H - m.t - m.b);

  const svg = s("svg", { class: "chart", viewBox: `0 0 ${W} ${H}`, role: "img" });
  for (const y of ticks) {
    svg.append(s("line", { class: "gridline", x1: m.l, x2: W - m.r, y1: Y(y), y2: Y(y) }));
    const lbl = s("text", { class: "axis", x: m.l - 6, y: Y(y) + 4, "text-anchor": "end" });
    lbl.textContent = fmtValue(y);
    svg.append(lbl);
  }
  for (let i = 0; i <= 4; i++) {
    const x = x0 + (x1 - x0) * i / 4;
    const lbl = s("text", { class: "axis", x: X(x), y: H - 6, "text-anchor": i === 0 ? "start" : i === 4 ? "end" : "middle" });
    lbl.textContent = fmtTime(new Date(x).toISOString()).slice(0, 8);
    svg.append(lbl);
  }
  svg.append(s("path", { class: "line", d: pts.map((p, i) => `${i ? "L" : "M"}${X(p.x).toFixed(1)},${Y(p.y).toFixed(1)}`).join("") }));

  const cross = s("line", { class: "cross", y1: m.t, y2: H - m.b, visibility: "hidden" });
  const dot = s("circle", { class: "dot", r: 4, visibility: "hidden" });
  const hit = s("rect", { x: m.l, y: m.t, width: W - m.l - m.r, height: H - m.t - m.b, fill: "transparent" });
  svg.append(cross, dot, hit);
  const tip = $("#tooltip");
  hit.addEventListener("mousemove", e => {
    const r = svg.getBoundingClientRect();
    const mx = (e.clientX - r.left) * (W / r.width);
    let best = pts[0];
    for (const p of pts) if (Math.abs(X(p.x) - mx) < Math.abs(X(best.x) - mx)) best = p;
    cross.setAttribute("x1", X(best.x)); cross.setAttribute("x2", X(best.x)); cross.setAttribute("visibility", "visible");
    dot.setAttribute("cx", X(best.x)); dot.setAttribute("cy", Y(best.y)); dot.setAttribute("visibility", "visible");
    tip.replaceChildren(h("div", { class: "muted" }, fmtTime(best.v.time, true)), h("b", {}, fmtValue(best.v.value)),
      best.v.status === "Good" ? "" : h("span", { class: "status-bad" }, " " + best.v.status));
    tip.hidden = false;
    tip.style.left = `${e.clientX + 12}px`;
    tip.style.top = `${e.clientY + 12}px`;
  });
  hit.addEventListener("mouseleave", () => {
    tip.hidden = true;
    cross.setAttribute("visibility", "hidden");
    dot.setAttribute("visibility", "hidden");
  });
  return svg;
}

// endpoints and server status

$("#cl-endpoints").addEventListener("click", async () => {
  showError(clErr, null);
  const box = $("#cl-eplist");
  try {
    const eps = await call("ClientService", "Endpoints", clForm.endpoint.value);
    $("#cl-eprows").replaceChildren(...(eps || []).map(ep => {
      const sec = ep.policy === "None" ? "none" : `${ep.policy}:${ep.mode}`;
      const use = () => { clForm.security.value = sec; box.hidden = true; };
      return h("tr", { onclick: use }, h("td", {}, ep.policy), h("td", {}, ep.mode), h("td", {}, (ep.logins || []).join(", ")),
        h("td", {}, h("button", { type: "button", class: "small ghost" }, t("client.ep.use"))));
    }));
    box.hidden = false;
  } catch (err) { showError(clErr, err); box.hidden = true; }
});

let status = null;

async function loadStatus() {
  try { status = await call("ClientService", "Status"); } catch { status = null; }
  showStatus();
}

function showStatus() {
  const p = $("#cl-status");
  p.hidden = !status || !connected;
  if (!connected) return;
  const f = formData(clForm);
  const who = `${f.endpoint} · ${f.user || "anonymous"}`;
  p.hidden = false;
  p.textContent = status
    ? `${who} · ${t("client.status", status.product, status.version, status.state, fmtTime(status.started, true).slice(0, 19))}`
    : who;
}

// events and alarms

let eventsOn = false;
const alarms = new Map(); // condition id -> newest event
let eventLog = [];

function showEventsState() {
  const btn = $("#ev-toggle");
  btn.dataset.i18n = eventsOn ? "events.stop" : "events.subscribe";
  btn.textContent = t(btn.dataset.i18n);
  btn.className = eventsOn ? "ghost" : "primary";
  $("#ev-refresh").disabled = !eventsOn;
  $("#ev-sub").notifier.disabled = eventsOn;
}

async function subscribeEvents() {
  showError(clErr, null);
  alarms.clear();
  eventLog = [];
  renderEvents();
  try {
    await call("ClientService", "SubscribeEvents", $("#ev-sub").notifier.value);
    eventsOn = true;
  } catch (err) { showError(clErr, err); eventsOn = false; }
  showEventsState();
}

$("#ev-sub").addEventListener("submit", async e => {
  e.preventDefault();
  if (eventsOn) {
    await call("ClientService", "StopEvents");
    eventsOn = false;
    showEventsState();
  } else {
    subscribeEvents();
  }
});
$("#ev-refresh").addEventListener("click", async () => {
  try { await call("ClientService", "RefreshEvents"); } catch (err) { showError(clErr, err); }
});

Events.On("client:event", ev => {
  const e = ev.data;
  if (e.type === "RefreshStart" || e.type === "RefreshEnd") return;
  if (e.conditionId) {
    if (e.retain) alarms.set(e.conditionId, e); else alarms.delete(e.conditionId);
  }
  eventLog.unshift(e);
  if (eventLog.length > 300) eventLog.length = 300;
  renderEvents();
});

function stateChips(e) {
  return [h("span", { class: "chip " + (e.active ? "bad" : "off") }, t(e.active ? "alarms.active" : "alarms.inactive")),
    e.acked ? " ✓ " + t("alarms.acked") : ""];
}

function renderEvents() {
  const list = [...alarms.values()].sort((a, b) => b.severity - a.severity || (a.time < b.time ? 1 : -1));
  renderRows($("#ev-alarms"), list, e => h("tr", {},
    h("td", {}, e.sourceName || e.source), h("td", {}, e.conditionName), h("td", {}, stateChips(e)),
    h("td", { class: "num" }, e.severity), h("td", { class: "wrap" }, e.message), h("td", {}, fmtTime(e.time)),
    h("td", {}, e.acked ? "" : h("button", {
      class: "small", onclick: async () => {
        try {
          await call("ClientService", "Acknowledge", e, "acknowledged in uatester");
          showError(clErr, null);
        } catch (err) { showError(clErr, err); }
      },
    }, t("alarms.ack")))), "events.noalarms");
  const tb = $("#ev-log");
  delete tb.dataset.key;
  tb.replaceChildren(...(eventLog.length ? eventLog.slice(0, 300).map(eventRow)
    : [h("tr", {}, h("td", { colspan: 5, class: "muted" }, t("events.none")))]));
}

function eventRow(e) {
  const type = e.type === "Alarm" || e.type === "Condition" || e.type === "Event" ? t("evtype." + e.type) : e.type;
  return h("tr", {}, h("td", {}, fmtTime(e.time)), h("td", {}, type, e.conditionId ? [" ", ...stateChips(e)] : ""),
    h("td", {}, e.sourceName || e.source), h("td", { class: "num" }, e.severity), h("td", { class: "wrap" }, e.message));
}

function eventTable(evs) {
  return h("div", { class: "scroll", style: "max-height: 420px" }, h("table", { class: "grid" },
    h("thead", {}, h("tr", {}, h("th", {}, t("points.time")), h("th", {}, t("events.type")), h("th", {}, t("alarms.source")),
      h("th", { class: "num" }, t("alarms.severity")), h("th", {}, t("alarms.message")))),
    h("tbody", {}, evs.map(eventRow))));
}

// load test

let stressRunning = false;
let lastStress = null;

$("#sx-form").addEventListener("submit", async e => {
  e.preventDefault();
  showError(clErr, null);
  try {
    if (stressRunning) {
      await call("ClientService", "StopStress");
    } else {
      await call("ClientService", "StartStress", clientParams(), formData(e.target));
      stressRunning = true;
      showStress(null);
    }
  } catch (err) { showError(clErr, err); }
});

function showStress(r) {
  lastStress = r;
  const btn = $("#sx-toggle");
  btn.dataset.i18n = stressRunning ? "stress.stop" : "stress.start";
  btn.textContent = t(btn.dataset.i18n);
  if (!r) { $("#sx-stats").replaceChildren(); return; }
  const tile = (key, v) => h("div", {}, h("span", {}, t(key)), h("b", {}, v));
  $("#sx-stats").replaceChildren(
    tile("stress.elapsed", `${Math.round(r.elapsed / 1e9)} s`),
    tile("stress.rate", r.perSecond.toLocaleString()),
    tile("stress.total", r.total.toLocaleString()),
    tile("stress.connected", r.connected),
    tile("stress.failed", r.failed));
  if (r.lastError) $("#sx-stats").append(h("p", { class: "status-bad note" }, r.lastError));
  if (r.done) $("#sx-stats").prepend(h("p", { class: "note" }, "✓ " + t("stress.done")));
}

Events.On("stress", ev => {
  if (ev.data.done) stressRunning = false;
  showStress(ev.data);
});

// ───────────── log ─────────────

let logSide = "";
const logBox = $("#log-lines");

function logLine(l) {
  const atEnd = logBox.scrollTop + logBox.clientHeight >= logBox.scrollHeight - 4;
  logBox.append(h("div", { class: l.level, "data-side": l.side, hidden: Boolean(logSide && l.side !== logSide) },
    h("span", { class: "t" }, fmtTime(l.time) + " "), h("span", { class: "side" }, t("log." + l.side)), h("span", { class: "msg" }, l.text)));
  while (logBox.childElementCount > 500) logBox.firstElementChild.remove();
  if (atEnd) logBox.scrollTop = logBox.scrollHeight;
}

Events.On("log", ev => logLine(ev.data));
$("#log-filter").addEventListener("click", e => {
  const b = e.target.closest("button");
  if (!b) return;
  logSide = b.dataset.side;
  $$("#log-filter button").forEach(x => x.classList.toggle("active", x === b));
  $$("#log-lines > div").forEach(d => { d.hidden = logSide && d.dataset.side !== logSide; });
});
$("#log-clear").addEventListener("click", () => logBox.replaceChildren());

// ───────────── start ─────────────

async function init() {
  applyText();
  const info = await call("AppService", "Info");
  $("#version").textContent = "v" + info.version;
  $("#datadir").textContent = info.dataDir;
  const d = await call("ServerService", "Defaults");
  for (const [k, v] of Object.entries(d)) {
    const el = srvForm.elements[k];
    if (!el) continue;
    if (el.type === "checkbox") el.checked = v; else el.value = v;
  }
  securities = await call("ClientService", "Securities");
  fillSecurities();
  (await call("AppService", "Logs") || []).forEach(logLine);
  showClientState();
  renderWatch();
  renderEvents();
  showEventsState();
  showStress(null);
  await refreshServer();
  setInterval(refreshServer, 1000);
}

init().catch(err => showError(srvErr, err));
