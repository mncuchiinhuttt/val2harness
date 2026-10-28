"""Exercise the real CLI with temporary homes, synthetic keys and no Val calls."""
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request

binary = str(Path(sys.argv[1]).resolve())
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
for wildcard in (False, True):
    with tempfile.TemporaryDirectory(prefix="val2harness-cli-") as directory:
        home = Path(directory)
        config_dir = home / ".config/val2harness"
        config_dir.mkdir(parents=True)
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        addr = f"{'0.0.0.0' if wildcard else '127.0.0.1'}:{port}"
        cfg = {"listen_addr":addr, "upstream_url":"http://127.0.0.1:1", "token":"synthetic-val"}
        if wildcard:
            cfg["api_key"] = "synthetic-configured-key"
        path = config_dir / "config.json"
        path.write_text(json.dumps(cfg))
        env = {k:v for k,v in os.environ.items() if k.upper() in {
            "PATH","SYSTEMROOT","WINDIR","COMSPEC","PATHEXT","TEMP","TMP","OS"}}
        env.update(HOME=str(home), USERPROFILE=str(home))
        def run(command):
            return subprocess.run([binary, command], cwd=home, env=env, text=True,
                                  capture_output=True, timeout=20,
                                  creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
        def call(method, route, token):
            request = urllib.request.Request(f"http://127.0.0.1:{port}"+route, method=method,
                data=b"" if method=="POST" else None, headers={"Authorization":"Bearer "+token})
            try:
                with opener.open(request, timeout=3) as response:
                    return response.status, json.load(response)
            except urllib.error.HTTPError as error:
                return error.code, None
        try:
            started = run("start")
            assert started.returncode == 0, started.stderr
            key = json.loads(path.read_text())["api_key"]
            record = json.loads((config_dir/"daemon.pid").read_text())
            status = run("status")
            assert "RUNNING" in status.stdout, status.stdout
            assert call("POST","/api/local/session-ticket",key)[0] == 200
            assert call("POST","/api/local/session-ticket","wrong")[0] == 401
            assert call("GET","/api/local/daemon-control",key)[0] == 401
            control = record["control_token"]
            assert call("GET","/v1/models",control)[0] == 401
            code, identity = call("GET","/api/local/daemon-control",control)
            assert code == 200 and identity["pid"] == record["pid"]
            changed = json.loads(path.read_text())
            changed.update(listen_addr="127.0.0.1:1", api_key="synthetic-rotated-key")
            path.write_text(json.dumps(changed))
            duplicate = run("start")
            assert duplicate.returncode != 0, "spawned a duplicate after settings changed"
            assert json.loads((config_dir/"daemon.pid").read_text())["pid"] == record["pid"]
            stopped = run("stop")
            assert stopped.returncode == 0, stopped.stderr
            assert not (config_dir/"daemon.pid").exists(), "stop retained the live PID record"
            assert "STOPPED" in run("status").stdout
            print("PASS: native CLI", "wildcard/configured-key" if wildcard else "loopback/generated-key",
                  "start/status, auth scope, duplicate prevention, changed-settings graceful stop", flush=True)
        finally:
            if (config_dir/"daemon.pid").exists():
                cleanup = run("stop")
                if cleanup.returncode:
                    raise RuntimeError("Synthetic child requires cleanup: " + cleanup.stderr)
