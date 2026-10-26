# val2harness

High-performance, lightweight bridge connecting **RMIT Val AI** (`https://val.rmit.edu.au`) to **OMP** and OpenAI-compatible agent harnesses (Cursor, OpenCode, Aider, Continue).

Written in **Go** with zero runtime dependencies, single binary (<10MB), ultra-low memory usage (<15MB RAM), and native SSE streaming.

---

## Features

- **Standard OpenAI API Gateway**: Exposes `http://127.0.0.1:8765/v1/chat/completions` and `/v1/models` with full Server-Sent Events (SSE) streaming support.
- **Access to 53 RMIT Val Models**: Including **Claude Opus 4.7**, **Claude Opus 5**, Claude Sonnet 4.6/5, GPT-5/6 series, Llama 4, and Mistral.
- **Interactive TUI**: Easy command-line menu for daemon management, terminal chat, port changing, and status checks.
- **Background Daemon**: Start, stop, and inspect the proxy server with zero terminal lock-in (`val2harness start / stop / status`).
- **Embedded Web Dashboard**: Visit `http://127.0.0.1:8765/` to test streaming, view available models, and manage authentication.
- **1-Click OMP Sync**: Safely syncs all 53 models into `~/.omp/models.yml` without affecting existing providers.
- **Direct Terminal Chat**: Chat with Opus 4.7 or Opus 5 directly in your terminal without opening a browser.
- **Chat History Exporter**: Export conversations from RMIT Val into clean Markdown and JSON.

---

## Quick Start

### 1. Build Single Binary

```bash
go build -o val2harness .
```

### 2. Launch Interactive TUI

Simply run:

```bash
./val2harness
```

You will see:

```text
+===========================================================+
| val2harness - RMIT Val AI Bridge                          |
| Status: [RUNNING (PID 2051)]  Port: 127.0.0.1:8765        |
+===========================================================+
  1. Start Server (Background Daemon)
  2. Stop Server
  3. Check Server Status & Health
  4. Open Web Dashboard in Browser
  5. Quick Chat in Terminal (Test Opus 4.7/5)
  6. Sync Models to ~/.omp/models.yml
  7. View Available Models
  8. Change Listen Port
  9. Update RMIT Val Token
 10. Export Chat History to Markdown
  0. Exit
```

---

## CLI Commands

You can also run commands directly from scripts or shell:

```bash
# Daemon Management
./val2harness start         # Starts server in background on 127.0.0.1:8765
./val2harness status        # Displays running PID and health
./val2harness stop          # Gracefully stops background server

# Interactive & Chat
./val2harness chat          # Starts interactive chat with Claude Opus 4.7
./val2harness chat <model>  # e.g. ./val2harness chat anthropic-claude-opus-5

# Integration & Models
./val2harness sync-omp      # Syncs models directly to ~/.omp/models.yml
./val2harness models        # Lists all 53 models available on RMIT Val
./val2harness export        # Exports chats to ./exports/*.md and .json

# Foreground server
./val2harness serve -addr 127.0.0.1:8765
```

---

## OMP Configuration (`~/.omp/models.yml`)

Running `./val2harness sync-omp` registers the provider automatically:

```yaml
providers:
  rmit-val:
    baseUrl: http://127.0.0.1:8765/v1
    apiKey: rmit-val-key
    api: openai-completions
    models:
      - id: anthropic-claude-opus-4-7
        name: Anthropic Claude Opus 4.7
        contextWindow: 200000
        maxTokens: 128000
      - id: anthropic-claude-opus-5
        name: Anthropic Claude Opus 5
        contextWindow: 200000
        maxTokens: 128000
      # ... 53 models
```

---

## Architecture

```text
[OMP / Cursor / CLI] 
       |
       | HTTP / SSE (OpenAI format)
       v
[val2harness Proxy: 127.0.0.1:8765]
       |
       | Bearer JWT (Open WebUI API)
       v
[https://val.rmit.edu.au (AWS Bedrock)]
```
