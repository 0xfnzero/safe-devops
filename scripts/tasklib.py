"""Shared, shell-free helpers for local development and release commands."""
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def fail(message):
    raise ValueError(message)


def env(name, default=""):
    return os.environ.get(name) or default


def required(name):
    value = env(name)
    if not value:
        fail(f"缺少 {name}；用 make <命令> {name}=值 指定。参见 docs/commands.md")
    return value


def run(args, **kwargs):
    # Public keys, paths and URLs remain individual arguments, never shell code.
    return subprocess.run([str(a) for a in args], check=True, **kwargs)


def tool(name):
    path = shutil.which(name)
    if not path:
        fail(f"未找到 {name}，请先安装；可运行 make doctor 检查环境。")
    return path


def version():
    value = env("VERSION", json.loads((ROOT / "web/package.json").read_text())["version"])
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", value):
        fail("VERSION 必须为语义版本，例如 0.4.1 或 0.4.1-rc.1")
    return value


def host():
    system = {"Darwin": "darwin", "Linux": "linux", "Windows": "windows"}.get(platform.system())
    arch = {"x86_64": "amd64", "AMD64": "amd64", "aarch64": "arm64", "arm64": "arm64", "ARM64": "arm64"}.get(platform.machine())
    if not system or not arch:
        fail("当前仅支持 Linux/macOS/Windows 的 amd64、arm64。")
    return system, arch


def digest(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def binary():
    path = Path(env("SAFE_DEVOPS_BIN", str(ROOT / "bin/safe-devops"))).resolve()
    if not path.is_file() or not os.access(path, os.X_OK):
        fail("未找到可执行 CLI；请先 make build，或设置 SAFE_DEVOPS_BIN=/可信路径/safe-devops。")
    return path


def cli(main):
    try:
        main()
    except (ValueError, OSError) as error:
        print(f"错误：{error}", file=sys.stderr)
        sys.exit(2)
    except subprocess.CalledProcessError as error:
        print(f"命令失败（退出码 {error.returncode}）。", file=sys.stderr)
        sys.exit(error.returncode if error.returncode > 0 else 1)
    except KeyboardInterrupt:
        sys.exit(130)
