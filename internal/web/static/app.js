document.addEventListener("DOMContentLoaded", () => {
  const signInTicket = new URLSearchParams(window.location.hash.slice(1)).get("session");
  if (signInTicket) history.replaceState(null, "", window.location.pathname + window.location.search);
  const authPanel = document.getElementById("localAuthPanel");
  const gatewayContent = document.getElementById("gatewayContent");
  const gatewayKeyInput = document.getElementById("gatewayKeyInput");
  const unlockGatewayBtn = document.getElementById("unlockGatewayBtn");
  const gatewayAuthError = document.getElementById("gatewayAuthError");
  let gatewaySession = "";
  try { gatewaySession = sessionStorage.getItem("val2harness.session") || ""; } catch {}

  const toastContainer = document.getElementById("toastContainer");
  const sessionBadge = document.getElementById("sessionBadge");
  const daemonBadge = document.getElementById("daemonBadge");
  const navLoginBtn = document.getElementById("navLoginBtn");
  const navLogoutBtn = document.getElementById("navLogoutBtn");
  const loginCardBtn = document.getElementById("loginCardBtn");
  const accountName = document.getElementById("accountName");
  const tokenExp = document.getElementById("tokenExp");
  const sessionStatusHeader = document.getElementById("sessionStatusHeader");
  const sessionStatusSub = document.getElementById("sessionStatusSub");
  const modelsCount = document.getElementById("modelsCount");
  const listenAddrInput = document.getElementById("listenAddrInput");
  const copyBookmarkletBtn = document.getElementById("copyBookmarkletBtn");
  const copyEnvBtn = document.getElementById("copyEnvBtn");
  const clearChatBtn = document.getElementById("clearChatBtn");
  const sendChatBtn = document.getElementById("sendChatBtn");
  const chatInput = document.getElementById("chatInput");
  const chatHistory = document.getElementById("chatHistory");
  const exportChatsBtn = document.getElementById("exportChatsBtn");

  // Custom Select DOM
  const selectTriggerBtn = document.getElementById("selectTriggerBtn");
  const selectedModelLabel = document.getElementById("selectedModelLabel");
  const selectDropdownMenu = document.getElementById("selectDropdownMenu");
  const modelSearchInput = document.getElementById("modelSearchInput");
  const selectOptionsList = document.getElementById("selectOptionsList");

  function lockGateway(message = "") {
    gatewaySession = "";
    try { sessionStorage.removeItem("val2harness.session"); } catch {}
    authPanel.hidden = false;
    gatewayContent.hidden = true;
    navLoginBtn.disabled = true;
    navLogoutBtn.disabled = true;
    sessionBadge.textContent = "Gateway locked";
    gatewayAuthError.textContent = message;
  }

  async function apiFetch(path, options = {}) {
    const headers = new Headers(options.headers);
    if (gatewaySession) headers.set("Authorization", "Bearer " + gatewaySession);
    const response = await window.fetch(path, { ...options, headers });
    if (response.status === 401) {
      lockGateway("Your local sign-in expired. Open the dashboard again or enter your connection key.");
      throw new Error("Unlock your local gateway first.");
    }
    return response;
  }

  async function exchangeTicket(ticket) {
    const response = await window.fetch("/api/local/session", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ticket }),
    });
    if (!response.ok) throw new Error("This sign-in link expired or was already used. Open the dashboard again.");
    const data = await response.json();
    if (!data.token) throw new Error("The gateway did not create a local session.");
    gatewaySession = data.token;
    try { sessionStorage.setItem("val2harness.session", data.token); } catch {}
  }

  async function showGateway() {
    authPanel.hidden = true;
    gatewayContent.hidden = false;
    navLoginBtn.disabled = false;
    navLogoutBtn.disabled = false;
    gatewayAuthError.textContent = "";
    await loadStatus();
  }

  unlockGatewayBtn.addEventListener("click", async () => {
    const key = gatewayKeyInput.value.trim();
    gatewayKeyInput.value = "";
    if (!key) { gatewayAuthError.textContent = "Enter your local connection key."; return; }
    unlockGatewayBtn.disabled = true;
    try {
      const response = await window.fetch("/api/local/session-ticket", {
        method: "POST", headers: { Authorization: "Bearer " + key },
      });
      if (!response.ok) throw new Error("That connection key was not accepted.");
      const data = await response.json();
      await exchangeTicket(data.ticket);
      await showGateway();
    } catch (error) { lockGateway(error.message); }
    finally { unlockGatewayBtn.disabled = false; }
  });
  gatewayKeyInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") unlockGatewayBtn.click();
  });

  let allModels = [];
  let currentModel = "anthropic-claude-opus-4-7";

  function showToast({ title, description, variant = "info" }) {
    const toast = document.createElement("div");
    toast.className = `toast toast-${variant}`;
    let iconSvg = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>';
    if (variant === "success") {
      iconSvg = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#059669" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>';
    } else if (variant === "error") {
      iconSvg = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#dc2626" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>';
    }
    const icon = document.createElement("div");
    icon.className = "toast-icon"; icon.innerHTML = iconSvg;
    const content = document.createElement("div"); content.className = "toast-content";
    const heading = document.createElement("div"); heading.className = "toast-title"; heading.textContent = title;
    content.appendChild(heading);
    if (description) { const detail = document.createElement("div"); detail.className = "toast-desc"; detail.textContent = description; content.appendChild(detail); }
    toast.append(icon, content);
    toastContainer.appendChild(toast);
    setTimeout(() => {
      toast.classList.add("toast-exit");
      setTimeout(() => toast.remove(), 250);
    }, 3200);
  }

  // Tabs Switching
  document.querySelectorAll(".tab-trigger").forEach((btn) => {
    btn.addEventListener("click", () => {
      document.querySelectorAll(".tab-trigger").forEach((b) => b.classList.remove("active"));
      document.querySelectorAll(".tab-panel").forEach((p) => p.classList.remove("active"));
      btn.classList.add("active");
      const target = document.getElementById(btn.getAttribute("data-tab"));
      if (target) target.classList.add("active");
    });
  });

  async function loadStatus() {
    try {
      const res = await apiFetch("/api/local/status");
      const data = await res.json();
      listenAddrInput.value = data.listen || "127.0.0.1:8765";

      if (data.token_configured && !data.is_expired) {
        sessionBadge.className = "badge badge-success";
        sessionBadge.innerHTML = '<span class="pulse-dot pulse-active"></span><span>Session Live</span>';
        navLoginBtn.textContent = "Switch Account";
        navLogoutBtn.style.display = "inline-flex";
        accountName.textContent = data.account || "Active Session";
        const expDate = data.token_exp ? new Date(data.token_exp * 1000).toLocaleString() : "Active";
        tokenExp.textContent = "Expires: " + expDate;
        sessionStatusHeader.textContent = "Signed in as " + (data.account || "RMIT User");
        sessionStatusSub.textContent = "Your 24-hour token is active. Valid until " + expDate;
      } else {
        sessionBadge.className = "badge badge-warning";
        const stateText = data.is_expired ? "Token Expired" : "Not Signed In";
        sessionBadge.innerHTML = `<span class="pulse-dot pulse-inactive"></span><span>${stateText}</span>`;
        navLoginBtn.textContent = "Sign In with RMIT SSO";
        navLogoutBtn.style.display = "none";
        accountName.textContent = stateText;
        tokenExp.textContent = "Click Sign In to authenticate via Microsoft";
        sessionStatusHeader.textContent = "Authentication Required";
        sessionStatusSub.textContent = "Click Sign In to authenticate with RMIT Microsoft single sign-on.";
      }

      if (data.daemon_running) {
        daemonBadge.className = "badge badge-success";
        daemonBadge.textContent = "Daemon: PID " + data.daemon_pid;
      } else {
        daemonBadge.className = "badge badge-neutral";
        daemonBadge.textContent = "Daemon: Stopped";
      }

      loadHarnesses();
      loadModels();
      loadEnvironment();
    } catch (e) {
      showToast({ title: "Gateway Offline", description: "Could not reach local server.", variant: "error" });
    }
  }

  async function loadHarnesses() {
    try {
      const res = await apiFetch("/api/local/harnesses");
      const list = await res.json();
      list.forEach((h) => {
        const badge = document.getElementById(`badge-${h.id}`);
        const restoreBtn = document.getElementById(`restore-${h.id}-btn`);
        if (badge) {
          badge.textContent = h.has_backup ? "Backup Ready" : "Original";
          badge.className = h.has_backup ? "badge badge-success" : "badge badge-neutral";
        }
        if (restoreBtn) {
          restoreBtn.disabled = !h.has_backup;
        }
      });
    } catch (e) {}
  }

  ["omp", "pi", "opencode", "continue"].forEach((id) => {
    const syncBtn = document.getElementById(`sync-${id}-btn`);
    const restoreBtn = document.getElementById(`restore-${id}-btn`);
    if (syncBtn) {
      syncBtn.addEventListener("click", async () => {
        syncBtn.textContent = "Syncing...";
        try {
          const res = await apiFetch("/api/local/sync", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ harness: id }),
          });
          const data = await res.json();
          if (res.ok) {
            showToast({ title: `${id.toUpperCase()} Synced`, description: `Synced ${data.count} models to ${data.path}`, variant: "success" });
            loadHarnesses();
          } else {
            showToast({ title: "Sync Failed", description: data.error, variant: "error" });
          }
        } catch (e) {
          showToast({ title: "Network Error", description: e.message, variant: "error" });
        } finally {
          syncBtn.textContent = `Sync ${id.toUpperCase()}`;
        }
      });
    }

    if (restoreBtn) {
      restoreBtn.addEventListener("click", async () => {
        restoreBtn.textContent = "Resetting...";
        try {
          const res = await apiFetch("/api/local/restore", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ harness: id }),
          });
          const data = await res.json();
          if (res.ok) {
            showToast({ title: `${id.toUpperCase()} Restored`, description: `Restored original configuration at ${data.path}`, variant: "success" });
            loadHarnesses();
          } else {
            showToast({ title: "Restore Failed", description: data.error, variant: "error" });
          }
        } catch (e) {
          showToast({ title: "Network Error", description: e.message, variant: "error" });
        } finally {
          restoreBtn.textContent = "Reset to Backup";
        }
      });
    }
  });

  async function loadEnvironment() {
    try {
      const response = await apiFetch("/api/local/env");
      if (!response.ok) throw new Error("Could not load connection settings.");
      document.getElementById("envSnippet").textContent = await response.text();
    } catch (error) { showToast({ title: "Connection Settings", description: error.message, variant: "error" }); }
  }

  // Custom Select Logic
  async function loadModels() {
    try {
      const res = await apiFetch("/v1/models");
      if (!res.ok) return;
      const data = await res.json();
      allModels = data.data || [];
      modelsCount.textContent = allModels.length + " Models";
      renderSelectOptions(allModels);
    } catch (e) {}
  }

  function renderSelectOptions(list) {
    selectOptionsList.innerHTML = "";
    list.forEach((m) => {
      const opt = document.createElement("div");
      opt.className = `select-option ${m.id === currentModel ? "selected" : ""}`;
      const label = document.createElement("span"); label.textContent = m.id; opt.appendChild(label);
      opt.addEventListener("click", () => {
        currentModel = m.id;
        selectedModelLabel.textContent = m.id;
        selectDropdownMenu.style.display = "none";
        renderSelectOptions(allModels);
        showToast({ title: "Model Selected", description: m.id, variant: "info" });
      });
      selectOptionsList.appendChild(opt);
    });
  }

  selectTriggerBtn.addEventListener("click", (e) => {
    e.stopPropagation();
    const isVisible = selectDropdownMenu.style.display === "flex";
    selectDropdownMenu.style.display = isVisible ? "none" : "flex";
    if (!isVisible) {
      modelSearchInput.value = "";
      modelSearchInput.focus();
      renderSelectOptions(allModels);
    }
  });

  modelSearchInput.addEventListener("input", (e) => {
    const q = e.target.value.toLowerCase().trim();
    const filtered = allModels.filter((m) => m.id.toLowerCase().includes(q));
    renderSelectOptions(filtered);
  });

  document.addEventListener("click", (e) => {
    if (!document.getElementById("modelSelectCustom").contains(e.target)) {
      selectDropdownMenu.style.display = "none";
    }
  });

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") selectDropdownMenu.style.display = "none";
  });

  // Auth & SSO
  async function triggerLogin() {
    try {
      const res = await apiFetch("/api/local/login", { method: "POST" });
      const data = await res.json();
      window.open(data.url || "https://val.rmit.edu.au", "_blank");
      showToast({ title: "SSO Opened", description: "Complete sign-in on val.rmit.edu.au", variant: "info" });
    } catch (e) {
      window.open("https://val.rmit.edu.au", "_blank");
    }
  }

  navLoginBtn.addEventListener("click", triggerLogin);
  loginCardBtn.addEventListener("click", triggerLogin);
  navLogoutBtn.addEventListener("click", async () => {
    await apiFetch("/api/local/logout", { method: "POST" });
    showToast({ title: "Logged Out", description: "Session cleared successfully", variant: "info" });
    loadStatus();
  });

  copyBookmarkletBtn.addEventListener("click", async () => {
    try {
      const response = await apiFetch("/api/local/import-ticket", { method: "POST" });
      if (!response.ok) throw new Error("Could not create a token import link.");
      const { ticket } = await response.json();
      const endpoint = window.location.origin + "/api/local/import-token";
      const code = "javascript:(function(){const t=localStorage.getItem('token');if(!t){alert('No Val token found. Sign in to Val first.');return;}fetch(" +
        JSON.stringify(endpoint) + ",{method:'POST',headers:{'Content-Type':'application/json','X-Val-Import':" +
        JSON.stringify(ticket) + "},body:JSON.stringify({token:t})}).then(r=>{if(!r.ok)throw new Error('Import failed or expired');return r.json();}).then(()=>alert('Val token saved locally.')).catch(e=>alert(e.message));})();";
      await navigator.clipboard.writeText(code);
      showToast({ title: "One-use Import Copied", description: "Run it on the Val page within five minutes. Your token stays in your local gateway.", variant: "success" });
    } catch (error) { showToast({ title: "Import Link Failed", description: error.message, variant: "error" }); }
  });

  document.getElementById("saveManualTokenBtn").addEventListener("click", async () => {
    const input = document.getElementById("manualTokenInput");
    const token = input.value.trim(); input.value = "";
    if (!token) return;
    try {
      const response = await apiFetch("/api/local/token", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token }) });
      if (!response.ok) throw new Error("Could not save your Val token.");
      await loadStatus();
      showToast({ title: "Val Token Saved", variant: "success" });
    } catch (error) { showToast({ title: "Token Not Saved", description: error.message, variant: "error" }); }
  });

  copyEnvBtn.addEventListener("click", () => {
    const snippet = document.getElementById("envSnippet").textContent;
    navigator.clipboard.writeText(snippet).then(() => {
      showToast({ title: "Copied to Clipboard", description: "Universal shell environment exported", variant: "success" });
    });
  });

  exportChatsBtn.addEventListener("click", async () => {
    exportChatsBtn.textContent = "Exporting...";
    try {
      const res = await apiFetch("/api/local/export", { method: "POST" });
      const data = await res.json();
      if (!res.ok) throw new Error(`${data.count ?? 0} chats fully saved in ${data.dir || "exports"}; partial files were retained. ${data.error || "Export failed."}`);
      showToast({ title: "Export Complete", description: `Exported ${data.count} chats to ${data.dir}/`, variant: "success" });
    } catch (e) {
      showToast({ title: "Export Failed", description: e.message, variant: "error" });
    } finally {
      exportChatsBtn.textContent = "Export All Chats";
    }
  });

  clearChatBtn.addEventListener("click", () => {
    chatHistory.innerHTML = '<div class="chat-bubble bubble-assistant">Chat history cleared. Send a prompt to start testing.</div>';
    showToast({ title: "Chat Cleared", description: "Playground transcript reset", variant: "info" });
  });

  // Streaming Chat
  sendChatBtn.addEventListener("click", async () => {
    const prompt = chatInput.value.trim();
    if (!prompt) return;

    chatInput.value = "";
    sendChatBtn.disabled = true;

    const userBubble = document.createElement("div");
    userBubble.className = "chat-bubble bubble-user";
    userBubble.textContent = prompt;
    chatHistory.appendChild(userBubble);

    const assistantBubble = document.createElement("div");
    assistantBubble.className = "chat-bubble bubble-assistant";
    assistantBubble.textContent = "● Thinking...";
    chatHistory.appendChild(assistantBubble);
    chatHistory.scrollTop = chatHistory.scrollHeight;

    try {
      const res = await apiFetch("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: currentModel,
          messages: [{ role: "user", content: prompt }],
          stream: true,
        }),
      });

      if (!res.ok) {
        assistantBubble.textContent = "Error: Upstream request failed with status " + res.status;
        showToast({ title: "Request Failed", description: "Status code: " + res.status, variant: "error" });
        return;
      }

      assistantBubble.textContent = "";
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed.startsWith("data:")) continue;
          const payload = trimmed.slice(5).trim();
          if (payload === "[DONE]") break;
          try {
            const parsed = JSON.parse(payload);
            const delta = parsed.choices?.[0]?.delta?.content || "";
            assistantBubble.textContent += delta;
            chatHistory.scrollTop = chatHistory.scrollHeight;
          } catch (e) {}
        }
      }
    } catch (e) {
      assistantBubble.textContent = "Stream error: " + e.message;
      showToast({ title: "Stream Error", description: e.message, variant: "error" });
    } finally {
      sendChatBtn.disabled = false;
    }
  });

  (async () => {
    if (signInTicket) {
      try { await exchangeTicket(signInTicket); await showGateway(); }
      catch (error) { lockGateway(error.message); }
    } else if (gatewaySession) { await showGateway(); }
    else { lockGateway(); }
  })();
});
