"""Bounded, loopback-only recorder for explicitly authorized synthetic checks.

The real upstream credential stays in this process. Only response structure and
fixed diagnostic categories leave it; arbitrary response text/arguments do not.
"""
import hmac
import hashlib
import json
import re
import secrets
import threading
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def response_summary(status, content_type, body):
    summary = {"http_status": status, "response_bytes": len(body),
               "json": False, "event_stream": content_type.startswith("text/event-stream")}
    if summary["event_stream"]:
        frames, done = [], False
        wire = body.decode("utf-8", errors="replace").replace("\r\n", "\n").replace("\r", "\n")
        for frame in wire.split("\n\n"):
            data = "\n".join(line[5:].removeprefix(" ") for line in frame.split("\n") if line.startswith("data:"))
            if data == "[DONE]":
                done = True
                continue
            if not data or len(frames) >= 40:
                continue
            try:
                chunk = json.loads(data)
            except ValueError:
                frames.append({"json": False})
                continue
            if not isinstance(chunk, dict):
                frames.append({"json_object": False})
                continue
            info = {"known_fields": [key for key in ("id", "model", "choices", "usage", "error", "type", "sources") if key in chunk]}
            if isinstance(chunk.get("usage"), dict):
                info["usage"] = response_summary(status, "application/json", json.dumps({"usage": chunk["usage"]}).encode()).get("usage", {})
            choices = chunk.get("choices")
            if isinstance(choices, list):
                info["choice_count"] = len(choices)
                if choices and isinstance(choices[0], dict):
                    choice = choices[0]
                    finish = choice.get("finish_reason")
                    info["finish_reason"] = finish if finish in (None, "stop", "length", "tool_calls", "content_filter") else "[unknown]"
                    delta = choice.get("delta")
                    if isinstance(delta, dict):
                        info["delta_fields"] = [key for key in ("role", "content", "tool_calls", "refusal", "reasoning_content", "function_call") if key in delta]
                        calls = delta.get("tool_calls")
                        if isinstance(calls, list):
                            info["tool_fragment_count"] = len(calls)
            frames.append(info)
        summary["sse_frames"], summary["sse_done"] = frames, done
        return summary
    try:
        data = json.loads(body)
    except (ValueError, UnicodeError):
        return summary
    if not isinstance(data, dict):
        return summary
    summary["json"] = True
    summary["known_fields"] = [key for key in ("choices", "usage", "error", "detail") if key in data]
    if status >= 400 or "error" in data:
        detail = json.dumps(data).lower()
        topics = ("ValidationException", "AccessDenied", "Unauthorized", "rate limit",
                  "tool_choice", "parallel_tool_calls", "tool", "image", "base64",
                  "max_tokens", "unsupported", "invalid", "permission", "content filter",
                  "not found", "not in", "must be", "defined", "toolConfig", "toolChoice",
                  "input_schema", "function")
        summary["error_topics"] = [topic for topic in topics if topic.lower() in detail]
    usage = data.get("usage")
    if isinstance(usage, dict):
        summary["usage"] = {key: usage[key] for key in (
            "prompt_tokens", "completion_tokens", "total_tokens",
            "cache_read_input_tokens", "cache_creation_input_tokens")
            if type(usage.get(key)) is int and usage[key] >= 0}
    choices = data.get("choices")
    if isinstance(choices, list):
        summary["choice_count"] = len(choices)
        summary["choices"] = []
        for choice in choices[:1]:
            if not isinstance(choice, dict):
                continue
            finish = choice.get("finish_reason")
            item = {"finish_reason": finish if finish in (
                "stop", "length", "tool_calls", "content_filter", None) else "[unknown]"}
            message = choice.get("message")
            if isinstance(message, dict):
                text = message.get("content")
                item["describes_generated_red_image"] = isinstance(text, str) and bool(re.search(r"\bred\b", text, re.I))
                calls = message.get("tool_calls")
                if isinstance(calls, list):
                    item["tool_count"] = len(calls)
                    item["tools"] = []
                    for call in calls[:128]:
                        if not isinstance(call, dict):
                            continue
                        function = call.get("function")
                        function = function if isinstance(function, dict) else {}
                        name = function.get("name")
                        raw = function.get("arguments")
                        kind = "string" if isinstance(raw, str) else "object" if isinstance(raw, dict) else "other"
                        arguments = raw
                        if isinstance(raw, str):
                            try:
                                arguments = json.loads(raw)
                            except ValueError:
                                arguments = None
                        keys = list(arguments) if isinstance(arguments, dict) else []
                        item["tools"].append({
                            "name": name if isinstance(name, str) and re.fullmatch(r"[A-Za-z0-9_-]{1,128}", name) else "[invalid-name]",
                            "function_type": call.get("type") == "function",
                            "id_present": isinstance(call.get("id"), str) and bool(call["id"]),
                            "argument_kind": kind, "argument_object": isinstance(arguments, dict),
                            "argument_key_count": len(keys),
                            "known_argument_keys": [key for key in ("value", "query", "tool_calls", "parameters", "input", "code") if key in keys],
                            "expected_synthetic_call": name == "compat_echo" and arguments == {"value": "compat-ok"},
                        })
            summary["choices"].append(item)
    return summary


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def request_summary(body):
    try:
        data = json.loads(body)
    except (ValueError, TypeError, UnicodeError):
        return {"json_object": False}
    if not isinstance(data, dict):
        return {"json_object": False}
    def safe_name(value):
        return value if isinstance(value, str) and re.fullmatch(r"[A-Za-z0-9_-]{1,128}", value) else "[invalid-name]"
    tools = data.get("tools")
    names = []
    if isinstance(tools, list):
        for tool in tools[:128]:
            function = tool.get("function") if isinstance(tool, dict) else None
            names.append(safe_name(function.get("name")) if isinstance(function, dict) else "[invalid-tool]")
    choice = data.get("tool_choice")
    selected = None
    if isinstance(choice, dict):
        function = choice.get("function")
        selected = safe_name(function.get("name")) if isinstance(function, dict) else "[invalid-tool]"
        choice = "specific"
    if choice not in (None, "none", "auto", "required", "specific"):
        choice = "[unknown]"
    params = data.get("params")
    mode = params.get("function_calling") if isinstance(params, dict) else None
    servers = data.get("tool_servers")
    client_names = []
    network_config = False
    if isinstance(servers, list):
        for server in servers[:128]:
            if not isinstance(server, dict):
                continue
            network_config |= any(key in server for key in ("url", "headers", "key", "auth_type"))
            specs = server.get("specs")
            if isinstance(specs, list):
                client_names.extend(safe_name(spec.get("name")) for spec in specs[:128] if isinstance(spec, dict))
    return {"tools_key_present": "tools" in data, "tool_names": names,
            "tool_choice": choice, "selected_tool": selected,
            "function_calling": mode if mode in (None, "native", "default", "legacy") else "[unknown]",
            "client_spec_names": client_names, "client_specs_have_network_config": network_config,
            "max_tokens": data.get("max_tokens") if type(data.get("max_tokens")) is int else None,
            "chat_id_present": bool(data.get("chat_id")), "session_id_present": bool(data.get("session_id"))}


class LiveRecorder:
    def __init__(self, upstream, token, max_requests=3, emit=lambda entry: None,
                 completion_path="/api/chat/completions", function_calling=None, client_tool_specs=False,
                 max_output_tokens=None):
        parsed = urllib.parse.urlsplit(upstream)
        if (parsed.scheme not in ("http", "https") or not parsed.hostname or parsed.username is not None
                or parsed.password is not None or parsed.query or parsed.fragment or not token):
            raise ValueError("Invalid upstream URL or empty credential")
        if type(max_requests) is not int or not 1 <= max_requests <= 10:
            raise ValueError("Inference budget must be between 1 and 10")
        if completion_path not in ("/api/chat/completions", "/openai/chat/completions"):
            raise ValueError("Unknown completion route")
        self.completion_path = completion_path
        if function_calling not in (None, "default", "legacy", "native"):
            raise ValueError("Unknown function calling mode")
        self.function_calling = function_calling
        self.client_tool_specs = bool(client_tool_specs)
        if max_output_tokens is not None and (type(max_output_tokens) is not int or not 1 <= max_output_tokens <= 512):
            raise ValueError("Output cap must be between 1 and 512")
        self.max_output_tokens = max_output_tokens
        self.upstream, self._token = upstream.rstrip("/"), token
        self.max_requests, self.emit = max_requests, emit
        self.key = secrets.token_urlsafe(32)
        self.inference_requests = 0
        self.model_requests = 0
        self.records = []
        self._inference_fingerprints = set()
        self._lock = threading.Lock()

    def __enter__(self):
        recorder = self
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                recorder._forward(self)
            def do_POST(self):
                recorder._forward(self)
            def log_message(self, *args):
                pass
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        return self

    def __exit__(self, *args):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)

    def _forward(self, handler):
        def reply(status, body=b"{}", content_type="application/json", retry_after=None):
            handler.send_response(status)
            handler.send_header("Content-Type", content_type)
            handler.send_header("Content-Length", str(len(body)))
            if retry_after is not None:
                handler.send_header("Retry-After", retry_after)
            handler.end_headers()
            try:
                handler.wfile.write(body)
            except (BrokenPipeError, ConnectionResetError):
                pass
        authorization = handler.headers.get("Authorization", "")
        if not hmac.compare_digest(authorization, "Bearer " + self.key):
            reply(401)
            return
        inference = handler.command == "POST" and handler.path == "/api/chat/completions"
        models = handler.command == "GET" and handler.path == "/api/models"
        if not inference and not models:
            reply(404)
            return
        try:
            length = int(handler.headers.get("Content-Length", "0"))
        except ValueError:
            reply(400)
            return
        if length < 0 or length > 8 << 20:
            reply(413)
            return
        body = handler.rfile.read(length) if length else None
        if inference and (self.function_calling is not None or self.client_tool_specs or self.max_output_tokens is not None):
            try:
                payload = json.loads(body)
                if not isinstance(payload, dict) or not isinstance(payload.get("params", {}), dict):
                    raise ValueError("Invalid request")
                if self.function_calling is not None:
                    payload.setdefault("params", {})["function_calling"] = self.function_calling
                if self.max_output_tokens is not None:
                    limit = payload.get("max_tokens", self.max_output_tokens)
                    if type(limit) is not int or limit <= 0:
                        raise ValueError("Invalid output token limit")
                    payload["max_tokens"] = min(limit, self.max_output_tokens)
                if self.client_tool_specs:
                    specs = []
                    tools = payload.get("tools", [])
                    if not isinstance(tools, list) or len(tools) > 128:
                        raise ValueError("Invalid tool registry")
                    for tool in tools:
                        if not isinstance(tool, dict) or tool.get("type") != "function" or not isinstance(tool.get("function"), dict):
                            raise ValueError("Invalid client function")
                        specs.append(tool["function"])
                    # Flat client-owned schemas only: no endpoint, credentials,
                    # chat/session identity or server-execution configuration.
                    if specs:
                        payload["tool_servers"] = [{"specs": specs}]
                body = json.dumps(payload).encode()
            except (ValueError, TypeError):
                reply(400)
                return
        fingerprint = None
        if inference:
            try:
                normalized = json.dumps(json.loads(body), sort_keys=True, separators=(",", ":"))
                fingerprint = hashlib.sha256(normalized.encode()).digest()
            except (ValueError, TypeError):
                reply(400)
                return
        with self._lock:
            if inference:
                if self.inference_requests >= self.max_requests or fingerprint in self._inference_fingerprints:
                    # Permanent local denial prevents SDK/CLI backoff from
                    # spending more quota on a retry. Failed attempts count.
                    reply(400, b'{"error":{"type":"invalid_request_error","message":"Live-check budget exhausted or duplicate request; no retries permitted."}}')
                    return
                self.inference_requests += 1
                self._inference_fingerprints.add(fingerprint)
            else:
                if self.model_requests >= 1:
                    reply(400)
                    return
                self.model_requests += 1
        path = self.completion_path if inference else handler.path
        request = urllib.request.Request(self.upstream + path, data=body,
            headers={"Authorization": "Bearer " + self._token, "Content-Type": "application/json",
                     "Accept": "application/json"}, method=handler.command)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), _NoRedirect())
        try:
            try:
                response = opener.open(request, timeout=45)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                data = response.read((64 << 20) + 1)
                if len(data) > 64 << 20:
                    reply(502)
                    return
                content_type = response.headers.get("Content-Type", "application/json")
                if inference:
                    redacted = data.replace(self._token.encode(), b"[redacted]")
                    entry = response_summary(response.status, content_type, redacted)
                    entry["request"] = request_summary(body.replace(self._token.encode(), b"[redacted]") if body else None)
                    with self._lock:
                        self.records.append(entry)
                    self.emit(entry)
                reply(response.status, data, content_type, response.headers.get("Retry-After"))
        except (OSError, urllib.error.URLError):
            if inference:
                self.emit({"network_or_tls_failure": True})
            reply(502)
