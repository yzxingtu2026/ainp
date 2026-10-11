"""Small authenticated Gitea API client used by the release toolchain."""
from __future__ import annotations

import base64
import json
import os
import subprocess
import urllib.error
import urllib.parse
import urllib.request
import uuid

try:
    from .common import DeployError, require
except ImportError:
    from common import DeployError, require

MAX_RESPONSE = 2 * 1024 * 1024


def _credential(server: str) -> tuple[str, str] | None:
    token = os.getenv("GITEA_TOKEN") or os.getenv("GITHUB_TOKEN")
    if token:
        return "token", token
    try:
        result = subprocess.run(
            ["git", "credential", "fill"], input=f"protocol={urllib.parse.urlsplit(server).scheme}\nhost={urllib.parse.urlsplit(server).netloc}\n\n",
            text=True, capture_output=True, timeout=10, check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    values = dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)
    if values.get("username") and values.get("password"):
        return values["username"], values["password"]
    return None


class Client:
    def __init__(self, server: str):
        parsed = urllib.parse.urlsplit(server.rstrip("/"))
        require(parsed.scheme in ("http", "https") and parsed.netloc and not parsed.query and not parsed.fragment,
                "Gitea server must be an absolute HTTP(S) URL")
        self.server = server.rstrip("/")
        self.auth = _credential(self.server)

    def _request(self, method: str, path: str, *, body: bytes | None = None, accept: str = "application/json", content_type: str | None = None):
        require(path.startswith("/") and ".." not in path.split("/"), "unsafe Gitea API path")
        headers = {"Accept": accept, "User-Agent": "ainp-release-tool"}
        if content_type:
            headers["Content-Type"] = content_type
        if self.auth:
            headers["Authorization"] = "Basic " + base64.b64encode(f"{self.auth[0]}:{self.auth[1]}".encode()).decode()
        request = urllib.request.Request(self.server + path, data=body, method=method, headers=headers)
        try:
            with urllib.request.urlopen(request, timeout=60) as response:
                raw = response.read(MAX_RESPONSE + 1)
        except (OSError, urllib.error.HTTPError, urllib.error.URLError, TimeoutError) as error:
            raise DeployError("Gitea API request failed; check server, credentials and repository access") from error
        require(len(raw) <= MAX_RESPONSE, "Gitea API response exceeds limit")
        return raw

    def json(self, path: str):
        try:
            return json.loads(self._request("GET", path))
        except (ValueError, UnicodeError) as error:
            raise DeployError("invalid Gitea API response") from error

    def repo_path(self, repository: str, suffix: str = "") -> str:
        parts = repository.split("/", 1)
        require(len(parts) == 2 and all(parts) and all(".." not in part for part in parts), "invalid Gitea repository")
        return "/api/v1/repos/" + "/".join(urllib.parse.quote(part, safe="") for part in parts) + suffix

    def release(self, repository: str, tag: str):
        return self.json(self.repo_path(repository, "/releases/tags/" + urllib.parse.quote(tag, safe="")))

    def asset(self, repository: str, release_id: int, asset_id: int) -> bytes:
        return self._request("GET", self.repo_path(repository, f"/releases/{release_id}/assets/{asset_id}"), accept="application/octet-stream")

    def tag_commit(self, repository: str, tag: str) -> str:
        value = self.json(self.repo_path(repository, "/git/refs/tags/" + urllib.parse.quote(tag, safe="")))
        if isinstance(value, list):
            require(len(value) == 1, "Gitea tag reference is ambiguous")
            value = value[0]
        return str((value.get("object") or {}).get("sha") or value.get("sha") or "")

    def merged_pr_head_sha(self, repository: str, merge_sha: str, base: str) -> str:
        prs = self.json(self.repo_path(repository, f"/pulls?state=closed&base={urllib.parse.quote(base, safe='')}&limit=100"))
        candidates = [p for p in prs if p.get("merged_at") and p.get("merge_commit_sha") == merge_sha and p.get("head", {}).get("sha")]
        require(len(candidates) == 1, f"could not resolve exactly one merged {base} PR for {merge_sha}")
        return candidates[0]["head"]["sha"]

    def successful_checks(self, repository: str, commit: str, required: set[str]) -> set[str]:
        payload = self.json(self.repo_path(repository, f"/commits/{urllib.parse.quote(commit, safe='')}/status"))
        return {item.get("context", "") for item in payload.get("statuses", []) if item.get("state") == "success" and item.get("context") in required}

    def ensure_draft_release(self, repository: str, tag: str, title: str, notes: str) -> dict:
        path = self.repo_path(repository, "/releases/tags/" + urllib.parse.quote(tag, safe=""))
        try:
            release = self.json(path)
        except DeployError:
            payload = json.dumps({"tag_name": tag, "name": title, "body": notes, "draft": True, "prerelease": False}).encode()
            release = json.loads(self._request("POST", self.repo_path(repository, "/releases"), body=payload, content_type="application/json"))
        require(type(release.get("id")) is int and release["id"] > 0, "invalid Gitea release identity")
        if release.get("draft") is not True:
            raise DeployError("Gitea release already published; refusing to overwrite immutable metadata")
        payload = json.dumps({"name": title, "body": notes, "draft": True}).encode()
        return json.loads(self._request("PATCH", self.repo_path(repository, f"/releases/{release['id']}"), body=payload, content_type="application/json"))

    def upload_release_asset(self, repository: str, release_id: int, path: str, content: bytes) -> dict:
        boundary = "ainp-" + uuid.uuid4().hex
        filename = urllib.parse.quote(path.rsplit("/", 1)[-1], safe="")
        body = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"attachment\"; filename=\"{filename}\"\r\n"
                "Content-Type: application/octet-stream\r\n\r\n").encode() + content + f"\r\n--{boundary}--\r\n".encode()
        return json.loads(self._request("POST", self.repo_path(repository, f"/releases/{release_id}/assets?name={filename}"), body=body, content_type=f"multipart/form-data; boundary={boundary}"))

    def publish_release(self, repository: str, release_id: int) -> dict:
        payload = json.dumps({"draft": False}).encode()
        return json.loads(self._request("PATCH", self.repo_path(repository, f"/releases/{release_id}"), body=payload, content_type="application/json"))
