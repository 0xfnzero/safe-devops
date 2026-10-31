#!/usr/bin/env python3
"""Disposable local production-mode TLS integration; no daemon or system install."""
import json
import os
from pathlib import Path
import ssl
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

root = Path(__file__).resolve().parent.parent
os.chdir(root)
base = Path(tempfile.mkdtemp(prefix="local-tls-", dir=root / ".safeops"))
key = base / "test-human.key"
public = subprocess.check_output(["./bin/safe-devops", "keygen", "--out", str(key)], text=True).strip()
device_key = base / "test-device.key"
subprocess.run(["./bin/safe-devops", "keygen", "--out", str(device_key)], check=True, stdout=subprocess.DEVNULL)
device_public = subprocess.check_output(["./bin/safe-devops", "device-public-key", "--key", str(device_key)], text=True).strip()
bundle = base / "bundle"
subprocess.run(["./bin/safe-devops", "init-compose", "--dir", str(bundle), "--workspace", "ci-local", "--project", "core", "--broker-url", "https://127.0.0.1:18443", "--web-url", "https://127.0.0.1:14443", "--approver-public-key", public, "--device-public-key", device_public], check=True, stdout=subprocess.DEVNULL)
for component in ("broker", "connector"):
    path = bundle / component / (component + ".json")
    config = json.loads(path.read_text())
    def relocate(value):
        if isinstance(value, dict):
            return {k: relocate(v) for k, v in value.items()}
        if isinstance(value, list):
            return [relocate(v) for v in value]
        if isinstance(value, str):
            value = value.replace("/etc/safe-devops/", str(bundle / component) + "/")
            value = value.replace("/var/lib/safe-devops-" + component, str(bundle / "data" / component))
        return value
    config = relocate(config)
    if component == "broker":
        config["listen"] = "127.0.0.1:18443"
    path.write_text(json.dumps(config))
    subprocess.run(["./bin/safe-devops", "check-config", "--role", component, "--config", str(path)], check=True)
ca = bundle / "credentials/ca.crt"
context = ssl.create_default_context(cafile=str(ca))
processes = []
logfiles = []
def start(args, env=None, cwd=None):
    log = open(base / ("process-" + str(len(processes)) + ".log"), "w")
    logfiles.append(log)
    processes.append(subprocess.Popen(args, env=env, cwd=cwd, stdout=log, stderr=subprocess.STDOUT))

def ready(url):
    for _ in range(60):
        if any(p.poll() is not None for p in processes):
            raise AssertionError("service exited; inspect " + str(base))
        try:
            with urllib.request.urlopen(url, context=context, timeout=1) as r:
                if r.status == 200:
                    return
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(0.2)
    raise AssertionError("service not ready: " + url)

try:
    start(["./bin/safe-devops", "broker", "--config", str(bundle / "broker/broker.json")])
    ready("https://127.0.0.1:18443/readyz")
    start(["./bin/safe-devops", "connector", "--config", str(bundle / "connector/connector.json")])
    env = dict(os.environ, NODE_ENV="production", SAFEOPS_API_ORIGIN="https://127.0.0.1:18443", NODE_EXTRA_CA_CERTS=str(ca))
    start(["node", "node_modules/next/dist/bin/next", "start", "--hostname", "127.0.0.1", "--port", "3210"], env=env, cwd=root / "web")
    ready("http://127.0.0.1:3210/")
    subprocess.run(["python3", "scripts/verify-deployment.py", str(bundle), str(key), str(device_key)], env=dict(os.environ, TEST_WEB_ORIGIN="http://127.0.0.1:3210"), check=True)
finally:
    for p in reversed(processes):
        p.terminate()
    for p in processes:
        try:
            p.wait(timeout=10)
        except subprocess.TimeoutExpired:
            p.kill(); p.wait()
    for log in logfiles:
        log.close()
print("PASS local production-mode TLS integration. Private test artifacts retained at", base)
