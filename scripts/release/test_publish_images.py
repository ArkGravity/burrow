"""Exercise publication failures with fake Docker/crane CLIs; no registry writes."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
COMMIT = "1" * 40
MOCK_DOCKER = r'''
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
scenario = os.environ.get("SCENARIO", "")
with open(os.environ["DOCKER_LOG"], "a") as log:
    log.write(json.dumps({"tool": "docker", "args": args, "anonymous": bool(os.environ.get("DOCKER_CONFIG"))}) + "\n")
def config(arch):
    return "sha256:" + ("a" if arch == "amd64" else "b") * 64
def digest(arch, hub=False):
    value = "c" if arch == "amd64" else "d"
    if scenario == "platform-digest-mismatch" and hub and arch == "arm64":
        value = "f"
    return "sha256:" + value * 64
if args[:1] == ["load"]:
    assert Path(args[2]).is_file()
elif args[:2] == ["image", "inspect"]:
    arch = args[2].rsplit("-", 1)[1]
    actual_arch = "amd64" if scenario == "wrong-architecture" else arch
    commit = "wrong" if scenario == "wrong-commit" else os.environ["GITHUB_SHA"]
    print(json.dumps({"Id": config(arch), "Os": "linux", "Architecture": actual_arch,
        "Config": {"Labels": {"org.opencontainers.image.revision": commit}}}))
elif args[:3] == ["buildx", "imagetools", "create"]:
    assert len(args) == 7
    assert all("@sha256:" in value for value in args[-2:])
elif args[:3] == ["buildx", "imagetools", "inspect"]:
    image = args[3]
    hub = image.startswith("docker.io/")
    arch = next((a for a, value in [("amd64", "c"), ("arm64", "d")]
        if image.endswith("@sha256:" + value * 64)), None)
    if scenario == "anonymous-denied" and os.environ.get("DOCKER_CONFIG"):
        sys.exit(1)
    if args[4] == "--raw":
        if arch:
            value = config(arch) if scenario != "wrong-config" else "sha256:" + "f" * 64
            print(json.dumps({"config": {"digest": value}}))
        else:
            platforms = [{"platform": {"os": "linux", "architecture": a}, "digest": digest(a, hub)}
                for a in ["amd64", "arm64"]]
            if scenario == "missing-platform":
                platforms.pop()
            if scenario == "extra-platform":
                platforms.append({"platform": {"os": "unknown", "architecture": "unknown"}, "digest": config("amd64")})
            if scenario == "wrong-platform-digest":
                platforms[1]["digest"] = config("arm64")
            if scenario == "wrong-os":
                platforms[1]["platform"]["os"] = "windows"
            print(json.dumps({"schemaVersion": 2, "manifests": platforms}))
    else:
        assert args[4:] == ["--format", "{{json .Manifest}}"]
        value = digest(arch, hub) if arch else "sha256:" + ("f" if scenario == "index-digest-mismatch" and hub else "e") * 64
        print(json.dumps({"digest": value}))
else:
    raise AssertionError("Unexpected Docker command: " + repr(args))
'''

MOCK_CRANE = r'''
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
scenario = os.environ.get("SCENARIO", "")
with open(os.environ["DOCKER_LOG"], "a") as log:
    log.write(json.dumps({"tool": "crane", "args": args, "anonymous": False}) + "\n")
if args[:2] == ["digest", "--tarball"]:
    assert len(args) == 3
    archive = Path(args[2])
    assert archive.is_file()
    arch = archive.stem.removeprefix("image-")
    assert arch in ["amd64", "arm64"]
    value = "sha256:" + ("c" if arch == "amd64" else "d") * 64
    print("invalid" if scenario == "invalid-local-digest" else value)
elif args[:1] == ["push"]:
    assert len(args) == 3
    assert Path(args[1]).is_file()
    arch = Path(args[1]).stem.removeprefix("image-")
    assert args[2].endswith("@sha256:" + ("c" if arch == "amd64" else "d") * 64)
    if scenario == "push-failed":
        sys.exit(1)
    print(args[2])
else:
    raise AssertionError("Unexpected crane command: " + repr(args))
'''


class PublishImageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.images = self.directory / "images"
        self.images.mkdir()
        for arch in ["amd64", "arm64"]:
            (self.images / f"image-{arch}.tar").write_bytes(b"saved image fixture")
        for name, mock in [("docker", MOCK_DOCKER), ("crane", MOCK_CRANE)]:
            executable = self.directory / name
            executable.write_text(f"#!{sys.executable}\n" + mock)
            executable.chmod(0o755)
        self.log = self.directory / "docker.log"
        self.output = self.directory / "IMAGES.txt"
        self.env = {**os.environ, "PATH": str(self.directory) + os.pathsep + os.environ["PATH"],
                    "GITHUB_SHA": COMMIT, "GITHUB_REPOSITORY": "ArkGravity/burrow",
                    "DOCKERHUB_NAMESPACE": "logic3579", "DOCKER_LOG": str(self.log)}
        self.env.pop("DOCKER_CONFIG", None)

    def publish(self, scenario="", tag="v0.2.0", kind="release", success=True):
        result = subprocess.run(
            ["bash", "scripts/release/publish-images.sh", tag, kind, str(self.images), str(self.output)],
            cwd=ROOT, env={**self.env, "SCENARIO": scenario}, text=True, capture_output=True,
        )
        self.assertEqual(result.returncode == 0, success, result.stderr)
        if not success:
            self.assertFalse(self.output.exists())

    def commands(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_both_registries_and_platforms_without_rebuilding(self):
        self.publish()
        lines = [line.split() for line in self.output.read_text().splitlines()]
        self.assertEqual(len(lines), 6)
        self.assertEqual(lines[0], ["ghcr.io/arkgravity/burrow:v0.2.0", "sha256:" + "e" * 64])
        self.assertEqual(lines[1], ["docker.io/logic3579/burrow:v0.2.0", lines[0][1]])
        self.assertEqual([line[1] for line in lines[2:]], ["sha256:" + x * 64 for x in ["c", "d", "c", "d"]])
        for reference, digest in lines[2:]:
            self.assertTrue(reference.endswith("@" + digest))
        commands = self.commands()
        self.assertEqual(sum(c["args"][0] == "push" for c in commands), 4)
        self.assertTrue(all(c["tool"] == "crane" and "@sha256:" in c["args"][2]
                            for c in commands if c["args"][0] == "push"))
        self.assertFalse(any(c["args"][0] == "tag" for c in commands))
        self.assertEqual(sum(c["args"][:3] == ["buildx", "imagetools", "create"] for c in commands), 2)
        self.assertEqual([c["args"][4] for c in commands if c["args"][:3] == ["buildx", "imagetools", "create"]],
                         ["ghcr.io/arkgravity/burrow:v0.2.0", "docker.io/logic3579/burrow:v0.2.0"])
        self.assertEqual(sum(c["anonymous"] for c in commands), 4)
        first_push = next(i for i, c in enumerate(commands) if c["args"][0] == "push")
        self.assertEqual(sum(c["args"][:2] == ["image", "inspect"] for c in commands[:first_push]), 2)
        self.assertEqual(sum(c["args"][:2] == ["digest", "--tarball"] for c in commands[:first_push]), 2)

    def test_main_image_publication(self):
        self.publish(tag="main-1111111", kind="ci")
        self.assertIn("burrow:ci-arm64", str(self.commands()))
        self.assertIn("burrow:main-1111111", self.output.read_text())

    def test_missing_image_prevents_any_registry_write(self):
        (self.images / "image-arm64.tar").unlink()
        self.publish(success=False)
        self.assertEqual(self.commands(), [])

    def test_wrong_local_architecture_or_commit_prevents_any_registry_write(self):
        for scenario in ["wrong-architecture", "wrong-commit", "invalid-local-digest"]:
            with self.subTest(scenario=scenario):
                self.publish(scenario, success=False)
                self.assertFalse(any(c["args"][0] == "push" for c in self.commands()))

    def test_remote_validation_failures(self):
        for scenario in ["push-failed", "wrong-config", "missing-platform", "extra-platform", "wrong-os",
                         "wrong-platform-digest", "platform-digest-mismatch",
                         "index-digest-mismatch", "anonymous-denied"]:
            with self.subTest(scenario=scenario):
                self.publish(scenario, success=False)

    def test_invalid_tag_prevents_any_registry_write(self):
        self.publish(tag="latest", success=False)
        self.assertEqual(self.commands(), [])


if __name__ == "__main__":
    unittest.main()
