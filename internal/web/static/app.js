document.addEventListener("DOMContentLoaded", () => {
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
    toast.innerHTML = `<div class="toast-icon">${iconSvg}</div><div class="toast-content"><div class="toast-title">${title}</div>${description ? `<div class="toast-desc">${description}</div>` : "" }</div>`;
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
      const res = await fetch("/api/local/status");
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
    } catch (e) {
      showToast({ title: "Gateway Offline", description: "Could not reach local server.", variant: "error" });
    }
  }

  async function loadHarnesses() {
    try {
      const res = await fetch("/api/local/harnesses");
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
          const res = await fetch("/api/local/sync", {
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
          const res = await fetch("/api/local/restore", {
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

  // Custom Select Logic
  async function loadModels() {
    try {
      const res = await fetch("/v1/models");
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
      opt.innerHTML = `<span>${m.id}</span>${m.id === currentModel ? '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>' : ""}`;
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
      const res = await fetch("/api/local/login", { method: "POST" });
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
    await fetch("/api/local/logout", { method: "POST" });
    showToast({ title: "Logged Out", description: "Session cleared successfully", variant: "info" });
    loadStatus();
  });

  copyBookmarkletBtn.addEventListener("click", () => {
    const host = window.location.origin;
    const code = `javascript:(function(){const t=localStorage.getItem("token");if(!t){alert("No token found in localStorage");return;}fetch("${host}/api/local/token",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({token:t})}).then(r=>r.json()).then(d=>{alert("Token synced successfully to val2harness!");}).catch(e=>{alert("Failed: "+e);});})();`;
    navigator.clipboard.writeText(code).then(() => {
      showToast({ title: "Bookmarklet Copied", description: "Drag/click on val.rmit.edu.au tab to sync token", variant: "success" });
    });
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
      const res = await fetch("/api/local/export", { method: "POST" });
      const data = await res.json();
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
      const res = await fetch("/v1/chat/completions", {
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

  loadStatus();
});
