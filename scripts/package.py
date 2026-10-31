#!/usr/bin/env python3
"""Platform-labelled, checksummed release artifacts; never includes live state."""
import argparse
from contextlib import contextmanager
import fnmatch
import os
from pathlib import Path
import re
import shutil
import tarfile
import tempfile
from tasklib import ROOT, cli, digest, env, fail, host, run, tool, version

PLATFORMS = [(system, arch) for system in ("linux", "darwin") for arch in ("amd64", "arm64")]
SOURCE_PATHS = ["Makefile", "README.md", "README.zh-CN.md", "CHANGELOG.md", "LICENSE", "SECURITY.md", "CONTRIBUTING.md", ".gitignore", ".dockerignore", ".github", "assets", "server", "web", "desktop", "scripts", "deploy", "docs"]
EXCLUDES = ["node_modules", ".next", ".git", ".safeops", "dist", "bin", "__pycache__", "*.pyc", "*.tsbuildinfo", "next-env.d.ts", ".env*", "*.key", "*.pem", "*.crt", "*.token", "*.db", "*.db-*", ".DS_Store", "safe-devops", "safeops"]


def source_ignore(directory, names):
    excluded = [name for name in names if any(fnmatch.fnmatch(name, pattern) for pattern in EXCLUDES)]
    for name in set(names) - set(excluded):
        if (Path(directory) / name).is_symlink():
            fail(f"源码包不接受符号链接：{Path(directory) / name}")
    return excluded


def checksums(directory, installers_only=False):
    lines = []
    for path in sorted(directory.rglob("*")):
        if installers_only and (path.parent != directory or path.suffix not in (".dmg", ".zip", ".exe", ".AppImage", ".deb")):
            continue
        if path.is_file() and path.name != "SHA256SUMS":
            lines.append(f"{digest(path)}  {path.relative_to(directory).as_posix()}\n")
    (directory / "SHA256SUMS").write_text("".join(lines))


def ensure_new(name):
    for suffix in ("", ".tar.gz", ".tar.gz.sha256", ".lock"):
        if (ROOT / "dist" / (name + suffix)).exists():
            fail(f"发布产物已存在：dist/{name}{suffix}。请选择新 VERSION 或单独归档旧产物。")


@contextmanager
def staging(name, archive=True, installers_only=False):
    dist = ROOT / "dist"
    dist.mkdir(exist_ok=True)
    ensure_new(name)
    lock = dist / (name + ".lock")
    with lock.open("x"):
        pass
    try:
        with tempfile.TemporaryDirectory(prefix=".package-", dir=dist) as tmp:
            stage = Path(tmp) / name
            stage.mkdir()
            yield stage
            checksums(stage, installers_only)
            if archive:
                packed = Path(tmp) / (name + ".tar.gz")
                with tarfile.open(packed, "w:gz") as tar:
                    tar.add(stage, arcname=name)
                checksum = f"{digest(packed)}  {packed.name}\n"
                packed.rename(dist / packed.name)
                (dist / (packed.name + ".sha256")).write_text(checksum)
            stage.rename(dist / name)
            print(f"已生成：{dist / name}{'.tar.gz' if archive else ''}")
    finally:
        lock.unlink(missing_ok=True)


def compile_binary(system, arch, destination):
    if (system, arch) not in PLATFORMS:
        fail("Go CLI 支持 OS=linux|darwin ARCH=amd64|arm64。Windows 请使用 Electron 客户端。")
    build_env = dict(os.environ, CGO_ENABLED="0", GOOS=system, GOARCH=arch)
    # Ignore inherited architecture tuning so the declared artifact stays portable.
    for key in ("GOAMD64", "GOARM64", "GOFLAGS"):
        build_env.pop(key, None)
    run([tool("go"), "build", "-trimpath", "-ldflags", f"-s -w -X main.version={version()}", "-o", destination, "./cmd/safe-devops"], cwd=ROOT / "server", env=build_env)


def server_name(system, arch):
    return f"safe-devops-server-{version()}-{system}-{arch}"


def server_package(system, arch):
    if (system, arch) not in PLATFORMS:
        fail("OS=linux|darwin；ARCH=amd64|arm64。服务端生产部署仅支持 Linux。")
    with staging(server_name(system, arch)) as stage:
        (stage / "bin").mkdir()
        compile_binary(system, arch, stage / "bin/safe-devops")
        (stage / "scripts").mkdir()
        shutil.copy2(ROOT / "scripts/install.sh", stage / "scripts/install.sh")
        (stage / "deploy").mkdir()
        for unit in (ROOT / "deploy").glob("*.service"):
            shutil.copy2(unit, stage / "deploy" / unit.name)
        shutil.copy2(ROOT / "LICENSE", stage / "LICENSE")
        (stage / "INSTALL.md").write_text(
            f"# safe-devops {version()} · {system}/{arch}\n\n"
            "先核验外部发布校验值，再用 SHA256SUMS 检查文件。bin/safe-devops 是 Go CLI。\n"
            "生产服务运行在 Linux；macOS 包用于 CLI / 本机开发。此包不含 Web 或 Docker 构建源码。\n"
            "Linux：scripts/install.sh --help 查看安装参数；先 --dry-run，配置与私钥单独安装。\n"
            "Web 使用相同目标 OS/架构的 web 包与 Node 22.18+；Docker 部署使用 source 包。\n"
        )


def web_package():
    system, arch = host()
    if system not in ("linux", "darwin"):
        fail("Web 部署包请在目标 Linux 或 macOS 系统构建。Windows 开发可使用 WSL。")
    name = f"safe-devops-web-{version()}-{system}-{arch}"
    with staging(name) as stage:
        run([tool("npm"), "run", "build"], cwd=ROOT / "web")
        standalone = ROOT / "web/.next/standalone"
        if not (standalone / "server.js").is_file():
            fail("缺少 Next.js standalone/server.js。")
        for path in standalone.rglob("*"):
            if path.is_symlink() and not path.resolve().is_relative_to(standalone.resolve()):
                fail(f"Standalone 包含外部链接：{path}")
        shutil.copytree(standalone, stage, dirs_exist_ok=True, symlinks=True)
        shutil.copytree(ROOT / "web/.next/static", stage / ".next/static", dirs_exist_ok=True)
        if (ROOT / "web/public").is_dir():
            shutil.copytree(ROOT / "web/public", stage / "public", dirs_exist_ok=True)
        # Next may trace local env files; never ship a developer's configuration.
        for path in stage.rglob(".env*"):
            if path.is_file() or path.is_symlink():
                path.unlink()
        (stage / "DEPLOYMENT.txt").write_text(f"safe-devops {version()} · {system}/{arch}\nNode.js >=22.18.0。仅用于同 OS/架构；不要跨系统复制 node_modules。\n设置 NODE_ENV=production、HOSTNAME=127.0.0.1、PORT、SAFEOPS_API_ORIGIN 和 NODE_EXTRA_CA_CERTS 后运行 node server.js。\nLinux systemd 安装：scripts/install.sh --component web --web-bundle 本目录 --env 受控web.env。\n")


def source_package():
    with staging(f"safe-devops-source-{version()}") as stage:
        for name in SOURCE_PATHS:
            source = ROOT / name
            if source.is_dir():
                shutil.copytree(source, stage / name, ignore=source_ignore)
            else:
                if source.is_symlink():
                    fail(f"不接受外部源码链接：{source}")
                shutil.copy2(source, stage / name)
        # The archive's default version must match its artifact label.
        makefile = stage / "Makefile"
        makefile.write_text(re.sub(r"^VERSION \?= .*$", f"VERSION ?= {version()}", makefile.read_text(), count=1, flags=re.MULTILINE))


def desktop_package(selected=None):
    native_os, native_arch = host()
    system = selected or env("DESKTOP_OS", "native")
    system = {"native": native_os, "macos": "darwin"}.get(system, system)
    arch = env("DESKTOP_ARCH", "native")
    arch = native_arch if arch == "native" else arch
    if system != native_os:
        fail("桌面安装包须在目标操作系统构建；使用 .github/workflows/packages.yml 原生平台矩阵。")
    if system not in ("darwin", "linux", "windows") or arch not in ("amd64", "arm64"):
        fail("桌面目标：macos/linux/windows，架构：amd64/arm64。")
    if not (ROOT / "desktop/node_modules/electron-builder").is_dir():
        fail("请先 make setup-desktop。")
    flag = {"darwin": "--mac", "linux": "--linux", "windows": "--win"}[system]
    with staging(f"safe-devops-desktop-{version()}-{system}-{arch}", archive=False, installers_only=True) as stage:
        run([tool("node"), ROOT / "desktop/node_modules/electron-builder/cli.js", flag,
             "--x64" if arch == "amd64" else "--arm64", "--publish", "never",
             f"--config.directories.output={stage}", f"--config.extraMetadata.version={version()}"],
            cwd=ROOT / "desktop", env=dict(os.environ, CSC_IDENTITY_AUTO_DISCOVERY="false"))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kind", choices=["server", "servers", "binaries", "web", "source", "desktop", "release"])
    parser.add_argument("--desktop-os", choices=["macos", "linux", "windows"])
    args = parser.parse_args()
    if args.kind == "binaries":
        with staging(f"safe-devops-binaries-{version()}", archive=False) as stage:
            for system, arch in PLATFORMS:
                compile_binary(system, arch, stage / f"safe-devops-{system}-{arch}")
    elif args.kind == "server":
        server_package(env("OS", "linux"), env("ARCH", "amd64"))
    elif args.kind in ("servers", "release"):
        names = [server_name(*p) for p in PLATFORMS]
        if args.kind == "release":
            names += [f"safe-devops-web-{version()}-{'-'.join(host())}", f"safe-devops-source-{version()}"]
        for name in names:
            ensure_new(name)
        for system, arch in PLATFORMS:
            server_package(system, arch)
        if args.kind == "release":
            web_package()
            source_package()
    elif args.kind == "web":
        web_package()
    elif args.kind == "source":
        source_package()
    else:
        desktop_package(args.desktop_os)


if __name__ == "__main__":
    cli(main)
