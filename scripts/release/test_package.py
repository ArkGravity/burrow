"""Check release archives/checksums using ELF fixtures, not production binaries."""

import hashlib
from pathlib import Path
import struct
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
TAG = "v0.2.0"


class ReleasePackageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.inputs = self.directory / "inputs"
        self.output = self.directory / "assets"
        self.binaries = {}
        for arch, machine in [("amd64", 62), ("arm64", 183)]:
            binary = self.directory / arch
            ident = b"\x7fELF\x02\x01\x01\x03" + b"\0" * 8
            binary.write_bytes(
                struct.pack("<16sHHIQQQIHHHHHH", ident, 2, machine, 1, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0)
            )
            binary.chmod(0o755)
            self.binaries[arch] = binary

    def package(self, arch="amd64", tag=TAG, binary=None, success=True):
        result = subprocess.run(
            ["bash", "scripts/release/package.sh", tag, arch,
             str(binary or self.binaries[arch]), str(self.inputs)],
            cwd=ROOT, text=True, capture_output=True,
        )
        self.assertEqual(result.returncode == 0, success, result.stderr)

    def assemble(self, success=True):
        result = subprocess.run(
            ["bash", "scripts/release/assemble.sh", TAG, str(self.inputs), str(self.output)],
            cwd=ROOT, text=True, capture_output=True,
        )
        self.assertEqual(result.returncode == 0, success, result.stderr)

    def check_checksums(self, directory, name, count):
        checksums = (directory / name).read_text().splitlines()
        self.assertEqual(len(checksums), count)
        for line in checksums:
            digest, filename = line.split(maxsplit=1)
            self.assertEqual(digest, hashlib.sha256((directory / filename).read_bytes()).hexdigest())

    def test_both_architecture_archives_preserve_binaries(self):
        for arch in self.binaries:
            with self.subTest(arch=arch):
                self.package(arch)
                with tarfile.open(self.inputs / f"burrow_{TAG}_linux_{arch}.tar.gz") as archive:
                    files = {m.name for m in archive.getmembers() if m.isfile()}
                    prefix = f"burrow_{TAG}_linux_{arch}/"
                    self.assertEqual(files, {prefix + n for n in ["burrow", "LICENSE", "README.md", "configs/config.yaml"]})
                    self.assertEqual(archive.extractfile(prefix + "burrow").read(), self.binaries[arch].read_bytes())
                    self.assertTrue(archive.getmember(prefix + "burrow").mode & 0o111)
                    readme = archive.extractfile(prefix + "README.md").read().decode()
                    self.assertIn(TAG, readme)
                    self.assertNotIn("@VERSION@", readme)
                self.check_checksums(self.inputs, f"SHA256SUMS-{arch}", 1)
        self.assertEqual(len(list(self.inputs.iterdir())), 4)

    def test_shared_attachments_and_checksums(self):
        for arch in self.binaries:
            self.package(arch)
        self.assemble()
        with tarfile.open(self.output / f"burrow_{TAG}_deploy.tar.gz") as archive:
            files = {m.name for m in archive.getmembers() if m.isfile()}
            prefix = f"burrow_{TAG}_deploy/"
            self.assertEqual(files, {prefix + n for n in ["docker-compose.yml", ".env.example", "LICENSE", "README.md"]})
            self.assertEqual(archive.extractfile(prefix + "docker-compose.yml").read(), (ROOT / "docker-compose.yml").read_bytes())
            environment = archive.extractfile(prefix + ".env.example").read().decode()
            images = [line for line in environment.splitlines() if line.startswith("BURROW_IMAGE=")]
            self.assertEqual(images, [f"BURROW_IMAGE=ghcr.io/arkgravity/burrow:{TAG}"])
            self.assertEqual(archive.extractfile(prefix + "LICENSE").read(), (ROOT / "LICENSE").read_bytes())
            self.assertEqual(archive.extractfile(prefix + "README.md").read(), (self.output / "INSTALL.md").read_bytes())
        for arch in self.binaries:
            name = f"burrow_{TAG}_linux_{arch}.tar.gz"
            self.assertEqual((self.output / name).read_bytes(), (self.inputs / name).read_bytes())
        self.assertEqual(len(list(self.output.iterdir())), 5)
        self.check_checksums(self.output, "SHA256SUMS", 4)

    def test_wrong_architecture_rejected(self):
        for arch, binary in [("arm64", self.binaries["amd64"]), ("amd64", self.binaries["arm64"])]:
            with self.subTest(arch=arch):
                self.package(arch, binary=binary, success=False)
        self.assertFalse(self.inputs.exists())

    def test_unsupported_architecture_rejected(self):
        self.package("armv7", binary=self.binaries["arm64"], success=False)
        self.assertFalse(self.inputs.exists())

    def test_non_linux_binary_rejected(self):
        self.binaries["amd64"].write_text("#!/bin/sh\nexit 0\n")
        self.package(success=False)
        self.assertFalse(self.inputs.exists())

    def test_non_executable_rejected(self):
        self.binaries["amd64"].chmod(0o644)
        self.package(success=False)

    def test_invalid_tag_rejected(self):
        self.package(tag="v0.2.0-rc.1", success=False)

    def test_missing_architecture_rejected(self):
        self.package()
        self.assemble(success=False)
        self.assertFalse(self.output.exists())

    def test_corrupt_archive_rejected(self):
        for arch in self.binaries:
            self.package(arch)
        (self.inputs / f"burrow_{TAG}_linux_arm64.tar.gz").write_bytes(b"corrupt")
        self.assemble(success=False)
        self.assertFalse(self.output.exists())

    def test_checksum_for_wrong_filename_rejected(self):
        for arch in self.binaries:
            self.package(arch)
        (self.inputs / "SHA256SUMS-arm64").write_text((self.inputs / "SHA256SUMS-amd64").read_text())
        self.assemble(success=False)
        self.assertFalse(self.output.exists())


if __name__ == "__main__":
    unittest.main()
