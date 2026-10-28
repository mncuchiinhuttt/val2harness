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
  const versionBadgeBtn = document.getElementById("versionBadgeBtn");
  const versionText = document.getElementById("versionText");
  const updateNowBtn = document.getElementById("updateNowBtn");

  // Analytics DOM
  const statTotalReqs = document.getElementById("statTotalReqs");
  const statTotalToks = document.getElementById("statTotalToks");
  const statTokensSub = document.getElementById("statTokensSub");
  const statActiveDays = document.getElementById("statActiveDays");
  const statAvgLat = document.getElementById("statAvgLat");
  const heatmapSubtitle = document.getElementById("heatmapSubtitle");
  const yearButtonsGroup = document.getElementById("yearButtonsGroup");
  const heatmapSvg = document.getElementById("heatmapSvg");
  const updatePortBtn = document.getElementById("updatePortBtn");
  const startDaemonBtn = document.getElementById("startDaemonBtn");
  const stopDaemonBtn = document.getElementById("stopDaemonBtn");
  const manualTokenInput = document.getElementById("manualTokenInput");
  const saveManualTokenBtn = document.getElementById("saveManualTokenBtn");
  const checkQuotaBtn = document.getElementById("checkQuotaBtn");
  const clearObservationsBtn = document.getElementById("clearObservationsBtn");
  const quotaStatusText = document.getElementById("quotaStatusText");
  const quotaMessageText = document.getElementById("quotaMessageText");
  const quotaObservedText = document.getElementById("quotaObservedText");
  const quotaRemainingText = document.getElementById("quotaRemainingText");
  const quotaResetText = document.getElementById("quotaResetText");
  const quotaMeasurementsText = document.getElementById("quotaMeasurementsText");
  const analyticsMeasurementText = document.getElementById("analyticsMeasurementText");
  const analyticsLiveText = document.getElementById("analyticsLiveText");
  let analyticsController = null;
  let analyticsRetry = null;
  let selectedYear = new Date().getFullYear();
  let currentAnalyticsData = null;
  const modelsBreakdownBody = document.getElementById("modelsBreakdownBody");
  const recentLogsBody = document.getElementById("recentLogsBody");

  // Custom Select DOM
  const selectTriggerBtn = document.getElementById("selectTriggerBtn");
  const selectedModelLabel = document.getElementById("selectedModelLabel");
  const selectDropdownMenu = document.getElementById("selectDropdownMenu");
  const modelSearchInput = document.getElementById("modelSearchInput");
  const selectOptionsList = document.getElementById("selectOptionsList");

  function lockGateway(message = "") {
    gatewaySession = "";
    if (analyticsController) analyticsController.abort();
    clearTimeout(analyticsRetry);
    try { sessionStorage.removeItem("val2harness.session"); } catch {}
    authPanel.hidden = false;
    gatewayContent.hidden = true;
    navLoginBtn.disabled = true;
    navLogoutBtn.disabled = true;
    sessionBadge.textContent = "Gateway locked";
    versionBadgeBtn.disabled = true;
    updateNowBtn.disabled = true;
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
    versionBadgeBtn.disabled = false;
    updateNowBtn.disabled = false;
    gatewayAuthError.textContent = "";
    await loadStatus();
    if (gatewaySession) {
      await loadAnalytics();
      initLiveAnalytics();
    }
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
      const res = await apiFetch("/api/local/status");
      if (!res.ok) throw new Error("Could not read gateway status.");
      const data = await res.json();
      listenAddrInput.value = data.listen || "127.0.0.1:8765";
      document.getElementById("localProxy").textContent = data.listen || "Unknown";
      document.getElementById("valUpstream").textContent = data.upstream || "Unknown";

      if (data.token_configured && !data.is_expired) {
        sessionBadge.className = "badge badge-success";
        sessionBadge.innerHTML = '<span class="pulse-dot pulse-active"></span><span>Token configured</span>';
        navLoginBtn.textContent = "Switch Account";
        navLogoutBtn.style.display = "inline-flex";
        accountName.textContent = data.account || "Token configured";
        const expDate = data.token_exp ? new Date(data.token_exp * 1000).toLocaleString() : "Unknown";
        tokenExp.textContent = "Expires: " + expDate;
        sessionStatusHeader.textContent = data.account ? "Token configured for " + data.account : "Val token configured";
        sessionStatusSub.textContent = data.token_exp ? "Token expiry: " + expDate + ". Observed Val status is shown separately below." : "Token expiry is unknown. Observed Val status is shown separately below.";
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
      loadQuota(false);
    } catch (e) {
      showToast({ title: "Gateway Offline", description: "Could not reach local server.", variant: "error" });
    }
  }

  // Real-time Analytics & Heatmap
  const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

  function formatLocalDate(d) {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
  }

  function validCount(value) {
    return Number.isSafeInteger(value) && value >= 0;
  }

  function formatCount(value) {
    return validCount(value) ? value.toLocaleString() : "Unknown";
  }

  function renderYearHeatmap(data, year) {
    if (!heatmapSvg || !data) return;
    const today = new Date();
    const todayStr = formatLocalDate(today);

    const jan1 = new Date(year, 0, 1);
    const sunWeekday = jan1.getDay(); // 0 = Sunday
    let curr = new Date(year, 0, 1 - sunWeekday);

    let totalYearReqs = 0;
    let totalYearToks = 0;

    let svgHtml = "";
    svgHtml += '<text x="12" y="44" fill="#64748b" font-size="9px" font-weight="500" text-anchor="middle">Mon</text>';
    svgHtml += '<text x="12" y="77" fill="#64748b" font-size="9px" font-weight="500" text-anchor="middle">Wed</text>';
    svgHtml += '<text x="12" y="110" fill="#64748b" font-size="9px" font-weight="500" text-anchor="middle">Fri</text>';

    let lastMonth = -1;
    for (let c = 0; c < 53; c++) {
      const colX = 34 + c * 16.5;
      for (let r = 0; r < 7; r++) {
        const rowY = 24 + r * 16.5;
        const dStr = formatLocalDate(curr);
        const mIdx = curr.getMonth();

        if (r === 0 && mIdx !== lastMonth && curr.getFullYear() === year && c < 52) {
          lastMonth = mIdx;
          svgHtml += `<text x="${colX}" y="14" fill="#64748b" font-size="10px" font-weight="500">${monthNames[mIdx]}</text>`;
        }

        const isCurrentYear = curr.getFullYear() === year;
        const isFuture = dStr > todayStr;
        const rawCount = data.daily_records?.[dStr];
        const rawTokens = data.daily_tokens?.[dStr];
        const count = validCount(rawCount) ? rawCount : 0;
        const tokens = validCount(rawTokens) ? rawTokens : 0;

        if (isCurrentYear) {
          totalYearReqs += count;
          totalYearToks += tokens;
        }

        let fill = "#ebedf0";
        let strokeAttr = "";
        if (!isCurrentYear) {
          fill = "transparent";
        } else if (isFuture) {
          fill = "transparent";
          strokeAttr = 'stroke="#e2e8f0" stroke-dasharray="2,2" stroke-width="1"';
        } else if (count >= 20) {
          fill = "#216e39";
        } else if (count >= 10) {
          fill = "#30a14e";
        } else if (count >= 4) {
          fill = "#40c463";
        } else if (count >= 1) {
          fill = "#9be9a8";
        }

        if (isCurrentYear) {
          const tText = tokens.toLocaleString();
          const tooltip = isFuture ? `Future date: ${dStr}` : `${count} observed requests (${tText} measured tokens) on ${dStr}`;
          const isToday = dStr === todayStr;
          svgHtml += `<rect class="heatmap-rect ${isToday && count > 0 ? "pulse-highlight" : ""}" x="${colX}" y="${rowY}" width="13" height="13" rx="2.5" fill="${fill}" ${strokeAttr} data-date="${dStr}" data-count="${count}" data-tokens="${tokens}"><title>${tooltip}</title></rect>`;
        }

        curr.setDate(curr.getDate() + 1);
      }
    }

    heatmapSvg.innerHTML = svgHtml;

    heatmapSvg.querySelectorAll(".heatmap-rect").forEach((rect) => {
      rect.addEventListener("click", () => {
        const d = rect.getAttribute("data-date");
        const c = rect.getAttribute("data-count");
        const t = formatCount(Number(rect.getAttribute("data-tokens")));
        showToast({ title: d, description: `${c} observed requests · ${t} measured tokens`, variant: "info" });
      });
    });

    if (heatmapSubtitle) {
      heatmapSubtitle.textContent = `${totalYearReqs.toLocaleString()} observed requests · ${totalYearToks.toLocaleString()} measured tokens in ${year} · gateway session only`;
    }

    if (yearButtonsGroup && data.available_years) {
      yearButtonsGroup.innerHTML = "";
      data.available_years.forEach((y) => {
        const btn = document.createElement("button");
        btn.className = `year-btn ${y === year ? "active" : ""}`;
        btn.textContent = y;
        btn.addEventListener("click", () => {
          selectedYear = y;
          renderYearHeatmap(data, y);
        });
        yearButtonsGroup.appendChild(btn);
      });
    }
  }

  function renderAnalytics(data, isLive = false) {
    if (!data) return;
    currentAnalyticsData = data;

    statTotalReqs.textContent = formatCount(data.total_requests);
    statTotalToks.textContent = data.measured_requests > 0 ? formatCount(data.total_tokens) : "Unknown";
    statTokensSub.textContent = data.measured_requests > 0
      ? `Measured prompt: ${formatCount(data.prompt_tokens)} · Output: ${formatCount(data.completion_tokens)}`
      : "No measured usage received";
    analyticsMeasurementText.textContent = `${formatCount(data.measured_requests)} measured · ${formatCount(data.unmeasured_requests)} without measured usage`;
    statActiveDays.textContent = `${data.active_days} Days`;
    statAvgLat.textContent = `${data.avg_latency_ms} ms`;

    renderYearHeatmap(data, selectedYear);
    if (modelsBreakdownBody && data.models_breakdown) {
      if (data.models_breakdown.length === 0) {
        modelsBreakdownBody.innerHTML = '<tr><td colspan="4" style="text-align: center; color: var(--text-muted); padding: 1.5rem;">No model requests recorded yet.</td></tr>';
      } else {
        modelsBreakdownBody.innerHTML = "";
        data.models_breakdown.forEach((m) => {
          const tr = document.createElement("tr");
          const known = m.measured_requests > 0;
          appendCells(tr, [m.model, formatCount(m.requests),
            known ? `${formatCount(m.prompt_tokens)} / ${formatCount(m.completion_tokens)}` : "Unknown",
            known ? formatCount(m.total_tokens) : "Unknown"]);
          modelsBreakdownBody.appendChild(tr);
        });
      }
    }

    // Recent Logs with In / Out and live pulse effect
    if (recentLogsBody && data.recent_logs) {
      if (data.recent_logs.length === 0) {
        recentLogsBody.innerHTML = '<tr><td colspan="5" style="text-align: center; color: var(--text-muted); padding: 1.5rem;">No requests logged yet.</td></tr>';
      } else {
        recentLogsBody.innerHTML = "";
        data.recent_logs.forEach((r, idx) => {
          const tr = document.createElement("tr");
          if (isLive && idx === 0) {
            tr.className = "live-incoming-row";
          }
          const timeStr = new Date(r.timestamp * 1000).toLocaleTimeString();
          appendCells(tr, [timeStr, r.model,
            r.usage_known ? `${formatCount(r.prompt_tokens)} in / ${formatCount(r.completion_tokens)} out` : "Unknown",
            r.usage_known ? formatCount(r.total_tokens) : "Unknown", `${formatCount(r.latency_ms)} ms`]);
          recentLogsBody.appendChild(tr);
        });
      }
    }
  }

  function appendCells(row, values) {
    values.forEach((value) => {
      const cell = document.createElement("td");
      cell.textContent = String(value ?? "Unknown");
      row.appendChild(cell);
    });
  }

  async function loadAnalytics() {
    try {
      const response = await apiFetch("/api/local/analytics");
      if (!response.ok) throw new Error("Could not read session observations.");
      renderAnalytics(await response.json());
    } catch (error) {
      analyticsLiveText.textContent = "Observations unavailable";
      if (gatewaySession) showToast({ title: "Usage Status", description: error.message, variant: "error" });
    }
  }

  // Fetch keeps the owner session in a header; EventSource cannot send it.
  async function initLiveAnalytics() {
    if (!gatewaySession || analyticsController) return;
    const controller = new AbortController();
    analyticsController = controller;
    let reader;
    try {
      const response = await apiFetch("/api/local/analytics/live", { signal: controller.signal });
      if (!response.ok || !response.body) throw new Error("Live observations unavailable");
      reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      analyticsLiveText.textContent = "Live observations";
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        if (buffer.length > 1048576) throw new Error("Live observation exceeded the local display limit");
        let end;
        while ((end = buffer.indexOf("\n\n")) >= 0) {
          const event = buffer.slice(0, end);
          buffer = buffer.slice(end + 2);
          const payload = event.split("\n").filter(line => line.startsWith("data:")).map(line => line.slice(5).trimStart()).join("\n");
          if (payload) {
            renderAnalytics(JSON.parse(payload), true);
            loadQuota(false);
          }
        }
      }
    } catch (error) {
      if (!controller.signal.aborted) analyticsLiveText.textContent = "Live connection interrupted · reconnecting";
    } finally {
      if (reader) {
        try { await reader.cancel(); } catch {}
        reader.releaseLock();
      }
      if (analyticsController === controller) analyticsController = null;
      if (gatewaySession && !controller.signal.aborted) analyticsRetry = setTimeout(initLiveAnalytics, 3000);
    }
  }

  window.addEventListener("pagehide", () => {
    clearTimeout(analyticsRetry);
    if (analyticsController) analyticsController.abort();
  });

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

  // Wired Dashboard Settings & Controls
  if (updatePortBtn) {
    updatePortBtn.addEventListener("click", async () => {
      const addr = listenAddrInput.value.trim();
      if (!addr) return;
      try {
        const res = await apiFetch("/api/local/port", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ addr }),
        });
        const data = await res.json();
        if (res.ok) {
          showToast({ title: "Address Saved", description: data.requires_restart ? "Saved " + data.addr + ". Restart the gateway to use this address." : "Saved " + data.addr + ".", variant: "success" });
        } else {
          showToast({ title: "Update Failed", description: data.error, variant: "error" });
        }
      } catch (e) {
        showToast({ title: "Error", description: e.message, variant: "error" });
      }
    });
  }

  if (startDaemonBtn) {
    startDaemonBtn.addEventListener("click", async () => {
      try {
        const res = await apiFetch("/api/local/daemon/start", { method: "POST" });
        const data = await res.json();
        if (res.ok) {
          showToast({ title: "Daemon Started", description: "Running on PID " + data.pid, variant: "success" });
          loadStatus();
        } else {
          showToast({ title: "Start Failed", description: data.error, variant: "error" });
        }
      } catch (e) {
        showToast({ title: "Error", description: e.message, variant: "error" });
      }
    });
  }

  if (stopDaemonBtn) {
    stopDaemonBtn.addEventListener("click", async () => {
      try {
        const res = await apiFetch("/api/local/daemon/stop", { method: "POST" });
        const data = await res.json();
        if (res.ok) {
          showToast({ title: "Daemon Stopped", description: "Server process terminated", variant: "info" });
          loadStatus();
        } else {
          showToast({ title: "Stop Failed", description: data.error, variant: "error" });
        }
      } catch (e) {
        showToast({ title: "Error", description: e.message, variant: "error" });
      }
    });
  }

  async function loadQuota(isManual = false) {
    try {
      const res = await apiFetch("/api/local/quota");
      const data = await res.json();
      if (!res.ok) throw new Error("Could not read gateway observations.");
      const labels = {
        unknown: "Unknown", available: "Available at last observation",
        exhausted: "Budget exhausted at last observation", rate_limited: "Rate limited at last observation",
        authentication_error: "Authentication failed at last observation", provider_error: "Provider error at last observation",
      };
      quotaStatusText.textContent = labels[data.status] ?? "Unknown";
      quotaMessageText.textContent = data.message || "No response has been observed in this gateway session.";
      const observed = data.observed_at ? new Date(data.observed_at) : null;
      quotaObservedText.textContent = observed && !Number.isNaN(observed.getTime()) ? observed.toLocaleString() : "No response observed yet";
      quotaRemainingText.textContent = formatCount(data.remaining);
      quotaResetText.textContent = data.reset_at ? new Date(data.reset_at).toLocaleString() : "Unknown";
      quotaMeasurementsText.textContent = `${formatCount(data.observed_requests)} observed · ${formatCount(data.measured_requests)} measured · ${formatCount(data.unmeasured_requests)} without measured usage`;
      if (isManual) {
        showToast({ title: "Observations Refreshed", description: "Read from gateway memory. No model request was sent.", variant: "info" });
      }
    } catch (e) {
      quotaStatusText.textContent = "Status unavailable";
      quotaMessageText.textContent = "Could not read gateway observations. Refresh when the gateway is available.";
      if (isManual) showToast({ title: "Refresh Failed", description: e.message, variant: "error" });
    }
  }

  if (checkQuotaBtn) {
    checkQuotaBtn.addEventListener("click", () => loadQuota(true));
  }
  clearObservationsBtn.addEventListener("click", async () => {
    clearObservationsBtn.disabled = true;
    try {
      const response = await apiFetch("/api/local/analytics", { method: "DELETE" });
      if (!response.ok) throw new Error("Could not clear gateway observations.");
      await Promise.all([loadQuota(false), loadAnalytics()]);
      showToast({ title: "Observations Cleared", description: "Session measurements were cleared from gateway memory.", variant: "success" });
    } catch (error) {
      showToast({ title: "Clear Failed", description: error.message, variant: "error" });
    } finally { clearObservationsBtn.disabled = false; }
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

  async function checkVersion(isManual = false) {
    try {
      if (isManual) showToast({ title: "Checking Updates", description: "Querying latest release from GitHub...", variant: "info" });
      const res = await apiFetch("/api/local/version");
      const data = await res.json();
      if (!res.ok || data.error) throw new Error("Could not check for a release. Try again later.");
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
      const res = await apiFetch("/api/local/update", { method: "POST" });
      const data = await res.json();
      if (res.ok) {
        showToast({ title: "Update Installed", description: "Installed " + data.version + ". Restart the gateway to use it. Active requests stay on the current version until restart.", variant: "success" });
        updateNowBtn.textContent = "Restart required";
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

  // Initial tab from ?tab=
  const initialSlug = new URLSearchParams(window.location.search).get("tab") || "overview";
  switchTab(idMap[initialSlug] || "tab-overview", false);

});
