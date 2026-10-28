"""Offline tests for the credential-bearing live-check recorder."""
import json
import threading
import unittest
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from live_probe_support import LiveRecorder, response_summary


class SummaryTests(unittest.TestCase):
    def test_sse_outline_reports_shape_and_usage_without_content_or_arguments(self):
        first = {"choices": [{"delta": {"tool_calls": [{"index": 0, "id": "private-id",
            "function": {"name": "compat_echo", "arguments": '{"value":"private-value"}'}}]}}]}
        final = {"choices": [{"delta": {}, "finish_reason": "tool_calls"}],
                 "usage": {"prompt_tokens": 10, "completion_tokens": 5}}
        wire = ("data:" + json.dumps(first) + "\n\ndata:" + json.dumps(final) + "\n\ndata:[DONE]\n\n").encode()
        summary = response_summary(200, "text/event-stream", wire)
        self.assertEqual(summary["sse_frames"][0]["delta_fields"], ["tool_calls"])
        self.assertEqual(summary["sse_frames"][1]["usage"]["prompt_tokens"], 10)
        self.assertTrue(summary["sse_done"])
        self.assertNotIn("private-value", json.dumps(summary))
        self.assertNotIn("private-id", json.dumps(summary))

    def test_tool_shape_keeps_no_argument_values_or_response_text(self):
        body = json.dumps({"choices": [{"finish_reason": "tool_calls", "message": {
            "content": "private@example.invalid", "tool_calls": [{"id": "synthetic-id", "type": "function",
            "function": {"name": "unexpected_tool", "arguments": '{"value":"private@example.invalid"}'}}]}}],
            "usage": {"prompt_tokens": 12, "completion_tokens": 4}}).encode()
        summary = response_summary(200, "application/json", body)
        self.assertEqual(summary["choices"][0]["tools"][0]["name"], "unexpected_tool")
        self.assertEqual(summary["choices"][0]["tools"][0]["argument_kind"], "string")
        self.assertEqual(summary["usage"]["prompt_tokens"], 12)
        self.assertNotIn("private@example.invalid", json.dumps(summary))
        self.assertNotIn("synthetic-id", json.dumps(summary))

    def test_object_arguments_are_distinguished_and_unknown_values_not_logged(self):
        body = json.dumps({"choices": [{"message": {"tool_calls": [{"type": "function", "function": {
            "name": "compat_echo", "arguments": {"value": "compat-ok"}}}]}}]}).encode()
        tool = response_summary(200, "application/json", body)["choices"][0]["tools"][0]
        self.assertEqual(tool["argument_kind"], "object")
        self.assertTrue(tool["expected_synthetic_call"])

    def test_errors_are_classified_without_printing_arbitrary_provider_detail(self):
        body = b'{"detail":"ValidationException: parallel_tool_calls unsupported; secret=do-not-log"}'
        summary = response_summary(400, "application/json", body)
        self.assertIn("parallel_tool_calls", summary["error_topics"])
        self.assertIn("ValidationException", summary["error_topics"])
        self.assertNotIn("do-not-log", json.dumps(summary))

    def test_unknown_and_oversized_names_are_not_echoed(self):
        for name in ("credential.with.dots", "x" * 129):
            body = json.dumps({"choices": [{"message": {"tool_calls": [{"function": {
                "name": name, "arguments": "{}"}}]}}]}).encode()
            self.assertEqual(response_summary(200, "application/json", body)["choices"][0]["tools"][0]["name"], "[invalid-name]")


class RecorderTests(unittest.TestCase):
    def test_completion_route_is_a_fixed_explicit_choice(self):
        with LiveRecorder("http://127.0.0.1:1", "synthetic", 1,
                          completion_path="/openai/chat/completions") as recorder:
            self.assertEqual(recorder.completion_path, "/openai/chat/completions")
        with self.assertRaises(ValueError):
            LiveRecorder("http://127.0.0.1:1", "synthetic", 1, completion_path="/api/v1/chats/")

    def test_function_mode_only_accepts_explicit_documented_values(self):
        with LiveRecorder("http://127.0.0.1:1", "synthetic", 1, function_calling="default") as recorder:
            self.assertEqual(recorder.function_calling, "default")
        with self.assertRaises(ValueError):
            LiveRecorder("http://127.0.0.1:1", "synthetic", 1, function_calling="unknown")

    def test_one_request_budget_authentication_routes_and_private_forwarding(self):
        calls = []
        class Fake(BaseHTTPRequestHandler):
            def do_POST(self):
                payload = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                calls.append((self.path, self.headers.get("Authorization"), payload))
                body = b'{"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0}}'
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            def log_message(self, *args):
                pass
        upstream = ThreadingHTTPServer(("127.0.0.1", 0), Fake)
        thread = threading.Thread(target=upstream.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(upstream.server_close)
        self.addCleanup(upstream.shutdown)
        emitted = []
        with LiveRecorder(f"http://127.0.0.1:{upstream.server_port}", "private-upstream-token", 1, emitted.append,
                          completion_path="/openai/chat/completions", function_calling="default", client_tool_specs=True,
                          max_output_tokens=512) as recorder:
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            def send(path, key):
                request = urllib.request.Request(recorder.url + path, data=b'{"tools":[{"type":"function","function":{"name":"compat_echo","parameters":{"type":"object"}}}],"params":{"temperature":0},"max_tokens":32000}',
                    headers={"Authorization": "Bearer " + key, "Content-Type": "application/json"})
                try:
                    with opener.open(request, timeout=3) as response:
                        return response.status
                except urllib.error.HTTPError as error:
                    error.close()
                    return error.code
            self.assertEqual(send("/api/chat/completions", "wrong"), 401)
            self.assertEqual(send("/api/v1/chats/", recorder.key), 404)
            self.assertEqual(send("/api/chat/completions", recorder.key), 200)
            self.assertEqual(send("/api/chat/completions", recorder.key), 400)
            self.assertEqual(recorder.inference_requests, 1)
        function = {"name": "compat_echo", "parameters": {"type": "object"}}
        self.assertEqual(calls, [("/openai/chat/completions", "Bearer private-upstream-token",
                                 {"tools": [{"type": "function", "function": function}],
                                  "tool_servers": [{"specs": [function]}],
                                  "max_tokens": 512,
                                  "params": {"temperature": 0, "function_calling": "default"}})])
        self.assertNotIn("private-upstream-token", json.dumps(emitted))
        self.assertEqual(emitted[0]["request"]["tool_names"], ["compat_echo"])
        self.assertTrue(emitted[0]["request"]["tools_key_present"])
        self.assertEqual(emitted[0]["request"]["function_calling"], "default")
        self.assertEqual(emitted[0]["request"]["client_spec_names"], ["compat_echo"])
        self.assertFalse(emitted[0]["request"]["client_specs_have_network_config"])

    def test_failed_inference_is_not_retried_and_new_turn_still_works(self):
        calls = []
        class Fake(BaseHTTPRequestHandler):
            def do_POST(self):
                calls.append(json.loads(self.rfile.read(int(self.headers["Content-Length"]))))
                self.send_response(502)
                self.send_header("Content-Length", "0")
                self.end_headers()
            def log_message(self, *args):
                pass
        upstream = ThreadingHTTPServer(("127.0.0.1", 0), Fake)
        threading.Thread(target=upstream.serve_forever, daemon=True).start()
        self.addCleanup(upstream.server_close)
        self.addCleanup(upstream.shutdown)
        with LiveRecorder(f"http://127.0.0.1:{upstream.server_port}", "synthetic", 2) as recorder:
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            def send(body):
                request = urllib.request.Request(recorder.url + "/api/chat/completions", data=body,
                    headers={"Authorization": "Bearer " + recorder.key, "Content-Type": "application/json"})
                try:
                    with opener.open(request, timeout=3) as response:
                        return response.status
                except urllib.error.HTTPError as error:
                    error.close()
                    return error.code
            self.assertEqual(send(b'{"messages":["first"],"max_tokens":32}'), 502)
            # Whitespace/key order must not turn an automatic retry into a new
            # billable attempt, even after the provider returned an error.
            self.assertEqual(send(b'{ "max_tokens":32, "messages": ["first"] }'), 400)
            self.assertEqual(recorder.inference_requests, 1)
            self.assertEqual(send(b'{"messages":["tool result"],"max_tokens":32}'), 502)
            self.assertEqual(send(b'{"messages":["third"],"max_tokens":32}'), 400)
            self.assertEqual(recorder.inference_requests, 2)
        self.assertEqual(len(calls), 2)


if __name__ == "__main__":
    unittest.main()
