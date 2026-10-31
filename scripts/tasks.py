#!/usr/bin/env python3
import shutil
import sys
from tasklib import ROOT, cli, env, fail, run, tool, version


def main():
    action = sys.argv[1]
    if action == "doctor":
        for name in ("go", "node", "npm", "python3", "bash", "docker", "systemctl"):
            print(f"{name:12} {shutil.which(name) or '未安装 / 当前系统不适用'}")
        print("开发：Go 1.26+、Node 22.18+、npm、Python 3.11+。\nCompose：Docker Engine + Compose v2（支持 --wait）。\n桌面安装包在对应操作系统构建；macOS 签名 / 公证另行配置。")
        if shutil.which("docker"):
            run(["docker", "compose", "version"])
        return
    if action == "build":
        output = ROOT / "bin"
        output.mkdir(exist_ok=True)
        run([tool("go"), "build", "-trimpath", "-ldflags", f"-X main.version={version()}", "-o", output / "safe-devops", "./cmd/safe-devops"], cwd=ROOT / "server")
        alias = output / "safeops"
        if alias.is_symlink():
            alias.unlink()
        if not alias.exists():
            alias.symlink_to("safe-devops")
    elif action == "dev-web":
        if not (ROOT / "web/node_modules/next").is_dir():
            fail("请先 make setup 安装 Web 依赖。")
        port = env("WEB_PORT", "3000")
        if not port.isdigit() or not 1 <= int(port) <= 65535:
            fail("WEB_PORT 必须为有效端口。")
        run([tool("npm"), "run", "dev", "--", "--port", port], cwd=ROOT / "web")
    elif action == "docker-build":
        for component in ("server", "web", "gateway"):
            run([tool("docker"), "build", "--build-arg", f"VERSION={version()}", "-f", f"deploy/docker/{component}.Dockerfile", "-t", f"safe-devops-{component}:{version()}", "."], cwd=ROOT)
    else:
        fail("未知命令。")


if __name__ == "__main__":
    cli(main)
