#!/usr/bin/env python3
"""Validate a disposable CI deployment using real TLS and an independent signer."""
import json
import os
from pathlib import Path
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.request
import tempfile

config = Path(sys.argv[1]).resolve()
key = Path(sys.argv[2]).resolve()
device_key = Path(sys.argv[3]).resolve()
origin = os.environ.get("TEST_BROKER_ORIGIN", "https://127.0.0.1:18443")
web = os.environ.get("TEST_WEB_ORIGIN", "https://127.0.0.1:14443")
ca = config / "credentials/ca.crt"
context = ssl.create_default_context(cafile=str(ca))

def call(path, who="developer", body=None, base=origin):
    headers = {"Content-Type": "application/json"}
    if who:
        headers["Authorization"] = "Bearer " + (config / f"credentials/{who}.token").read_text().strip()
    data = None if body is None else json.dumps(body).encode()
    if who:
        with tempfile.NamedTemporaryFile() as payload:
            payload.write(data or b""); payload.flush()
            env = dict(os.environ, SAFEOPS_TOKEN=headers["Authorization"][7:])
            signed = subprocess.check_output(["./bin/safe-devops", "device-headers", "--key", str(key if who == "reviewer" else device_key), "--method", "GET" if data is None else "POST", "--path", path, "--body-file", payload.name], env=env, text=True)
            headers.update(json.loads(signed))
    req = urllib.request.Request(base + path, data=data, headers=headers)
    with urllib.request.urlopen(req, context=context, timeout=10) as res:
        return json.load(res)

assert call("/readyz", who=None)["ok"]
# Credential alone cannot authenticate on an unenrolled machine.
try:
    req = urllib.request.Request(origin + "/api/me", headers={"Authorization": "Bearer " + (config / "credentials/developer.token").read_text().strip()})
    urllib.request.urlopen(req, context=context)
    raise AssertionError("token-only login passed device gate")
except urllib.error.HTTPError as err:
    assert err.code == 403
assert call("/api/me")["workspace"]["id"].startswith("ci-")
assert call("/api/me", base=web)["principal"]["id"] == "developer"
for path, base in [("/api/agent/poll", origin), ("/api/agent/poll", web), ("/api/demo", origin)]:
    try:
        call(path, body={} if "poll" in path else None, base=base)
        raise AssertionError("unauthorized endpoint allowed: " + path)
    except urllib.error.HTTPError as err:
        assert err.code in (401, 403, 404), err.code
request = call("/api/requests", body={"target": "server-01", "action": "system-info", "reason": "CI verify independent production TLS approval"})
args = ["./bin/safe-devops", "approve", "--url", origin, "--ca", str(ca), "--request", request["intent"]["id"], "--key", str(key)]
env = dict(os.environ, SAFEOPS_TOKEN=(config / "credentials/reviewer.token").read_text().strip())
preview = subprocess.run(args, env=env, check=True, capture_output=True, text=True)
assert request["fingerprint"] in preview.stdout
assert call("/api/requests/" + request["intent"]["id"])["status"] == "pending"
subprocess.run(args + ["--confirm", request["fingerprint"]], env=env, check=True, capture_output=True)
for _ in range(40):
    result = call("/api/requests/" + request["intent"]["id"])
    if result["status"] == "completed":
        assert result["exit_code"] == 0 and result["output"].strip()
        break
    time.sleep(0.5)
else:
    raise AssertionError("connector did not execute approved task: " + result["status"])
print("PASS: TLS readiness, token-only denial, per-request device signatures through Web proxy, independent human signature, connector execution.")
