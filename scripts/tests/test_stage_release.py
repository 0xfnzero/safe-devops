import hashlib
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from stage_release import collect_verified, expected_assets


class ReleaseAssetsTest(unittest.TestCase):
    def test_all_native_targets_are_distinct(self):
        names = expected_assets("0.4.1")
        self.assertEqual(len(names), 19)
        # electron-builder uses package-format-specific architecture names.
        self.assertIn("safe-devops-0.4.1-linux-x86_64.AppImage", names)
        self.assertIn("safe-devops-0.4.1-linux-amd64.deb", names)
        self.assertIn("safe-devops-0.4.1-win-x64.exe", names)

    def test_checksum_and_completeness_are_required(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            asset = root / "test.zip"
            asset.write_bytes(b"test")
            manifest = root / "SHA256SUMS"
            manifest.write_text(f"{hashlib.sha256(b'test').hexdigest()}  test.zip\n")
            self.assertEqual(set(collect_verified([root], {"test.zip"})), {"test.zip"})
            with self.assertRaises(ValueError):
                collect_verified([root], {"test.zip", "missing.zip"})
            asset.write_bytes(b"tampered")
            with self.assertRaises(ValueError):
                collect_verified([root], {"test.zip"})

    def test_paths_duplicates_and_unknown_assets_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "test.zip").write_bytes(b"test")
            line = f"{hashlib.sha256(b'test').hexdigest()}  test.zip\n"
            manifest = root / "SHA256SUMS"
            for body in (line.replace("test.zip", "../test.zip"), line + line, line.replace("test.zip", "unknown.zip")):
                manifest.write_text(body)
                with self.assertRaises(ValueError):
                    collect_verified([root], {"test.zip"})
