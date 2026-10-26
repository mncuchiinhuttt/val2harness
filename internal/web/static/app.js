document.addEventListener("DOMContentLoaded", () => {
  const sessionBadge = document.getElementById("sessionBadge");
  const sessionBadgeText = document.getElementById("sessionBadgeText");
  const daemonBadge = document.getElementById("daemonBadge");
  const navLoginBtn = document.getElementById("navLoginBtn");
  const navLogoutBtn = document.getElementById("navLogoutBtn");
  const loginCardBtn = document.getElementById("loginCardBtn");
  const accountName = document.getElementById("accountName");
  const tokenExp = document.getElementById("tokenExp");
  const sessionStatusHeader = document.getElementById("sessionStatusHeader");
  const sessionStatusSub = document.getElementById("sessionStatusSub");
  const modelsCount = document.getElementById("modelsCount");
  const modelSelect = document.getElementById("modelSelect");
  const syncAllBtn = document.getElementById("syncAllBtn");
  const syncOmpBtn = document.getElementById("syncOmpBtn");
  const exportChatsBtn = document.getElementById("exportChatsBtn");
  const syncStatus = document.getElementById("syncStatus");
  const copyEnvBtn = document.getElementById("copyEnvBtn");
  const bookmarkletBtn = document.getElementById("bookmarkletBtn");
  const manualTokenInput = document.getElementById("manualTokenInput");
  const saveManualTokenBtn = document.getElementById("saveManualTokenBtn");
  const listenAddrInput = document.getElementById("listenAddrInput");
  const sendChatBtn = document.getElementById("sendChatBtn");
  const chatInput = document.getElementById("chatInput");
  const chatHistory = document.getElementById("chatHistory");

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

      loadModels();
    } catch (e) {
      sessionBadge.className = "badge badge-warning";
      sessionBadgeText.textContent = "Server Offline";
    }
  }

  async function loadModels() {
    try {
      const res = await fetch("/v1/models");
      if (!res.ok) return;
      const data = await res.json();
      const list = data.data || [];
      modelsCount.textContent = list.length + " Models";

      modelSelect.innerHTML = "";
      list.forEach((m) => {
        const opt = document.createElement("option");
        opt.value = m.id;
        opt.textContent = m.id;
        if (m.id.includes("opus-4-7")) opt.selected = true;
        modelSelect.appendChild(opt);
      });
    } catch (e) {}
  }

  async function triggerLogin() {
    try {
      const res = await fetch("/api/local/login", { method: "POST" });
      const data = await res.json();
      window.open(data.url || "https://val.rmit.edu.au", "_blank");
      syncStatus.textContent = "Login page opened. Complete sign-in on val.rmit.edu.au, then use 1-Click Bookmarklet or paste token.";
    } catch (e) {
      window.open("https://val.rmit.edu.au", "_blank");
    }
  }

  async function triggerLogout() {
    await fetch("/api/local/logout", { method: "POST" });
    loadStatus();
  }

  navLoginBtn.addEventListener("click", triggerLogin);
  loginCardBtn.addEventListener("click", triggerLogin);
  navLogoutBtn.addEventListener("click", triggerLogout);

  saveManualTokenBtn.addEventListener("click", async () => {
    const token = manualTokenInput.value.trim();
    if (!token) return;
    await fetch("/api/local/token", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
    manualTokenInput.value = "";
    loadStatus();
  });

  syncAllBtn.addEventListener("click", async () => {
    syncAllBtn.textContent = "Syncing...";
    try {
      const res = await fetch("/api/local/sync-all", { method: "POST" });
      const data = await res.json();
      if (res.ok && data.results) {
        syncStatus.textContent = data.results.map((r) => `${r.Harness.split(" ")[0]}: ${r.Error ? "Failed" : "OK (" + r.Count + ")"}`).join(" · ");
      }
    } catch (e) {
      syncStatus.textContent = "Sync error: " + e.message;
    } finally {
      syncAllBtn.textContent = "Sync All Harnesses";
    }
  });

  syncOmpBtn.addEventListener("click", async () => {
    syncOmpBtn.textContent = "Syncing...";
    try {
      const res = await fetch("/api/local/sync-omp", { method: "POST" });
      const data = await res.json();
      syncStatus.textContent = `Synced ${data.count} models to ${data.path}`;
    } catch (e) {
      syncStatus.textContent = "OMP sync error: " + e.message;
    } finally {
      syncOmpBtn.textContent = "Sync OMP Only";
    }
  });

  exportChatsBtn.addEventListener("click", async () => {
    exportChatsBtn.textContent = "Exporting...";
    try {
      const res = await fetch("/api/local/export", { method: "POST" });
      const data = await res.json();
      syncStatus.textContent = `Exported ${data.count} chats to ${data.dir}/`;
    } catch (e) {
      syncStatus.textContent = "Export error: " + e.message;
    } finally {
      exportChatsBtn.textContent = "Export Chat History";
    }
  });

  copyEnvBtn.addEventListener("click", () => {
    const snippet = document.getElementById("envSnippet").textContent;
    navigator.clipboard.writeText(snippet).then(() => {
      copyEnvBtn.textContent = "Copied!";
      setTimeout(() => (copyEnvBtn.textContent = "Copy Shell Export"), 2000);
    });
  });

  bookmarkletBtn.addEventListener("click", () => {
    const host = window.location.origin;
    const code = `javascript:(function(){const t=localStorage.getItem("token");if(!t){alert("No token found in localStorage");return;}fetch("${host}/api/local/token",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({token:t})}).then(r=>r.json()).then(d=>{alert("Token synced successfully to val2harness!");}).catch(e=>{alert("Failed: "+e);});})();`;
    navigator.clipboard.writeText(code).then(() => {
      bookmarkletBtn.textContent = "Bookmarklet Copied!";
      setTimeout(() => (bookmarkletBtn.textContent = "Copy 1-Click Bookmarklet"), 2000);
    });
  });

  sendChatBtn.addEventListener("click", async () => {
    const model = modelSelect.value;
    const prompt = chatInput.value.trim();
    if (!model || !prompt) return;

    chatInput.value = "";
    sendChatBtn.disabled = true;

    // User message bubble
    const userBubble = document.createElement("div");
    userBubble.className = "chat-bubble bubble-user";
    userBubble.textContent = prompt;
    chatHistory.appendChild(userBubble);

    // Assistant streaming bubble
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
          model: model,
          messages: [{ role: "user", content: prompt }],
          stream: true,
        }),
      });

      if (!res.ok) {
        assistantBubble.textContent = "Error: Upstream request failed with status " + res.status;
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
    } finally {
      sendChatBtn.disabled = false;
    }
  });

  loadStatus();
});
