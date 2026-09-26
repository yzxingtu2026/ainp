#!/usr/bin/env python3
"""Verify a native release locally and, optionally, against its private OSS objects."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import tempfile

from common import DeployError, extract_verified, key, read_json, require, sha256, validate_manifest, version
from storage import bucket

FILES = ("release.tar.gz", "release.json", "SHA256SUMS", "reference.json")


def verify_local(directory, expected_version=None, expected_commit=None):
    directory = Path(directory)
    reference = read_json(directory / "reference.json")
    manifest = validate_manifest(read_json(directory / "release.json"))
    require(all(reference.get(k) == manifest.get(k) for k in ("project", "version", "commit", "platform")), "reference and manifest identity mismatch")
    if expected_version is not None:
        require(reference["version"] == expected_version, "unexpected release version")
    if expected_commit is not None:
        require(reference["commit"] == expected_commit, "release commit does not match workflow commit")
    version(reference["version"])
    archive = directory / "release.tar.gz"
    archive_sha = sha256(archive)
    require(archive_sha == reference.get("sha256"), "release archive checksum mismatch")
    require((directory / "SHA256SUMS").read_text() == f"{archive_sha}  release.tar.gz\n", "SHA256SUMS mismatch")
    with tempfile.TemporaryDirectory() as extracted:
        extracted_manifest = extract_verified(archive, Path(extracted) / "release", archive_sha)
    require(extracted_manifest == manifest, "archive manifest differs from release.json")
    return reference


def stream_sha256(response):
    digest = hashlib.sha256()
    size = 0
    while True:
        chunk = response.read(1024 * 1024)
        if not chunk:
            break
        digest.update(chunk)
        size += len(chunk)
    return size, digest.hexdigest()


def verify_remote(directory, reference):
    import oss2

    config = reference.get("oss")
    base = reference.get("key", "")
    require(isinstance(config, dict) and base.endswith("/release.tar.gz"), "published reference has no OSS location")
    base = base[: -len("/release.tar.gz")]
    b = bucket(config)
    for filename in FILES:
        local = Path(directory) / filename
        object_key = f"{base}/{filename}"
        key(object_key)
        head = b.head_object(object_key)
        require(head.content_length == local.stat().st_size, f"OSS size mismatch: {filename}")
        remote = b.get_object(object_key)
        size, digest = stream_sha256(remote)
        require(size == local.stat().st_size and digest == sha256(local), f"OSS bytes mismatch: {filename}")
    return base


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", default="dist-release")
    parser.add_argument("--expected-version")
    parser.add_argument("--expected-commit")
    parser.add_argument("--remote", action="store_true")
    args = parser.parse_args()
    reference = verify_local(args.directory, args.expected_version, args.expected_commit)
    if args.remote:
        base = verify_remote(args.directory, reference)
        print(f"PASS: local release and four immutable OSS objects verified at {base}/")
    else:
        print("PASS: local release manifest, archive, checksums and safe extraction verified")


if __name__ == "__main__":
    try:
        main()
    except (DeployError, KeyError, ValueError, OSError) as exc:
        print(str(exc))
        raise SystemExit(1)
