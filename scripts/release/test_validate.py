"""Exercise release eligibility using local Git repositories and mocked GitHub APIs."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("validate.sh").resolve()


def git(directory, *args):
    return subprocess.check_output(
        ["git", "-C", str(directory), *args], stderr=subprocess.DEVNULL, text=True
    ).strip()


class ReleaseValidationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        remote = self.root / "origin"
        remote.mkdir()
        git(remote, "init", "-b", "main")
        git(remote, "config", "user.email", "release-test@example.invalid")
        git(remote, "config", "user.name", "Release test")
        notes = remote / "docs/releases"
        notes.mkdir(parents=True)
        (notes / "v0.1.0.md").write_text("Release notes\n")
        git(remote, "add", ".")
        git(remote, "commit", "-m", "Tested release")
        self.sha = git(remote, "rev-parse", "HEAD")
        git(remote, "tag", "-a", "v0.1.0", "-m", "Test release")
        (remote / "later").write_text("Main can advance after tagging\n")
        git(remote, "add", ".")
        git(remote, "commit", "-m", "Later main commit")
        self.repo = self.root / "checkout"
        git(self.root, "clone", "--quiet", str(remote), str(self.repo))
        git(self.repo, "checkout", "--quiet", "v0.1.0")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        gh = self.bin / "gh"
        gh.write_text(
            "#!/usr/bin/env python3\n"
            "import os, pathlib, sys\n"
            "if os.environ.get('MOCK_API_FAIL'): sys.exit(1)\n"
            "name = 'runs.json' if any('/runs' in a for a in sys.argv) else 'releases.json'\n"
            "print((pathlib.Path(os.environ['MOCK_ROOT']) / name).read_text())\n"
        )
        gh.chmod(0o755)
        self.run_data = {
            "head_sha": self.sha,
            "head_branch": "main",
            "event": "push",
            "status": "completed",
            "conclusion": "success",
        }
        self.releases = []
        self.env = os.environ | {
            "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
            "MOCK_ROOT": str(self.root),
            "GITHUB_REF_TYPE": "tag",
            "GITHUB_REF_NAME": "v0.1.0",
            "GITHUB_SHA": self.sha,
            "GITHUB_REPOSITORY": "example/burrow",
            "GITHUB_OUTPUT": str(self.root / "output"),
        }

    def validate(self, success):
        runs = [] if self.run_data is None else [self.run_data]
        (self.root / "runs.json").write_text(json.dumps({"workflow_runs": runs}))
        (self.root / "releases.json").write_text(json.dumps(self.releases))
        output = self.root / "output"
        output.unlink(missing_ok=True)
        result = subprocess.run(
            ["bash", str(SCRIPT)], cwd=self.repo, env=self.env,
            text=True, capture_output=True,
        )
        self.assertEqual(result.returncode == 0, success, result.stderr)
        if success:
            self.assertIn(f"commit={self.sha}\n", output.read_text())
            self.assertIn("version=v0.1.0\n", output.read_text())
            self.assertIn("build_time=", output.read_text())
        else:
            self.assertFalse(output.exists(), "Rejected releases must not emit outputs")

    def test_success_after_main_advances(self):
        self.validate(True)

    def test_draft_retry(self):
        self.releases = [{"tag_name": "v0.1.0", "draft": True}]
        self.validate(True)

    def test_other_published_version(self):
        self.releases = [{"tag_name": "v0.0.9", "draft": False}]
        self.validate(True)

    def test_published_version_rejected(self):
        self.releases = [{"tag_name": "v0.1.0", "draft": False}]
        self.validate(False)

    def test_invalid_tags(self):
        for tag in ["v0.1", "v01.1.0", "v0.1.0-rc.1", "main", "v0.1.0;echo bad"]:
            with self.subTest(tag=tag):
                self.env["GITHUB_REF_NAME"] = tag
                self.validate(False)

    def test_branch_rejected(self):
        self.env["GITHUB_REF_TYPE"] = "branch"
        self.validate(False)

    def test_wrong_checkout(self):
        self.env["GITHUB_SHA"] = "0" * 40
        self.validate(False)

    def test_wrong_tag(self):
        git(self.repo, "tag", "v0.1.1", "origin/main")
        self.env["GITHUB_REF_NAME"] = "v0.1.1"
        self.validate(False)

    def test_commit_outside_main(self):
        git(self.repo, "config", "user.email", "release-test@example.invalid")
        git(self.repo, "config", "user.name", "Release test")
        git(self.repo, "checkout", "-b", "unmerged")
        (self.repo / "unmerged").write_text("Unmerged change\n")
        git(self.repo, "add", ".")
        git(self.repo, "commit", "-m", "Unmerged")
        self.sha = git(self.repo, "rev-parse", "HEAD")
        git(self.repo, "tag", "-f", "v0.1.0")
        self.env["GITHUB_SHA"] = self.sha
        self.run_data["head_sha"] = self.sha
        self.validate(False)

    def test_ci_boundaries(self):
        for field, values in {
            "head_sha": ["0" * 40],
            "head_branch": ["dev"],
            "event": ["pull_request", "workflow_dispatch"],
            "status": ["in_progress", "queued"],
            "conclusion": ["failure", "cancelled", "skipped", None],
        }.items():
            previous = self.run_data[field]
            for value in values:
                with self.subTest(field=field, value=value):
                    self.run_data[field] = value
                    self.validate(False)
            self.run_data[field] = previous

    def test_missing_ci(self):
        self.run_data = None
        self.validate(False)

    def test_api_failure(self):
        self.env["MOCK_API_FAIL"] = "1"
        self.validate(False)

    def test_missing_notes(self):
        (self.repo / "docs/releases/v0.1.0.md").unlink()
        self.validate(False)


if __name__ == "__main__":
    unittest.main()
