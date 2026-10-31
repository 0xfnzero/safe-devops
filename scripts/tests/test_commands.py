import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
import deploy
import package
import tasklib
import dev


class CommandsTest(unittest.TestCase):
    def test_default_make_is_help_and_every_public_target_described(self):
        result = subprocess.run(["make", "--no-print-directory"], cwd=ROOT, check=True, capture_output=True, text=True)
        explicit = subprocess.run(["make", "--no-print-directory", "help"], cwd=ROOT, check=True, capture_output=True, text=True)
        self.assertEqual(result.stdout, explicit.stdout)
        targets = next(line for line in (ROOT / "Makefile").read_text().splitlines() if line.startswith(".PHONY:")).split()[1:]
        for target in targets:
            self.assertIn(f"  {target} ", result.stdout)

    def test_shell_syntax(self):
        for script in (ROOT / "scripts").glob("*.sh"):
            subprocess.run(["bash", "-n", script], check=True)

    def test_version_rejects_paths_and_shell_fragments(self):
        for value in ("../escape", "0.4.0;id", "$(id)", "0.4.0/one"):
            with patch.dict(os.environ, {"VERSION": value}), self.assertRaises(ValueError):
                tasklib.version()

    def test_archive_failure_does_not_publish_partial_release(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(package, "ROOT", Path(tmp)):
            with self.assertRaises(RuntimeError):
                with package.staging("test") as stage:
                    (stage / "partial").write_text("partial")
                    raise RuntimeError("build failed")
            self.assertEqual(list((Path(tmp) / "dist").iterdir()), [])
            with package.staging("test") as stage:
                (stage / "payload").write_text("verified")
            target = Path(tmp) / "dist/test.tar.gz"
            self.assertTrue(target.is_file())
            with tarfile.open(target) as tar:
                self.assertIn("test/SHA256SUMS", tar.getnames())
            with self.assertRaises(ValueError):
                with package.staging("test"):
                    self.fail("existing artifacts must not be overwritten")

    def test_source_excludes_secrets_state_and_dependencies(self):
        with tempfile.TemporaryDirectory() as tmp:
            names = [".env.production", "reviewer.key", "device.token", "broker.db-wal", "node_modules", ".next", ".safeops", "dist", "main.go"]
            self.assertEqual(set(package.source_ignore(tmp, names)), set(names) - {"main.go"})
            if os.name == "posix":
                (Path(tmp) / "outside").symlink_to("/etc/passwd")
                with self.assertRaises(ValueError):
                    package.source_ignore(tmp, ["outside"])

    def test_desktop_rejects_wrong_host_before_build(self):
        with patch.object(package, "host", return_value=("darwin", "arm64")), self.assertRaises(ValueError):
            package.desktop_package("windows")

    def test_desktop_manifest_matches_distributed_installers(self):
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            (directory / "app.dmg").write_bytes(b"installer")
            (directory / "builder-debug.yml").write_text("debug")
            (directory / "unpacked").mkdir()
            (directory / "unpacked/app").write_bytes(b"application")
            package.checksums(directory, installers_only=True)
            self.assertEqual((directory / "SHA256SUMS").read_text(), f"{tasklib.digest(directory / 'app.dmg')}  app.dmg\n")

    def test_preflight_does_not_claim_busy_port(self):
        import socket
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            sock.listen()
            with self.assertRaises(ValueError):
                dev.free_port(sock.getsockname()[1])


class DeploymentTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="deployment with spaces ")
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        (self.directory / "broker").mkdir()
        (self.directory / "backups").mkdir()
        (self.directory / ".env").write_text("COMPOSE_PROJECT_NAME=test-only\n")
        (self.directory / ".database").write_text("sqlite\n")
        (self.directory / "broker/broker.json").write_text(json.dumps({"workspaces": [{"id": "alpha"}, {"id": "beta"}]}))
        self.environment = patch.dict(os.environ, {"CONFIG_DIR": str(self.directory), "WORKSPACE": "", "BACKUP_FILE": "", "SERVICE": "", "WAIT_TIMEOUT": "10"})
        self.environment.start()
        self.addCleanup(self.environment.stop)

    def invoke(self, action):
        with patch.object(sys, "argv", ["deploy.py", action]):
            deploy.main()

    def test_backup_requires_explicit_workspace_when_multiple(self):
        with self.assertRaises(ValueError):
            deploy.Deployment().workspace()

    def test_backup_never_overwrites_or_accepts_path(self):
        with patch.dict(os.environ, {"WORKSPACE": "alpha", "BACKUP_FILE": "../escape.json"}), self.assertRaises(ValueError):
            deploy.Deployment().backup()
        (self.directory / "backups/existing.json").write_text("original")
        with patch.dict(os.environ, {"WORKSPACE": "alpha", "BACKUP_FILE": "existing.json"}), self.assertRaises(ValueError):
            deploy.Deployment().backup()

    def test_restore_refuses_live_broker(self):
        (self.directory / "backups/snapshot.json").write_text("{}")
        with patch.dict(os.environ, {"WORKSPACE": "alpha", "BACKUP_FILE": "snapshot.json"}), patch.object(deploy.Deployment, "compose", return_value=subprocess.CompletedProcess([], 0, "broker\nweb\n")) as call:
            with self.assertRaises(ValueError):
                self.invoke("restore")
            self.assertEqual(call.call_count, 1)

    def test_restore_starts_postgres_only_when_selected(self):
        (self.directory / "backups/snapshot.json").write_text("{}")
        for database in ("sqlite", "postgres"):
            (self.directory / ".database").write_text(database)
            with patch.dict(os.environ, {"WORKSPACE": "alpha", "BACKUP_FILE": "snapshot.json"}), patch.object(deploy.Deployment, "compose", return_value=subprocess.CompletedProcess([], 0, "")) as call:
                self.invoke("restore")
                invocations = [c.args for c in call.call_args_list]
                self.assertEqual(any("postgres" in c for c in invocations), database == "postgres")
                self.assertTrue(any("restore" in c for c in invocations))

    def test_upgrade_backs_up_all_tenants_before_start(self):
        with patch.object(deploy.Deployment, "compose", return_value=subprocess.CompletedProcess([], 0, "")) as call:
            self.invoke("upgrade")
            commands = [c.args for c in call.call_args_list]
            backups = [c for c in commands if "backup" in c]
            self.assertEqual({c[c.index("--workspace") + 1] for c in backups}, {"alpha", "beta"})
            start = next(i for i, c in enumerate(commands) if c[0] == "up")
            self.assertTrue(all(commands.index(c) < start for c in backups))

    def test_upgrade_aborts_if_backup_fails(self):
        with patch.object(deploy.Deployment, "compose") as call, patch.object(deploy.Deployment, "backup", side_effect=ValueError("backup failed")):
            with self.assertRaises(ValueError):
                self.invoke("upgrade")
            self.assertFalse(any(c.args[0] == "up" for c in call.call_args_list))

    def test_down_preserves_volumes(self):
        with patch.object(deploy.Deployment, "compose") as call:
            self.invoke("down")
            call.assert_called_once_with("down")

    def test_compose_preserves_pinned_version_and_accepts_explicit_override(self):
        (self.directory / ".env").write_text("SAFEOPS_VERSION=1.2.3\n")
        with patch.dict(os.environ, {"SAFEOPS_VERSION": ""}), patch.object(deploy, "run") as call:
            deploy.Deployment().compose("ps")
            self.assertNotIn("SAFEOPS_VERSION", call.call_args.kwargs["env"])
        with patch.dict(os.environ, {"SAFEOPS_VERSION": "2.0.0"}), patch.object(deploy, "run") as call:
            deploy.Deployment().compose("ps")
            self.assertEqual(call.call_args.kwargs["env"]["SAFEOPS_VERSION"], "2.0.0")

    def test_compose_uses_literal_directory_and_current_mount(self):
        with patch.object(deploy, "run") as call:
            deploy.Deployment().compose("ps")
            command = call.call_args.args[0]
            self.assertEqual(command[2], self.directory.resolve())
            self.assertNotIn("shell", call.call_args.kwargs)
        # Execute the actual Bash wrapper against a recording Docker substitute.
        docker = self.directory / "docker"
        docker.write_text("#!/usr/bin/env python3\nimport json,os,sys\nprint(json.dumps({'args':sys.argv[1:],'mount':os.environ.get('SAFEOPS_CONFIG_DIR')}))\n")
        docker.chmod(0o755)
        result = subprocess.run(["bash", str(ROOT / "scripts/compose.sh"), str(self.directory), "config", "--quiet"], check=True, capture_output=True, text=True, env=dict(os.environ, PATH=f"{self.directory}{os.pathsep}{os.environ['PATH']}"))
        recorded = json.loads(result.stdout)
        self.assertEqual(recorded["mount"], str(self.directory))
        self.assertEqual(recorded["args"][-2:], ["config", "--quiet"])
        self.assertNotIn(str(ROOT / "deploy/compose.postgres.yaml"), recorded["args"])


if __name__ == "__main__":
    unittest.main()
