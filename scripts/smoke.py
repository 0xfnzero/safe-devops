#!/usr/bin/env python3
"""Exercise the running loopback demo and independent-device CLI. No real assets."""
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.request

root = Path(__file__).resolve().parent.parent
users = {u["id"]: u for u in json.loads((root / ".safeops/v4/demo-users.json").read_text())["users"]}
origin = "http://127.0.0.1:8088"


def call(identity, path, body=None):
    req = urllib.request.Request(origin + "/api" + path,
        data=None if body is None else json.dumps(body).encode(),
        headers={"Authorization": "Bearer " + users[identity]["token"], "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=5) as response:
        return json.load(response)


request = call("alice", "/requests", {"target": "linux-demo", "action": "host-uptime",
    "reason": "CLI-QA: verify exact-fingerprint approval using the local test key", "ttl_seconds": 300})
args = [str(root / "bin/safe-devops"), "approve", "--demo", "--url", origin,
    "--request", request["intent"]["id"], "--key", str(root / ".safeops/v4/bob.key")]
env = dict(os.environ, SAFEOPS_TOKEN=users["bob"]["token"])
preview = subprocess.run(args, env=env, capture_output=True, text=True, check=True)
assert request["fingerprint"] in preview.stdout
assert call("alice", "/requests/" + request["intent"]["id"])["status"] == "pending"
bad = subprocess.run(args + ["--confirm", "wrong-fingerprint"], env=env, capture_output=True)
assert bad.returncode != 0
subprocess.run(args + ["--confirm", request["fingerprint"]], env=env, capture_output=True, check=True)
for _ in range(15):
    result = call("alice", "/requests/" + request["intent"]["id"])
    if result["status"] == "completed":
        assert result["exit_code"] == 0 and result["output"].strip()
        break
    time.sleep(1)
else:
    raise AssertionError("Agent did not complete the signed task within 15 seconds")
print("PASS: preview does not sign; wrong fingerprint denied; exact approval executes once.")


def wait_result(request, status):
    for _ in range(20):
        current = call("alice", "/requests/" + request["intent"]["id"])
        if current["status"] == status:
            return current
        time.sleep(0.5)
    raise AssertionError(f"Expected {status}, got {current['status']}")


auto = call("alice", "/requests", {"target": "linux-demo", "action": "list-directory"})
auto = wait_result(auto, "completed")
assert not auto["approvals"] and "README.txt" in auto["output"]
ai = call("alice", "/requests", {"target": "linux-demo", "action": "disk-usage",
    "reason": "QA bounded disk inspection", "max_uses": 2})
ai = wait_result(ai, "ready")
assert ai["used"] == 1 and ai["approvals"][0]["kind"] == "ai"
assert ai["assessments"][0]["model"] == "demo-policy-simulator"
call("alice", "/requests/" + ai["intent"]["id"] + "/execute", {})
ai = wait_result(ai, "completed")
assert ai["used"] == 2 and len(ai["executions"]) == 2
escalated = call("alice", "/requests", {"target": "linux-demo", "action": "disk-usage",
    "reason": "[human] QA escalation to independent human"})
for _ in range(20):
    escalated = call("alice", "/requests/" + escalated["intent"]["id"])
    if escalated["needs_human"]:
        break
    time.sleep(0.5)
assert escalated["needs_human"] and escalated["status"] == "pending"
args[args.index("--request") + 1] = escalated["intent"]["id"]
subprocess.run(args + ["--confirm", escalated["fingerprint"]], env=env, capture_output=True, check=True)
wait_result(escalated, "completed")
print("PASS: fixed ls auto-authorization; simulated AI approval; bounded reuse; human fallback.")
