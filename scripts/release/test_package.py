"""Check archive contents/checksums with an ELF header fixture, not a release binary."""

import hashlib
from pathlib import Path
import struct
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class ReleasePackageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.binary = self.directory / "fixture"
        ident = b"\x7fELF\x02\x01\x01\x03" + b"\0" * 8
        self.binary.write_bytes(
            struct.pack("<16sHHIQQQIHHHHHH", ident, 2, 62, 1, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0)
        )
        self.binary.chmod(0o755)
        self.output = self.directory / "assets"

    def package(self, tag="v0.1.0", success=True):
        result = subprocess.run(
            ["bash", "scripts/release/package.sh", tag, str(self.binary), str(self.output)],
            cwd=ROOT, text=True, capture_output=True,
        )
        self.assertEqual(result.returncode == 0, success, result.stderr)

    def test_archive_contents_and_checksums(self):
        self.package()
        with tarfile.open(self.output / "burrow_v0.1.0_linux_amd64.tar.gz") as archive:
            files = {m.name for m in archive.getmembers() if m.isfile()}
            prefix = "burrow_v0.1.0_linux_amd64/"
            self.assertEqual(files, {prefix + n for n in ["burrow", "LICENSE", "README.md", "configs/config.yaml"]})
            self.assertEqual(archive.extractfile(prefix + "burrow").read(), self.binary.read_bytes())
            self.assertTrue(archive.getmember(prefix + "burrow").mode & 0o111)
        with tarfile.open(self.output / "burrow_v0.1.0_deploy.tar.gz") as archive:
            files = {m.name for m in archive.getmembers() if m.isfile()}
            prefix = "burrow_v0.1.0_deploy/"
            self.assertEqual(files, {prefix + n for n in ["docker-compose.yml", ".env.example", "LICENSE", "README.md"]})
            self.assertEqual(archive.extractfile(prefix + "docker-compose.yml").read(), (ROOT / "docker-compose.yml").read_bytes())
            environment = archive.extractfile(prefix + ".env.example").read().decode()
            images = [line for line in environment.splitlines() if line.startswith("BURROW_IMAGE=")]
            self.assertEqual(images, ["BURROW_IMAGE=ghcr.io/arkgravity/burrow:v0.1.0"])
            self.assertEqual(archive.extractfile(prefix + "LICENSE").read(), (ROOT / "LICENSE").read_bytes())
        checksums = (self.output / "SHA256SUMS").read_text().splitlines()
        self.assertEqual(len(checksums), 3)
        for line in checksums:
            digest, name = line.split(maxsplit=1)
            self.assertEqual(digest, hashlib.sha256((self.output / name).read_bytes()).hexdigest())

    def test_non_linux_binary_rejected(self):
        self.binary.write_text("#!/bin/sh\nexit 0\n")
        self.package(success=False)
        self.assertFalse(self.output.exists())

    def test_non_executable_rejected(self):
        self.binary.chmod(0o644)
        self.package(success=False)

    def test_invalid_tag_rejected(self):
        self.package(tag="v0.1.0-rc.1", success=False)


if __name__ == "__main__":
    unittest.main()
