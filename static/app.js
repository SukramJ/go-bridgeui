// SPDX-License-Identifier: MIT
// Copyright (C) 2026 SukramJ
//
// go-bridgeui dashboard logic. Plain ES module-free vanilla JS: load the
// layout + language, the writable items and the configuration once, then
// live-update from the SSE stream. No framework, no build step.
//
// The page is the bridge's data: GET api/ui names the data sections and the
// groups they show, every value arrives with its name and unit, every
// status/config tile with an i18n label. Built-in strings and the bridge's
// own labels come from one merged bundle, i18n/<lang>.json.
"use strict";

const THEME_KEY = "bridgeui-theme";
const DEVICE_KEY = "bridgeui-device";

let I18N = {};
let LANG = "en";
let LOCALE = "en-US";
let LAYOUT = { brand: "", product: "", icon: "", sections: [], system_groups: [] };
let groupSection = {}; // entry group -> section id
let tableSection = {}; // table id -> section id
let subSection = null; // id of the section that shows Entry.sub cards
let items = []; // writable items
let itemsByKey = {};
let latest = { health: {}, snapshot: { devices: [] } };
let selected = null; // selected device id

// ---------- i18n ----------
function t(key) {
  return key in I18N ? I18N[key] : key;
}

// tf looks up key and substitutes {name} placeholders from params.
function tf(key, params) {
  let s = t(key);
  // Replacement is passed as a function, not a string: params[k] may be
  // user-supplied (e.g. a value typed into the write form), and
  // String.replace() would otherwise interpret "$&", "$$", "$1" etc. in a
  // string replacement as special patterns.
  for (const k in params) s = s.replace("{" + k + "}", () => params[k]);
  return s;
}

async function loadI18n(lang) {
  LANG = lang === "de" ? "de" : "en";
  LOCALE = LANG === "de" ? "de-DE" : "en-US";
  document.documentElement.lang = LANG;
  try {
    I18N = await fetchJSON("i18n/" + LANG + ".json");
  } catch (e) {
    I18N = {}; // fall back to raw keys / English HTML defaults
  }
  applyStaticI18n();
}

// applyStaticI18n fills every element carrying a data-i18n / data-i18n-title
// attribute from the loaded bundle. Missing keys leave the HTML default.
function applyStaticI18n() {
  document.querySelectorAll("[data-i18n]").forEach((node) => {
    const key = node.dataset.i18n;
    if (key in I18N) node.textContent = I18N[key];
  });
  document.querySelectorAll("[data-i18n-title]").forEach((node) => {
    const key = node.dataset.i18nTitle;
    if (key in I18N) node.title = I18N[key];
  });
}

// ---------- bootstrap ----------
document.addEventListener("DOMContentLoaded", init);

async function init() {
  initTheme();
  selected = storageGet(DEVICE_KEY);
  // Layout and language first, so every section exists and every string
  // is localised before the first frame. Relative URLs (no leading slash)
  // keep the SPA working behind the Home-Assistant ingress path prefix.
  let ui = null;
  try {
    ui = await fetchJSON("api/ui");
  } catch (e) {
    /* render the fixed sections only */
  }
  if (ui) applyLayout(ui.layout || {});
  await loadI18n((ui && ui.language) || "en");
  initNav();
  try {
    indexItems(await fetchJSON("api/items"));
  } catch (e) {
    toast(t("toast.itemsFail") + e.message, "err");
  }
  try {
    setGrid("config-grid", (await fetchJSON("api/config")).map(tileView));
  } catch (e) {
    /* non-critical */
  }
  connectSSE();
}

// applyLayout brands the page and builds the data sections and their
// navigation entries.
function applyLayout(layout) {
  LAYOUT = Object.assign(LAYOUT, layout);
  document.title = LAYOUT.product ? LAYOUT.product + " · " + LAYOUT.brand : LAYOUT.brand;
  document.getElementById("brand-name").textContent = LAYOUT.brand;
  document.getElementById("brand-product").textContent = LAYOUT.product;
  document.getElementById("brand-icon").textContent = LAYOUT.icon;
  document.getElementById("favicon").href =
    "data:image/svg+xml," + encodeURIComponent(
      "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'><text y='20' font-size='20'>" +
      LAYOUT.icon + "</text></svg>");

  const host = document.getElementById("data-sections");
  const navControl = document.getElementById("nav-control");
  groupSection = {};
  tableSection = {};
  subSection = null;
  for (const s of LAYOUT.sections || []) {
    for (const g of s.groups || []) groupSection[g] = s.id;
    for (const tb of s.tables || []) tableSection[tb] = s.id;
    if (s.subs && !subSection) subSection = s.id;

    const sec = el("section", "section");
    sec.id = "sec-" + s.id;
    const h2 = el("h2", "section-title", s.title);
    h2.dataset.i18n = s.title;
    sec.appendChild(h2);
    const stack = el("div", "group-stack");
    stack.id = "groups-" + s.id;
    sec.appendChild(stack);
    const tables = el("div", "group-stack");
    tables.id = "tables-" + s.id;
    sec.appendChild(tables);
    host.appendChild(sec);

    const a = el("a", "nav-item", s.nav || s.title);
    a.href = "#sec-" + s.id;
    a.dataset.target = sec.id;
    a.dataset.i18n = s.nav || s.title;
    navControl.before(a);
  }

  // Links to the bridge's own pages follow the fixed entries.
  const nav = document.getElementById("nav");
  for (const l of LAYOUT.links || []) {
    const a = el("a", "nav-item nav-link", l.title);
    a.href = l.href;
    a.dataset.i18n = l.title;
    nav.appendChild(a);
  }
}

function indexItems(list) {
  items = list || [];
  itemsByKey = {};
  for (const it of items) itemsByKey[it.key] = it;
}

// ---------- SSE ----------
function connectSSE() {
  const es = new EventSource("api/events");
  es.addEventListener("update", (ev) => {
    streamDot(true);
    try {
      render(JSON.parse(ev.data));
    } catch (e) {
      /* ignore malformed frame */
    }
  });
  es.onerror = () => streamDot(false); // EventSource auto-reconnects
}

// ---------- render ----------
function render(view) {
  latest = {
    health: view.health || {},
    snapshot: view.snapshot || { devices: [] },
  };
  const devices = latest.snapshot.devices || [];
  // Keep the choice while that device exists; otherwise prefer one that
  // has reported over one only known to be unreachable.
  if (!devices.some((d) => d.id === selected)) {
    const first = devices.find((d) => !isZeroTime(d.updated_at)) || devices[0];
    selected = first ? first.id : null;
  }
  const dev = devices.find((d) => d.id === selected) || null;

  renderHeader(latest.health, dev);
  renderChips(devices);
  renderHealth(latest.health);
  renderDevices(devices);
  renderProblems(latest.health.problems || []);
  renderGroups(dev);
  renderTables(dev);
  hideEmptySections();
  renderControls(dev);
  document.getElementById("footer-app").textContent = tf("footer.app", { brand: LAYOUT.brand });
  document.getElementById("last-update").textContent =
    t("footer.updated") + new Date().toLocaleTimeString(LOCALE);
}

function renderHeader(h, dev) {
  const sub = [];
  if (dev && dev.subtitle) sub.push(dev.subtitle);
  if (dev && !isZeroTime(dev.updated_at)) {
    sub.push(tf("group.age", { age: formatAge(ageSeconds(dev.updated_at)) }));
  }
  document.getElementById("device-title").textContent =
    (dev && (dev.title || dev.id)) || LAYOUT.product || LAYOUT.brand;
  document.getElementById("device-sub").textContent = sub.join("  ·  ");
  if (h.version) document.getElementById("version").textContent = "v" + h.version;

  const badge = document.getElementById("conn-badge");
  const tones = { ok: "badge-ok", degraded: "badge-warn", starting: "badge-neutral" };
  badge.className = "badge " + (tones[h.status] || "badge-neutral");
  badge.textContent = h.status ? t("status." + h.status) : "—";
}

function renderChips(devices) {
  const host = document.getElementById("device-chips");
  host.hidden = devices.length < 2;
  host.innerHTML = "";
  if (host.hidden) return;
  for (const d of devices) {
    const chip = el("button", "chip" + (d.id === selected ? " active" : ""));
    chip.type = "button";
    chip.appendChild(el("span", "dot " + onlineTone(d.online)));
    chip.appendChild(document.createTextNode(d.title && d.title !== d.id ? d.title + " · " + d.id : d.id));
    chip.addEventListener("click", () => {
      selected = d.id;
      storageSet(DEVICE_KEY, d.id);
      render(latest);
    });
    host.appendChild(chip);
  }
}

function renderHealth(h) {
  const tiles = [
    { label: t("health.status"), value: h.status ? t("status." + h.status) : "—", tone: statusTone(h.status) },
    ...(h.tiles || []).map(tileView),
  ];
  if (h.started_at && !isZeroTime(h.started_at)) {
    tiles.push({ label: t("health.uptime"), value: formatUptime(h.uptime_seconds) });
  }
  setGrid("health-grid", tiles);
}

// renderDevices lists every device with its reachability and data age;
// only shown when the bridge serves more than one.
function renderDevices(devices) {
  const card = document.getElementById("devices-card");
  card.hidden = devices.length < 2;
  const host = document.getElementById("device-grid");
  host.innerHTML = "";
  if (card.hidden) return;
  for (const d of devices) {
    const tone = onlineTone(d.online);
    const item = el("div", "status-item" + (tone !== "neutral" ? " tone-" + tone : ""));
    item.appendChild(el("div", "label wrap", d.id));
    const state = d.online === undefined || d.online === null
      ? t("val.unknown")
      : d.online ? t("val.online") : t("val.offline");
    item.appendChild(el("div", "value", state));
    const age = !isZeroTime(d.updated_at)
      ? tf("group.age", { age: formatAge(ageSeconds(d.updated_at)) })
      : t("dev.noReport");
    const title = d.title && d.title !== d.id ? d.title : null;
    item.appendChild(el("div", "label wrap", [title, age].filter(Boolean).join(" · ")));
    host.appendChild(item);
  }
}

function renderProblems(problems) {
  const card = document.getElementById("problems-card");
  const list = document.getElementById("problem-list");
  card.hidden = !problems.length;
  list.innerHTML = "";
  for (const p of problems) {
    const li = el("li");
    const head = el("div", "p-head");
    head.appendChild(el("span", "p-event", p.event));
    const when = new Date(p.at);
    head.appendChild(el("span", "muted", (p.device ? p.device + " · " : "") + when.toLocaleTimeString(LOCALE)));
    li.appendChild(head);
    if (p.detail) li.appendChild(el("span", "p-detail", p.detail));
    list.appendChild(li);
  }
}

// renderGroups buckets the selected device's entries into the layout's
// sections: by group, and entries with a sub-unit by that sub-unit.
function renderGroups(dev) {
  const containers = { system: document.getElementById("system-groups") };
  for (const s of LAYOUT.sections || []) containers[s.id] = document.getElementById("groups-" + s.id);
  for (const c of Object.values(containers)) c.innerHTML = "";
  const entries = (dev && dev.entries) || [];

  const groups = {};
  const subs = {};
  for (const e of entries) {
    if (e.sub && subSection) (subs[e.sub] = subs[e.sub] || []).push(e);
    else (groups[e.group] = groups[e.group] || []).push(e);
  }

  // Section groups in layout order; then the system groups in their
  // order, then whatever no section claims.
  for (const s of LAYOUT.sections || []) {
    for (const g of s.groups || []) {
      if (g in groups) containers[s.id].appendChild(groupCard(groupLabel(g), groups[g], groupAge(dev, g)));
    }
  }
  const sysOrder = (LAYOUT.system_groups || []).filter((g) => g in groups && !(g in groupSection));
  for (const g of Object.keys(groups).sort()) {
    if (!(g in groupSection) && !sysOrder.includes(g)) sysOrder.push(g);
  }
  for (const g of sysOrder) containers.system.appendChild(groupCard(groupLabel(g), groups[g], groupAge(dev, g)));

  if (subSection) {
    const s = LAYOUT.sections.find((x) => x.id === subSection);
    for (const sub of Object.keys(subs).sort()) {
      containers[subSection].appendChild(
        groupCard(tf(s.sub_title || "{sub}", { sub }), subs[sub], groupAge(dev, subs[sub][0].group)));
    }
  }

}

// hideEmptySections hides an optional section the selected device has
// nothing for, and marks an empty mandatory one.
function hideEmptySections() {
  for (const s of LAYOUT.sections || []) {
    const groups = document.getElementById("groups-" + s.id);
    const empty = !groups.children.length && !document.getElementById("tables-" + s.id).children.length;
    const hide = empty && s.optional;
    showSection("sec-" + s.id, !hide);
    if (empty && !hide) groups.innerHTML = `<div class="card empty">${t("group.empty")}</div>`;
  }
}

function showSection(id, show) {
  document.getElementById(id).hidden = !show;
  const nav = document.querySelector('.nav-item[data-target="' + id + '"]');
  if (nav) nav.hidden = !show;
}

// ---------- tables ----------
// renderTables places the selected device's tables into their sections.
// A table's card is kept across updates and only its rows are redrawn, so
// a filter being typed keeps its focus while the values move.
const tableCards = {}; // container id + "/" + table id -> card parts
function renderTables(dev) {
  const seen = new Set();
  for (const tb of (dev && dev.tables) || []) {
    const host = tableSection[tb.id] ? "tables-" + tableSection[tb.id] : "system-tables";
    const id = host + "/" + tb.id;
    seen.add(id);
    let c = tableCards[id];
    if (!c) c = tableCards[id] = tableCard();
    const container = document.getElementById(host);
    // Keep layout order: the section's tables first, in its order.
    if (c.card.parentNode !== container) container.appendChild(c.card);
    fillTable(c, tb);
  }
  for (const id of Object.keys(tableCards)) {
    if (!seen.has(id)) {
      tableCards[id].card.remove();
      delete tableCards[id];
    }
  }
}

function tableCard() {
  const card = el("div", "card table-card");
  const head = el("div", "group-head");
  const title = el("h3");
  const count = el("span", "group-age");
  head.appendChild(title);
  head.appendChild(count);
  const filter = el("input", "table-filter");
  filter.type = "search";
  filter.placeholder = t("table.filter");
  head.appendChild(filter);
  card.appendChild(head);
  const wrap = el("div", "table-wrap");
  const table = el("table", "data-table");
  const thead = el("thead");
  const tbody = el("tbody");
  table.appendChild(thead);
  table.appendChild(tbody);
  wrap.appendChild(table);
  card.appendChild(wrap);
  const c = { card, title, count, filter, thead, tbody, last: null };
  filter.addEventListener("input", () => c.last && fillTable(c, c.last));
  return c;
}

// tableFilterMin is the row count from which a table offers a filter.
const tableFilterMin = 10;

function fillTable(c, tb) {
  c.last = tb;
  c.title.textContent = t(tb.title || tb.id);
  const cols = tb.columns || [];
  const rows = tb.rows || [];
  c.filter.hidden = rows.length < tableFilterMin;

  const sig = JSON.stringify(cols);
  if (c.thead.dataset.sig !== sig) {
    c.thead.dataset.sig = sig;
    c.thead.innerHTML = "";
    const tr = el("tr");
    for (const col of cols) {
      const th = el("th", col.align === "right" ? "num" : null, t(col.label));
      if (col.unit) th.appendChild(el("span", "unit", col.unit));
      tr.appendChild(th);
    }
    c.thead.appendChild(tr);
  }

  const q = c.filter.hidden ? "" : c.filter.value.trim().toLocaleLowerCase(LOCALE);
  c.tbody.innerHTML = "";
  let shown = 0;
  for (const r of rows) {
    const texts = cols.map((col, i) => cellText(col, (r.cells || [])[i]));
    if (q && !texts.some((x) => x.toLocaleLowerCase(LOCALE).includes(q))) continue;
    shown++;
    const tr = el("tr", r.tone ? "tone-" + r.tone : null);
    texts.forEach((x, i) => tr.appendChild(el("td", cols[i].align === "right" ? "num" : null, x)));
    c.tbody.appendChild(tr);
  }
  if (!rows.length) {
    const tr = el("tr");
    const td = el("td", "empty", t("group.empty"));
    td.colSpan = Math.max(1, cols.length);
    tr.appendChild(td);
    c.tbody.appendChild(tr);
  }
  c.count.textContent = shown === rows.length
    ? tf("table.rows", { n: rows.length })
    : tf("table.shown", { n: shown, total: rows.length });
}

function cellText(col, v) {
  if (v === null || v === undefined || v === "") return "–";
  switch (col.format) {
    case "duration":
      return formatUptime(v);
    case "age":
      return formatAge(v);
    case "bitrate":
      return formatBitrate(v);
    case "i18n":
      return t(String(v));
  }
  return fmtValue(v);
}

function formatBitrate(bps) {
  bps = Number(bps) || 0;
  const units = ["bit/s", "kbit/s", "Mbit/s", "Gbit/s"];
  let i = 0;
  while (bps >= 1000 && i < units.length - 1) {
    bps /= 1000;
    i++;
  }
  return bps.toLocaleString(LOCALE, { maximumFractionDigits: i ? 1 : 0 }) + " " + units[i];
}

function groupLabel(g) {
  const key = "group." + g;
  return key in I18N ? I18N[key] : prettify(g);
}

function groupAge(dev, g) {
  if (!dev) return null;
  const at = (dev.group_updated_at && dev.group_updated_at[g]) || dev.updated_at;
  return isZeroTime(at) ? null : ageSeconds(at);
}

function groupCard(label, entries, age) {
  const card = el("div", "card group-card");
  const head = el("div", "group-head");
  head.appendChild(el("h3", null, label));
  if (age !== null) {
    head.appendChild(el("span", "group-age", tf("group.age", { age: formatAge(age) })));
  }
  card.appendChild(head);

  const grid = el("div", "status-grid");
  const sorted = [...entries].sort((a, b) =>
    entryLabel(a).localeCompare(entryLabel(b), LOCALE));
  if (!sorted.length) grid.appendChild(el("div", "empty", "—"));
  for (const e of sorted) grid.appendChild(valueTile(e));
  card.appendChild(grid);
  return card;
}

function valueTile(e) {
  const item = el("div", "status-item" + (e.tone ? " tone-" + e.tone : ""));
  item.appendChild(el("div", "label", entryLabel(e)));
  const v = el("div", "value");
  v.textContent = displayValue(e.key, e.value);
  if (e.unit) {
    const u = el("span", "unit");
    u.textContent = e.unit;
    v.appendChild(u);
  }
  item.appendChild(v);
  return item;
}

function entryLabel(e) {
  return e.name || prettify(e.key);
}

// ---------- control (writable items) ----------
// renderControls lists the selected device's writable items. Rows are
// rebuilt only when the device or its item set changes, so a value typed
// into a field survives the live updates in between. A bridge with no
// writable item at all is read-only: it has no control section.
let controlSig = "";
function renderControls(dev) {
  showSection("sec-control", items.length > 0);
  const host = document.getElementById("control-list");
  const values = dev ? currentValues(dev) : {};
  const mine = dev ? items.filter((it) => !it.device || it.device === dev.id) : [];
  const sig = (dev ? dev.id : "") + "|" + mine.map((it) => it.key).join(",");
  if (sig !== controlSig) {
    controlSig = sig;
    host.innerHTML = "";
    if (!mine.length) {
      host.innerHTML = `<div class="empty">${t(dev ? "ctl.none" : "dev.none")}</div>`;
    }
    for (const it of mine) host.appendChild(controlRow(it));
  }
  updateControlValues(values);
}

// currentValues maps a device's entries (no sub-unit entries) by key.
function currentValues(dev) {
  const out = {};
  for (const e of dev.entries || []) {
    if (!e.sub) out[e.key] = e.value;
  }
  return out;
}

function controlRow(it) {
  const row = el("div", "control-row");

  const name = el("div", "c-name");
  name.appendChild(el("strong", null, it.name || it.key));
  const sub = [it.key];
  if (it.unit) sub.push(it.unit);
  if (it.min !== undefined && it.max !== undefined) sub.push(it.min + "–" + it.max);
  if (it.confirm) sub.push(t("ctl.confirmed"));
  name.appendChild(el("small", null, sub.join(" · ")));
  row.appendChild(name);

  const current = el("span", "c-current");
  current.dataset.key = it.key;
  current.textContent = it.component === "button" ? "" : "—";
  row.appendChild(current);

  const ctrls = el("div", "c-ctrls");
  // A button writes its fixed payload (unlock, start fast charging).
  if (it.component === "button") {
    const btn = el("button", "btn", t("btn.press"));
    btn.addEventListener("click", async () => {
      if (!confirmWrite(it, t("btn.press"))) return;
      btn.disabled = true;
      try {
        await writeValue(it, it.payload || "PRESS");
      } finally {
        btn.disabled = false;
      }
    });
    ctrls.appendChild(btn);
    row.appendChild(ctrls);
    return row;
  }
  // A switch renders as an immediate on/off toggle.
  if (it.component === "switch") {
    const toggle = el("input");
    toggle.type = "checkbox";
    toggle.className = "c-switch";
    toggle.dataset.key = it.key;
    toggle.addEventListener("change", async () => {
      const value = toggle.checked;
      if (!confirmWrite(it, t(value ? "val.on" : "val.off"))) {
        toggle.checked = !value;
        return;
      }
      await writeValue(it, value);
    });
    ctrls.appendChild(toggle);
    row.appendChild(ctrls);
    return row;
  }

  let input;
  if (it.options && it.options.length) {
    input = el("select");
    for (const o of it.options) {
      const opt = el("option", null, o.label);
      opt.value = o.value; // what the write path takes
      input.appendChild(opt);
    }
    input.dataset.key = it.key;
    input.className = "c-select";
    input.addEventListener("change", () => (input.dataset.touched = "1"));
  } else {
    input = el("input");
    input.type = "number";
    input.step = it.step !== undefined ? String(it.step) : "any";
    if (it.min !== undefined) input.min = String(it.min);
    if (it.max !== undefined) input.max = String(it.max);
    input.placeholder = it.unit ? tf("ctl.placeholderUnit", { unit: it.unit }) : t("ctl.placeholder");
  }

  const btn = el("button", "btn", t("btn.set"));
  btn.addEventListener("click", () => writeItem(it, input, btn));

  ctrls.appendChild(input);
  ctrls.appendChild(btn);
  row.appendChild(ctrls);
  return row;
}

function updateControlValues(values) {
  document.querySelectorAll(".c-switch[data-key]").forEach((cb) => {
    const k = cb.dataset.key;
    if (k in values) cb.checked = !!values[k];
  });
  // A select follows the device until the user picks something else.
  document.querySelectorAll(".c-select[data-key]").forEach((sel) => {
    const k = sel.dataset.key;
    if (!(k in values) || sel === document.activeElement || sel.dataset.touched) return;
    sel.value = String(values[k]);
  });
  document.querySelectorAll(".c-current[data-key]").forEach((sp) => {
    const k = sp.dataset.key;
    const it = itemsByKey[k];
    if (!(k in values) || (it && it.component === "button")) return;
    if (it && it.component === "switch") {
      sp.textContent = t("ctl.current") + t(values[k] ? "val.on" : "val.off");
      return;
    }
    sp.textContent = t("ctl.current") + displayValue(k, values[k]) + (it && it.unit ? " " + it.unit : "");
  });
}

// writeItem is the input+button flow: validate, confirm, disable, write.
async function writeItem(it, input, btn) {
  const value = input.value;
  if (value === "" || value === null || value === undefined) {
    toast(t("toast.enterValue"), "err");
    return;
  }
  if (!confirmWrite(it, displayValue(it.key, value) + (it.unit ? " " + it.unit : ""))) return;
  btn.disabled = true;
  try {
    await writeValue(it, value);
  } finally {
    btn.disabled = false;
    delete input.dataset.touched;
  }
}

// confirmWrite asks before an item the bridge marks as needing it: it
// changes how the device operates or what it persists.
function confirmWrite(it, shown) {
  if (!it.confirm) return true;
  return window.confirm(tf("ctl.confirm", { name: it.name || it.key, value: shown, device: selected }));
}

// writeValue posts a single write for the selected device and toasts the result.
async function writeValue(it, value) {
  try {
    const res = await fetch("api/write", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ device: selected, key: it.key, value }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || res.statusText);
    toast(tf("toast.set", { key: it.name || it.key, value: displayValue(it.key, value) }), "ok");
  } catch (e) {
    toast(t("toast.errorPrefix") + e.message, "err");
  }
}

// ---------- small helpers ----------
// tileView renders a status/config tile from the API: label and value text
// localised, value formatted.
function tileView(tile) {
  const value = tile.text ? t(tile.text) : fmtValue(tile.value);
  return { label: t(tile.label), value: tile.sub ? value + "  ·  " + tile.sub : value, tone: tile.tone || null };
}

// displayValue renders a value, showing a select's option value as its label.
function displayValue(key, v) {
  const it = itemsByKey[key];
  if (it && it.options && v !== null && v !== undefined) {
    const o = it.options.find((o) => o.value === String(v));
    if (o) return o.label;
  }
  return fmtValue(v);
}

function fmtValue(v) {
  if (v === null || v === undefined || v === "") return "–";
  if (typeof v === "boolean") return v ? t("val.yes") : t("val.no");
  if (typeof v === "number") {
    if (Number.isInteger(v)) return v.toLocaleString(LOCALE);
    return v.toLocaleString(LOCALE, { maximumFractionDigits: 2 });
  }
  return String(v);
}

function statusTone(s) {
  return { ok: "ok", degraded: "warn" }[s] || null;
}

function onlineTone(online) {
  if (online === undefined || online === null) return "neutral";
  return online ? "ok" : "err";
}

function setGrid(id, tiles) {
  const host = document.getElementById(id);
  host.innerHTML = "";
  for (const it of tiles) {
    const item = el("div", "status-item" + (it.tone ? " tone-" + it.tone : ""));
    item.appendChild(el("div", "label", it.label));
    item.appendChild(el("div", "value", it.value));
    host.appendChild(item);
  }
}

function isZeroTime(iso) {
  return !iso || iso.startsWith("0001-01-01");
}

function ageSeconds(iso) {
  return Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
}

function formatUptime(sec) {
  sec = Number(sec) || 0;
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d) return `${d} d ${h} h`;
  if (h) return `${h} h ${m} min`;
  if (m) return `${m} min`;
  return `${Math.floor(sec)} s`;
}

function formatAge(sec) {
  sec = Math.max(0, Math.floor(Number(sec) || 0));
  if (sec < 60) return sec + " s";
  if (sec < 3600) return Math.floor(sec / 60) + " min";
  return Math.floor(sec / 3600) + " h";
}

function prettify(key) {
  return String(key)
    .replace(/^.*\//, "")
    .replace(/[_\-]+/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined && text !== null) e.textContent = text;
  return e;
}

async function fetchJSON(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(res.status + " " + res.statusText);
  return res.json();
}

// storageGet / storageSet keep per-viewer conveniences; a browser without
// storage (private window, blocked site data) simply forgets them.
function storageGet(key) {
  try {
    return localStorage.getItem(key);
  } catch (e) {
    return null;
  }
}

function storageSet(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch (e) {
    /* not persisted */
  }
}

let toastTimer = null;
function toast(msg, kind) {
  const box = document.getElementById("toast");
  box.textContent = msg;
  box.className = "toast " + (kind || "ok");
  box.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (box.hidden = true), 3500);
}

function streamDot(live) {
  document.getElementById("stream-dot").classList.toggle("live", !!live);
}

// ---------- nav + theme ----------
function initNav() {
  const navItems = [...document.querySelectorAll(".nav-item")];
  const sections = navItems
    .map((i) => document.getElementById(i.dataset.target))
    .filter(Boolean);
  const obs = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (e.isIntersecting) {
          navItems.forEach((i) =>
            i.classList.toggle("active", i.dataset.target === e.target.id));
        }
      }
    },
    { rootMargin: "-40% 0px -55% 0px" }
  );
  sections.forEach((s) => obs.observe(s));
}

function initTheme() {
  const saved = storageGet(THEME_KEY);
  if (saved) document.documentElement.dataset.theme = saved;
  document.getElementById("theme-toggle").addEventListener("click", () => {
    const cur = document.documentElement.dataset.theme === "light" ? "dark" : "light";
    document.documentElement.dataset.theme = cur;
    storageSet(THEME_KEY, cur);
  });
}
