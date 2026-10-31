"""Verify native build artifacts and upload to an existing, unpublished release."""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
from tasklib import ROOT, cli, digest, fail, required, run, version


def output(args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def gh_json(args):
    return json.loads(output(["gh", *args]))


def expected_assets(ver):
    names = {f"safe-devops-source-{ver}.tar.gz"}
    for component in ("server", "web"):
        for system in ("linux", "darwin"):
            for arch in ("amd64", "arm64"):
                names.add(f"safe-devops-{component}-{ver}-{system}-{arch}.tar.gz")
    for system, extensions in (("linux", ("AppImage", "deb")), ("mac", ("dmg", "zip")), ("win", ("exe",))):
        for arch in ("x64", "arm64"):
            for ext in extensions:
                artifact_arch = {"AppImage": "x86_64", "deb": "amd64"}.get(ext, arch) if arch == "x64" else arch
                names.add(f"safe-devops-{ver}-{system}-{artifact_arch}.{ext}")
    return names


def collect_verified(roots, expected):
    verified = {}
    for root in roots:
        manifests = [*root.rglob("*.tar.gz.sha256"), *root.rglob("SHA256SUMS")]
        for manifest in manifests:
            for line in manifest.read_text().splitlines():
                match = re.fullmatch(r"([0-9a-f]{64})  ([^/\\]+)", line)
                if not match or match[2] in (".", ".."):
                    fail(f"Invalid checksum entry in {manifest}")
                checksum, name = match.groups()
                path = manifest.parent / name
                if name not in expected or name in verified or path.is_symlink() or not path.is_file():
                    fail(f"Unexpected, duplicate or missing asset: {name}")
                if digest(path) != checksum:
                    fail(f"Checksum mismatch: {name}")
                verified[name] = (path, checksum)
    if set(verified) != expected:
        fail(f"Missing release assets: {sorted(expected - set(verified))}")
    return verified


def main():
    ver = version()
    tag = "v" + ver
    run_id = required("PACKAGE_RUN")
    if not run_id.isdecimal():
        fail("PACKAGE_RUN must be a numeric GitHub Actions run ID")
    repo = required("GITHUB_REPOSITORY")
    sha = output(["git", "rev-parse", "HEAD"])
    build = gh_json(["run", "view", run_id, "--repo", repo, "--json", "headSha,conclusion,workflowName,event"])
    if build != {"headSha": sha, "conclusion": "success", "workflowName": "Build packages", "event": "workflow_dispatch"}:
        fail("Package run must be a successful manual Build packages run for this commit")
    ci = gh_json(["run", "list", "--repo", repo, "--workflow", "ci.yml", "--commit", sha, "--status", "success", "--limit", "1", "--json", "headSha"])
    if not ci or ci[0]["headSha"] != sha:
        fail("This commit needs a successful CI run")
    ref = gh_json(["api", f"repos/{repo}/git/ref/tags/{tag}"])["object"]
    if ref["type"] == "tag":
        ref = gh_json(["api", f"repos/{repo}/git/tags/{ref['sha']}"])["object"]
    if ref["type"] != "commit" or ref["sha"] != sha:
        fail("Release tag must resolve to this exact commit")
    release = gh_json(["release", "view", tag, "--repo", repo, "--json", "isDraft,tagName,assets"])
    if not release["isDraft"] or release["tagName"] != tag or release["assets"]:
        fail("Release must be an empty draft; existing assets are never overwritten")
    base = ROOT / ".safeops/release-stage"
    base.mkdir(parents=True, exist_ok=False)
    downloaded = base / "downloaded"
    run(["gh", "run", "download", run_id, "--repo", repo, "--dir", downloaded])
    run(["python3", "scripts/package.py", "source"], cwd=ROOT)
    # Include only the source archive's external checksum, not its internal list.
    source = base / "source"
    source.mkdir()
    for suffix in (".tar.gz", ".tar.gz.sha256"):
        shutil.copy2(ROOT / "dist" / f"safe-devops-source-{ver}{suffix}", source)
    verified = collect_verified([downloaded, source], expected_assets(ver))
    staged = base / "assets"
    staged.mkdir()
    for name, (path, _) in verified.items():
        shutil.copy2(path, staged / name)
    manifest = "".join(f"{verified[name][1]}  {name}\n" for name in sorted(verified))
    (staged / "SHA256SUMS").write_text(manifest)
    run(["gh", "release", "upload", tag, "--repo", repo, *sorted(staged.iterdir())])
    print(manifest)
    print(f"Uploaded {len(verified)} verified packages and SHA256SUMS. Release remains a draft.")
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        Path(os.environ["GITHUB_STEP_SUMMARY"]).write_text(f"# {tag}\n\nCommit: `{sha}`\n\nVerified {len(verified)} packages. Release remains a draft.\n\n```text\n{manifest}```\n")


if __name__ == "__main__":
    cli(main)
