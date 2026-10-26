document.addEventListener("DOMContentLoaded", () => {
  const statusBadge = document.getElementById("statusBadge");
  const modelsCount = document.getElementById("modelsCount");
  const tokenExp = document.getElementById("tokenExp");
  const tokenInput = document.getElementById("tokenInput");
  const saveTokenBtn = document.getElementById("saveTokenBtn");
  const bookmarkletBtn = document.getElementById("bookmarkletBtn");
  const syncOmpBtn = document.getElementById("syncOmpBtn");
  const exportChatsBtn = document.getElementById("exportChatsBtn");
  const ompStatus = document.getElementById("ompStatus");
  const modelSelect = document.getElementById("modelSelect");
  const sendChatBtn = document.getElementById("sendChatBtn");
  const chatInput = document.getElementById("chatInput");
  const chatOutput = document.getElementById("chatOutput");

  async function loadStatus() {
    try {
      const res = await fetch("/api/local/status");
      const data = await res.json();
      if (data.token_configured) {
        statusBadge.textContent = "Connected";
        statusBadge.className = "status-badge";
        if (data.token_exp) {
          tokenExp.textContent = new Date(data.token_exp * 1000).toLocaleString();
        } else {
          tokenExp.textContent = "Active";
        }
      } else {
        statusBadge.textContent = "Token Missing";
        statusBadge.className = "status-badge error";
        tokenExp.textContent = "Not configured";
      }
      loadModels();
    } catch (e) {
      statusBadge.textContent = "Offline";
      statusBadge.className = "status-badge error";
    }
  }

  async function loadModels() {
    try {
      const res = await fetch("/v1/models");
      if (!res.ok) {
        modelsCount.textContent = "Error loading";
        return;
      }
      const data = await res.json();
      const list = data.data || [];
      modelsCount.textContent = list.length + " models";

      modelSelect.innerHTML = "";
      list.forEach((m) => {
        const opt = document.createElement("option");
        opt.value = m.id;
        opt.textContent = m.id;
        if (m.id.includes("opus-4-7")) {
          opt.selected = true;
        }
        modelSelect.appendChild(opt);
      });
    } catch (e) {
      modelsCount.textContent = "Unavailable";
    }
  }

  saveTokenBtn.addEventListener("click", async () => {
    const token = tokenInput.value.trim();
    if (!token) return;
    try {
      saveTokenBtn.textContent = "Saving...";
      const res = await fetch("/api/local/token", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
      });
      if (res.ok) {
        tokenInput.value = "";
        saveTokenBtn.textContent = "Saved";
        setTimeout(() => (saveTokenBtn.textContent = "Update Token"), 2000);
        loadStatus();
      } else {
        alert("Failed to save token");
        saveTokenBtn.textContent = "Update Token";
      }
    } catch (e) {
      alert("Error: " + e.message);
      saveTokenBtn.textContent = "Update Token";
    }
  });

  bookmarkletBtn.addEventListener("click", () => {
    const host = window.location.origin;
    const code = `javascript:(function(){const t=localStorage.getItem("token");if(!t){alert("No token found in localStorage");return;}fetch("${host}/api/local/token",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({token:t})}).then(r=>r.json()).then(d=>{alert("Token synced successfully to val2harness!");}).catch(e=>{alert("Failed to send token: "+e);});})();`;
    navigator.clipboard.writeText(code).then(() => {
      bookmarkletBtn.textContent = "Copied to Clipboard";
      setTimeout(() => (bookmarkletBtn.textContent = "Copy 1-Click Bookmarklet"), 2000);
    });
  });

  syncOmpBtn.addEventListener("click", async () => {
    try {
      syncOmpBtn.textContent = "Syncing...";
      const res = await fetch("/api/local/sync-omp", { method: "POST" });
      const data = await res.json();
      if (res.ok) {
        ompStatus.textContent = `Synced ${data.count} models to ${data.path}`;
      } else {
        ompStatus.textContent = `Error: ${data.error}`;
      }
    } catch (e) {
      ompStatus.textContent = `Error: ${e.message}`;
    } finally {
      syncOmpBtn.textContent = "Sync Models to ~/.omp/models.yml";
    }
  });

  exportChatsBtn.addEventListener("click", async () => {
    try {
      exportChatsBtn.textContent = "Exporting...";
      const res = await fetch("/api/local/export", { method: "POST" });
      const data = await res.json();
      if (res.ok) {
        ompStatus.textContent = `Exported ${data.count} chats to ${data.dir}`;
      } else {
        ompStatus.textContent = `Export failed: ${data.error}`;
      }
    } catch (e) {
      ompStatus.textContent = `Export error: ${e.message}`;
    } finally {
      exportChatsBtn.textContent = "Export Chat History";
    }
  });

  sendChatBtn.addEventListener("click", async () => {
    const model = modelSelect.value;
    const prompt = chatInput.value.trim();
    if (!model || !prompt) return;

    chatOutput.textContent = "";
    sendChatBtn.disabled = true;
    sendChatBtn.textContent = "Streaming...";

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
        const err = await res.text();
        chatOutput.textContent = "Error: " + err;
        return;
      }

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
          if (!trimmed || !trimmed.startsWith("data:")) continue;
          const payload = trimmed.slice(5).trim();
          if (payload === "[DONE]") break;
          try {
            const parsed = JSON.parse(payload);
            const delta = parsed.choices?.[0]?.delta?.content || "";
            chatOutput.textContent += delta;
            chatOutput.scrollTop = chatOutput.scrollHeight;
          } catch (e) {}
        }
      }
    } catch (e) {
      chatOutput.textContent = "Stream error: " + e.message;
    } finally {
      sendChatBtn.disabled = false;
      sendChatBtn.textContent = "Stream Test";
    }
  });

  loadStatus();
});
