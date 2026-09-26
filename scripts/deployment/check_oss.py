#!/usr/bin/env python3
"""Explicit OSS preflight; writes only a unique, non-deployable test object."""
import argparse
import os
from pathlib import Path
import tempfile
import uuid

from common import DeployError, key, name, require
from storage import bucket, upload_immutable


def check(b, object_key, directory):
    path = directory / "probe.txt"
    original = b"AINP OSS preflight: not a release artifact.\n"
    path.write_bytes(original)
    upload_immutable(b, object_key, path)
    require(b.get_object(object_key).read() == original, "OSS read-back mismatch")
    # A versioned bucket ignores forbid-overwrite. Stop before the conflict probe.
    version_id = b.head_object(object_key).headers.get("x-oss-version-id")
    require(not version_id or version_id == "null", "versioned bucket: release overwrite protection requires separate validation")
    upload_immutable(b, object_key, path)
    path.write_bytes(b"This conflicting test content must be rejected.\n")
    try:
        upload_immutable(b, object_key, path)
    except DeployError as e:
        if str(e) != "immutable OSS object already exists with different bytes":
            raise
    else:
        raise DeployError("OSS allowed an overwrite; do not publish releases to this bucket")
    require(b.get_object(object_key).read() == original, "OSS test object changed unexpectedly")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", default="ainp")
    parser.add_argument("--apply", action="store_true", help="write a small test object to OSS")
    args = parser.parse_args()
    prefix = key(os.environ["OSS_PREFIX"].rstrip("/"))
    object_key = f"{prefix}/{name(args.project)}/_checks/{uuid.uuid4().hex}/probe.txt"
    print(f"OSS preflight target: {object_key}")
    if not args.apply:
        print("Plan only. Pass --apply to test OIDC-issued credentials against OSS.")
        return
    config = {"bucket": os.environ["OSS_BUCKET"], "endpoint": os.environ["OSS_ENDPOINT"]}
    with tempfile.TemporaryDirectory() as directory:
        check(bucket(config), object_key, Path(directory))
    print("PASS: upload, read-back, identical retry and conflicting overwrite rejection.")
    print("Test object retained; no delete permission required. No release or ECS deployment created.")


if __name__ == "__main__":
    main()
