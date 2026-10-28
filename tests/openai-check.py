"""Official OpenAI SDK checks. Offline by default; --live spends only synthetic quota."""
import argparse
import base64
import binascii
from contextlib import contextmanager
import json
import os
from pathlib import Path
import socket
import struct
import subprocess
import tempfile
import threading
import time
import urllib.request
import zlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from live_probe_support import LiveRecorder


class ProbeCheckFailed(Exception):
    def __init__(self, stage):
        self.stage = stage


def check(condition, stage):
    if not condition:
        raise ProbeCheckFailed(stage)


def red_image():
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", binascii.crc32(kind + data) & 0xffffffff)
    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 32, 32, 8, 2, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress((b"\x00" + b"\xff\x00\x00" * 32) * 32)) + chunk(b"IEND", b"")
    return "data:image/png;base64," + base64.b64encode(png).decode()


@contextmanager
def gateway(binary, recorder, directory):
    home = Path(directory)
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    env = {k: v for k, v in os.environ.items() if k.upper() in {
        "PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP", "OS"}}
    env.update(HOME=str(home), USERPROFILE=str(home), VAL_RMIT_TOKEN=recorder.key,
               VAL_RMIT_UPSTREAM=recorder.url, VAL_RMIT_ADDR=f"127.0.0.1:{port}")
    base = f"http://127.0.0.1:{port}"
    with (home / "gateway.log").open("w") as log:
        child = subprocess.Popen([str(binary.resolve()), "serve"], cwd=home, env=env, stdout=log, stderr=log,
            creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
    try:
        config = home / ".config/val2harness/config.json"
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(100):
            if child.poll() is not None:
                raise RuntimeError("Isolated gateway stopped before inference")
            if config.exists():
                try:
                    with opener.open(base + "/healthz", timeout=.2):
                        break
                except OSError:
                    pass
            time.sleep(.05)
        else:
            raise RuntimeError("Isolated gateway did not become ready")
        private = json.loads(config.read_text())
        assert private.get("token") != recorder._token, "Real upstream credential persisted in child"
        yield base, private["api_key"]
    finally:
        child.terminate()
        try:
            child.wait(timeout=10)
        except subprocess.TimeoutExpired:
            child.kill()
            child.wait()


def completion(client, options, stream):
    if not stream:
        reply = client.chat.completions.create(**options)
        assert len(reply.choices) == 1
        choice = reply.choices[0]
        return choice.message.model_dump(exclude_none=True), choice.finish_reason, reply.usage
    text, calls, finish, usage = "", {}, None, None
    with client.chat.completions.create(**options, stream=True, stream_options={"include_usage": True}) as events:
        for event in events:
            if event.usage is not None:
                usage = event.usage
            for choice in event.choices:
                check(choice.index == 0, "choice-index")
                if choice.finish_reason is not None:
                    finish = choice.finish_reason
                delta = choice.delta
                text += delta.content or ""
                for fragment in delta.tool_calls or []:
                    call = calls.setdefault(fragment.index, {"id": "", "type": "function", "function": {"name": "", "arguments": ""}})
                    if fragment.id:
                        check(not call["id"] or call["id"] == fragment.id, "tool-id-changed")
                        call["id"] = fragment.id
                    if fragment.function:
                        call["function"]["name"] += fragment.function.name or ""
                        call["function"]["arguments"] += fragment.function.arguments or ""
    message = {"role": "assistant", "content": text or None}
    if calls:
        message["tool_calls"] = [calls[i] for i in sorted(calls)]
    return message, finish, usage


def tool_loop(client, model, stream, live, first_turn_only=False):
    tools = [{"type": "function", "function": {"name": "compat_echo", "description": "Report a synthetic value; no files or network.",
        "parameters": {"type": "object", "properties": {"value": {"type": "string"}}, "required": ["value"], "additionalProperties": False}}}]
    content = "Call compat_echo once with value compat-ok."
    expected = "compat-ok"
    if stream:
        content = [{"type": "text", "text": "Report the dominant image color by calling compat_echo with that color. After receiving the tool result, reply with exactly compat-ok."},
                   {"type": "image_url", "image_url": {"url": red_image()}}]
        expected = "red"
    messages = [{"role": "user", "content": content}]
    options = {"model": model, "max_tokens": 128, "messages": messages, "tools": tools,
               "tool_choice": "auto" if stream else "required"}
    if not live:
        options["extra_body"] = {"future_option": {"exact": 9007199254740993}}
    assistant, finish, usage = completion(client, options, stream)
    calls = assistant.get("tool_calls", [])
    shape = {"one_call": len(calls) == 1, "tool_finish": finish == "tool_calls",
             "usage_present": usage is not None}
    if len(calls) == 1:
        function = calls[0].get("function", {})
        try:
            arguments = json.loads(function.get("arguments", ""))
        except (ValueError, TypeError):
            arguments = None
        value = arguments.get("value") if isinstance(arguments, dict) else None
        shape.update(id_present=bool(calls[0].get("id")), expected_name=function.get("name") == "compat_echo",
                     arguments_object=isinstance(arguments, dict), exact_expected_arguments=arguments == {"value": expected},
                     color_class="exact_red" if value == "red" else "casefold_red" if isinstance(value, str) and value.strip().lower() == "red" else "other")
    print("SDK_SHAPE", json.dumps(shape, sort_keys=True), flush=True)
    check(finish == "tool_calls" and len(calls) == 1, "first-turn-shape")
    call = calls[0]
    check(call["id"] and call["function"]["name"] == "compat_echo", "first-turn-name-or-id")
    arguments_match = shape.get("exact_expected_arguments")
    if stream and isinstance(arguments, dict) and set(arguments) == {"value"}:
        arguments_match = isinstance(value, str) and value.strip().lower() == "red"
    check(arguments_match, "first-turn-arguments")
    check(usage and usage.prompt_tokens > 0 and usage.completion_tokens > 0, "first-turn-usage")
    if first_turn_only:
        print("PASS: first turn only; tool-result loop is not verified", flush=True)
        return
    messages += [assistant, {"role": "tool", "tool_call_id": call["id"], "content": "compat-ok"}]
    options["tool_choice"] = "auto"
    final, finish, final_usage = completion(client, options, stream)
    assert finish == "stop" and not final.get("tool_calls") and "compat-ok" in (final.get("content") or "")
    assert final_usage and final_usage.prompt_tokens > 0 and final_usage.completion_tokens > 0
    print("PASS", json.dumps({"stream": stream, "image_assisted": stream, "tool_result_loop": True,
        "usage": [[usage.prompt_tokens, usage.completion_tokens], [final_usage.prompt_tokens, final_usage.completion_tokens]]}), flush=True)


@contextmanager
def fake_upstream():
    requests = []
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            body = json.dumps({"data": [{"id": "synthetic-openai", "object": "model", "created": 0, "owned_by": "fixture"}]}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        def do_POST(self):
            request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            requests.append(request)
            tools = request["tools"]
            assert request["tool_servers"] == [{"specs": [tool["function"] for tool in tools]}]
            assert request["future_option"]["exact"] == 9007199254740993
            assert not any(key in request for key in ("tool_ids", "chat_id", "session_id"))
            final = any(m["role"] == "tool" and m["tool_call_id"] == "call_fixture" and m["content"] == "compat-ok" for m in request["messages"])
            image = isinstance(request["messages"][0]["content"], list)
            if image:
                assert request["messages"][0]["content"][1]["image_url"]["url"] == red_image()
            arguments = json.dumps({"value": "red" if image else "compat-ok"})
            call = {"id": "call_fixture", "type": "function", "function": {"name": "compat_echo", "arguments": arguments}}
            finish = "stop" if final else "tool_calls"
            usage = {"prompt_tokens": 19, "completion_tokens": 7, "total_tokens": 26}
            common = {"id": "synthetic-completion", "created": 0, "model": "synthetic-openai"}
            if request.get("stream"):
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.end_headers()
                def emit(delta, reason=None, measured=None):
                    payload = {**common, "object": "chat.completion.chunk", "choices": [{"index": 0, "delta": delta, "finish_reason": reason}]}
                    if measured:
                        payload["usage"] = measured
                    self.wfile.write(("data:" + json.dumps(payload) + "\n\n").encode())
                    self.wfile.flush()
                if final:
                    emit({"role": "assistant", "content": "Acknowledged compat-ok"})
                else:
                    emit({"role": "assistant", "tool_calls": [{**call, "index": 0, "function": {"name": "compat_", "arguments": arguments[:5]}}]})
                    emit({"tool_calls": [{"index": 0, "function": {"name": "echo", "arguments": arguments[5:]}}]})
                emit({}, finish)
                emit({"role": "assistant", "content": ""})
                emit({}, measured=usage)
                self.wfile.write(b"data:[DONE]\n\n")
            else:
                message = {"role": "assistant", "content": "Acknowledged compat-ok" if final else None}
                if not final:
                    message["tool_calls"] = [call]
                body = json.dumps({**common, "object": "chat.completion", "choices": [{"index": 0, "message": message, "finish_reason": finish}], "usage": usage}).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
        def log_message(self, *args):
            pass
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", requests
    finally:
        server.shutdown()
        server.server_close()


def run(binary, upstream, token, model, budget, live, first_turn_only=False):
    import openai
    emit = lambda entry: print("UPSTREAM", json.dumps(entry, sort_keys=True), flush=True)
    with LiveRecorder(upstream, token, budget, emit, max_output_tokens=128) as recorder, \
            tempfile.TemporaryDirectory(prefix="val2harness-openai-") as directory, \
            gateway(binary, recorder, directory) as (base, key):
        with openai.OpenAI(base_url=base + "/v1", api_key=key, max_retries=0, timeout=60,
                http_client=openai.DefaultHttpxClient(trust_env=False), _strict_response_validation=True) as client:
            assert model in [item.id for item in client.models.list()]
            print("OpenAI SDK", openai.__version__, flush=True)
            for stream in ([True] if live else [False, True]):
                tool_loop(client, model, stream, live, first_turn_only)
        print("INFERENCE_REQUESTS", recorder.inference_requests, flush=True)
        assert recorder.inference_requests == (1 if first_turn_only else 2 if live else 4)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--live", action="store_true", help="Read configured credentials and spend exactly two authorized synthetic attempts")
    parser.add_argument("--max-requests", type=int, help="Explicit live budget; must be exactly 2")
    parser.add_argument("--first-turn-only", action="store_true", help="Diagnose a single live tool turn without a result request")
    parser.add_argument("--model", default="anthropic-claude-sonnet-4-6")
    args = parser.parse_args()
    if args.live:
        required = 1 if args.first_turn_only else 2
        if args.max_requests != required:
            parser.error("Live OpenAI check requires the explicitly authorized exact request budget")
        # No local credentials are touched until explicit live opt-in and budget.
        home = Path(os.environ.get("USERPROFILE") or os.environ["HOME"])
        path = home / ".config/val2harness/config.json"
        config = json.loads(path.read_text()) if path.exists() else {}
        token = os.environ.get("VAL_RMIT_TOKEN") or config.get("token")
        upstream = os.environ.get("VAL_RMIT_UPSTREAM") or config.get("upstream_url") or "https://val.rmit.edu.au"
        if not token:
            raise SystemExit("No locally configured token; connect through the dashboard, never chat.")
        run(args.binary, upstream, token, args.model, required, True, args.first_turn_only)
    else:
        with fake_upstream() as (upstream, requests):
            run(args.binary, upstream, "synthetic-openai-token", "synthetic-openai", 4, False)
            assert len(requests) == 4


def guarded_main():
    try:
        main()
    except Exception as error:
        # SDK exceptions can contain arbitrary upstream bodies. Structural
        # diagnostics above are sufficient; never print that body/traceback.
        failure = {"failure": type(error).__name__}
        if isinstance(error, ProbeCheckFailed):
            failure["check"] = error.stage
        status = getattr(error, "status_code", None)
        if type(status) is int:
            failure["http_status"] = status
        print("FAIL", json.dumps(failure), flush=True)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(guarded_main())
