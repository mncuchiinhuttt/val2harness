"""Offline guards and acceptance evidence for the disposable CLI edit check."""
import contextlib
import io
import json
import os
from pathlib import Path
import runpy
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch


CHECK = Path(__file__).with_name("claude-code-check.py")
MARKER = "synthetic-edit-0123456789abcdef0123456789abcdef"


class ClaudeEditCheckTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="val2harness-edit-unit-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name).resolve()
        self.initial = ("before: " + MARKER + "\n").encode()
        (self.root / "fixture.txt").write_bytes(self.initial)
        self.gateway = {"url": "http://127.0.0.1:12345", "key": "disposable-local-owner",
                        "root": str(self.root), "fixture_kind": "synthetic-edit-v1",
                        "fixture_marker": MARKER}

    def invoke(self, run_cli, mode="--edit-fixture"):
        (self.root / "gateway.json").write_text(json.dumps(self.gateway), encoding="utf-8")
        arguments = [str(CHECK), str(self.root), str(self.root / "claude.exe")]
        if mode:
            arguments.append(mode)
        with patch.object(sys, "argv", arguments), \
                patch("subprocess.run", side_effect=run_cli) as started, \
                contextlib.redirect_stdout(io.StringIO()):
            try:
                runpy.run_path(str(CHECK), run_name="__main__")
            except SystemExit as error:
                if error.code not in (None, 0):
                    raise
        return started

    def successful_cli(self, args, **options):
        if args[-1] == "--version":
            return subprocess.CompletedProcess(args, 0, "2.1.310 (Claude Code)\n", "")
        self.assertNotIn("--bare", args)
        self.assertEqual(args[args.index("--tools") + 1], "Read,Edit")
        self.assertEqual(args[args.index("--allowedTools") + 1:args.index("--permission-mode")],
                         ["Read(./fixture.txt)", "Edit(./fixture.txt)"])
        self.assertEqual(args[args.index("--permission-mode") + 1], "dontAsk")
        self.assertEqual(args[args.index("--setting-sources") + 1], "")
        self.assertEqual(args[args.index("--mcp-config") + 1], '{"mcpServers":{}}')
        self.assertIn("--strict-mcp-config", args)
        self.assertEqual(options["env"]["CLAUDE_CODE_MAX_RETRIES"], "0")
        self.assertEqual(options["env"]["USERPROFILE"], str(self.root / "cli-home"))
        (self.root / "fixture.txt").write_bytes(("after: " + MARKER + "\n").encode())
        return subprocess.CompletedProcess(args, 0, json.dumps({"result": "after: " + MARKER,
                                                              "num_turns": 3, "permission_denials": []}), "")

    def test_edit_accepts_only_fixed_fixture_and_scoped_tools(self):
        self.assertEqual(self.invoke(self.successful_cli).call_count, 2)

    def test_final_reply_format_does_not_override_exact_edit_evidence(self):
        def explained(args, **options):
            result = self.successful_cli(args, **options)
            if args[-1] != "--version":
                payload = json.loads(result.stdout)
                payload["result"] = "Updated the fixture:\n`" + payload["result"] + "`"
                result.stdout = json.dumps(payload)
            return result
        self.invoke(explained)

    def test_final_reply_still_requires_the_fixture_marker(self):
        def missing(args, **options):
            result = self.successful_cli(args, **options)
            if args[-1] != "--version":
                payload = json.loads(result.stdout)
                payload["result"] = "Completed"
                result.stdout = json.dumps(payload)
            return result
        with self.assertRaises(AssertionError):
            self.invoke(missing)

    def test_invalid_edit_metadata_and_endpoint_never_start_cli(self):
        changes = [
            {"root": str(self.root / "elsewhere")}, {"fixture_kind": "ordinary-file"},
            {"fixture_marker": "user-content"}, {"key": ""},
            {"url": "https://127.0.0.1:12345"}, {"url": "http://example.com:12345"},
            {"url": "http://user@localhost:12345"}, {"url": "http://localhost:12345/v1"},
            {"url": "http://localhost:12345?x=y"}, {"url": "http://localhost:12345#fragment"},
        ]
        for change in changes:
            with self.subTest(change=change), patch("subprocess.run") as process:
                original = self.gateway.copy()
                self.gateway.update(change)
                with self.assertRaises(SystemExit):
                    self.invoke(process)
                self.gateway = original
                process.assert_not_called()
                self.assertFalse((self.root / "cli-home").exists())

    def test_changed_fixture_never_starts_cli(self):
        for content in (b"ordinary user file\n", self.initial + b"extra\n",
                        self.initial.replace(b"before:", b"after:"), self.initial.rstrip(b"\n")):
            with self.subTest(content=content), patch("subprocess.run") as process:
                (self.root / "fixture.txt").write_bytes(content)
                with self.assertRaises(SystemExit):
                    self.invoke(process)
                process.assert_not_called()

    def test_fake_write_gate_does_not_accept_edit_metadata(self):
        with patch("subprocess.run") as process:
            with self.assertRaises(SystemExit):
                self.invoke(process, mode=None)
            process.assert_not_called()

    def test_cli_exit_zero_without_tool_result_turns_is_failure(self):
        def incomplete(args, **options):
            result = self.successful_cli(args, **options)
            if args[-1] != "--version":
                payload = json.loads(result.stdout)
                payload["num_turns"] = 1
                result.stdout = json.dumps(payload)
            return result
        with self.assertRaises(AssertionError):
            self.invoke(incomplete)

    def test_permission_denials_fail_even_if_cli_returns_marker(self):
        def denied(args, **options):
            result = self.successful_cli(args, **options)
            if args[-1] != "--version":
                payload = json.loads(result.stdout)
                payload["permission_denials"] = [{"tool_name": "Edit"}]
                result.stdout = json.dumps(payload)
            return result
        with self.assertRaises(AssertionError):
            self.invoke(denied)

    def test_fixture_bytes_must_match_exactly_after_edit(self):
        def changed(args, **options):
            result = self.successful_cli(args, **options)
            if args[-1] != "--version":
                (self.root / "fixture.txt").write_bytes(("after: " + MARKER + "\nextra\n").encode())
            return result
        with self.assertRaises(AssertionError):
            self.invoke(changed)


class LiveClaudeEditPreparationTests(unittest.TestCase):
    def test_larger_output_cap_is_denied_for_other_checks_before_account_access(self):
        with patch.object(sys, "argv", ["live-val-check.py", "--live", "--binary", "unused",
                                        "--checks", "tools", "--max-output-tokens", "512"]), \
                patch.object(Path, "read_text", side_effect=AssertionError("Account file read")), \
                contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as denied:
                runpy.run_path(str(CHECK.with_name("live-val-check.py")), run_name="__main__")
            self.assertEqual(denied.exception.code, 2)

    def test_preparation_creates_only_random_fixed_edit_fixture(self):
        helpers = runpy.run_path(str(CHECK.with_name("live-val-check.py")))
        self.assertTrue("prepare_claude_fixture" in helpers, "Live edit fixture preparation is missing")
        with tempfile.TemporaryDirectory(prefix="val2harness-prepare-unit-") as directory:
            root = helpers["prepare_claude_fixture"](Path(directory), "http://127.0.0.1:12345",
                                                      "disposable-owner", edit=True)
            self.assertEqual(root, Path(directory) / "cli-fixture")
            metadata = json.loads((root / "gateway.json").read_text())
            self.assertEqual(metadata["root"], str(root.resolve()))
            self.assertEqual(metadata["fixture_kind"], "synthetic-edit-v1")
            self.assertRegex(metadata["fixture_marker"], r"^synthetic-edit-[0-9a-f]{32}$")
            self.assertEqual((root / "fixture.txt").read_bytes(),
                             ("before: " + metadata["fixture_marker"] + "\n").encode())
            self.assertEqual({path.name for path in root.iterdir()}, {"fixture.txt", "gateway.json"})

    def test_edit_live_check_rejects_missing_consent_before_account_access(self):
        with patch.object(sys, "argv", ["live-val-check.py", "--binary", "unused", "--checks", "claude-edit"]), \
                patch.object(Path, "read_text", side_effect=AssertionError("Account file read")), \
                contextlib.redirect_stderr(io.StringIO()) as error:
            with self.assertRaises(SystemExit) as denied:
                runpy.run_path(str(CHECK.with_name("live-val-check.py")), run_name="__main__")
            self.assertEqual(denied.exception.code, 2)
            self.assertIn("Explicit --live consent", error.getvalue())

    def test_edit_live_check_rejects_more_than_three_attempts_before_account_access(self):
        with patch.object(sys, "argv", ["live-val-check.py", "--live", "--binary", "unused",
                                        "--checks", "claude-edit", "--claude-binary", "unused", "--max-requests", "4"]), \
                patch.object(Path, "read_text", side_effect=AssertionError("Account file read")), \
                contextlib.redirect_stderr(io.StringIO()) as error:
            with self.assertRaises(SystemExit) as denied:
                runpy.run_path(str(CHECK.with_name("live-val-check.py")), run_name="__main__")
            self.assertEqual(denied.exception.code, 2)
            self.assertIn("exactly three", error.getvalue())


@unittest.skipUnless(os.environ.get("VAL2HARNESS_CLAUDE_BINARY") and
                     os.environ.get("VAL2HARNESS_INTEROP_TEST_BINARY"),
                     "opt-in installed Claude Code and compiled Go fixture required")
class ActualClaudeCodeTests(unittest.TestCase):
    """Explicit local executables, synthetic temp files and fake Val only."""

    def run_fixture(self, mode):
        with tempfile.TemporaryDirectory(prefix="val2harness-cli-offline-") as directory:
            root = Path(directory).resolve()
            env = {key: value for key, value in os.environ.items() if key.upper() in {
                "PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP", "OS"}}
            env.update(HOME=str(root), USERPROFILE=str(root), VAL2HARNESS_INTEROP_DIR=str(root))
            if mode == "--edit-fixture":
                env["VAL2HARNESS_INTEROP_EDIT"] = "1"
            process = subprocess.Popen([os.environ["VAL2HARNESS_INTEROP_TEST_BINARY"],
                                        "-test.run=^TestAgentInteropFixture$", "-test.timeout=180s"],
                                       env=env, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                       text=True, creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
            try:
                deadline = time.monotonic() + 30
                while not (root / "gateway.json").exists():
                    if process.poll() is not None or time.monotonic() > deadline:
                        self.fail("Go fake fixture failed to start")
                    time.sleep(.05)
                initial = (root / "fixture.txt").read_bytes()
                metadata = json.loads((root / "gateway.json").read_text())
                args = [sys.executable, "-u", str(CHECK.resolve()), str(root),
                        os.environ["VAL2HARNESS_CLAUDE_BINARY"]]
                if mode:
                    args.append(mode)
                result = subprocess.run(args, cwd=root, env=env, text=True, encoding="utf-8",
                                        capture_output=True, timeout=105)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                output = json.loads((root / "claude-stdout.json").read_text(encoding="utf-8"))
                self.assertEqual(output.get("permission_denials"), [])
                requests = json.loads((root / "requests.json").read_text())
                if mode == "--edit-fixture":
                    marker = metadata["fixture_marker"]
                    self.assertEqual(initial, ("before: " + marker + "\n").encode())
                    self.assertEqual((root / "fixture.txt").read_bytes(), ("after: " + marker + "\n").encode())
                    self.assertEqual(len(requests), 3)
                    final_messages = requests[-1]["messages"]
                    for call_id in ("call_read_1", "call_edit_3"):
                        self.assertTrue(any(message.get("role") == "tool" and
                                            message.get("tool_call_id") == call_id for message in final_messages),
                                        "Missing tool-result turn: " + call_id)
                    self.assertTrue(all({tool["function"]["name"] for tool in request["tools"]} ==
                                        {"Read", "Edit"} for request in requests))
                elif mode == "--read-only":
                    self.assertEqual(len(requests), 2)
                    self.assertEqual((root / "fixture.txt").read_bytes(), initial)
                else:
                    self.assertEqual(len(requests), 3)
                    self.assertEqual((root / "result.txt").read_bytes(), b"fixture-token-519\n")
            finally:
                (root / "stop").touch()
                try:
                    log, _ = process.communicate(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.communicate()
                    self.fail("Go fake fixture did not stop")
                self.assertEqual(process.returncode, 0, log)

    def test_actual_cli_edit_tool_result_round_trip(self):
        self.run_fixture("--edit-fixture")

    def test_actual_cli_read_stays_bare_and_read_only(self):
        self.run_fixture("--read-only")

    def test_actual_cli_fake_write_stays_supported(self):
        self.run_fixture(None)


if __name__ == "__main__":
    unittest.main()
