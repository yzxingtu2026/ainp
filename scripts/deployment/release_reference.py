"""Resolve published stable versions using authenticated GitHub metadata (no ECS calls)."""
import hashlib
import json
from pathlib import Path
import re
import subprocess

from common import DeployError, atomic_json, digest, key, lock, oss_config, read_json, require, version

DEFAULT_REPOSITORY = "yzxingtu2026/ainp"
REFERENCE_LIMIT = 64 * 1024


def github(endpoint, *, accept="application/vnd.github+json", query=None):
    argv = ["gh", "api", "--hostname", "github.com", endpoint, "-H", f"Accept: {accept}"]
    if query:
        argv += ["--jq", query]
    try:
        result = subprocess.run(argv, capture_output=True, timeout=60)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise DeployError("GitHub release lookup unavailable; install/authenticate gh and retry before deployment") from error
    require(result.returncode == 0, "GitHub release lookup failed; check gh auth, repository access and published version")
    require(len(result.stdout) <= 1024 * 1024, "GitHub release response exceeds limit")
    return result.stdout


def json_object(raw):
    try:
        value = json.loads(raw)
    except (ValueError, UnicodeError) as error:
        raise DeployError("invalid GitHub release metadata") from error
    require(isinstance(value, dict), "invalid GitHub release metadata")
    return value


def resolve(repository, requested_version, project):
    require(isinstance(repository, str) and re.fullmatch(r"[A-Za-z0-9_-]+/[A-Za-z0-9_.-]+", repository)
            and repository.split("/")[1] not in (".", ".."), "repository must be OWNER/REPO on github.com")
    version(requested_version)
    require(not requested_version.endswith("-ci"), "--version requires a stable version; CI packages need --release to select an exact commit")
    prefix = f"repos/{repository}"
    release = json_object(github(f"{prefix}/releases/tags/{requested_version}"))
    require(release.get("tag_name") == requested_version and release.get("draft") is False
            and release.get("prerelease") is False and release.get("published_at"),
            "version must have a published, non-draft, stable GitHub Release")
    assets = release.get("assets")
    require(isinstance(assets, list) and all(isinstance(a, dict) for a in assets), "invalid release assets")
    references = [a for a in assets if a.get("name") == "reference.json"]
    require(len(references) == 1, "published reference.json missing or ambiguous; legacy releases need a native release package")
    asset = references[0]
    require(type(asset.get("id")) is int and asset["id"] > 0 and asset.get("state") == "uploaded"
            and type(asset.get("size")) is int and 0 < asset["size"] <= REFERENCE_LIMIT,
            "invalid reference.json asset")
    raw = github(f"{prefix}/releases/assets/{asset['id']}", accept="application/octet-stream")
    require(len(raw) == asset["size"], "reference.json asset size mismatch")
    if asset.get("digest"):
        require(asset["digest"] == "sha256:" + hashlib.sha256(raw).hexdigest(), "reference.json asset digest mismatch")
    reference = json_object(raw)
    require(reference.get("schema") == 1 and reference.get("project") == project
            and reference.get("version") == requested_version and reference.get("platform") == "ubuntu-22.04-amd64",
            "published reference project/version/schema/platform mismatch")
    commit = reference.get("commit", "")
    require(isinstance(commit, str) and re.fullmatch(r"[0-9a-f]{40}", commit), "invalid release commit")
    digest(reference.get("sha256"))
    key(reference.get("key"))
    oss = reference.get("oss")
    require(isinstance(oss, dict) and all(isinstance(oss.get(field), str) for field in ("bucket", "endpoint")),
            "invalid published OSS configuration")
    oss_config(oss)
    require(reference["key"].endswith(f"/{project}/{requested_version}/{commit}/release.tar.gz"),
            "published OSS key does not match project/version/commit")
    # refs/tags prevents a same-named branch from being resolved instead of the tag.
    tag_commit = github(f"{prefix}/commits/refs/tags/{requested_version}", query=".sha").decode().strip()
    require(tag_commit == commit, "release reference does not match the Git tag commit")
    return reference


def pin_reference(state_dir, repository, reference, *, write=False):
    """Detect changed release metadata; plan checks existing pins without writing."""
    path = Path(state_dir) / "releases" / repository / reference["version"] / "reference.json"
    def check():
        require(not path.exists() or read_json(path) == reference,
                "published release differs from pinned reference; inspect release audit, do not overwrite")
    if write:
        with lock(str(path) + ".lock"):
            check()
            if not path.exists():
                atomic_json(path, reference)
    else:
        check()
    return path
