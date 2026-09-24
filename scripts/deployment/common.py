"""Dependency-free, shared release/deployment contracts (schema version 1)."""
from __future__ import annotations

import contextlib
import fcntl
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import tarfile


class DeployError(RuntimeError):
    pass


def require(condition, message):
    if not condition:
        raise DeployError(message)


def name(value):
    require(isinstance(value, str) and re.fullmatch(r"[a-z][a-z0-9-]{1,30}", value), "invalid project/environment name")
    return value


def version(value):
    require(isinstance(value, str) and re.fullmatch(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-ci)?", value), "invalid release version")
    return value


def digest(value):
    require(isinstance(value, str) and re.fullmatch(r"[0-9a-f]{64}", value), "invalid SHA256")
    return value


def key(value):
    require(isinstance(value, str) and re.fullmatch(r"[A-Za-z0-9_./-]+", value) and not value.startswith("/") and all(p not in ("", ".", "..") for p in value.split("/")), "invalid OSS key")
    return value


def oss_config(config):
    require(isinstance(config, dict), "OSS configuration required")
    require(re.fullmatch(r"https://oss-[a-z0-9-]+\.aliyuncs\.com", config.get("endpoint", "")), "only HTTPS Alibaba OSS endpoints allowed")
    require(re.fullmatch(r"[a-z0-9][a-z0-9-]{1,61}[a-z0-9]", config.get("bucket", "")), "invalid OSS bucket")
    return config


def sha256(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def read_json(path):
    return json.loads(Path(path).read_text())


def atomic_json(path, value, mode=0o600):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temp = path.with_name(path.name + ".tmp")
    fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, mode)
    with os.fdopen(fd, "w") as f:
        os.fchmod(f.fileno(), mode)
        json.dump(value, f, sort_keys=True, indent=2)
        f.write("\n")
        f.flush()
        os.fsync(f.fileno())
    os.replace(temp, path)
    fd = os.open(path.parent, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


@contextlib.contextmanager
def lock(path):
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    with open(path, "a") as f:
        try:
            fcntl.flock(f, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as e:
            raise DeployError("another deployment holds the lock") from e
        yield


def validate_manifest(m):
    require(m.get("schema") == 1 and m.get("platform") == "ubuntu-22.04-amd64", "unsupported release contract")
    name(m["project"])
    version(m["version"])
    require(re.fullmatch(r"[0-9a-f]{40}", m["commit"]), "invalid commit")
    require(isinstance(m.get("build_time"), str) and re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", m["build_time"]), "build_time must be UTC RFC3339")
    require(isinstance(m.get("files"), dict) and len(m["files"]) <= 20000, "invalid file manifest")
    for p, h in m["files"].items():
        key(p)
        digest(h)
    require({"bin/api", "bin/worker", "bin/migrate", "web/index.html", "web/build.json"} <= m["files"].keys(), "incomplete release")
    return m


def extract_verified(archive, destination, expected_sha):
    """No extractall: reject links, traversal, duplicates, oversized and extra files."""
    require(sha256(archive) == digest(expected_sha), "archive checksum mismatch")
    destination = Path(destination)
    require(not destination.exists(), "staging destination already exists")
    with tarfile.open(archive, "r:gz") as tar:
        members = []
        seen = set()
        size = 0
        for item in tar:
            key(item.name)
            require(item.isfile() and item.name not in seen, "archive contains non-regular or duplicate member")
            seen.add(item.name)
            size += item.size
            require(len(seen) <= 20001 and size <= 4 * 1024**3, "archive exceeds extraction limits")
            members.append(item)
        require("release.json" in seen, "missing release manifest")
        info = tar.getmember("release.json")
        require(info.size <= 8 * 1024**2, "manifest too large")
        manifest = validate_manifest(json.load(tar.extractfile(info)))
        require(seen == set(manifest["files"]) | {"release.json"}, "archive manifest does not match payload")
        destination.mkdir(parents=True, mode=0o755)
        destination.chmod(0o755)
        for item in members:
            target = destination / PurePosixPath(item.name)
            target.parent.mkdir(parents=True, exist_ok=True)
            parent = target.parent
            while parent != destination:
                parent.chmod(0o755)
                parent = parent.parent
            with tar.extractfile(item) as src, target.open("xb") as out:
                while chunk := src.read(1024 * 1024):
                    out.write(chunk)
            if item.name != "release.json":
                require(sha256(target) == manifest["files"][item.name], "payload checksum mismatch")
            target.chmod(0o755 if item.name.startswith("bin/") else 0o644)
    return manifest
