#!/usr/bin/env python3
"""Explicit local deployment operations, with persistent data preserved."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import uuid
from tasklib import ROOT, binary, cli, env, fail, required, run, tool, version

COMPONENTS = {"broker", "web", "connector", "reviewer", "change-runner"}
SERVICES = {"broker", "web", "gateway", "postgres"}


def number(name, default):
    value = env(name, default)
    if not value.isdigit() or not 1 <= int(value) <= 86400:
        fail(f"{name} 必须是 1..86400 的整数。")
    return value


def flag(name):
    value = env(name, "0")
    if value not in ("0", "1"):
        fail(f"{name} 必须是 0 或 1。")
    return value == "1"


def public_key(name):
    path = Path(required(name)).expanduser()
    data = path.read_text().strip()
    if not data or "PRIVATE KEY" in data or "\n" in data:
        fail(f"{name} 必须是单行公钥文件，不能是私钥。")
    return data


def initialize():
    directory = Path(env("CONFIG_DIR", ".safeops/production")).expanduser().resolve()
    if directory.exists():
        fail("CONFIG_DIR 已存在；初始化不会覆盖已有配置、密钥或数据库。")
    database = env("DATABASE", "sqlite")
    if database not in ("sqlite", "postgres"):
        fail("DATABASE 只能是 sqlite 或 postgres。")
    args = [binary(), "init-compose", "--dir", directory, "--workspace", required("WORKSPACE"),
            "--project", required("PROJECT"), "--broker-url", required("BROKER_URL"),
            "--web-url", required("WEB_URL"), "--database", database,
            "--approver-public-key", public_key("APPROVER_PUBLIC_KEY_FILE"),
            "--device-public-key", public_key("DEVICE_PUBLIC_KEY_FILE")]
    if env("DIAGNOSTIC_ROOT"):
        args += ["--diagnostic-root", env("DIAGNOSTIC_ROOT")]
    run(args)


class Deployment:
    def __init__(self):
        self.directory = Path(env("CONFIG_DIR", ".safeops/production")).expanduser().resolve()
        if not (self.directory / ".env").is_file() or not (self.directory / ".database").is_file():
            fail("CONFIG_DIR 不是已初始化的 Compose 配置目录；先 make deploy-init。")
        self.database = (self.directory / ".database").read_text().strip()
        if self.database not in ("sqlite", "postgres"):
            fail("未知数据库类型；不允许通过切换 .database 迁移数据库。")
        self.wait = number("WAIT_TIMEOUT", "240")

    def compose(self, *args, capture=False):
        environment = dict(os.environ)
        if not environment.get("SAFEOPS_VERSION"):
            environment.pop("SAFEOPS_VERSION", None)
            if not re.search(r"(?m)^\s*(?:export\s+)?SAFEOPS_VERSION\s*=", (self.directory / ".env").read_text()):
                environment["SAFEOPS_VERSION"] = version()
        return run(["bash", ROOT / "scripts/compose.sh", self.directory, *args],
                   cwd=ROOT, capture_output=capture, text=True, env=environment)

    def workspaces(self):
        data = json.loads((self.directory / "broker/broker.json").read_text())
        ids = [item["id"] for item in data.get("workspaces", [])]
        if not ids or any(not re.fullmatch(r"[a-z0-9][a-z0-9_-]{0,63}", name) for name in ids):
            fail("Broker 配置缺少有效的 workspaces；请先校验配置。")
        return ids

    def workspace(self):
        ids = self.workspaces()
        chosen = env("WORKSPACE") or (ids[0] if len(ids) == 1 else "")
        if chosen not in ids:
            fail("请用 WORKSPACE 指定配置中的团队。")
        return chosen

    def backup_name(self, workspace, supplied=True):
        name = env("BACKUP_FILE") if supplied else ""
        name = name or f"{workspace}-{datetime.now(timezone.utc):%Y%m%dT%H%M%SZ}-{uuid.uuid4().hex[:8]}.json"
        if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9._-]*\.json", name):
            fail("BACKUP_FILE 必须是 backups/ 下的 JSON 文件名，不允许路径。")
        return name

    def backup(self, workspace=None, supplied=True):
        workspace = workspace or self.workspace()
        name = self.backup_name(workspace, supplied)
        if (self.directory / "backups" / name).exists():
            fail("备份文件已存在，拒绝覆盖。")
        self.compose("exec", "-T", "broker", "safe-devops", "backup", "--config", "/etc/safe-devops/broker.json",
                     "--workspace", workspace, "--out", f"/backups/{name}")
        print(f"备份完成：{self.directory / 'backups' / name}")

    def require_stopped(self):
        running = self.compose("ps", "--status", "running", "--services", capture=True).stdout.splitlines()
        if "broker" in running:
            fail("Broker 仍在运行。先 make deploy-stop，再执行恢复或策略迁移。")

    def maintenance(self, command, *args):
        self.compose("run", "--rm", "--no-deps", "broker", command, "--config", "/etc/safe-devops/broker.json", *args)

    def check(self):
        self.compose("config", "--quiet")
        self.maintenance("check-config", "--role", "broker")

    def up(self, build=False):
        self.compose("config", "--quiet")
        self.compose("up", "-d", *(["--build"] if build else []), "--wait", "--wait-timeout", self.wait)

    def health(self):
        self.compose("exec", "-T", "broker", "safe-devops", "health", "--url", "https://broker:8443/readyz", "--ca", "/etc/safe-devops/db-ca.crt")
        self.compose("exec", "-T", "web", "node", "-e", "fetch('http://127.0.0.1:3000/').then(r=>{if(!r.ok)process.exit(1);console.log('Web ready')}).catch(()=>process.exit(1))")


def systemd(action, dry_run=False):
    component = required("COMPONENT")
    if component not in COMPONENTS:
        fail("COMPONENT 只能是 broker/web/connector/reviewer/change-runner。")
    if action == "install":
        args = ["bash", ROOT / "scripts/install.sh", "--component", component]
        if component == "web":
            args += ["--web-bundle", required("WEB_BUNDLE"), "--env", required("ENV_FILE")]
        else:
            args += ["--binary", required("BINARY"), "--sha256", required("SHA256"), "--config", required("CONFIG")]
            if component in ("broker", "reviewer"):
                args += ["--env", required("ENV_FILE")]
        for name in ("UPGRADE", "START"):
            if flag(name):
                args.append("--" + name.lower())
        if dry_run or flag("DRY_RUN"):
            args.append("--dry-run")
        run(args)
        return
    service = f"safe-devops-{component}.service"
    if action == "service-logs":
        run([tool("journalctl"), "-u", service, "-n", number("LOG_TAIL", "100"), "-f"])
    elif action == "service-start":
        run([tool("systemctl"), "enable", "--now", service])
    elif action == "service-stop":
        run([tool("systemctl"), "stop", service])
    else:
        run([tool("systemctl"), "--no-pager", "--full", "status", service])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["init", "check", "up", "status", "logs", "health", "restart", "stop", "down", "backup", "restore", "upgrade", "policy-apply", "install", "service-start", "service-stop", "service-status", "service-logs"])
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    if args.action == "init":
        return initialize()
    if args.action == "install" or args.action.startswith("service-"):
        return systemd(args.action, args.dry_run)
    deployment = Deployment()
    service = env("SERVICE")
    if service and service not in SERVICES:
        fail("SERVICE 只能是 broker/web/gateway/postgres。")
    selected = [service] if service else []
    action = args.action
    if action == "check":
        deployment.check()
    elif action == "up":
        deployment.up(build=True)
    elif action == "status":
        deployment.compose("ps", "--all")
    elif action == "logs":
        deployment.compose("logs", "--tail", number("LOG_TAIL", "100"), "--follow", *selected)
    elif action == "health":
        deployment.health()
    elif action == "restart":
        deployment.compose("restart", *selected)
        deployment.up()
    elif action == "stop":
        deployment.compose("stop")
    elif action == "down":
        deployment.compose("down")  # Intentionally no -v or database deletion.
    elif action == "backup":
        deployment.backup()
    elif action == "restore":
        required("BACKUP_FILE")
        workspace = deployment.workspace()
        name = deployment.backup_name(workspace)
        source = deployment.directory / "backups" / name
        if not source.is_file() or source.is_symlink():
            fail("恢复文件必须是配置目录 backups/ 下的普通文件。")
        deployment.require_stopped()
        if deployment.database == "postgres":
            deployment.compose("up", "-d", "--wait", "--wait-timeout", deployment.wait, "postgres")
        deployment.maintenance("restore", "--workspace", workspace, "--in", f"/backups/{name}")
        print("恢复完成；活动授权已撤销。确认配置后 make deploy-up 启动。")
    elif action == "policy-apply":
        workspace = deployment.workspace()
        deployment.require_stopped()
        if deployment.database == "postgres":
            deployment.compose("up", "-d", "--wait", "--wait-timeout", deployment.wait, "postgres")
        deployment.maintenance("policy-apply", "--workspace", workspace)
    elif action == "upgrade":
        workspaces = deployment.workspaces()
        deployment.compose("config", "--quiet")
        deployment.compose("build", "--pull", "broker", "web")
        deployment.check()
        for workspace in workspaces:
            deployment.backup(workspace, supplied=False)
        deployment.up()
        deployment.health()


if __name__ == "__main__":
    cli(main)
