const state = {
  sessions: [],
  whitelist: [],
  settings: { days: 7 },
};

const el = (id) => document.getElementById(id);

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const data = await res.json();
  if (!res.ok || data.error) throw new Error(data.error || res.statusText);
  return data;
}

function setStatus(text) {
  el("status").textContent = text || "";
}

function chatIDOf(s) {
  return s.chat_id || s.username || s.chat || "";
}

function friendlyChatName(id, chatType = "", isGroup = false) {
  if (id === "filehelper") return "文件传输助手";
  if (isGroup || chatType === "group" || id.endsWith("@chatroom")) return `群聊 ${shortID(id)}`;
  if (id.startsWith("wxid_")) return `好友 ${shortID(id)}`;
  return id || "(未知会话)";
}

function shortID(id) {
  if (!id || id.length <= 12) return id;
  return `${id.slice(0, 6)}...${id.slice(-4)}`;
}

function sessionTitle(s) {
  const id = chatIDOf(s);
  return s.display_name || s.chat_name || friendlyChatName(id, s.chat_type, s.is_group);
}

function whitelistTitle(w) {
  const id = chatIDOf(w);
  const name = w.chat_name || friendlyChatName(id, w.chat_type, w.is_group);
  if (name === id) return friendlyChatName(id, w.chat_type, w.is_group);
  return name;
}

async function loadSettings() {
  state.settings = await api("/api/settings");
  el("daysInput").value = state.settings.days;
}

async function saveSettings() {
  const days = Number(el("daysInput").value || 7);
  state.settings = await api("/api/settings", {
    method: "PUT",
    body: JSON.stringify({ days }),
  });
  setStatus(`已保存最近 ${state.settings.days} 天`);
}

async function loadSessions() {
  setStatus("正在读取 wx sessions...");
  const data = await api("/api/sessions?limit=100");
  state.sessions = data.sessions || [];
  renderSessions();
  setStatus(`已读取 ${state.sessions.length} 个最近会话。wx-cli 暂未提供真实群名/好友名，可加入白名单后手动重命名。`);
}

async function loadWhitelist() {
  const data = await api("/api/whitelist");
  state.whitelist = data.items || [];
  renderWhitelist();
  renderChatFilter();
}

async function saveWhitelist(item) {
  await api("/api/whitelist", {
    method: "POST",
    body: JSON.stringify(item),
  });
  await loadWhitelist();
}

async function addWhitelist(s) {
  const id = chatIDOf(s);
  const defaultName = sessionTitle(s);
  const name = window.prompt("给这个会话起一个白名单显示名", defaultName);
  if (name === null) return;
  const chatName = name.trim() || defaultName;
  await saveWhitelist({
    chat_id: id,
    chat_name: chatName,
    chat_type: s.chat_type || "",
    is_group: Boolean(s.is_group),
    enabled: true,
    analysis_enabled: true,
    realtime_enabled: false,
    notes: "",
  });
  setStatus(`已加入白名单：${chatName}`);
}

async function renameWhitelist(chatID) {
  const item = state.whitelist.find((w) => w.chat_id === chatID);
  if (!item) return;
  const name = window.prompt("修改白名单显示名", whitelistTitle(item));
  if (name === null) return;
  const chatName = name.trim();
  if (!chatName) return;
  await saveWhitelist({ ...item, chat_name: chatName });
  setStatus(`已重命名：${chatName}`);
}

async function removeWhitelist(chatID) {
  await api(`/api/whitelist/${encodeURIComponent(chatID)}`, { method: "DELETE" });
  await loadWhitelist();
  await loadMessages();
}

function formatFailures(failures) {
  if (!failures.length) return "";
  return failures
    .map((f) => {
      const name = f.name || f.chat_id || "未知会话";
      const hint = f.hint ? `\n  建议：${f.hint}` : "";
      return `- ${name}：${f.error}${hint}`;
    })
    .join("\n");
}

async function syncMessages() {
  const days = Number(el("daysInput").value || state.settings.days || 7);
  setStatus("正在用 wx history 同步白名单聊天记录...");
  const data = await api(`/api/sync?days=${days}`, { method: "POST" });
  const failures = data.failures || [];
  const details = formatFailures(failures);
  setStatus(
    `历史同步完成：白名单 ${data.chats} 个，新增 ${data.inserted} 条，失败 ${failures.length} 个` +
      (details ? `\n${details}` : "")
  );
  await loadMessages();
}

async function pollMessages() {
  setStatus("正在用 wx new-messages 拉取增量新消息...");
  const data = await api("/api/poll?limit=500", { method: "POST" });
  const emptyHint =
    data.checked === 0
      ? "wx-cli 认为自上次检查后没有新消息；首次导入或补历史请点“同步白名单聊天”。"
      : "这只适合持续运行时补充刚出现的新消息。";
  setStatus(
    `新消息拉取完成：检查 ${data.checked} 条，命中白名单 ${data.matched} 条，新增 ${data.inserted} 条。` +
      emptyHint
  );
  await loadMessages();
}

async function loadMessages() {
  const days = Number(el("daysInput").value || state.settings.days || 7);
  const chatID = el("chatFilter").value;
  const q = el("searchInput").value.trim();
  const params = new URLSearchParams({ days: String(days), limit: "300" });
  if (chatID) params.set("chat_id", chatID);
  if (q) params.set("q", q);
  const data = await api(`/api/messages?${params.toString()}`);
  renderMessages(data.messages || []);
}

function renderSessions() {
  const known = new Set(state.whitelist.map((w) => w.chat_id));
  el("sessionsList").innerHTML = state.sessions
    .map((s) => {
      const id = chatIDOf(s);
      const added = known.has(id);
      return `
        <div class="item">
          <div class="item-title">
            <span>${escapeHTML(sessionTitle(s))}</span>
            <span class="badge">${escapeHTML(s.chat_type || "")}</span>
          </div>
          <div class="item-meta">${escapeHTML(id)} · ${escapeHTML(s.time || "")} · unread ${s.unread ?? 0}</div>
          <div class="item-summary">${escapeHTML(s.summary || "")}</div>
          <div class="item-actions">
            <button ${added ? "disabled" : ""} data-add="${escapeAttr(id)}">${added ? "已在白名单" : "加入白名单"}</button>
          </div>
        </div>`;
    })
    .join("");
  el("sessionsList").querySelectorAll("[data-add]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const id = btn.getAttribute("data-add");
      const session = state.sessions.find((s) => chatIDOf(s) === id);
      if (session) addWhitelist(session).catch(showError);
    });
  });
}

function renderWhitelist() {
  const selectedChatID = el("chatFilter").value;
  el("whitelistList").innerHTML = state.whitelist.length
    ? state.whitelist
        .map((w) => `
          <div class="item whitelist-item ${selectedChatID === w.chat_id ? "selected" : ""}" data-filter="${escapeAttr(w.chat_id)}">
            <div class="item-title">
              <span>${escapeHTML(whitelistTitle(w))}</span>
              <span class="badge">${escapeHTML(w.chat_type || "")}</span>
            </div>
            <div class="item-meta">${escapeHTML(w.chat_id)} · ${w.enabled ? "启用" : "停用"}</div>
            <div class="item-actions">
              <button data-rename="${escapeAttr(w.chat_id)}">重命名</button>
              <button class="danger" data-remove="${escapeAttr(w.chat_id)}">移除</button>
            </div>
          </div>`)
        .join("")
    : `<div class="item"><div class="item-summary">白名单为空。先从左侧最近会话中添加。</div></div>`;
  el("whitelistList").querySelectorAll("[data-filter]").forEach((item) => {
    item.addEventListener("click", () => toggleWhitelistFilter(item.getAttribute("data-filter")).catch(showError));
  });
  el("whitelistList").querySelectorAll("[data-rename]").forEach((btn) => {
    btn.addEventListener("click", (event) => {
      event.stopPropagation();
      renameWhitelist(btn.getAttribute("data-rename")).catch(showError);
    });
  });
  el("whitelistList").querySelectorAll("[data-remove]").forEach((btn) => {
    btn.addEventListener("click", (event) => {
      event.stopPropagation();
      removeWhitelist(btn.getAttribute("data-remove")).catch(showError);
    });
  });
}

async function toggleWhitelistFilter(chatID) {
  const filter = el("chatFilter");
  filter.value = filter.value === chatID ? "" : chatID;
  renderWhitelist();
  await loadMessages();
}

function renderChatFilter() {
  const selected = el("chatFilter").value;
  el("chatFilter").innerHTML =
    `<option value="">全部白名单会话</option>` +
    state.whitelist
      .map((w) => `<option value="${escapeAttr(w.chat_id)}">${escapeHTML(whitelistTitle(w))}</option>`)
      .join("");
  if (state.whitelist.some((w) => w.chat_id === selected)) {
    el("chatFilter").value = selected;
  }
}

function renderMessages(messages) {
  el("messagesList").innerHTML = messages.length
    ? messages
        .map((m) => `
          <article class="message">
            <div class="message-head">
              <span>${escapeHTML(m.chat_name || friendlyChatName(m.chat_id, m.chat_type, m.is_group))} · ${escapeHTML(m.sender_name || m.sender_id || "system")} · ${escapeHTML(m.type || "")}</span>
              <span>${escapeHTML(m.time_text || "")}</span>
            </div>
            <div class="message-content">${escapeHTML(m.content || "")}</div>
          </article>`)
        .join("")
    : `<div class="item"><div class="item-summary">暂无消息。首次导入或补历史请点“同步白名单聊天”；持续运行时再用“拉取增量新消息”。</div></div>`;
}

function showError(err) {
  console.error(err);
  setStatus(`错误：${err.message}`);
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function escapeAttr(value) {
  return escapeHTML(value).replaceAll("'", "&#39;");
}

async function boot() {
  el("saveSettingsBtn").addEventListener("click", () => saveSettings().catch(showError));
  el("loadSessionsBtn").addEventListener("click", () => loadSessions().catch(showError));
  el("loadWhitelistBtn").addEventListener("click", () => loadWhitelist().catch(showError));
  el("syncBtn").addEventListener("click", () => syncMessages().catch(showError));
  el("pollBtn").addEventListener("click", () => pollMessages().catch(showError));
  el("searchBtn").addEventListener("click", () => loadMessages().catch(showError));
  el("searchInput").addEventListener("keydown", (event) => {
    if (event.key === "Enter") loadMessages().catch(showError);
  });
  el("chatFilter").addEventListener("change", () => {
    renderWhitelist();
    loadMessages().catch(showError);
  });

  await loadSettings();
  await loadWhitelist();
  await loadSessions();
  await loadMessages();
}

boot().catch(showError);
