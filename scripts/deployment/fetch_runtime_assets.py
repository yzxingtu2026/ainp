#!/usr/bin/env python3
"""Pull only declared DVC pointers and verify the assets staged for native Web builds."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

try:
    from .project import relative_path, require
except ImportError:
    from project import relative_path, require


class AssetError(ValueError):
    pass


def safe_path(root, value):
    relative_path(value)
    root = Path(root).resolve()
    path = root / value
    require(path.resolve().is_relative_to(root), "asset path escapes repository")
    for part in (root / value).relative_to(root).parts:
        root = root / part
        require(not root.is_symlink(), "asset path contains a symlink")
    return path


def web_targets(apps):
    if isinstance(apps, str):
        apps = [{"source_dir": apps, "output_dir": "web"}]
    require(isinstance(apps, list) and apps, "missing Web applications")
    return {app["source_dir"] + "/public/": app["output_dir"] for app in apps}


def staged_path(asset, apps, stage):
    for public, output in web_targets(apps).items():
        if asset["destination"].startswith(public):
            return Path(stage) / output / asset["destination"][len(public):]
    raise AssetError("runtime asset destination has no Web application")


def load_contract(root, config, apps):
    config_path = safe_path(root, config)
    cfg = json.loads(config_path.read_text())
    require(isinstance(cfg, dict) and set(cfg) == {"schema", "remote", "assets"} and cfg["schema"] == 1, "invalid runtime asset contract")
    require(isinstance(cfg["remote"], str) and re.fullmatch(r"[A-Za-z][A-Za-z0-9_-]*", cfg["remote"]), "invalid DVC remote")
    require(isinstance(cfg["assets"], list) and cfg["assets"], "runtime assets must be nonempty")
    public_dirs = web_targets(apps)
    targets = set()
    sources = set()
    for asset in cfg["assets"]:
        require(isinstance(asset, dict) and set(asset) == {"pointer", "source", "destination", "size", "sha256"}, "invalid runtime asset fields")
        pointer = asset["pointer"]
        source = asset["source"]
        destination = asset["destination"]
        require(isinstance(pointer, str) and isinstance(source, str) and isinstance(destination, str), "invalid runtime asset paths")
        require(pointer.endswith(".dvc") and pointer not in targets, "asset pointer must uniquely name its output")
        require(source not in sources and destination not in targets, "duplicate runtime asset path")
        pointer_path = safe_path(root, pointer)
        source_path = safe_path(root, source)
        destination_path = safe_path(root, destination)
        require(pointer_path.is_file() and not pointer_path.is_symlink(), "runtime asset pointer missing")
        pointer_text = pointer_path.read_text()
        paths = re.findall(r"(?m)^\s*path\s*:\s*(.*?)\s*$", pointer_text)
        hashes = re.findall(r"(?m)^\s*-\s*md5\s*:\s*([0-9a-f]{32}(?:\.dir)?)\s*$", pointer_text)
        require(len(paths) == 1 and len(hashes) == 1
                and len(re.findall(r"(?m)^\s*-\s*", pointer_text)) == 1
                and not re.search(r"(?m)^\s*remote\s*:", pointer_text),
                "pointer must describe exactly one output on the declared remote")
        output = pointer_path.parent / paths[0]
        require(paths[0] == pointer_path.name[:-4] and output == pointer_path.with_suffix(""), "pointer output mismatch")
        if hashes[0].endswith(".dir"):
            require(source_path.is_relative_to(output) and source_path != output, "asset source must be inside pointer directory")
        else:
            require(source_path == output, "file pointer source mismatch")
        require(any(destination.startswith(public) for public in public_dirs), "asset destination must be inside Web public")
        require(source_path != destination_path, "asset source and destination must differ")
        require(type(asset["size"]) is int and asset["size"] > 0, "invalid runtime asset size")
        require(isinstance(asset["sha256"], str) and re.fullmatch(r"[0-9a-f]{64}", asset["sha256"]), "invalid runtime asset digest")
        targets.add(pointer)
        targets.add(destination)
        sources.add(source)
    return cfg


def verified(path, asset):
    require(path.is_file() and not path.is_symlink(), "runtime asset missing")
    require(path.stat().st_size == asset["size"], "runtime asset size mismatch")
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    require(digest.hexdigest() == asset["sha256"], "runtime asset SHA-256 mismatch")


def verify_destinations(root, config, apps):
    cfg = load_contract(root, config, apps)
    for asset in cfg["assets"]:
        verified(safe_path(root, asset["destination"]), asset)


def verify_staged(root, config, apps, stage):
    cfg = load_contract(root, config, apps)
    for asset in cfg["assets"]:
        verified(staged_path(asset, apps, stage), asset)


def execute(root, config, apps, offline=False, cleanup=False):
    cfg = load_contract(root, config, apps) if not cleanup else None
    # Cleanup must validate the same paths, but destinations now exist.
    if cleanup:
        cfg_path = safe_path(root, config)
        cfg = json.loads(cfg_path.read_text())
        require(isinstance(cfg, dict) and isinstance(cfg.get("assets"), list), "invalid cleanup config")
        for asset in cfg["assets"]:
            require(isinstance(asset, dict) and isinstance(asset.get("source"), str)
                    and isinstance(asset.get("pointer"), str), "invalid cleanup source")
            pointer = safe_path(root, asset["pointer"])
            source = safe_path(root, asset["source"])
            require(pointer.is_file() and pointer.suffix == ".dvc" and
                    (source == pointer.with_suffix("") or source.is_relative_to(pointer.with_suffix(""))), "invalid cleanup source")
        for asset in cfg["assets"]:
            source = safe_path(root, asset["source"])
            if source.is_file():
                source.unlink()
        if os.environ.get("GITHUB_ACTIONS") == "true":
            cache = safe_path(root, ".dvc/cache")
            if cache.is_dir():
                shutil.rmtree(cache)
        print("Runtime asset DVC workspace cleaned")
        return
    if not offline:
        credentials = {"OSS_ACCESS_KEY_ID": os.environ.get("AINP_ASSET_STORE_KEY_ID", ""),
                       "OSS_ACCESS_KEY_SECRET": os.environ.get("AINP_ASSET_STORE_KEY_SECRET", ""),
                       "OSS_ENDPOINT": os.environ.get("AINP_ASSET_STORE_ENDPOINT", "")}
        require(all(credentials.values()), "missing runtime asset store credentials")
        env = dict(os.environ, **credentials, DVC_NO_ANALYTICS="true")
        # DVC may echo configuration on errors; never forward its stdout/stderr to CI.
        try:
            subprocess.run(["dvc", "pull", "--quiet", "--remote", cfg["remote"],
                            *(asset["pointer"] for asset in cfg["assets"])],
                           cwd=root, env=env, stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, check=True)
        except (subprocess.CalledProcessError, FileNotFoundError):
            raise AssetError("DVC pull failed for declared runtime pointers") from None
    for asset in cfg["assets"]:
        require(not safe_path(root, asset["destination"]).exists(), "asset destination already exists")
    for asset in cfg["assets"]:
        source = safe_path(root, asset["source"])
        destination = safe_path(root, asset["destination"])
        verified(source, asset)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, destination)
        verified(destination, asset)
    print(f"Verified {len(cfg['assets'])} runtime asset(s)")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True)
    parser.add_argument("--frontend-dir", help="single Web application compatibility mode")
    parser.add_argument("--offline", action="store_true")
    parser.add_argument("--cleanup", action="store_true")
    args = parser.parse_args()
    try:
        if args.frontend_dir:
            apps = args.frontend_dir
        else:
            try:
                from . import project
            except ImportError:
                import project
            root = Path.cwd()
            cfg = project.load(root / "deploy/linux/project.json")
            apps = project.build_web_apps(project.build_config(cfg, root), root)
        execute(Path.cwd(), args.config, apps, args.offline, args.cleanup)
    except (ValueError, OSError, KeyError, TypeError):
        # A fixed diagnostic prevents accidental exposure of secret-bearing paths/values.
        print("Runtime asset hook failed: configuration, pull or verification error", file=sys.stderr)
        raise SystemExit(1) from None


if __name__ == "__main__":
    main()
