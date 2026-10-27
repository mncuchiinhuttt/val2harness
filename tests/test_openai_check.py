"""Credential-access and diagnostic guards for the opt-in SDK live check."""
import contextlib
import io
import json
from pathlib import Path
import runpy
import sys
import unittest
from types import SimpleNamespace
from unittest.mock import patch


CHECK = Path(__file__).with_name("openai-check.py")


class OpenAICheckGuards(unittest.TestCase):
    def test_image_color_allows_case_and_whitespace_but_not_another_color(self):
        scope = runpy.run_path(str(CHECK))
        for value, accepted in (("red", True), ("Red", True), (" RED ", True), ("green", False)):
            with self.subTest(value=value):
                reply = {"role": "assistant", "tool_calls": [{"id": "synthetic-id", "type": "function",
                    "function": {"name": "compat_echo", "arguments": json.dumps({"value": value})}}]}
                def completed(*args):
                    return reply, "tool_calls", SimpleNamespace(prompt_tokens=9, completion_tokens=3)
                with patch.dict(scope["tool_loop"].__globals__, {"completion": completed}), contextlib.redirect_stdout(io.StringIO()):
                    if accepted:
                        scope["tool_loop"](None, "synthetic", True, True, first_turn_only=True)
                    else:
                        with self.assertRaises(scope["ProbeCheckFailed"]):
                            scope["tool_loop"](None, "synthetic", True, True, first_turn_only=True)

    def test_invalid_live_budget_does_not_read_account(self):
        for budget in ([], ["--max-requests", "3"]):
            with self.subTest(budget=budget), patch.object(sys, "argv", [str(CHECK), "--binary", "unused", "--live", *budget]), \
                    patch.object(Path, "read_text", side_effect=AssertionError("Account read")), \
                    contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as error:
                    runpy.run_path(str(CHECK), run_name="__main__")
                self.assertEqual(error.exception.code, 2)

    def test_no_live_flag_uses_fake_upstream_without_account_read(self):
        scope = runpy.run_path(str(CHECK))
        received = []
        def fake_run(binary, upstream, token, model, budget, live):
            received.append((upstream, token, model, budget, live))
            fake_requests.extend([{}] * 4)
        fake_requests = []
        @contextlib.contextmanager
        def fake():
            yield "http://127.0.0.1:1", fake_requests
        with patch.object(sys, "argv", [str(CHECK), "--binary", "unused"]), \
                patch.object(Path, "read_text", side_effect=AssertionError("Account read")), \
                patch.dict(scope["main"].__globals__, {"run": fake_run, "fake_upstream": fake}):
            scope["main"]()
        self.assertEqual(received, [("http://127.0.0.1:1", "synthetic-openai-token", "synthetic-openai", 4, False)])

    def test_sdk_failure_cannot_print_reflected_secret_body(self):
        scope = runpy.run_path(str(CHECK))
        class SDKFailure(Exception):
            status_code = 401
        def fail():
            raise SDKFailure("private-token-and-response-body")
        output = io.StringIO()
        with patch.dict(scope["guarded_main"].__globals__, {"main": fail}), contextlib.redirect_stdout(output):
            self.assertEqual(scope["guarded_main"](), 1)
        self.assertIn('"http_status": 401', output.getvalue())
        self.assertNotIn("private-token", output.getvalue())


if __name__ == "__main__":
    unittest.main()
