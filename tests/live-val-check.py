"""Explicit opt-in, bounded live compatibility check; never paste tokens here.

Uses the local Val configuration only with --live, sends synthetic data, and
starts an isolated gateway without persisting the Val token. Requires the
official anthropic SDK in the invoking Python environment.
"""
import argparse
import base64
import binascii
import json
import os
from pathlib import Path
import socket
import struct
import subprocess
import secrets
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import zlib

from live_probe_support import LiveRecorder


def prepare_claude_fixture(home, gateway_url, owner_key, *, edit=False):
    """Create fixed synthetic inputs; never accept a caller's file or contents."""
    fixture = home.resolve() / "cli-fixture"
    fixture.mkdir(mode=0o700)
    metadata = {"url": gateway_url, "key": owner_key}
    if edit:
        marker = "synthetic-edit-" + secrets.token_hex(16)
        initial = "before: " + marker + "\n"
        metadata.update(root=str(fixture), fixture_kind="synthetic-edit-v1", fixture_marker=marker)
    else:
        initial = "synthetic-bridge-" + secrets.token_hex(16)
    (fixture / "fixture.txt").write_bytes(initial.encode("utf-8"))
    (fixture / "gateway.json").write_text(json.dumps(metadata), encoding="utf-8")
    return fixture


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--live", action="store_true", help="Permit configured account access and synthetic Val quota use")
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--model", help="An available tool/vision-capable Val model ID")
    parser.add_argument("--checks", choices=("all", "tools", "vision", "text", "claude", "claude-edit"), default="all")
    parser.add_argument("--claude-binary", type=Path, help="Official Claude Code executable for the scoped Read or synthetic Edit check")
    parser.add_argument("--tool-choice", choices=("any", "specific", "auto"), default="any",
                        help="First synthetic tool turn: required, named, or automatic selection")
    parser.add_argument("--max-requests", type=int, choices=range(1, 11), default=3,
                        help="Hard upstream inference-request limit; use only the authorized budget")
    parser.add_argument("--max-output-tokens", type=int, choices=(128, 512), default=128,
                        help="Authorized cap; 512 is restricted to the synthetic Edit check")
    parser.add_argument("--completion-path", choices=("/api/chat/completions", "/openai/chat/completions"),
                        default="/api/chat/completions", help="Explicit remote route for diagnosis")
    parser.add_argument("--function-calling", choices=("default", "legacy", "native"),
                        help="Explicit request-scoped Open WebUI mode for diagnosis")
    parser.add_argument("--client-tool-specs", action="store_true",
                        help="Register URL-free client schemas through Open WebUI's client-tool path")
    parser.add_argument("--stream", action="store_true", help="Use official SDK stream accumulation")
    parser.add_argument("--image-tool", action="store_true", help="Tool input must describe the generated image color")
    args = parser.parse_args()
    if not args.live:
        parser.error("Account access is disabled. Explicit --live consent is required.")
    if args.max_output_tokens != 128 and args.checks != "claude-edit":
        parser.error("A 512-token cap is restricted to the explicitly authorized synthetic Edit check")
    if args.checks in ("claude", "claude-edit") and not args.claude_binary:
        parser.error("--claude-binary is required for the Claude Code check")
    if args.checks == "claude-edit" and args.max_requests != 3:
        parser.error("The synthetic Claude Code edit check requires exactly three authorized attempts; no retries.")
    import anthropic
    import httpx2

    original_home = Path(os.environ.get("USERPROFILE") or os.environ["HOME"])
    config_path = original_home / ".config/val2harness/config.json"
    config = json.loads(config_path.read_text()) if config_path.exists() else {}
    token = os.environ.get("VAL_RMIT_TOKEN") or config.get("token")
    upstream = os.environ.get("VAL_RMIT_UPSTREAM") or config.get("upstream_url") or "https://val.rmit.edu.au"
    parsed = urllib.parse.urlsplit(upstream)
    if parsed.scheme not in ("http", "https") or not parsed.hostname or parsed.username is not None or parsed.password is not None or parsed.query or parsed.fragment:
        raise SystemExit("Configured Val URL must be HTTP(S), without credentials/query/fragment.")
    if not token:
        raise SystemExit("No local Val token is configured. Connect through the dashboard; do not paste a token into chat.")
    emit = lambda entry: print("UPSTREAM", json.dumps(entry, sort_keys=True), flush=True)
    with LiveRecorder(upstream, token, args.max_requests, emit, args.completion_path,
                      args.function_calling, args.client_tool_specs, max_output_tokens=args.max_output_tokens) as recorder, \
            tempfile.TemporaryDirectory(prefix="val2harness-live-") as directory:
        home = Path(directory)
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        base = f"http://127.0.0.1:{port}"
        env = {k: v for k, v in os.environ.items() if k.upper() in {
            "PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP", "OS"}}
        # The child knows only a disposable loopback capability. The real Val
        # credential remains in the recorder, which authorizes the remote hop.
        env.update(HOME=str(home), USERPROFILE=str(home), VAL_RMIT_TOKEN=recorder.key,
                   VAL_RMIT_UPSTREAM=recorder.url, VAL_RMIT_ADDR=f"127.0.0.1:{port}")
        with (home / "gateway.log").open("w") as log:
            child = subprocess.Popen([str(args.binary.resolve()), "serve"], cwd=home, env=env,
                                     stdout=log, stderr=log,
                                     creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
        try:
            local_config = home / ".config/val2harness/config.json"
            for _ in range(100):
                if child.poll() is not None:
                    raise SystemExit("Isolated gateway failed to start; no Val request was attempted.")
                if local_config.exists():
                    try:
                        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
                        with opener.open(base + "/healthz", timeout=.2):
                            break
                    except (OSError, urllib.error.URLError):
                        pass
                time.sleep(.05)
            private = json.loads(local_config.read_text())
            assert private.get("token") != token, "Val token unexpectedly persisted"
            key = private["api_key"]
            with httpx2.Client(trust_env=False, verify=False, timeout=45) as http:
                response = http.get(base + "/v1/models", headers={"Authorization": "Bearer " + key})
                response.raise_for_status()
                ids = [item["id"] for item in response.json()["data"]]
                model = args.model or next((m for m in ids if "sonnet" in m.lower()), None)
                if model not in ids:
                    raise SystemExit("Choose an available tool/vision model using --model; none was selected automatically.")
                print("MODEL", json.dumps({"id": model, "available_count": len(ids)}), flush=True)
                if args.checks in ("claude", "claude-edit"):
                    if model != "anthropic-claude-sonnet-4-6":
                        raise SystemExit("The scoped Claude Code fixture targets Sonnet 4.6")
                    edit = args.checks == "claude-edit"
                    fixture = prepare_claude_fixture(home, base, key, edit=edit)
                    check = Path(__file__).resolve().with_name("claude-code-check.py")
                    result = subprocess.run([sys.executable, "-u", str(check), str(fixture),
                                             str(args.claude_binary.resolve()), "--edit-fixture" if edit else "--read-only"],
                                            env=env, timeout=105)
                    print("INFERENCE_REQUESTS", recorder.inference_requests, flush=True)
                    if result.returncode:
                        raise SystemExit("Scoped live Claude Code check failed")
                    if edit and recorder.inference_requests != 3:
                        raise SystemExit("Synthetic Edit did not complete exactly three inference turns")
                    return
                client = anthropic.Anthropic(base_url=base, api_key=key, max_retries=0,
                    timeout=45, http_client=anthropic.DefaultHttpxClient(trust_env=False, verify=False),
                    _strict_response_validation=True)
                def create_message(**options):
                    if args.stream:
                        with client.messages.stream(**options) as stream:
                            return stream.get_final_message()
                    return client.messages.create(**options)
                try:
                    tools = [{"name":"compat_echo", "description":"Echo a fixed synthetic marker; no files or network.",
                              "input_schema":{"type":"object","properties":{"value":{"type":"string","enum":["compat-ok"]}},
                                              "required":["value"],"additionalProperties":False}}]
                    def chunk(kind, value):
                        return struct.pack(">I", len(value)) + kind + value + struct.pack(">I", binascii.crc32(kind + value) & 0xffffffff)
                    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 32,32,8,2,0,0,0))
                    png += chunk(b"IDAT", zlib.compress((b"\x00" + b"\xff\x00\x00"*32)*32)) + chunk(b"IEND", b"")
                    failures = []
                    checks = ("tools", "vision") if args.checks == "all" else (args.checks,)
                    for check in checks:
                        if recorder.inference_requests >= args.max_requests:
                            failures.append(check)
                            print("SKIP:", check, "authorized inference budget exhausted", flush=True)
                            continue
                        print("CHECK:", check, flush=True)
                        try:
                            if check == "tools":
                                messages = [{"role":"user","content":"Call compat_echo once with value compat-ok."}]
                                expected = {"value": "compat-ok"}
                                if args.image_tool:
                                    tools[0]["input_schema"]["properties"]["value"] = {"type": "string"}
                                    tools[0]["description"] = "Report the dominant image color. No files or network."
                                    messages = [{"role":"user", "content":[
                                        {"type":"text", "text":"Inspect this image and call compat_echo once with its dominant color as value. After receiving the tool result, include that exact result marker in your final answer."},
                                        {"type":"image", "source":{"type":"base64", "media_type":"image/png", "data":base64.b64encode(png).decode()}}]}]
                                    expected = {"value": "red"}
                                choice = {"type": "tool", "name": "compat_echo"} if args.tool_choice == "specific" else {"type": args.tool_choice}
                                first = create_message(model=model, max_tokens=128, messages=messages,
                                                               tools=tools, tool_choice=choice)
                                calls = [b for b in first.content if b.type == "tool_use"]
                                assert len(calls) == 1 and calls[0].name == "compat_echo" and calls[0].input == expected, "Tool selection or arguments did not match the fixture"
                                if recorder.inference_requests >= args.max_requests:
                                    raise AssertionError("Budget exhausted before the result turn")
                                messages += [{"role":"assistant","content":[b.model_dump(exclude_none=True) for b in first.content]},
                                             {"role":"user","content":[{"type":"tool_result","tool_use_id":calls[0].id,"content":"compat-ok"}]}]
                                final = create_message(model=model, max_tokens=64, messages=messages, tools=tools)
                                assert final.stop_reason == "end_turn" and "compat-ok" in " ".join(b.text for b in final.content if b.type == "text"), "Tool result was not acknowledged"
                            elif check == "vision":
                                vision = create_message(model=model, max_tokens=32, messages=[{"role":"user","content":[
                                    {"type":"text","text":"Name the dominant color in this synthetic image."},
                                    {"type":"image","source":{"type":"base64","media_type":"image/png","data":base64.b64encode(png).decode()}}]}])
                                assert "red" in " ".join(b.text.lower() for b in vision.content if b.type == "text"), "Image interpretation did not match the fixture"
                            else:
                                reply = create_message(model=model, max_tokens=32,
                                    messages=[{"role":"user", "content":"Reply exactly compat-ok."}])
                                assert reply.stop_reason == "end_turn" and "compat-ok" in " ".join(b.text for b in reply.content if b.type == "text"), "Text check did not acknowledge the marker"
                            print("PASS:", check, flush=True)
                            if check == "tools" and args.image_tool:
                                print("PASS: image interpretation through client tool arguments", flush=True)
                        except anthropic.APIStatusError as error:
                            failures.append(check)
                            print(f"FAIL: {check} HTTP {error.status_code}; see redacted structural diagnostics", flush=True)
                            body = error.body if isinstance(error.body, dict) else {}
                            problem = body.get("error")
                            message = problem.get("message") if isinstance(problem, dict) else None
                            if isinstance(message, str):
                                for credential in (token, recorder.key, key):
                                    message = message.replace(credential, "[redacted]")
                                print("GATEWAY_ERROR", message[:512], flush=True)
                            if error.status_code in (401, 403):
                                break
                        except AssertionError as error:
                            failures.append(check)
                            print(f"FAIL: {check}: {error}", flush=True)
                    print("INFERENCE_REQUESTS", recorder.inference_requests, flush=True)
                    if failures:
                        raise SystemExit("Live checks incomplete: " + ", ".join(failures))
                finally:
                    client.close()
        finally:
            if child.poll() is None:
                child.terminate()
                try:
                    child.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait()


if __name__ == "__main__":
    main()
