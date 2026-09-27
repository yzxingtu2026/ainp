#!/usr/bin/env python3
"""Versioned deployment files: published sources, three-way plans and recoverable apply."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile

from common import DeployError, atomic_json, digest, lock, read_json, require, sha256
import project
from release_reference import github, json_object

UPSTREAM = "yzxingtu2026/ainp"
POLICY = "deploy/baseline/policy.json"
LOCK = "deploy/baseline.lock.json"
TRANSACTION = ".deployment-baseline-transaction.json"
ASSET = "deployment-baseline.json"
STABLE = re.compile(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$")
# Independent minima prevent a policy edit from quietly dropping critical dependencies.
REQUIRED = {
    *("scripts/deployment/" + name for name in (
        "apt.sh", "baseline.py", "bootstrap.sh", "check_oss.py", "cloud.py", "common.py",
        "host.py", "launcher.py", "package.py", "project.py", "publish.py", "release_reference.py",
        "reporting.py", "storage.py", "verify_release.py")),
    "scripts/release.py", "scripts/generate_release_notes.py", "scripts/test_release.py",
    "scripts/test_generate_release_notes.py", "scripts/test_deployment.py", "scripts/test_release_reference.py",
    "scripts/test_deployment_baseline.py", "scripts/test_project.py", "docker/build-mupdf-cjk.sh",
    ".github/workflows/native-build.yml", ".github/workflows/release-binaries.yml",
    ".github/workflows/test-native-release.yml", ".github/workflows/check-oss.yml",
    ".github/prompts/release-notes.zh-CN.md", ".github/prompts/release-notes.schema.json",
    "deploy/linux/project.json", "deploy/linux/build.json", "deploy/linux/target.example.json",
    "deploy/linux/private.example.json", POLICY, "deploy/baseline/manifest.schema.json",
    "docs/development/deployment-baseline.md", "docs/development/binary-deployment.md",
    ".agent/rules/binary-deployment.md",
    ".github/workflows/deployment-baseline.yml",
    "deploy/baseline/examples/project.json", "deploy/baseline/examples/build.json",
    "deploy/baseline/templates/gitignore.txt", "deploy/baseline/templates/agent-registration.txt",
}


def safe_path(root, value):
    project.relative_path(value)
    path = Path(root)
    for part in value.split("/"):
        path = path / part
        require(not path.is_symlink(), f"symlink path forbidden: {value}")
    require(path.resolve().is_relative_to(Path(root).resolve()), f"path escapes root: {value}")
    return path


def allowed(path):
    project.relative_path(path)
    return (path.startswith(("scripts/deployment/", "deploy/baseline/", ".github/prompts/"))
            or path in REQUIRED or path in {".github/workflows/deployment-baseline.yml",
                "docs/development/release-versioning.md", ".agent/rules/release-versioning.md"})


def fingerprint(path):
    if not path.exists():
        return None
    require(path.is_file() and not path.is_symlink(), f"not a regular file: {path}")
    return {"sha256": sha256(path), "mode": "0755" if path.stat().st_mode & 0o111 else "0644"}


def validate_manifest(manifest):
    require(isinstance(manifest, dict) and set(manifest) == {"schema", "repository", "version", "commit", "hash_policy", "files"}, "invalid deployment manifest fields")
    require(manifest["schema"] == 1 and manifest["repository"] == UPSTREAM and STABLE.fullmatch(manifest["version"])
            and re.fullmatch(r"[0-9a-f]{40}", manifest["commit"]), "invalid deployment source identity")
    require(manifest["hash_policy"] == "raw-bytes", "unsupported hash policy")
    require(isinstance(manifest["files"], dict) and manifest["files"], "empty deployment manifest")
    sources = set()
    for target, entry in manifest["files"].items():
        require(allowed(target), f"forbidden target: {target}")
        require(isinstance(entry, dict) and set(entry) == {"source", "owner", "sha256", "mode"}, f"invalid entry: {target}")
        project.relative_path(entry["source"])
        require(entry["source"] == target, f"source/target mapping not allowed in schema 1: {target}")
        require(entry["owner"] in ("upstream", "project", "integration"), f"invalid ownership: {target}")
        require(entry["mode"] in ("0644", "0755"), f"invalid mode: {target}")
        digest(entry["sha256"])
        if target in ("deploy/linux/project.json", "deploy/linux/build.json"):
            require(entry["owner"] == "project", f"project configuration cannot be managed: {target}")
        elif target.startswith(".github/workflows/"):
            require(entry["owner"] == "integration", f"workflow requires explicit integration ownership: {target}")
        else:
            require(entry["owner"] == "upstream", f"public dependency must be upstream managed: {target}")
        sources.add(entry["source"])
    require(REQUIRED <= sources, "missing deployment dependencies: " + ", ".join(sorted(REQUIRED - sources)))
    return manifest


def manifest_for(root, version, commit):
    root = Path(root)
    policy = read_json(safe_path(root, POLICY))
    require(policy.get("schema") == 1, "missing deployment policy")
    entries = {}
    for pattern in policy["public"]:
        project.relative_path(pattern.replace("*", "wildcard"))
        paths = sorted(root.glob(pattern))
        require(paths, f"manifest pattern has no files: {pattern}")
        for path in paths:
            source = path.relative_to(root).as_posix()
            safe_path(root, source)
            require(path.is_file(), f"manifest source is not a file: {source}")
            entries[source] = {"source": source, "owner": "upstream", **fingerprint(path)}
    for owner in ("project", "integration"):
        for source, target in policy[owner].items():
            require(target not in entries, f"duplicate target: {target}")
            entries[target] = {"source": source, "owner": owner, **fingerprint(safe_path(root, source))}
    closure = {p.relative_to(root).as_posix() for p in (root / "scripts/deployment").iterdir() if p.suffix in (".py", ".sh")}
    require(closure <= set(entries), "scripts/deployment dependency missing from policy")
    return validate_manifest(dict(schema=1, repository=UPSTREAM, version=version, commit=commit, hash_policy="raw-bytes", files=entries))


def fetch_source(version, commit, destination):
    require(STABLE.fullmatch(version) and re.fullmatch(r"[0-9a-f]{40}", commit), "explicit stable version and full commit required")
    prefix = f"repos/{UPSTREAM}"
    release = json_object(github(f"{prefix}/releases/tags/{version}"))
    require(release.get("tag_name") == version and release.get("draft") is False and release.get("prerelease") is False and release.get("published_at"), "source must be a published stable Release")
    for tag in (version, "platform/" + version):
        resolved = github(f"{prefix}/commits/refs/tags/{tag}", query=".sha").decode().strip()
        require(resolved == commit, f"source Tag/commit mismatch: {tag}")
    assets = [a for a in release.get("assets", []) if a.get("name") == ASSET]
    require(len(assets) == 1, "deployment manifest release asset missing or ambiguous")
    asset = assets[0]
    require(type(asset.get("id")) is int and asset["id"] > 0 and asset.get("state") == "uploaded" and type(asset.get("size")) is int and 0 < asset["size"] < 1024 * 1024, "invalid deployment manifest asset")
    raw = github(f"{prefix}/releases/assets/{asset['id']}", accept="application/octet-stream")
    require(len(raw) == asset["size"] and asset.get("digest") == "sha256:" + hashlib.sha256(raw).hexdigest(), "deployment manifest asset hash mismatch or missing")
    manifest = validate_manifest(json_object(raw))
    require(manifest["version"] == version and manifest["commit"] == commit, "manifest source mismatch")
    archive = destination / "source.tar.gz"
    with archive.open("wb") as output:
        result = subprocess.run(["gh", "api", "--hostname", "github.com", f"{prefix}/tarball/{commit}"], stdout=output, stderr=subprocess.DEVNULL, timeout=180)
    require(result.returncode == 0, "source archive unavailable; check read-only GitHub credentials")
    source = destination / "source"
    source.mkdir()
    # Extract only manifested files, never symlinks, special files or arbitrary tar paths.
    wanted = {entry["source"] for entry in manifest["files"].values()}
    found = set()
    with tarfile.open(archive) as tar:
        for member in tar:
            parts = member.name.split("/", 1)
            if len(parts) != 2 or parts[1] not in wanted:
                continue
            value = parts[1]
            require(value not in found and member.isfile() and member.size < 16 * 1024 * 1024, f"invalid archive member: {value}")
            path = safe_path(source, value)
            path.parent.mkdir(parents=True, exist_ok=True)
            with tar.extractfile(member) as body, path.open("wb") as output:
                shutil.copyfileobj(body, output)
            path.chmod(0o755 if member.mode & 0o111 else 0o644)
            found.add(value)
    require(found == wanted, "source archive missing declared files")
    # Compare the complete committed tree listing too: omitted helpers must fail closed.
    tree = json_object(github(f"{prefix}/git/trees/{commit}?recursive=1"))
    require(tree.get("truncated") is False, "source tree lookup truncated")
    closure = {e["path"] for e in tree["tree"] if e["path"].startswith("scripts/deployment/") and e["path"].endswith((".py", ".sh"))}
    require(closure <= wanted, "source manifest omits deployment helpers")
    require(manifest_for(source, version, commit) == manifest, "source file hashes/modes/closure do not match release manifest")
    return source, manifest


def read_lock(root):
    path = safe_path(root, LOCK)
    if not path.exists():
        return None
    value = read_json(path)
    require(set(value) == {"schema", "manifest", "manifest_sha256"} and value["schema"] == 1, "invalid baseline lock")
    validate_manifest(value["manifest"])
    require(value["manifest_sha256"] == manifest_hash(value["manifest"]), "baseline lock manifest hash mismatch")
    return value


def manifest_hash(manifest):
    return hashlib.sha256(json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def check_migration(root):
    config = safe_path(root, "deploy/linux/project.json")
    if config.exists():
        cfg = project.load(config)
        for retired in cfg["retired_workflows"]:
            require(not safe_path(root, retired).exists(), f"retired image workflow still enabled: {retired}")
    workflows = safe_path(root, ".github/workflows")
    if workflows.exists():
        for workflow in workflows.iterdir():
            if workflow.suffix in (".yml", ".yaml"):
                text = safe_path(root, workflow.relative_to(root).as_posix()).read_text()
                image_publish = "docker push " in text or "docker/build-push-action@" in text and re.search(r"push:\s*true", text)
                require(not image_publish, f"legacy image publishing workflow must be explicitly migrated: {workflow.name}")


def plan(root, manifest, adopt=()):
    validate_manifest(manifest)
    require(not safe_path(root, TRANSACTION).exists(), "unfinished baseline transaction; run recover before retry")
    check_migration(root)
    old = read_lock(root)
    if old and old["manifest"]["version"] == manifest["version"]:
        require(old["manifest"] == manifest, "published baseline changed for locked version")
    before = old["manifest"]["files"] if old else {}
    after = manifest["files"]
    unknown = set(adopt) - set(after)
    require(not unknown, "unknown adoption paths: " + ", ".join(sorted(unknown)))
    actions, conflicts = [], []
    for name in sorted(set(before) | set(after)):
        previous, desired = before.get(name), after.get(name)
        if desired and desired["owner"] == "project":
            require(not previous or previous["owner"] == "project", f"ownership changed: {name}")
            continue
        if previous and previous["owner"] == "project":
            require(not desired, f"cannot take over project configuration: {name}")
            continue
        path = safe_path(root, name)
        actual = fingerprint(path)
        expected = {k: previous[k] for k in ("sha256", "mode")} if previous else None
        new = {k: desired[k] for k in ("sha256", "mode")} if desired else None
        if previous and actual != expected:
            conflicts.append(f"{name}: local drift (restore old bytes/mode or preserve your change elsewhere)")
        elif not previous and actual is not None and (name not in adopt or actual != new):
            conflicts.append(f"{name}: unmanaged collision (review ownership; --adopt requires identical bytes/mode)")
        elif actual != new:
            actions.append({"path": name, "action": "delete" if new is None else "add" if actual is None else "update", "before": actual, "after": new})
    require(not conflicts, "baseline conflicts:\n" + "\n".join(conflicts))
    return actions


def write_bytes(path, content, mode):
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as output:
        temp = Path(output.name)
        try:
            output.write(content)
            output.flush()
            os.fchmod(output.fileno(), int(mode, 8))
            os.fsync(output.fileno())
        except BaseException:
            temp.unlink()
            raise
    try:
        os.replace(temp, path)
        directory = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        temp.unlink(missing_ok=True)


def apply(root, source, manifest, adopt=()):
    with lock(safe_path(root, ".deployment-baseline.guard")):
        actions = plan(root, manifest, adopt)
        # Read and hash every source before touching destination; no successful partial lock.
        bodies = {}
        for name, entry in manifest["files"].items():
            body = safe_path(source, entry["source"]).read_bytes()
            require(hashlib.sha256(body).hexdigest() == entry["sha256"], f"source hash mismatch: {name}")
            bodies[name] = body
        new_lock = dict(schema=1, manifest=manifest, manifest_sha256=manifest_hash(manifest))
        if not actions and read_lock(root) == new_lock:
            return
        backups = []
        for action in actions + [{"path": LOCK, "after": None}]:
            path = safe_path(root, action["path"])
            before = fingerprint(path)
            backups.append({"path": action["path"], "before": before, "after": action["after"], "content": base64.b64encode(path.read_bytes()).decode() if before else None})
        lock_body = (json.dumps(new_lock, sort_keys=True, indent=2) + "\n").encode()
        backups[-1]["after"] = {"sha256": hashlib.sha256(lock_body).hexdigest(), "mode": "0644"}
        atomic_json(safe_path(root, TRANSACTION), {"schema": 1, "backups": backups})
        for action in actions:
            path = safe_path(root, action["path"])
            require(fingerprint(path) == action["before"], f"concurrent file change: {action['path']}")
            if action["action"] == "delete":
                path.unlink()
            else:
                write_bytes(path, bodies[action["path"]], action["after"]["mode"])
        write_bytes(safe_path(root, LOCK), lock_body, "0644")
        safe_path(root, TRANSACTION).unlink()


def recover(root):
    with lock(safe_path(root, ".deployment-baseline.guard")):
        journal = read_json(safe_path(root, TRANSACTION))
        require(journal.get("schema") == 1 and isinstance(journal.get("backups"), list), "invalid recovery journal")
        # Preflight all entries before restoring anything. Preserve edits made after failure.
        for entry in journal["backups"]:
            require((allowed(entry["path"]) or entry["path"] == LOCK) and entry["path"] not in ("deploy/linux/project.json", "deploy/linux/build.json"), "forbidden recovery path")
            actual = fingerprint(safe_path(root, entry["path"]))
            require(actual in (entry["before"], entry["after"]), f"recovery conflict: {entry['path']}")
            if entry["before"]:
                body = base64.b64decode(entry["content"], validate=True)
                require(hashlib.sha256(body).hexdigest() == entry["before"]["sha256"], "corrupt recovery backup")
                require(entry["before"]["mode"] in ("0644", "0755"), "invalid recovery mode")
        for entry in reversed(journal["backups"]):
            path = safe_path(root, entry["path"])
            if entry["before"]:
                write_bytes(path, base64.b64decode(entry["content"]), entry["before"]["mode"])
            else:
                path.unlink(missing_ok=True)
        safe_path(root, TRANSACTION).unlink()


def verify(root):
    state = read_lock(root)
    require(state is not None, "deployment baseline lock missing; explicitly adopt published deployment files")
    require(not plan(root, state["manifest"]), "deployment baseline is incomplete")
    cfg = project.load(safe_path(root, "deploy/linux/project.json"))
    build = project.build_config(cfg, root)
    remote = subprocess.check_output(["git", "-C", str(root), "remote", "get-url", "origin"], text=True).strip()
    project.validate_repository(cfg, remote, os.environ.get("GITHUB_REPOSITORY"))
    require(cfg["platform_version"] == state["manifest"]["version"], "Go/Admin intended platform version differs from deployment baseline")
    go_mod = safe_path(root, build["go_dir"] + "/go.mod").read_text()
    require(re.search(r"(?m)^\s*(?:require\s+)?github\.com/yzxingtu2026/ainp/platform\s+" + re.escape(cfg["platform_version"]) + r"(?:\s|$)", go_mod), "Go dependency differs from deployment baseline")
    require(not re.search(r"(?m)^\s*(?:replace\s+)?github\.com/yzxingtu2026/ainp/platform[^\n]*=>", go_mod), "release baseline forbids platform replacement")
    # Consume the existing Admin gate metadata; do not replace its private format.
    admin = safe_path(root, cfg["admin_baseline"]["path"])
    require(admin.is_file(), "Admin baseline source metadata missing; integrate the project Admin gate first")
    recorded = read_json(admin)
    for field in cfg["admin_baseline"]["commit_field"].split("."):
        require(isinstance(recorded, dict) and field in recorded, "Admin baseline source commit field missing")
        recorded = recorded[field]
    require(recorded == state["manifest"]["commit"], "Admin baseline source commit differs from deployment baseline")
    workflow = safe_path(root, ".github/workflows/deployment-baseline.yml")
    require("scripts/deployment/baseline.py verify" in workflow.read_text(), "deployment verification CI missing")
    ignore = safe_path(root, ".gitignore").read_text()
    for item in ("/.deploy-state/", "/dist-release/", "/deploy/linux/*.local.json", "/.deployment-baseline"):
        require(item in ignore, f"gitignore integration missing: {item}")
    rules = safe_path(root, ".agent/rules/main.md")
    require("binary-deployment" in rules.read_text(), "Agent deployment rule registration missing")
    adapters = read_json(safe_path(root, ".agent/adapters/rules.json"))
    require(any(rule.get("source") == ".agent/rules/binary-deployment.md" for rule in adapters["rules"]), "Agent adapter registration missing")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("plan", "apply", "verify", "recover", "manifest"), nargs="?", default="plan")
    parser.add_argument("--root", type=Path, default=Path.cwd())
    parser.add_argument("--version")
    parser.add_argument("--commit")
    parser.add_argument("--adopt", action="append", default=[], metavar="PATH")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        if args.action == "verify":
            verify(args.root)
            print("Deployment baseline verified (actual files, project and CI contracts).")
        elif args.action == "recover":
            recover(args.root)
            print("Previous files and baseline lock restored; application/database unchanged.")
        elif args.action == "manifest":
            require(args.output is not None, "manifest generation needs --output")
            require(args.commit == subprocess.check_output(["git", "-C", str(args.root), "rev-parse", "HEAD"], text=True).strip(), "manifest commit must be current checkout")
            atomic_json(args.output, manifest_for(args.root, args.version, args.commit), 0o644)
        else:
            require(args.version and args.commit, "source requires --version and --commit; main/latest forbidden")
            with tempfile.TemporaryDirectory(prefix="deployment-source-") as temp:
                source, manifest = fetch_source(args.version, args.commit, Path(temp))
                actions = plan(args.root, manifest, args.adopt)
                print(json.dumps({"mode": args.action, "version": args.version, "commit": args.commit, "actions": actions}, ensure_ascii=False, indent=2))
                if args.action == "apply":
                    apply(args.root, source, manifest, args.adopt)
    except (DeployError, project.ProjectError, ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError) as error:
        print(f"baseline aborted: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
