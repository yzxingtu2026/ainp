#!/usr/bin/env python3
"""Publish archive first, reference last as completion marker. No deployment."""
import argparse
import os
from pathlib import Path

from common import atomic_json, key, read_json, require, sha256, validate_manifest
from storage import bucket, upload_immutable


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--directory", default="dist-release")
    args = p.parse_args()
    directory = Path(args.directory)
    reference = read_json(directory / "reference.json")
    manifest = validate_manifest(read_json(directory / "release.json"))
    require(all(reference[k] == manifest[k] for k in ("project", "version", "commit", "platform")), "reference mismatch")
    require(sha256(directory / "release.tar.gz") == reference["sha256"], "archive checksum mismatch")
    config = {"endpoint": os.environ["OSS_ENDPOINT"], "bucket": os.environ["OSS_BUCKET"]}
    prefix = key(os.environ["OSS_PREFIX"].rstrip("/"))
    base = f"{prefix}/{reference['project']}/{reference['version']}/{reference['commit']}"
    reference.update(oss=config, key=f"{base}/release.tar.gz")
    atomic_json(directory / "reference.json", reference, 0o644)
    b = bucket(config)
    for filename in ("release.tar.gz", "release.json", "SHA256SUMS", "reference.json"):
        upload_immutable(b, f"{base}/{filename}", directory / filename)
    print(f"Published {reference['project']} {reference['version']} ({reference['commit']}); production unchanged")


if __name__ == "__main__":
    main()
