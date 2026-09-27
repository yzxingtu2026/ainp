#!/usr/bin/env python3
"""Create and push one immutable AINP release tag after fail-closed checks."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from scripts.deployment import project

PROJECT = project.load()
REPOSITORY = PROJECT["repository"]
REQUIRED_CHECKS = set(PROJECT["required_checks"])
DEFAULT_BRANCH = PROJECT["default_branch"]
VERSION_PATTERN = re.compile(r"^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$")
ReleaseError = project.ProjectError


def run(*args: str, check: bool = True) -> str:
    result = subprocess.run(args, check=False, text=True, encoding="utf-8", errors="replace", capture_output=True)
    if check and result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or f"exit {result.returncode}"
        raise ReleaseError(f"{' '.join(args)}: {detail}")
    return result.stdout.strip()


def validate_version(value: str) -> str:
    if not VERSION_PATTERN.fullmatch(value):
        raise ReleaseError("version must match vMAJOR.MINOR.PATCH")
    return value


def platform_tag(version: str) -> str:
    validate_version(version)
    return f"platform/{version}"


def validate_remote(value: str) -> None:
    project.validate_repository(PROJECT, value, os.environ.get("GITHUB_REPOSITORY"))


def successful_checks(payload: dict) -> set[str]:
    return {
        item.get("name", "")
        for item in payload.get("check_runs", [])
        if item.get("conclusion") == "success" and item.get("name") in REQUIRED_CHECKS
    }


def merged_pull_request_head_sha(merge_sha: str) -> str:
    """Resolve the merged PR head whose result produced ``merge_sha``.

    Full CI runs on pull requests only. A squash merge creates a new ``main``
    commit, so the merge commit itself has no duplicate PR check runs. Release
    validation must follow that merge commit back to the exact PR head and
    inspect the checks that authorized the merge.
    """

    payload = json.loads(
        run(
            "gh",
            "api",
            f"repos/{REPOSITORY}/commits/{merge_sha}/pulls?per_page=100",
        )
    )
    candidates = [
        item
        for item in payload
        if item.get("base", {}).get("ref") == DEFAULT_BRANCH
        and item.get("merged_at")
        and item.get("merge_commit_sha") == merge_sha
        and item.get("head", {}).get("sha")
    ]
    if len(candidates) != 1:
        raise ReleaseError(
            f"could not resolve exactly one merged {DEFAULT_BRANCH} PR for {merge_sha}"
        )
    return candidates[0]["head"]["sha"]


def preflight(version: str) -> tuple[str, str]:
    validate_version(version)
    root = run("git", "rev-parse", "--show-toplevel")
    if Path.cwd().resolve() != Path(root).resolve():
        raise ReleaseError("run the release command from the repository root")
    if run("git", "branch", "--show-current") != DEFAULT_BRANCH:
        raise ReleaseError(f"release must run from {DEFAULT_BRANCH}")
    if run("git", "status", "--porcelain=v1", "--untracked-files=all"):
        raise ReleaseError("working tree, index, or untracked files are not clean")
    remote = run("git", "config", "--get", "remote.origin.url")
    validate_remote(remote)
    run("gh", "auth", "status", "--hostname", "github.com")
    run("git", "fetch", "origin", DEFAULT_BRANCH, "--tags", "--prune")
    head = run("git", "rev-parse", "HEAD")
    if head != run("git", "rev-parse", f"origin/{DEFAULT_BRANCH}"):
        raise ReleaseError(f"HEAD must exactly match origin/{DEFAULT_BRANCH}")
    for tag in project.tags(PROJECT, version):
        if run("git", "tag", "--list", tag):
            raise ReleaseError(f"local tag already exists: {tag}")
        if run("git", "ls-remote", "--tags", "origin", f"refs/tags/{tag}"):
            raise ReleaseError(f"remote tag already exists: {tag}")
    ci_head = merged_pull_request_head_sha(head)
    payload = json.loads(run("gh", "api", f"repos/{REPOSITORY}/commits/{ci_head}/check-runs?per_page=100"))
    missing = REQUIRED_CHECKS - successful_checks(payload)
    if missing:
        raise ReleaseError(f"PR CI is not successful for {ci_head}: missing {', '.join(sorted(missing))}")
    oss_bucket = run("gh", "variable", "get", "OSS_BUCKET", "--env", "release", check=False)
    if not oss_bucket:
        raise ReleaseError("release environment variable OSS_BUCKET is not configured")
    return head, oss_bucket


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("version", type=validate_version)
    parser.add_argument("--yes", action="store_true", help="skip interactive confirmation")
    args = parser.parse_args()
    try:
        commit, oss_bucket = preflight(args.version)
        print(f"Version: {args.version}\nCommit: {commit}\nRepository: {REPOSITORY}\nArtifact bucket: {oss_bucket}")
        if not args.yes and input("Push immutable annotated tag? [y/N] ").strip().lower() not in {"y", "yes"}:
            raise ReleaseError("release cancelled")
        release_tags = project.tags(PROJECT, args.version)
        for tag in release_tags:
            run("git", "tag", "--annotate", tag, "--message", f"Release {args.version}")
        run("git", "push", "--atomic", "origin", *(f"refs/tags/{tag}" for tag in release_tags))
        print(f"Actions: https://github.com/{REPOSITORY}/actions/workflows/release-binaries.yml")
        print(f"Release: https://github.com/{REPOSITORY}/releases/tag/{args.version}")
        print("Production deployment is a separate explicit operation.")
        return 0
    except (ReleaseError, json.JSONDecodeError) as error:
        print(f"release aborted: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
