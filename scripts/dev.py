#!/usr/bin/env python3
"""Run only disposable loopback services, supervising their process groups."""
import argparse
import json
import os
import signal
import socket
import subprocess
import time
import urllib.error
import urllib.request
from tasklib import ROOT, binary, cli, env, fail, run, tool


def free_port(port):
    with socket.socket() as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            sock.bind(("127.0.0.1", port))
        except OSError:
            fail(f"127.0.0.1:{port} 已占用；请使用已有服务或先停止对应开发进程。不会自动杀死其他进程。")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--backend-only", action="store_true")
    group.add_argument("--desktop", action="store_true")
    args = parser.parse_args()
    if os.name != "posix":
        fail("开发服务请在 macOS、Linux 或 WSL 中运行。Windows Electron 可单独启动。")
    if env("SAFEOPS_DATABASE_URL"):
        fail("请先清除 SAFEOPS_DATABASE_URL；本机演示不会连接外部数据库。")
    executable = binary()
    port = env("WEB_PORT", "3000")
    if not port.isdigit() or not 1 <= int(port) <= 65535:
        fail("WEB_PORT 必须是有效端口。")
    broker_port = env("BROKER_PORT", "8088")
    if not broker_port.isdigit() or not 1 <= int(broker_port) <= 65535 or broker_port == port:
        fail("BROKER_PORT 必须是有效端口，且不能与 WEB_PORT 相同。")
    free_port(int(broker_port))
    if not args.backend_only:
        free_port(int(port))
        tool("node")
        if not (ROOT / "web/node_modules/next").is_dir():
            fail("请先 make setup。")
    if args.desktop and not (ROOT / "desktop/node_modules/electron").is_dir():
        fail("请先 make setup-desktop。")
    directory = (ROOT / env("DEV_DIR", ".safeops/v4")).resolve()
    if not (directory / "broker.json").exists():
        if directory.exists():
            fail("演示目录不完整；请保留旧目录并由管理员检查，脚本不会覆盖它。")
        directory.parent.mkdir(exist_ok=True)
        run([executable, "init-demo", "--dir", directory])
        if broker_port != "8088":
            def remap(value):
                if isinstance(value, dict):
                    return {key: remap(item) for key, item in value.items()}
                if isinstance(value, list):
                    return [remap(item) for item in value]
                if value == "127.0.0.1:8088":
                    return f"127.0.0.1:{broker_port}"
                if value == "http://127.0.0.1:8088":
                    return f"http://127.0.0.1:{broker_port}"
                return value
            for path in directory.glob("*.json"):
                path.write_text(json.dumps(remap(json.loads(path.read_text())), indent=2))
    config = json.loads((directory / "broker.json").read_text())
    if config.get("listen") != f"127.0.0.1:{broker_port}":
        fail("已有演示目录与 BROKER_PORT 不匹配；请选择新 DEV_DIR 或使用原端口。")
    processes = []

    def start(name, command, cwd=ROOT, extra=None):
        process = subprocess.Popen([str(x) for x in command], cwd=cwd,
                                   env=dict(os.environ, **(extra or {})), start_new_session=True)
        processes.append((name, process))

    def alive():
        for name, process in processes:
            if process.poll() is not None:
                fail(f"{name} 已退出（{process.returncode}），停止本次启动的其他服务。")

    def ready(url, timeout):
        deadline = time.monotonic() + timeout
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        while time.monotonic() < deadline:
            alive()
            try:
                with opener.open(url, timeout=1) as response:
                    if response.status == 200:
                        return
            except (OSError, urllib.error.URLError):
                pass
            time.sleep(0.2)
        fail(f"启动超时：{url}")

    def stop_signal(_signum, _frame):
        raise KeyboardInterrupt

    previous = {sig: signal.signal(sig, stop_signal) for sig in (signal.SIGTERM, signal.SIGINT)}
    try:
        start("Broker", [executable, "broker", "--demo", "--config", directory / "broker.json"])
        ready(f"http://127.0.0.1:{broker_port}/healthz", 15)
        for name, role, filename in [("Connector", "connector", "agent.json"), ("Sandbox Connector", "connector", "agent-sandbox.json"), ("AI simulator", "reviewer", "reviewer.json")]:
            start(name, [executable, role, "--demo", "--config", directory / filename])
        if not args.backend_only:
            start("Web", [tool("node"), "node_modules/next/dist/bin/next", "dev", "--hostname", "127.0.0.1", "--port", port], ROOT / "web", {"SAFEOPS_API_ORIGIN": f"http://127.0.0.1:{broker_port}"})
            ready(f"http://127.0.0.1:{port}", 90)
        if args.desktop:
            start("Electron", [tool("node"), "node_modules/electron/cli.js", "."], ROOT / "desktop", {"SAFE_DEVOPS_ALLOW_LOCAL": "1"})
        print(f"开发环境已就绪：{'Broker http://127.0.0.1:' + broker_port if args.backend_only else 'http://127.0.0.1:' + port}；Ctrl+C 停止。", flush=True)
        while True:
            alive()
            time.sleep(0.5)
    except KeyboardInterrupt:
        print("正在停止本次开发环境…", flush=True)
    finally:
        # Kill only groups created by this run, including Next/Electron descendants.
        for sig in previous:
            signal.signal(sig, signal.SIG_IGN)
        for _, process in reversed(processes):
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
        deadline = time.monotonic() + 5
        for _, process in processes:
            try:
                process.wait(timeout=max(0, deadline - time.monotonic()))
            except subprocess.TimeoutExpired:
                pass
        for _, process in reversed(processes):
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
        for sig, handler in previous.items():
            signal.signal(sig, handler)


if __name__ == "__main__":
    cli(main)
