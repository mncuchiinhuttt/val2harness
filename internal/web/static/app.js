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
  const versionBadgeBtn = document.getElementById("versionBadgeBtn");
  const versionText = document.getElementById("versionText");
  const updateNowBtn = document.getElementById("updateNowBtn");

  // Analytics DOM
  const statTotalReqs = document.getElementById("statTotalReqs");
  const statTotalToks = document.getElementById("statTotalToks");
  const statTokensSub = document.getElementById("statTokensSub");
  const statActiveDays = document.getElementById("statActiveDays");
  const statAvgLat = document.getElementById("statAvgLat");
  const heatmapMonthsRow = document.getElementById("heatmapMonthsRow");
  const heatmapGrid = document.getElementById("heatmapGrid");
  const modelsBreakdownBody = document.getElementById("modelsBreakdownBody");
  const recentLogsBody = document.getElementById("recentLogsBody");

  // Custom Select DOM
  const selectTriggerBtn = document.getElementById("selectTriggerBtn");
  const selectedModelLabel = document.getElementById("selectedModelLabel");
  const selectDropdownMenu = document.getElementById("selectDropdownMenu");
  const modelSearchInput = document.getElementById("modelSearchInput");
  const selectOptionsList = document.getElementById("selectOptionsList");

  let allModels = [];
  let currentModel = "anthropic-claude-opus-4-7";

  const slugMap = {
    "tab-overview": "overview",
    "tab-analytics": "analytics",
    "tab-harnesses": "harnesses",
    "tab-playground": "playground",
    "tab-exports": "exports",
  };
  const idMap = {
    overview: "tab-overview",
    analytics: "tab-analytics",
    harnesses: "tab-harnesses",
    playground: "tab-playground",
    exports: "tab-exports",
  };

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

  // Tabs Switching with ?tab= Preservation
  function switchTab(tabId, pushHistory = true) {
    document.querySelectorAll(".tab-trigger").forEach((b) => b.classList.remove("active"));
    document.querySelectorAll(".tab-panel").forEach((p) => p.classList.remove("active"));

    const triggerBtn = document.querySelector(`.tab-trigger[data-tab="${tabId}"]`);
    const panel = document.getElementById(tabId);
    if (triggerBtn) triggerBtn.classList.add("active");
    if (panel) panel.classList.add("active");

    const slug = slugMap[tabId] || "overview";
    if (pushHistory) {
      const url = new URL(window.location);
      url.searchParams.set("tab", slug);
      window.history.replaceState(null, "", url);
    }
  }

  document.querySelectorAll(".tab-trigger").forEach((btn) => {
    btn.addEventListener("click", () => {
      switchTab(btn.getAttribute("data-tab"), true);
    });
  });

  window.addEventListener("popstate", () => {
    const slug = new URLSearchParams(window.location.search).get("tab") || "overview";
    switchTab(idMap[slug] || "tab-overview", false);
  });

  // Load Status
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

  // Real-time Analytics & Heatmap
  function renderAnalytics(data, isLive = false) {
    if (!data) return;

    statTotalReqs.textContent = data.total_requests.toLocaleString();
    statTotalToks.textContent = data.total_tokens.toLocaleString();
    statTokensSub.textContent = `Prompt: ${data.prompt_tokens.toLocaleString()} · Output: ${data.completion_tokens.toLocaleString()}`;
    statActiveDays.textContent = `${data.active_days} Days`;
    statAvgLat.textContent = `${data.avg_latency_ms} ms`;

    // Render Authentic GitHub Heatmap (53 Weeks x 7 Days)
    if (heatmapGrid && data.heatmap) {
      heatmapGrid.innerHTML = "";
      heatmapMonthsRow.innerHTML = "";

      let lastMonth = "";
      const weekCols = Math.ceil(data.heatmap.length / 7);

      data.heatmap.forEach((cell, idx) => {
        const colIdx = Math.floor(idx / 7);
        const rowIdx = idx % 7;

        // Render Month Label on first day of week if month changed
        if (rowIdx === 0 && cell.Month && cell.Month !== lastMonth && colIdx < weekCols - 1) {
          lastMonth = cell.Month;
          const mLabel = document.createElement("span");
          mLabel.className = "heatmap-month-label";
          mLabel.style.left = `${colIdx * 15}px`;
          mLabel.textContent = cell.Month;
          heatmapMonthsRow.appendChild(mLabel);
        }

        const div = document.createElement("div");
        div.className = `heatmap-cell ${cell.is_future ? "is-future" : ""}`;
        div.setAttribute("data-level", cell.level);

        if (!cell.is_future) {
          const tText = cell.tokens ? cell.tokens.toLocaleString() : "0";
          div.title = `${cell.date}: ${cell.count} requests (${tText} tokens)`;
          div.addEventListener("click", () => {
            showToast({ title: cell.date, description: `${cell.count} requests · ${tText} tokens`, variant: "info" });
          });
          // Pulse today's cell on live query
          if (isLive && idx === data.heatmap.length - 1) {
            div.classList.add("pulse-highlight");
          }
        }
        heatmapGrid.appendChild(div);
      });
    }

    // Models Breakdown
    if (modelsBreakdownBody && data.models_breakdown) {
      if (data.models_breakdown.length === 0) {
        modelsBreakdownBody.innerHTML = '<tr><td colspan="3" style="text-align: center; color: var(--text-muted); padding: 1.5rem;">No model requests recorded yet.</td></tr>';
      } else {
        modelsBreakdownBody.innerHTML = "";
        data.models_breakdown.forEach((m) => {
          const tr = document.createElement("tr");
          tr.innerHTML = `<td><span style="font-weight: 600;">${m.model}</span></td><td>${m.requests.toLocaleString()}</td><td>${m.total_tokens.toLocaleString()}</td>`;
          modelsBreakdownBody.appendChild(tr);
        });
      }
    }

    // Recent Logs
    if (recentLogsBody && data.recent_logs) {
      if (data.recent_logs.length === 0) {
        recentLogsBody.innerHTML = '<tr><td colspan="4" style="text-align: center; color: var(--text-muted); padding: 1.5rem;">No requests logged yet.</td></tr>';
      } else {
        recentLogsBody.innerHTML = "";
        data.recent_logs.forEach((r, idx) => {
          const tr = document.createElement("tr");
          if (isLive && idx === 0) {
            tr.className = "new-row-highlight";
          }
          const timeStr = new Date(r.timestamp * 1000).toLocaleTimeString();
          tr.innerHTML = `<td style="color: var(--text-muted);">${timeStr}</td><td>${r.model}</td><td>${r.total_tokens.toLocaleString()}</td><td>${r.latency_ms}ms</td>`;
          recentLogsBody.appendChild(tr);
        });
      }
    }
  }

  // Live SSE Connection for Real-time Analytics
  function initLiveAnalytics() {
    try {
      const sse = new EventSource("/api/local/analytics/live");
      sse.onmessage = (e) => {
        try {
          const data = JSON.parse(e.data);
          renderAnalytics(data, true);
        } catch (err) {}
      };
      sse.onerror = () => {
        sse.close();
        setTimeout(initLiveAnalytics, 3000);
      };
    } catch (e) {
      setTimeout(initLiveAnalytics, 3000);
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

  async function checkVersion(isManual = false) {
    try {
      if (isManual) showToast({ title: "Checking Updates", description: "Querying latest release from GitHub...", variant: "info" });
      const res = await fetch("/api/local/version");
      const data = await res.json();
      if (versionText && data.current_version) {
        versionText.textContent = data.current_version;
      }
      if (data.has_update) {
        versionBadgeBtn.className = "badge badge-warning";
        versionBadgeBtn.title = "Update available: " + data.latest_version;
        updateNowBtn.style.display = "inline-flex";
        updateNowBtn.textContent = "Update to " + data.latest_version;
        if (isManual) {
          showToast({ title: "Update Available", description: "Version " + data.latest_version + " is available.", variant: "info" });
        }
      } else {
        versionBadgeBtn.className = "badge badge-neutral";
        updateNowBtn.style.display = "none";
        if (isManual) {
          showToast({ title: "Up to Date", description: "Running latest release (" + data.current_version + ")", variant: "success" });
        }
      }
    } catch (e) {
      if (isManual) showToast({ title: "Check Failed", description: e.message, variant: "error" });
    }
  }

  versionBadgeBtn.addEventListener("click", () => checkVersion(true));
  updateNowBtn.addEventListener("click", async () => {
    updateNowBtn.disabled = true;
    updateNowBtn.textContent = "Updating...";
    showToast({ title: "Downloading Update", description: "Fetching and replacing binary...", variant: "info" });
    try {
      const res = await fetch("/api/local/update", { method: "POST" });
      const data = await res.json();
      if (res.ok) {
        showToast({ title: "Update Successful", description: "Updated to " + data.version + ". Reloading page...", variant: "success" });
        setTimeout(() => window.location.reload(), 2000);
      } else {
        showToast({ title: "Update Failed", description: data.error, variant: "error" });
        updateNowBtn.disabled = false;
        updateNowBtn.textContent = "Retry Update";
      }
    } catch (e) {
      showToast({ title: "Update Error", description: e.message, variant: "error" });
      updateNowBtn.disabled = false;
      updateNowBtn.textContent = "Retry Update";
    }
  });

  checkVersion(false);

  // Initial tab from ?tab=
  const initialSlug = new URLSearchParams(window.location.search).get("tab") || "overview";
  switchTab(idMap[initialSlug] || "tab-overview", false);

  loadStatus();
  initLiveAnalytics();
});
