#!/usr/bin/env python3
"""Disposable SQLite workflow test: commands, session, AI lock, restart, unlock, plan."""
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

root = Path(__file__).resolve().parent.parent
binary = root / "bin/safe-devops"
with tempfile.TemporaryDirectory(prefix="safe-devops-workflows-") as temp:
    base = Path(temp)
    config = base / "config"
    subprocess.run([str(binary), "init-demo", "--dir", str(config)], check=True, stdout=subprocess.DEVNULL)
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0)); port = sock.getsockname()[1]
    origin = f"http://127.0.0.1:{port}"
    for name in ("broker", "agent", "reviewer"):
        path = config / (name + ".json")
        data = json.loads(path.read_text())
        if name == "broker": data["listen"] = f"127.0.0.1:{port}"
        else: data["broker_url"] = origin
        path.write_text(json.dumps(data))
    users = {u["id"]: u for u in json.loads((config / "demo-users.json").read_text())["users"]}
    processes = []
    env = dict(os.environ); env.pop("SAFEOPS_DEVICE_KEY", None); env.pop("SAFEOPS_DATABASE_URL", None)
    log = open(base / "services.log", "w+")
    def start(role, filename):
        p = subprocess.Popen([str(binary), role, "--demo", "--config", str(config / filename)], env=env, stdout=log, stderr=subprocess.STDOUT)
        processes.append(p)
        return p
    def stop_all():
        for p in processes:
            if p.poll() is None: p.terminate()
        for p in processes:
            try: p.wait(timeout=10)
            except subprocess.TimeoutExpired: p.kill(); p.wait()
        processes.clear()
    def call(path, body=None, who="alice", want=200):
        headers = {"Content-Type": "application/json", "Authorization": "Bearer " + users[who]["token"]}
        req = urllib.request.Request(origin + "/api" + path, headers=headers, data=None if body is None else json.dumps(body).encode())
        try:
            with urllib.request.urlopen(req, timeout=5) as response:
                assert response.status == want
                return json.load(response)
        except urllib.error.HTTPError as error:
            if error.code != want: raise
            return json.load(error)
    def wait(fn, predicate, limit=20):
        end = time.monotonic() + limit
        while time.monotonic() < end:
            try:
                value = fn()
                if predicate(value): return value
            except (urllib.error.URLError, ConnectionError): pass
            time.sleep(.2)
        raise AssertionError("timed out waiting for workflow")
    def terminal(command, session=None):
        return call("/terminal/command", {"target": "linux-demo", "profile": "diagnostics", "command": command, "session": session})
    def result(r, statuses=("completed",)):
        return wait(lambda: call("/requests/" + r["intent"]["id"]), lambda v: v["status"] in statuses)
    def approve(r, who):
        subprocess.run([str(binary), "approve", "--demo", "--url", origin, "--request", r["intent"]["id"], "--key", str(config / (who + ".key")), "--confirm", r["fingerprint"]], env=dict(env, SAFEOPS_TOKEN=users[who]["token"]), check=True, stdout=subprocess.DEVNULL)
    try:
        start("broker", "broker.json")
        wait(lambda: call("/me"), lambda v: v["principal"]["id"] == "alice")
        start("connector", "agent.json"); start("reviewer", "reviewer.json")
        simple = result(terminal("ls")); assert "app.log" in simple["output"]
        reviewed = result(terminal("grep 'ERROR' logs:app.log"))
        assert "upstream timeout" in reviewed["output"] and reviewed["assessments"][0]["context_digest"]
        call("/terminal/command", {"target": "linux-demo", "profile": "diagnostics", "command": "cat logs:../private"}, want=400)
        session = call("/requests", {"target": "linux-demo", "action": "diagnostics", "reason": "Disposable integration diagnostic session", "max_uses": 10})
        session = result(session, ("ready",)); assert session["used"] == 0
        for n in range(5):
            terminal("cat logs:missing-file-" + str(n), session["intent"]["id"])
            result(session, ("ready", "revoked"))
        locks = wait(lambda: call("/account-locks"), lambda v: len(v["locks"]) == 1)
        item = locks["locks"][0]
        call("/terminal/command", {"target": "linux-demo", "profile": "diagnostics", "command": "status"}, want=423)
        stop_all(); start("broker", "broker.json")
        wait(lambda: call("/account-locks"), lambda v: len(v["locks"]) == 1)
        call("/terminal/command", {"target": "linux-demo", "profile": "diagnostics", "command": "status"}, want=423)
        subprocess.run([str(binary), "unlock-account", "--demo", "--url", origin, "--lock", item["lock"]["id"], "--key", str(config / "bob.key"), "--confirm", item["fingerprint"]], env=dict(env, SAFEOPS_TOKEN=users["bob"]["token"]), check=True, stdout=subprocess.DEVNULL)
        assert call("/requests/" + session["intent"]["id"])["status"] == "revoked"
        start("connector", "agent.json"); start("reviewer", "reviewer.json")
        plan = call("/requests", {"target": "linux-demo", "action": "demo-change", "reason": "Disposable ordered plan validation", "steps": ["check", "verify"]})
        approve(plan, "bob"); assert call("/requests/" + plan["intent"]["id"])["status"] == "pending"
        approve(plan, "carol"); completed = result(plan)
        assert completed["exit_code"] == 0 and "[2/2]" in completed["output"]
        print("PASS: command routing, AI context, traversal denial, scoped session, behavior lock, SQLite restart persistence, independent unlock, two-human ordered plan.")
    except Exception:
        log.flush(); log.seek(0); print(log.read()[-4000:]); raise
    finally:
        stop_all(); log.close()
