#!/usr/bin/env python3
"""Create and push one immutable AINP release tag after fail-closed checks."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

REPOSITORY = "yzxingtu2026/ainp"
REQUIRED_CHECKS = {"backend", "containers", "frontend", "repository-gates"}
VERSION_PATTERN = re.compile(r"^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$")
EXPECTED_REMOTES = {
    "https://github.com/yzxingtu2026/ainp.git",
    "git@github.com:yzxingtu2026/ainp.git",
    "ssh://git@github.com/yzxingtu2026/ainp.git",
}


class ReleaseError(RuntimeError):
    pass


def run(*args: str, check: bool = True) -> str:
    result = subprocess.run(args, check=False, text=True, capture_output=True)
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
    if value not in EXPECTED_REMOTES:
        raise ReleaseError(f"origin does not point to https://github.com/{REPOSITORY}: {value}")


def successful_checks(payload: dict) -> set[str]:
    return {
        item.get("name", "")
        for item in payload.get("check_runs", [])
        if item.get("conclusion") == "success" and item.get("name") in REQUIRED_CHECKS
    }


def preflight(version: str) -> tuple[str, str]:
    validate_version(version)
    module_version = platform_tag(version)
    root = run("git", "rev-parse", "--show-toplevel")
    if Path.cwd().resolve() != Path(root).resolve():
        raise ReleaseError("run the release command from the repository root")
    if run("git", "branch", "--show-current") != "main":
        raise ReleaseError("release must run from main")
    if run("git", "status", "--porcelain=v1", "--untracked-files=all"):
        raise ReleaseError("working tree, index, or untracked files are not clean")
    remote = run("git", "remote", "get-url", "origin")
    validate_remote(remote)
    run("gh", "auth", "status", "--hostname", "github.com")
    run("git", "fetch", "origin", "main", "--tags", "--prune")
    head = run("git", "rev-parse", "HEAD")
    if head != run("git", "rev-parse", "origin/main"):
        raise ReleaseError("HEAD must exactly match origin/main")
    for tag in (version, module_version):
        if run("git", "tag", "--list", tag):
            raise ReleaseError(f"local tag already exists: {tag}")
        if run("git", "ls-remote", "--tags", "origin", f"refs/tags/{tag}"):
            raise ReleaseError(f"remote tag already exists: {tag}")
    payload = json.loads(run("gh", "api", f"repos/{REPOSITORY}/commits/{head}/check-runs?per_page=100"))
    missing = REQUIRED_CHECKS - successful_checks(payload)
    if missing:
        raise ReleaseError(f"main CI is not successful for {head}: missing {', '.join(sorted(missing))}")
    demo_url = run("gh", "variable", "get", "DEMO_URL", "--env", "demo", check=False) or "not configured"
    return head, demo_url


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("version", type=validate_version)
    parser.add_argument("--yes", action="store_true", help="skip interactive confirmation")
    args = parser.parse_args()
    try:
        commit, demo_url = preflight(args.version)
        print(f"Version: {args.version}\nCommit: {commit}\nRepository: {REPOSITORY}\nEnvironment: demo ({demo_url})")
        if not args.yes and input("Push immutable annotated tag? [y/N] ").strip().lower() not in {"y", "yes"}:
            raise ReleaseError("release cancelled")
        run("git", "tag", "--annotate", args.version, "--message", f"Release {args.version}")
        module_version = platform_tag(args.version)
        run("git", "tag", "--annotate", module_version, "--message", f"Platform module {args.version}")
        run("git", "push", "origin", f"refs/tags/{args.version}", f"refs/tags/{module_version}")
        print(f"Actions: https://github.com/{REPOSITORY}/actions/workflows/release-images.yml")
        print(f"Release: https://github.com/{REPOSITORY}/releases/tag/{args.version}")
        print(f"Demo: {demo_url}")
        return 0
    except (ReleaseError, json.JSONDecodeError) as error:
        print(f"release aborted: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
