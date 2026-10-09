"""Reuse successful PR regression only when its tested tree matches main exactly."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


REGRESSION_JOBS = ("web", "backend-quality", "backend-test", "browser")
ARTIFACT = "ci-verification"


def command(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.PIPE, timeout=60).strip()


def api(repository, endpoint, **query):
    args = ["gh", "api", "--method", "GET", f"repos/{repository}/{endpoint}"]
    for key, value in query.items():
        args.extend(["-f", f"{key}={value}"])
    return json.loads(command(*args))


def matching_pr_run(repository, commit, tree):
    """Missing/expired evidence raises or returns None; the caller runs full CI."""
    pulls = api(repository, f"commits/{commit}/pulls", per_page=100)
    for associated in pulls:
        number = associated["number"]
        pull = api(repository, f"pulls/{number}")
        if not (
            pull.get("merged") is True
            and pull.get("merge_commit_sha") == commit
            and pull["base"]["ref"] == "main"
            and pull["base"]["repo"]["full_name"] == repository
        ):
            continue
        head = pull["head"]["sha"]
        runs = api(repository, "actions/workflows/ci.yml/runs",
                   event="pull_request", head_sha=head, per_page=100)["workflow_runs"]
        for run in runs:
            if not (
                run["event"] == "pull_request"
                and run["head_sha"] == head
                and run["status"] == "completed"
                and run["conclusion"] == "success"
                and run["repository"]["full_name"] == repository
                and run["path"] == ".github/workflows/ci.yml"
            ):
                continue
            with tempfile.TemporaryDirectory() as directory:
                command("gh", "run", "download", str(run["id"]), "--repo", repository,
                        "--name", ARTIFACT, "--dir", directory)
                record = json.loads((Path(directory) / "verification.json").read_text())
            expected = {
                "schema": 1, "repository": repository, "pull_request": number,
                "head_sha": head, "tree": tree, "run_id": run["id"],
                "run_attempt": run["run_attempt"],
            }
            if all(record.get(key) == value for key, value in expected.items()):
                return run["id"]
    return None


def plan():
    full = True
    reason = "PR, dev and manual runs require full regression."
    if os.environ["GITHUB_EVENT_NAME"] == "push" and os.environ["GITHUB_REF"] == "refs/heads/main":
        reason = "No successful PR verification matching this commit's tree; running full regression."
        try:
            commit = os.environ["GITHUB_SHA"]
            if command("git", "rev-parse", "HEAD") != commit:
                raise ValueError("Checkout does not match workflow commit")
            tree = command("git", "rev-parse", "HEAD^{tree}")
            run_id = matching_pr_run(os.environ["GITHUB_REPOSITORY"], commit, tree)
            if run_id is not None:
                full = False
                reason = f"Reusing full PR regression from run {run_id}; tested tree matches main."
        except (subprocess.SubprocessError, OSError, ValueError, KeyError, TypeError, AttributeError):
            reason = "PR verification unavailable or invalid; running full regression."
    print(reason)
    with open(os.environ["GITHUB_OUTPUT"], "a") as output:
        output.write(f"full_regression={str(full).lower()}\n")
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as summary:
            summary.write(reason + "\n")


def verify(needs, event, ref):
    for job in ("plan", "image-build"):
        if needs[job]["result"] != "success":
            raise ValueError(f"Required job {job} did not succeed")
    full = needs["plan"]["outputs"].get("full_regression")
    if full not in ("true", "false"):
        raise ValueError("Missing or invalid regression plan")
    if full == "false" and (event != "push" or ref != "refs/heads/main"):
        raise ValueError("Only a verified main push can reuse PR regression")
    expected = "success" if full == "true" else "skipped"
    for job in REGRESSION_JOBS:
        if needs[job]["result"] != expected:
            raise ValueError(f"Job {job} must be {expected} for this regression plan")


def record():
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
    data = {
        "schema": 1,
        "repository": os.environ["GITHUB_REPOSITORY"],
        "run_id": int(os.environ["GITHUB_RUN_ID"]),
        "run_attempt": int(os.environ["GITHUB_RUN_ATTEMPT"]),
        "pull_request": event["number"],
        "head_sha": event["pull_request"]["head"]["sha"],
        "tested_commit": command("git", "rev-parse", "HEAD"),
        "tree": command("git", "rev-parse", "HEAD^{tree}"),
    }
    Path("verification.json").write_text(json.dumps(data, indent=2) + "\n")


if __name__ == "__main__":
    action = sys.argv[1]
    if action == "plan":
        plan()
    elif action == "verify":
        verify(json.loads(os.environ["NEEDS_JSON"]), os.environ["GITHUB_EVENT_NAME"],
               os.environ["GITHUB_REF"])
    elif action == "record" and os.environ["GITHUB_EVENT_NAME"] == "pull_request":
        record()
    else:
        raise SystemExit("Expected plan, verify or PR record")
