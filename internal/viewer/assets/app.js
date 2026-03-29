let currentSession = null;
let currentItems = [];
let currentItemIdx = null;
let lastStatusKey = "";
let followLatest = true;
let autoRefresh = false;
let themeMode = "auto";
let sessionFilter = "";
let copySeq = 0;
const copyRegistry = {};
let isSelectingSession = false;
let isSelectingItem = false;

async function fetchJson(url) {
    const r = await fetch(url);
    if (!r.ok) throw new Error(`HTTP ${r.status}: ${url}`);
    return await r.json();
}

function escapeHtml(s) {
    return String(s || "")
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#39;");
}

function sanitizeClassName(v) {
    return String(v || "unknown").replace(/[^a-zA-Z0-9_-]/g, "_");
}

function formatNumber(v) {
    const n = Number(v || 0);
    return isNaN(n) ? "0" : n.toLocaleString();
}

function formatTime(ts) {
    const n = Number(ts || 0);
    if (!n) return "-";
    return new Date(n * 1000).toLocaleString();
}

function safeJsonParse(text, fallback) {
    try {
        return JSON.parse(text);
    } catch (_) {
        return fallback;
    }
}

function tryFormatJSON(text) {
    if (typeof text !== "string") return "";
    const raw = text.trim();
    if (!raw) return "";
    try {
        return JSON.stringify(JSON.parse(raw), null, 2);
    } catch (_) {
        return text;
    }
}

function formatAnyJSON(value) {
    if (typeof value === "string") return tryFormatJSON(value);
    try {
        return JSON.stringify(value, null, 2);
    } catch (_) {
        return String(value || "");
    }
}

function registerCopyText(text) {
    const id = "copy_" + (++copySeq);
    copyRegistry[id] = text || "";
    return id;
}

async function copyRegistered(id) {
    const text = copyRegistry[id] || "";
    try {
        await navigator.clipboard.writeText(text);
        setStatusMeta("Copied");
    } catch (_) {
        setStatusMeta("Copy failed");
    }
}

function renderPreWithCopy(text, copyText) {
    const id = registerCopyText(copyText === undefined ? text : copyText);
    return '<div class="pre-wrap"><div class="pre-tools"><button class="btn" onclick="copyRegistered(\'' + id + '\')">Copy</button></div><pre>' + escapeHtml(text || "") + "</pre></div>";
}

function shouldHighlightAsErrorJSON(content) {
    try {
        const obj = JSON.parse(String(content || ""));
        if (!obj || typeof obj !== "object") return false;
        const hasError = !!obj.error;
        const hasDetail = typeof obj.detail === "string" && obj.detail.length > 0;
        const hasStatusCode = Number(obj.status_code || obj.status || 0) >= 400;
        return hasError || hasDetail || hasStatusCode;
    } catch (_) {
        return false;
    }
}

function renderJSONBlock(text, errorStyle) {
    if (!errorStyle) return "<pre>" + escapeHtml(text || "") + "</pre>";
    return '<pre class="error-json">' + escapeHtml(text || "") + "</pre>";
}

function setStatusMeta(text) {
    document.getElementById("status-meta").textContent = text;
}

function getSystemTheme() {
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function applyTheme() {
    const resolved = themeMode === "auto" ? getSystemTheme() : themeMode;
    document.documentElement.setAttribute("data-theme", resolved);
}

function updateControlLabels() {
    document.getElementById("toggle-theme").textContent = "Theme: " + themeMode.toUpperCase();
    document.getElementById("toggle-follow").textContent = "Follow Latest: " + (followLatest ? "ON" : "OFF");
    document.getElementById("toggle-refresh").textContent = "Auto Refresh: " + (autoRefresh ? "ON" : "OFF");
}

function getVisibleItems() {
    return Array.isArray(currentItems) ? currentItems.slice() : [];
}

function nextItemIdx(offset) {
    const list = getVisibleItems();
    if (!list.length) return null;
    const at = list.findIndex(it => it.idx === currentItemIdx);
    if (at < 0) return list[0].idx;
    const nextAt = at + offset;
    if (nextAt < 0 || nextAt >= list.length) return null;
    return list[nextAt].idx;
}

async function loadSessions(options = {}) {
    const preferLatest = !!options.preferLatest;
    const data = await fetchJson("/api/sessions");
    const box = document.getElementById("sessions");
    box.innerHTML = "";
    if (!Array.isArray(data) || !data.length) {
        box.innerHTML = '<div class="empty">(no sessions)</div>';
        currentItems = [];
        currentItemIdx = null;
        document.getElementById("items").innerHTML = '<div class="empty">(no requests)</div>';
        document.getElementById("detail-title").textContent = "Detail";
        document.getElementById("detail").innerHTML = "<pre>No sessions found.</pre>";
        return;
    }
    const filtered = sessionFilter.trim() ? data.filter(s => String(s.name || "").toLowerCase().includes(sessionFilter.trim().toLowerCase())) : data;
    if (!filtered.length) {
        box.innerHTML = '<div class="empty">(no matched sessions)</div>';
        return;
    }

    let autoSelectElement = null;
    let autoSelectName = null;
    filtered.forEach((s, idx) => {
        const d = document.createElement("div");
        d.className = "item";
        if (s.name === currentSession) d.classList.add("active");
        d.innerHTML = '<div class="meta">Session</div><div class="session-name">' + escapeHtml(s.name) + '</div><div class="meta" style="margin-top:6px;">' + escapeHtml(String(s.provider)) + ' · ' + escapeHtml(String(s.count)) + ' requests</div>';
        d.onclick = () => selectSession(s.name, d);
        box.appendChild(d);
        if ((currentSession && s.name === currentSession) || (!currentSession && idx === 0) || (preferLatest && idx === 0)) {
            autoSelectElement = d;
            autoSelectName = s.name;
        }
    });
    if (autoSelectElement && autoSelectName && !isSelectingSession) {
        await selectSession(autoSelectName, autoSelectElement, {preferLatest});
    }
}

async function selectSession(name, element, options = {}) {
    const preferLatest = !!options.preferLatest;
    if (!element || isSelectingSession) return;
    isSelectingSession = true;
    if (currentSession !== name) {
        currentItemIdx = null;
    }
    currentSession = name;
    document.querySelectorAll("#sessions .item").forEach(x => x.classList.remove("active"));
    element.classList.add("active");
    try {
        currentItems = await fetchJson("/api/session/" + encodeURIComponent(name));
        await renderItems({preferLatest});
    } finally {
        isSelectingSession = false;
    }
}

async function renderItems(options = {}) {
    const preferLatest = !!options.preferLatest;
    const box = document.getElementById("items");
    box.innerHTML = "";
    const items = getVisibleItems();
    const preferLatestIdx = preferLatest && items.length ? items[items.length - 1].idx : null;
    if (!items.length) {
        box.innerHTML = '<div class="empty">(no requests)</div>';
        document.getElementById("detail").innerHTML = "<pre>No request items in this session.</pre>";
        return;
    }
    let autoSelectElement = null;
    let autoSelectIdx = null;
    items.forEach((it, idx) => {
        const d = document.createElement("div");
        d.className = "item";
        if (currentItemIdx === it.idx) d.classList.add("active");
        d.innerHTML = '<div class="request-row"><span class="request-id">#' + escapeHtml(String(it.idx)) + '</span><span class="request-model">' + escapeHtml(it.provider) + ' · ' + escapeHtml(it.model || "unknown") + "</span></div>";
        d.onclick = () => selectItem(it.idx, d);
        box.appendChild(d);
        if ((currentItemIdx && currentItemIdx === it.idx) || (!currentItemIdx && idx === 0) || (preferLatestIdx !== null && it.idx === preferLatestIdx)) {
            autoSelectElement = d;
            autoSelectIdx = it.idx;
        }
    });
    if (autoSelectElement && autoSelectIdx !== null && !isSelectingItem) {
        await selectItem(autoSelectIdx, autoSelectElement);
    }
}

async function selectItem(idx, element) {
    if (!element || isSelectingItem) return;
    isSelectingItem = true;
    currentItemIdx = idx;
    document.querySelectorAll("#items .item").forEach(x => x.classList.remove("active"));
    element.classList.add("active");
    try {
        const it = await fetchJson("/api/detail/" + encodeURIComponent(currentSession) + "/" + idx);
        document.getElementById("detail-title").textContent = "Detail #" + it.idx + " " + it.provider + " " + (it.model || "");
        document.getElementById("detail").innerHTML = renderDetail(it);
    } catch (err) {
        document.getElementById("detail").innerHTML = "<pre>" + escapeHtml(err.message || String(err)) + "</pre>";
    } finally {
        isSelectingItem = false;
    }
}

function renderDetail(it) {
    const prevDisabled = nextItemIdx(-1) === null ? " disabled" : "";
    const nextDisabled = nextItemIdx(1) === null ? " disabled" : "";
    const hasBlocks = Array.isArray(it.response_blocks) && it.response_blocks.length > 0;
    const rawFallback = !hasBlocks && it.response_raw ? renderRawResponseCard(it.response_raw) : "";
    return '<div class="status-bar"><div><div class="meta">Current Session</div><div>' + escapeHtml(currentSession || "-") + '</div></div><div class="status-actions"><span class="pill">Auto refresh ' + (autoRefresh ? "ON" : "OFF") + '</span><button class="btn' + prevDisabled + '" onclick="goPrevItem()">Prev</button><button class="btn' + nextDisabled + '" onclick="goNextItem()">Next</button></div></div>' +
        renderRequestCard(it) +
        renderResponseBlocksCard(it.response_blocks || []) +
        renderResponseOverview(it) +
        rawFallback;
}

function renderRequestCard(it) {
    const data = safeJsonParse(it.request_json || "{}", {});
    const items = Array.isArray(it.request_new_messages) ? it.request_new_messages : [];
    const summary = '<div class="summary">' +
        summaryItem("provider", it.provider) +
        summaryItem("model", data.model || "-") +
        summaryItem("messages_total", it.request_message_count || 0) +
        summaryItem("messages_new", it.request_new_message_count || items.length) +
        "</div>";
    const body = items.map((item, i) => renderRequestItemCard(item, i)).join("") || "<pre>(no new request items)</pre>";
    const rawRequest = tryFormatJSON(it.request_json || "");
    return '<div class="card"><div class="card-head"><div>Request Increment</div><div>Only new meassages</div></div><div class="card-body">' + summary + body + '<details><summary>Show raw request JSON</summary>' + renderPreWithCopy(rawRequest, rawRequest) + "</details></div></div>";
}

function summaryItem(name, value) {
    return '<div class="summary-item"><div class="summary-name">' + escapeHtml(String(name)) + '</div><div class="summary-value">' + escapeHtml(String(value)) + "</div></div>";
}

function renderRequestItemCard(item, idx) {
    const type = String((item && item.type) || "unknown");
    if (type === "message" || (item && (item.role || Array.isArray(item.content)))) {
        return renderMessageCard(item, idx);
    }
    const head = type.toUpperCase() + (item && item.role ? " / " + String(item.role).toUpperCase() : "");
    return '<details class="message-card"><summary class="message-head message-summary"><span>' + escapeHtml(head) + "</span><span>#" + (idx + 1) + '</span></summary><div class="message-body">' + renderGenericRequestItem(item) + "</div></details>";
}

function renderMessageCard(message, idx) {
    const role = String((message && message.role) || "unknown");
    const content = Array.isArray(message.content) ? message.content : [];
    return '<details class="message-card"><summary class="message-head message-summary"><span>' + escapeHtml(role.toUpperCase()) + "</span><span>#" + (idx + 1) + '</span></summary><div class="message-body">' + (content.map((b, i) => renderContentBlock(b, i)).join("") || "<pre>(empty content)</pre>") + "</div></details>";
}

function renderGenericRequestItem(item) {
    const copy = formatAnyJSON(item);
    return renderPreWithCopy(copy, copy);
}

function renderContentBlock(block, idx) {
    const type = String((block && block.type) || "unknown");
    let body = "";
    if (type === "text" || type === "input_text" || type === "output_text") body = escapeHtml(block.text || "");
    else if (type === "thinking") body = escapeHtml(block.thinking || "");
    else if (type === "reasoning") {
        const summary = Array.isArray(block.summary) ? block.summary.map(part => part && part.text ? part.text : "").join("\n") : "";
        body = escapeHtml(summary || block.summary_text || "");
    }
    else if (type === "tool_use") {
        const content = formatAnyJSON({
            name: block.name,
            id: block.id,
            input: block.input
        });
        body = "<pre>" + escapeHtml(content || "") + "</pre>";
    } else if (type === "function_call") {
        const content = formatAnyJSON({
            name: block.name,
            call_id: block.call_id,
            arguments: safeJsonParse(block.arguments || "null", block.arguments || "")
        });
        body = "<pre>" + escapeHtml(content || "") + "</pre>";
    } else if (type === "function_call_output") {
        const content = formatAnyJSON({
            call_id: block.call_id,
            output: safeJsonParse(block.output || "null", block.output || "")
        });
        body = "<pre>" + escapeHtml(content || "") + "</pre>";
    } else if (type === "tool_result") {
        const result = formatAnyJSON(block.content);
        body = "<pre>" + escapeHtml(result || "") + "</pre>";
    } else {
        const fallback = formatAnyJSON(block);
        body = renderJSONBlock(fallback, true);
    }
    return '<div class="content-block type-' + sanitizeClassName(type) + '"><div class="content-head"><span>' + escapeHtml(type.toUpperCase()) + "</span><span>#" + (idx + 1) + '</span></div><div class="content-body">' + body + "</div></div>";
}

function renderResponseOverview(it) {
    return '<div class="card"><div class="card-head"><div>Response Overview</div><div>Tokens</div></div><div class="card-body"><div class="summary">' +
        summaryItem("input_tokens", formatNumber(it.response_tokens && it.response_tokens.input_tokens)) +
        summaryItem("output_tokens", formatNumber(it.response_tokens && it.response_tokens.output_tokens)) +
        "</div></div></div>";
}

function renderResponseBlocksCard(blocks) {
    const body = (blocks || []).map(block => {
        const type = String(block.type || "unknown");
        const content = typeof block.content === "string" ? block.content : formatAnyJSON(block.content);
        const maybeFormatted = tryFormatJSON(content);
        const text = maybeFormatted !== content ? maybeFormatted : content;
        const rendered = shouldHighlightAsErrorJSON(text) ? renderJSONBlock(text, true) : renderJSONBlock(text, false);
        return '<div class="response-block response-' + sanitizeClassName(type) + '"><div class="response-head"><span>' + escapeHtml(type.toUpperCase()) + "</span><span>#" + escapeHtml(String(block.index || 0)) + '</span></div><div class="response-content">' + rendered + "</div></div>";
    }).join("") || "<pre>(empty response)</pre>";
    return '<div class="card"><div class="card-head"><div>Response Blocks</div><div>Parsed output</div></div><div class="card-body">' + body + "</div></div>";
}

function renderRawResponseCard(raw) {
    const formatted = tryFormatJSON(raw || "");
    const isError = shouldHighlightAsErrorJSON(formatted);
    return '<div class="card"><div class="card-head"><div>Raw Response Fallback</div><div>No parsed blocks</div></div><div class="card-body">' + renderJSONBlock(formatted || String(raw || ""), isError) + "</div></div>";
}

async function focusItemByIdx(idx) {
    if (idx === null || idx === undefined) return;
    const target = Array.from(document.querySelectorAll("#items .item")).find(el => {
        const text = el.querySelector(".request-id");
        return text && text.textContent === "#" + idx;
    });
    if (!target) {
        await renderItems();
        return;
    }
    await selectItem(idx, target);
}

async function goPrevItem() {
    const idx = nextItemIdx(-1);
    if (idx !== null) {
        await focusItemByIdx(idx);
    }
}

async function goNextItem() {
    const idx = nextItemIdx(1);
    if (idx !== null) {
        await focusItemByIdx(idx);
    }
}

async function refreshStatus(force = false) {
    if (!autoRefresh && !force) return;
    const status = await fetchJson("/api/status");
    const nextKey = status.latest_session + ":" + status.latest_request_count + ":" + status.latest_updated_at;
    if (nextKey === lastStatusKey) {
        setStatusMeta("Synced");
        return;
    }
    lastStatusKey = nextKey;
    if (isSelectingSession || isSelectingItem) return;
    if (followLatest && status.latest_session) {
        currentSession = status.latest_session;
        currentItemIdx = null;
        await loadSessions({preferLatest: true});
    } else {
        await loadSessions();
        await renderItems();
    }
    setStatusMeta("Updated at " + formatTime(status.latest_updated_at));
}

document.getElementById("toggle-follow").onclick = async () => {
    followLatest = !followLatest;
    updateControlLabels();
    if (followLatest) {
        currentItemIdx = null;
        await refreshStatus(true);
    }
};

document.getElementById("toggle-theme").onclick = () => {
    themeMode = themeMode === "auto" ? "dark" : (themeMode === "dark" ? "light" : "auto");
    localStorage.setItem("ccecho-viewer-theme", themeMode);
    applyTheme();
    updateControlLabels();
};

document.getElementById("toggle-refresh").onclick = () => {
    autoRefresh = !autoRefresh;
    updateControlLabels();
    setStatusMeta(autoRefresh ? "Auto refresh resumed" : "Auto refresh paused");
};

document.getElementById("refresh-now").onclick = async () => {
    setStatusMeta("Refreshing...");
    await refreshStatus(true);
};

document.getElementById("session-filter").addEventListener("input", async e => {
    sessionFilter = String(e.target.value || "");
    await loadSessions();
});

document.addEventListener("keydown", async e => {
    if (e.key === "ArrowUp") {
        e.preventDefault();
        await goPrevItem();
    } else if (e.key === "ArrowDown") {
        e.preventDefault();
        await goNextItem();
    } else if (e.key.toLowerCase() === "r") {
        if (e.metaKey || e.ctrlKey) return;
        e.preventDefault();
        await refreshStatus(true);
    }
});

themeMode = localStorage.getItem("ccecho-viewer-theme") || "auto";
applyTheme();
if (window.matchMedia) {
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
        if (themeMode === "auto") applyTheme();
    });
}
updateControlLabels();
loadSessions({preferLatest: true}).catch(err => {
    document.getElementById("detail").innerHTML = "<pre>" + escapeHtml(err.message || String(err)) + "</pre>";
});
window.setInterval(() => {
    refreshStatus().catch(err => console.error(err));
}, 3000);
