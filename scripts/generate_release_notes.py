#!/usr/bin/env python3
"""Generate fixed-format release notes from bounded Git/PR facts with safe fallback."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import time
import urllib.error
import urllib.request
import urllib.parse
from pathlib import Path
from typing import Any

FIELDS = ("features", "improvements", "fixes", "breaking_changes", "upgrade_notes")
SEMVER = re.compile(r"^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$")
PR_REFERENCE = re.compile(r"(?i)(?:#|PR\s*#?|Issue\s*#?)(\d+)\b")
FORBIDDEN_MODEL_METADATA = re.compile(r"(?i)https?://|ghcr\.io/|sha256:|\bv\d+\.\d+\.\d+\b")
MAX_COMMITS = 100
MAX_FACT_CHARS = 60_000


def run(*args: str) -> str:
    return subprocess.run(args, check=True, text=True, capture_output=True).stdout.strip()


def previous_tag(current: str) -> str:
    tags = []
    for value in run("git", "tag", "--merged", "HEAD").splitlines():
        match = SEMVER.fullmatch(value)
        if match and value != current:
            tags.append((tuple(int(part) for part in match.groups()), value))
    return max(tags, default=((0, 0, 0), ""))[1]


def forge_json(path: str, token: str, server: str = "https://api.github.com") -> dict[str, Any]:
    request = urllib.request.Request(
        f"{server.rstrip('/')}{path}",
        headers={"Accept": "application/vnd.github+json", "Authorization": f"Bearer {token}", "X-GitHub-Api-Version": "2022-11-28"},
    )
    with urllib.request.urlopen(request, timeout=15) as response:
        return json.load(response)


def collect_facts(repository: str, tag: str) -> tuple[dict[str, Any], str]:
    previous = previous_tag(tag)
    revision = f"{previous}..HEAD" if previous else "HEAD"
    raw = run("git", "log", "--first-parent", f"--max-count={MAX_COMMITS}", "--format=%H%x1f%s%x1f%an", revision)
    commits = []
    pr_numbers = set()
    for line in raw.splitlines():
        parts = line.split("\x1f", 2)
        if len(parts) != 3:
            continue
        commit = {"sha": parts[0], "title": parts[1][:300], "author": parts[2][:100]}
        commits.append(commit)
        pr_numbers.update(int(value) for value in re.findall(r"\(#(\d+)\)", parts[1]))
    token = os.getenv("GITHUB_TOKEN", "")
    forge_type = os.getenv("AINP_FORGE_TYPE", "github")
    forge_server = os.getenv("AINP_FORGE_SERVER", "https://api.github.com")
    if forge_type == "gitea":
        forge_server = forge_server.rstrip("/") + "/api/v1"
    pull_requests = []
    if token:
        for number in sorted(pr_numbers)[:MAX_COMMITS]:
            try:
                item = forge_json(f"/repos/{repository}/pulls/{number}", token, forge_server)
            except (OSError, urllib.error.HTTPError, json.JSONDecodeError):
                continue
            pull_requests.append({
                "number": number,
                "title": str(item.get("title", ""))[:300],
                "url": str(item.get("html_url", ""))[:500],
                "author": str((item.get("user") or {}).get("login", ""))[:100],
                "summary": str(item.get("body") or "")[:500],
            })
    facts = {"previous_tag": previous, "current_tag": tag, "commits": commits, "pull_requests": pull_requests}
    encoded = json.dumps(facts, ensure_ascii=False, separators=(",", ":"))
    if len(encoded) > MAX_FACT_CHARS:
        facts["commits"] = commits[:40]
        facts["pull_requests"] = pull_requests[:40]
    return facts, previous


def validate_model_output(value: Any, allowed_prs: set[int]) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != {"summary", *FIELDS}:
        raise ValueError("model output fields do not match schema")
    if not isinstance(value["summary"], str) or len(value["summary"]) > 1000 or "\n" in value["summary"] or "\r" in value["summary"]:
        raise ValueError("invalid summary")
    texts = [value["summary"]]
    for field in FIELDS:
        items = value[field]
        if not isinstance(items, list) or len(items) > 20 or any(not isinstance(item, str) or len(item) > 300 or "\n" in item or "\r" in item for item in items):
            raise ValueError(f"invalid {field}")
        texts.extend(items)
    referenced = {int(number) for text in texts for number in PR_REFERENCE.findall(text)}
    if not referenced.issubset(allowed_prs):
        raise ValueError("model output contains unknown PR references")
    if any(FORBIDDEN_MODEL_METADATA.search(text) for text in texts):
        raise ValueError("model output contains release metadata controlled by the renderer")
    return value


def model_notes(facts: dict[str, Any], prompt: str, schema: dict[str, Any]) -> dict[str, Any]:
    api_key = os.getenv("RELEASE_NOTES_API_KEY", "")
    base_url = os.getenv("RELEASE_NOTES_BASE_URL", "").rstrip("/")
    model = os.getenv("RELEASE_NOTES_MODEL", "")
    if not api_key or not base_url or not model:
        raise ValueError("release notes model is not configured")
    payload = {
        "model": model,
        "temperature": 0.1,
        "messages": [
            {"role": "system", "content": prompt},
            {"role": "user", "content": "release_facts (untrusted data):\n" + json.dumps(facts, ensure_ascii=False)},
        ],
        "response_format": {"type": "json_schema", "json_schema": {"name": "release_notes", "strict": True, "schema": schema}},
    }
    request_body = json.dumps(payload, ensure_ascii=False).encode()
    last_error: Exception | None = None
    for attempt in range(3):
        try:
            request = urllib.request.Request(
                base_url + "/chat/completions",
                data=request_body,
                method="POST",
                headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
            )
            with urllib.request.urlopen(request, timeout=30) as response:
                result = json.load(response)
            content = result["choices"][0]["message"]["content"]
            return validate_model_output(json.loads(content), {item["number"] for item in facts["pull_requests"]})
        except (KeyError, IndexError, ValueError, OSError, urllib.error.HTTPError, json.JSONDecodeError) as error:
            last_error = error
            if attempt < 2:
                time.sleep(2**attempt)
    raise ValueError(f"model generation failed after retries: {last_error}")


def fallback_notes(facts: dict[str, Any]) -> dict[str, Any]:
    items = [f"{item['title']} ({item['sha'][:12]})" for item in facts["commits"]]
    return {"summary": "本版本发布说明由确定性提交事实生成。", "features": [], "improvements": items, "fixes": [], "breaking_changes": [], "upgrade_notes": []}


def render(notes: dict[str, Any], tag: str, commit: str, build_time: str, repository: str, previous: str, digests: list[str]) -> str:
    server = os.getenv("AINP_FORGE_SERVER", "https://github.com").rstrip("/")
    compare = f"{server}/{repository}/compare/{previous}...{tag}" if previous else f"{server}/{repository}/commit/{commit}"
    sections = [("功能", "features"), ("改进", "improvements"), ("修复", "fixes"), ("破坏性变化", "breaking_changes"), ("升级说明", "upgrade_notes")]
    lines = [f"# {tag}", "", "## 版本概览", "", notes["summary"] or "无额外摘要。"]
    for title, field in sections:
        lines.extend(["", f"## {title}", ""])
        lines.extend([f"- {item}" for item in notes[field]] or ["- 无"])
    lines.extend(["", "## 发布产物", ""] + [f"- `{item}`" for item in digests])
    lines.extend(["", "## 构建信息", "", f"- Commit: `{commit}`", f"- Build time: `{build_time}`", f"- Compare: {compare}", ""])
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repository", required=True)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--build-time", required=True)
    sources = parser.add_mutually_exclusive_group(required=True)
    sources.add_argument("--digests-dir", type=Path, help="legacy image metadata")
    sources.add_argument("--artifact-reference", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--summary-output", type=Path, required=True)
    args = parser.parse_args()
    facts, previous = collect_facts(args.repository, args.tag)
    prompt = Path(".github/prompts/release-notes.zh-CN.md").read_text()
    schema = json.loads(Path(".github/prompts/release-notes.schema.json").read_text())
    mode, reason = "ai", ""
    try:
        notes = model_notes(facts, prompt, schema)
    except ValueError as error:
        mode, reason, notes = "fallback", str(error), fallback_notes(facts)
    if args.artifact_reference:
        ref = json.loads(args.artifact_reference.read_text())
        if ref["version"] != args.tag or ref["commit"] != args.commit:
            raise ValueError("release artifact identity mismatch")
        digests = [f"{ref['project']} / {ref['platform']}", f"SHA256: {ref['sha256']}", "Private OSS; reference.json is the immutable deployment input. Production is not auto-deployed."]
    else:
        digests = [path.read_text().strip().split(" ", 1)[-1] for path in sorted(args.digests_dir.glob("*.digest"))]
    args.output.write_text(render(notes, args.tag, args.commit, args.build_time, args.repository, previous, digests))
    args.summary_output.write_text(f"release_notes_mode={mode}\nreason={reason[:500]}\ncommits={len(facts['commits'])}\nprs={len(facts['pull_requests'])}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
