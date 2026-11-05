#!/usr/bin/env python3
"""Trusted main-only release gate and deterministic first-parent versioning."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess

REPOSITORY = "AlexandreZanata/Abastevo"
WORKFLOWS = {".github/workflows/quick.yml", ".github/workflows/ci.yml", ".github/workflows/backend.yml"}
SHA = re.compile(r"^[0-9a-f]{40}$")


def version_for(config, first_parents):
    if config["epoch"] not in first_parents:
        raise ValueError("release epoch is not on main's first-parent history")
    deliveries = len(first_parents) - first_parents.index(config["epoch"]) - 1
    if deliveries < 1 or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", config["base_version"]):
        raise ValueError("release must follow the explicit baseline")
    major, minor, patch = map(int, config["base_version"].split("."))
    code = config["base_code"] + deliveries
    if not 1 <= code <= 2100000000:
        raise ValueError("Android version code exhausted")
    return {"name": f"{major}.{minor}.{patch + deliveries}", "code": code}


def trusted_target(event, event_name, repository, ref, current_main):
    if repository != REPOSITORY or event.get("repository", {}).get("full_name") != repository or ref != "refs/heads/main":
        raise ValueError("only this repository main can release")
    if event_name == "workflow_dispatch":
        sha = event.get("inputs", {}).get("sha")
    elif event_name == "workflow_run":
        run = event.get("workflow_run", {})
        if (run.get("head_repository", {}).get("full_name") != repository or run.get("head_branch") != "main" or
            run.get("event") != "push" or run.get("name") not in {"quick", "CI", "backend"} or run.get("conclusion") != "success"):
            raise ValueError("untrusted or unsuccessful trigger")
        sha = run.get("head_sha")
    else:
        raise ValueError("unsupported release trigger")
    if not isinstance(sha, str) or not SHA.fullmatch(sha) or sha != current_main:
        raise ValueError("target must be the current full main SHA")
    return sha


def gate_ready(runs, sha, repository):
    for path in WORKFLOWS:
        candidates = [r for r in runs if r.get("path") == path and r.get("event") == "push" and
                      r.get("head_branch") == "main" and r.get("head_sha") == sha and
                      r.get("head_repository", {}).get("full_name") == repository]
        if not candidates:
            return False
        latest = max(candidates, key=lambda r: (r.get("id", 0), r.get("run_attempt", 1)))
        if latest.get("status") != "completed" or latest.get("conclusion") != "success":
            return False
    return True


def gh(path):
    return json.loads(subprocess.check_output(["gh", "api", path], text=True))


def workflow_runs(sha):
    runs = []
    for page in range(1, 101):
        batch = gh(f"repos/{REPOSITORY}/actions/runs?head_sha={sha}&per_page=100&page={page}")["workflow_runs"]
        runs.extend(batch)
        if len(batch) < 100:
            return runs
    raise ValueError("workflow pagination budget exceeded")


def assert_ready(sha):
    if not SHA.fullmatch(sha) or gh(f"repos/{REPOSITORY}/git/ref/heads/main")["object"]["sha"] != sha:
        raise ValueError("release target is no longer current main")
    if not gate_ready(workflow_runs(sha), sha, REPOSITORY):
        raise ValueError("current main workflows no longer pass")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=["gate", "version", "assert-ready"])
    parser.add_argument("--sha")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    if args.command == "gate":
        event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
        repository = os.environ["GITHUB_REPOSITORY"]
        current = gh(f"repos/{REPOSITORY}/git/ref/heads/main")["object"]["sha"]
        sha = trusted_target(event, os.environ["GITHUB_EVENT_NAME"], repository, os.environ["GITHUB_REF"], current)
        runs = workflow_runs(sha)
        ready = gate_ready(runs, sha, repository)
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
            output.write(f"sha={sha}\nready={str(ready).lower()}\n")
        print("All current-main release checks passed" if ready else "Release waits for all three current-main push workflows")
    elif args.command == "assert-ready":
        assert_ready(args.sha or "")
    else:
        if not args.sha or not SHA.fullmatch(args.sha):
            raise ValueError("full release SHA required")
        config = json.loads(Path("release/android/channel.json").read_text())
        parents = subprocess.check_output(["git", "rev-list", "--first-parent", "--reverse", args.sha], text=True).splitlines()
        version = version_for(config, parents)
        result = {"schema": 1, "commit": args.sha, "channel": config["channel"], "application_id": config["application_id"],
                  "version_name": version["name"], "version_code": version["code"], "tag": f"android-v{version['name']}"}
        raw = json.dumps(result, indent=2) + "\n"
        if args.output:
            args.output.write_text(raw)
        else:
            print(raw, end="")


if __name__ == "__main__":
    main()
