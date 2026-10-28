"""Isolated Claude Code checks; file changes require a fixed synthetic fixture."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import urlsplit

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("root", type=Path)
parser.add_argument("binary", type=Path)
mode = parser.add_mutually_exclusive_group()
mode.add_argument("--read-only", action="store_true")
mode.add_argument("--edit-fixture", action="store_true",
                  help="Read and replace only the fixed parent-created synthetic fixture")
options = parser.parse_args()
root, binary = options.root.resolve(), options.binary.resolve()
read_only, edit_fixture = options.read_only, options.edit_fixture
gateway = json.loads((root / "gateway.json").read_text())
if edit_fixture:
    # The parent creates only this shape. Arbitrary file paths, content and
    # endpoints cannot opt into the CLI's full tool mode.
    marker = gateway.get("fixture_marker")
    try:
        endpoint = urlsplit(gateway.get("url", ""))
        valid_endpoint = (endpoint.scheme == "http" and endpoint.hostname in ("127.0.0.1", "localhost", "::1")
                          and endpoint.username is None and endpoint.password is None
                          and not endpoint.path and not endpoint.query and not endpoint.fragment
                          and endpoint.port is not None)
    except (ValueError, TypeError):
        valid_endpoint = False
    if (gateway.get("root") != str(root) or gateway.get("fixture_kind") != "synthetic-edit-v1"
            or not isinstance(marker, str) or re.fullmatch(r"synthetic-edit-[0-9a-f]{32}", marker) is None
            or not isinstance(gateway.get("key"), str) or not gateway["key"] or not valid_endpoint):
        raise SystemExit("Edit mode requires parent-created synthetic metadata and a loopback HTTP gateway.")
    fixture = root / "fixture.txt"
    expected_initial = ("before: " + marker + "\n").encode("utf-8")
    if fixture.is_symlink() or not fixture.is_file() or fixture.read_bytes() != expected_initial:
        raise SystemExit("Edit mode accepts only fixture.txt with the exact initial synthetic bytes.")
elif not read_only:
    # --bare enables Claude Code's minimal mode, which omits Write. Full tool
    # mode is restricted to the known disposable Go fixture, never the live
    # helper's random local key. The home/settings/MCP isolation still applies.
    endpoint = urlsplit(gateway["url"])
    if (gateway.get("key") != "synthetic-interop-owner" or gateway.get("root") != str(root)
            or endpoint.scheme != "http" or endpoint.hostname not in ("127.0.0.1", "localhost", "::1")
            or endpoint.username is not None or endpoint.password is not None
            or endpoint.path not in ("", "/") or endpoint.query or endpoint.fragment):
        raise SystemExit("Read/Write mode requires the synthetic localhost Go fixture; use --read-only for live checks.")
    marker = "fixture-token-519"
else:
    marker = (root / "fixture.txt").read_text().strip()
home = root / "cli-home"
home.mkdir(exist_ok=True)
env = {k: v for k, v in os.environ.items() if k.upper() in {
    "PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP",
    "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS", "OS", "PROGRAMFILES"}}
env.update({
    "HOME": str(home), "USERPROFILE": str(home), "APPDATA": str(home / "appdata"),
    "LOCALAPPDATA": str(home / "localappdata"), "CLAUDE_CONFIG_DIR": str(home / ".claude"),
    "ANTHROPIC_BASE_URL": gateway["url"], "ANTHROPIC_API_KEY": gateway["key"],
    "ANTHROPIC_MODEL": "anthropic-claude-sonnet-4-6", "MAX_THINKING_TOKENS": "0",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1", "DISABLE_TELEMETRY": "1",
    "DISABLE_ERROR_REPORTING": "1", "CLAUDE_CODE_SKIP_PROMPT_HISTORY": "1",
    "CLAUDE_CODE_DISABLE_AUTO_MEMORY": "1", "NO_PROXY": "127.0.0.1,localhost",
    "CLAUDE_CODE_MAX_RETRIES": "0", "CLAUDE_CODE_NONSTREAMING_TIMEOUT_RETRIES": "0",
    "API_TIMEOUT_MS": "60000",
})
bash = Path(os.environ.get("ProgramFiles", "C:/Program Files")) / "Git/bin/bash.exe"
if bash.exists():
    env["CLAUDE_CODE_GIT_BASH_PATH"] = str(bash)
version = subprocess.run([str(binary), "--version"], cwd=root, env=env,
                         text=True, capture_output=True, timeout=20)
print("Claude Code version:", version.stdout.strip(), flush=True)
if edit_fixture:
    prompt = ("Use Read to read fixture.txt. Then call Edit with file_path=fixture.txt, old_string=before:, new_string=after:. "
              "Preserve the synthetic marker and newline exactly. Reply with only the updated line. Do not access other paths.")
elif read_only:
    prompt = "Read exactly " + str(root / "fixture.txt") + " using Read, then reply with its exact contents only. Do not access other paths."
else:
    prompt = "Read fixture.txt, write its exact contents to result.txt, then report completion."
# Claude Code matches file modifications with Edit(path), including Write.
# Write(path) is accepted by the CLI but never matches a file permission.
permissions = ["Read(./fixture.txt)"]
if not read_only:
    permissions.append("Edit(./fixture.txt)" if edit_fixture else "Edit(./result.txt)")
system = "You are testing a local gateway. " + (
    "Only Read fixture.txt, then Edit before: to after: in that file. Preserve the marker and reply with only the updated line."
    if edit_fixture else "Only read fixture.txt, then reply with its exact contents. Do not write files."
    if read_only else "Only read fixture.txt and write its contents to result.txt, then report completion.")
args = [str(binary), *(["--bare"] if read_only else []), "-p", prompt,
        "--model", "anthropic-claude-sonnet-4-6", "--tools", "Read,Edit" if edit_fixture else "Read" if read_only else "Read,Write", "--allowedTools", *permissions,
        "--permission-mode", "dontAsk", "--max-turns", "4", "--output-format", "json",
        "--no-session-persistence", "--setting-sources", "", "--strict-mcp-config",
        "--mcp-config", '{"mcpServers":{}}', "--settings", '{"alwaysThinkingEnabled":false}',
        "--system-prompt", system]
try:
    run = subprocess.run(args, cwd=root, env=env, text=True, encoding="utf-8",
                         capture_output=True, timeout=90)
except subprocess.TimeoutExpired:
    raise SystemExit("Claude Code exceeded the 90-second synthetic-check deadline; no automatic rerun.") from None
(root / "claude-stdout.json").write_text(run.stdout, encoding="utf-8")
(root / "claude-stderr.txt").write_text(run.stderr, encoding="utf-8")
print("Claude Code exit:", run.returncode, flush=True)
if run.returncode:
    print(run.stdout[-6000:])
    print(run.stderr[-3000:])
    raise SystemExit(run.returncode)
result = json.loads(run.stdout)
assert marker in result.get("result", ""), "Claude Code did not return the fixture marker"
if edit_fixture:
    assert result.get("num_turns", 0) >= 3, "Claude Code did not complete both tool-result turns"
    assert result.get("permission_denials") == [], "Claude Code reported a permission denial"
    assert (root / "fixture.txt").read_bytes() == ("after: " + marker + "\n").encode("utf-8"), "Edit did not preserve the exact synthetic fixture bytes"
    # Exact bytes, completed tool turns and the returned random marker prove
    # the edit. Extra prose/Markdown in the final reply is not a protocol fault.
    print("PASS: real Claude Code scoped Read/Edit execution, tool-result turns, exact file bytes and final marker", flush=True)
    raise SystemExit(0)
if read_only:
    assert result.get("num_turns", 0) >= 2, "Claude Code did not complete the tool-result turn"
    print("PASS: real Claude Code scoped Read execution, tool result, and final marker", flush=True)
    raise SystemExit(0)
assert (root / "result.txt").read_text().strip() == marker, "Write tool did not save the fixture"
requests = json.loads((root / "requests.json").read_text())
assert any(m.get("role") == "tool" and "fixture-token-519" in str(m.get("content"))
           for req in requests if req.get("model") == "anthropic-claude-sonnet-4-6"
           for m in req.get("messages", [])), "No Claude Code Read result reached fake Val"
print("PASS: real Claude Code Read/Write execution, tool-result returns, and final response", flush=True)
