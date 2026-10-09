"""Check PR provenance, safe fallback and the required CI result boundaries."""

import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import verification


class VerificationTests(unittest.TestCase):
    def setUp(self):
        self.repository = "example/burrow"
        self.commit = "1" * 40
        self.head = "2" * 40
        self.tree = "3" * 40
        self.pull = {
            "number": 7, "merged": True, "merge_commit_sha": self.commit,
            "base": {"ref": "main", "repo": {"full_name": self.repository}},
            "head": {"sha": self.head},
        }
        self.run = {
            "id": 42, "run_attempt": 2, "event": "pull_request", "head_sha": self.head,
            "status": "completed", "conclusion": "success",
            "repository": {"full_name": self.repository}, "path": ".github/workflows/ci.yml",
        }
        self.record = {
            "schema": 1, "repository": self.repository, "pull_request": 7,
            "run_id": 42, "run_attempt": 2, "head_sha": self.head, "tree": self.tree,
        }
        self.downloads = []

    def api(self, repository, endpoint, **query):
        self.assertEqual(repository, self.repository)
        if endpoint == f"commits/{self.commit}/pulls":
            return [{"number": 7}]
        if endpoint == "pulls/7":
            return self.pull
        self.assertEqual(endpoint, "actions/workflows/ci.yml/runs")
        self.assertEqual(query["event"], "pull_request")
        self.assertEqual(query["head_sha"], self.head)
        return {"workflow_runs": [self.run]}

    def command(self, *args):
        if args[:2] == ("git", "rev-parse"):
            return self.tree if args[2] == "HEAD^{tree}" else self.commit
        self.assertEqual(args[:3], ("gh", "run", "download"))
        self.assertEqual(args[3], "42")
        self.assertIn(self.repository, args)
        self.downloads.append(args)
        directory = Path(args[args.index("--dir") + 1])
        (directory / "verification.json").write_text(json.dumps(self.record))
        return ""

    def match(self):
        with patch.object(verification, "api", self.api), patch.object(verification, "command", self.command):
            return verification.matching_pr_run(self.repository, self.commit, self.tree)

    def test_squash_merge_reuses_identical_tested_tree(self):
        self.assertNotEqual(self.head, self.commit)
        self.assertEqual(self.match(), 42)

    def test_wrong_pull_cannot_authorize_reuse(self):
        changes = [
            {"merged": False}, {"merge_commit_sha": "4" * 40},
            {"base": {"ref": "dev", "repo": {"full_name": self.repository}}},
            {"base": {"ref": "main", "repo": {"full_name": "other/burrow"}}},
        ]
        original = copy.deepcopy(self.pull)
        for change in changes:
            with self.subTest(change=change):
                self.pull = original | change
                self.assertIsNone(self.match())
        self.assertEqual(self.downloads, [])

    def test_wrong_or_incomplete_run_cannot_authorize_reuse(self):
        original = copy.deepcopy(self.run)
        for field, value in [
            ("event", "push"), ("head_sha", self.commit), ("status", "in_progress"),
            ("conclusion", "failure"), ("conclusion", "cancelled"), ("conclusion", "skipped"),
            ("repository", {"full_name": "other/burrow"}), ("path", ".github/workflows/other.yml"),
        ]:
            with self.subTest(field=field, value=value):
                self.run = original | {field: value}
                self.assertIsNone(self.match())
        self.assertEqual(self.downloads, [])

    def test_changed_tree_and_mismatched_records_cannot_authorize_reuse(self):
        original = self.record.copy()
        for field, value in [
            ("tree", "4" * 40), ("repository", "other/burrow"), ("pull_request", 8),
            ("head_sha", self.commit), ("run_id", 43), ("run_attempt", 1), ("schema", 0),
        ]:
            with self.subTest(field=field):
                self.record = original | {field: value}
                self.assertIsNone(self.match())

    def run_plan(self, event="push", ref="refs/heads/main", failure=None, run_id=42):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "output"
            env = {"GITHUB_EVENT_NAME": event, "GITHUB_REF": ref,
                   "GITHUB_SHA": self.commit, "GITHUB_REPOSITORY": self.repository,
                   "GITHUB_OUTPUT": str(output)}
            with patch.dict(os.environ, env, clear=True), patch.object(verification, "api", self.api), \
                    patch.object(verification, "command", self.command), \
                    patch.object(verification, "matching_pr_run", side_effect=failure, return_value=run_id):
                verification.plan()
            return output.read_text()

    def test_plan_reuses_only_main_push(self):
        self.assertEqual(self.run_plan(), "full_regression=false\n")
        for event, ref in [("pull_request", "refs/pull/7/merge"), ("push", "refs/heads/dev"),
                           ("workflow_dispatch", "refs/heads/main")]:
            with self.subTest(event=event, ref=ref):
                self.assertEqual(self.run_plan(event, ref), "full_regression=true\n")

    def test_unavailable_or_invalid_evidence_requires_full_regression(self):
        for failure in [subprocess.CalledProcessError(1, "gh"), subprocess.TimeoutExpired("gh", 60),
                        FileNotFoundError("expired artifact"), ValueError("invalid JSON"),
                        KeyError("missing field"), TypeError("invalid response"),
                        AttributeError("invalid record shape")]:
            with self.subTest(failure=failure):
                self.assertEqual(self.run_plan(failure=failure), "full_regression=true\n")

    def test_no_matching_pr_record_requires_full_regression(self):
        self.assertEqual(self.run_plan(run_id=None), "full_regression=true\n")

    def test_checkout_mismatch_requires_full_regression(self):
        with patch.object(self, "command", return_value="0" * 40):
            self.assertEqual(self.run_plan(), "full_regression=true\n")

    def test_record_uses_actual_checkout_tree(self):
        with tempfile.TemporaryDirectory() as directory:
            event = Path(directory) / "event.json"
            event.write_text(json.dumps({"number": 7, "pull_request": {"head": {"sha": self.head}}}))
            env = {"GITHUB_EVENT_PATH": str(event), "GITHUB_REPOSITORY": self.repository,
                   "GITHUB_RUN_ID": "42", "GITHUB_RUN_ATTEMPT": "2"}
            previous = Path.cwd()
            try:
                os.chdir(directory)
                with patch.dict(os.environ, env), patch.object(verification, "command", self.command):
                    verification.record()
                data = json.loads(Path("verification.json").read_text())
                self.assertEqual(data, self.record | {"tested_commit": self.commit})
            finally:
                os.chdir(previous)


class AggregationTests(unittest.TestCase):
    def needs(self, full):
        return {
            "plan": {"result": "success", "outputs": {"full_regression": str(full).lower()}},
            "image-build": {"result": "success"},
        } | {job: {"result": "success" if full else "skipped"} for job in verification.REGRESSION_JOBS}

    def test_full_regression_and_verified_main(self):
        verification.verify(self.needs(True), "pull_request", "refs/pull/7/merge")
        verification.verify(self.needs(True), "workflow_dispatch", "refs/heads/main")
        verification.verify(self.needs(False), "push", "refs/heads/main")

    def test_required_failures_cancellation_and_unexpected_skips_block_publication(self):
        for full in [True, False]:
            for job in self.needs(full):
                expected = self.needs(full)[job]["result"]
                for result in ["success", "failure", "cancelled", "skipped"]:
                    if result == expected:
                        continue
                    with self.subTest(full=full, job=job, result=result):
                        needs = self.needs(full)
                        needs[job]["result"] = result
                        with self.assertRaises(ValueError):
                            verification.verify(needs, "push", "refs/heads/main")

    def test_missing_or_invalid_plan_blocks_publication(self):
        for output in [None, "", "FALSE", "anything"]:
            needs = self.needs(False)
            needs["plan"]["outputs"]["full_regression"] = output
            with self.assertRaises(ValueError):
                verification.verify(needs, "push", "refs/heads/main")

    def test_pr_dev_and_manual_cannot_skip_regression(self):
        for event, ref in [("pull_request", "refs/pull/7/merge"), ("push", "refs/heads/dev"),
                           ("workflow_dispatch", "refs/heads/main")]:
            with self.subTest(event=event, ref=ref), self.assertRaises(ValueError):
                verification.verify(self.needs(False), event, ref)


if __name__ == "__main__":
    unittest.main()
