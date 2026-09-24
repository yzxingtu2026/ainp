"""Private OSS. CI uses STS, hosts use IMDSv2 only; no public/signed URLs."""
import json
import os
from pathlib import Path
import re
import urllib.request

from common import digest, key, oss_config, require, sha256


def metadata_credentials(role):
    require(re.fullmatch(r"[A-Za-z0-9_.-]+", role), "invalid RAM role")
    base = "http://100.100.100.200/latest/"
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    req = urllib.request.Request(base + "api/token", method="PUT", headers={"X-aliyun-ecs-metadata-token-ttl-seconds": "21600"})
    with opener.open(req, timeout=10) as r:
        token = r.read().decode()
    req = urllib.request.Request(base + "meta-data/ram/security-credentials/" + role, headers={"X-aliyun-ecs-metadata-token": token})
    with opener.open(req, timeout=10) as r:
        data = json.load(r)
    require(data.get("Code") == "Success", "instance RAM credentials unavailable")
    return data["AccessKeyId"], data["AccessKeySecret"], data["SecurityToken"]


def bucket(config, role=None):
    import oss2
    oss_config(config)
    credentials = metadata_credentials(role) if role else tuple(os.environ[k] for k in ("ALIBABA_CLOUD_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_SECRET", "ALIBABA_CLOUD_SECURITY_TOKEN"))
    require(all(credentials), "temporary STS credentials required")
    return oss2.Bucket(oss2.StsAuth(*credentials), config["endpoint"], config["bucket"], connect_timeout=30)


def download(config, object_key, destination, expected_sha, role):
    b = bucket(config, role)
    key(object_key)
    digest(expected_sha)
    require(b.head_object(object_key).content_length <= 2 * 1024**3, "OSS object exceeds download limit")
    b.get_object_to_file(object_key, str(destination))
    require(sha256(destination) == expected_sha, "download checksum mismatch")


def upload_immutable(b, object_key, path):
    """Retry only if exact bytes match; never replace an existing version."""
    import oss2
    key(object_key)
    expected = sha256(path)
    try:
        b.put_object_from_file(object_key, str(path), headers={"x-oss-forbid-overwrite": "true", "x-oss-meta-sha256": expected})
    except oss2.exceptions.ServerError as e:
        if e.status != 409:
            raise
        # Verify content, not just mutable object metadata.
        import hashlib
        h = hashlib.sha256()
        for block in b.get_object(object_key):
            h.update(block)
        require(h.hexdigest() == expected, "immutable OSS object already exists with different bytes")
    require(b.head_object(object_key).headers.get("x-oss-meta-sha256") == expected, "OSS upload verification failed")
