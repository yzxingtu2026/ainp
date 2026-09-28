#!/usr/bin/env python3
"""Build all app processes once; deterministic, checksummed native release archive."""
import argparse
import gzip
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile

from common import atomic_json, name, require, sha256, validate_manifest, version


def run(args, cwd, env):
    subprocess.run(args, cwd=cwd, env=env, check=True)


def build_web(web_dir, stage, env, identity, cfg=None, output_dir=None, build_command=None, build_env=None, health_path=None):
    # Keep the pre-multi-app positional helper contract usable by offline callers/tests.
    if not isinstance(cfg, dict):
        cfg, output_dir, build_command, build_env = None, cfg, output_dir, build_command
    # Release Web is served behind the same Nginx origin in every environment.
    # Override inherited shell values as well as Vite's .env files.
    if cfg is None:
        cfg = {"admin_application": os.environ.get("AINP_ADMIN_APPLICATION", "./src/composition/application.ts"),
               "admin_modules": os.environ.get("AINP_ADMIN_MODULES", "core,system,operations,ai,demo").split(",")}
    web_env = dict(env, VITE_BUILD_VERSION=identity["version"],
                   VITE_BUILD_COMMIT=identity["commit"], VITE_BUILD_TIME=identity["build_time"],
                   VITE_SERVICE_BASE_URL="/", VITE_OTHER_SERVICE_BASE_URL='{"demo":"/"}',
                   AINP_ADMIN_APPLICATION=cfg["admin_application"],
                   AINP_ADMIN_MODULES=','.join(cfg["admin_modules"]))
    web_env.update(build_env or {})
    mount = "/" if not output_dir or output_dir == "web" else "/" + output_dir.removeprefix("web/").rstrip("/") + "/"
    web_env.update(VITE_BUILD_VERSION=identity["version"], VITE_BUILD_COMMIT=identity["commit"], VITE_BUILD_TIME=identity["build_time"],
                   VITE_BASE_URL=f"/__web/{identity['version']}{mount}", VITE_APP_MOUNT=mount)
    run(["pnpm", "install", "--frozen-lockfile"], web_dir, web_env)
    run(build_command or ["pnpm", "build"], web_dir, web_env)
    effective_output_dir = output_dir or "web"
    destination = stage / effective_output_dir
    require(not destination.exists(), f"duplicate web output: {destination}")
    shutil.copytree(web_dir / "dist", destination)
    if health_path is None:
        manifest_path = destination / "build.json"
    else:
        # Convert the public URL path back to a path inside this app's output.
        # The release stores every app below web/, while health_path includes
        # the app mount (for example /bigscreen/build.json).
        mount = "/" if effective_output_dir == "web" else "/" + effective_output_dir.removeprefix("web/").rstrip("/") + "/"
        require(health_path == "/build.json" if mount == "/" else health_path.startswith(mount),
                "web application health_path must be inside its mounted output")
        relative_path = health_path.lstrip("/") if mount == "/" else health_path[len(mount):]
        manifest_path = destination / relative_path
    require(manifest_path.is_relative_to(destination), "web application health_path must be inside its mounted output")
    atomic_json(manifest_path, identity, 0o644)


def pack(stage, output, identity, epoch):
    require(not (stage / "release.json").exists(), "staging already contains release manifest")
    files = sorted(p for p in stage.rglob("*") if p.is_file())
    require(not any(p.is_symlink() for p in stage.rglob("*")), "release must not contain symlinks")
    manifest = validate_manifest({**identity, "files": {str(p.relative_to(stage)): sha256(p) for p in files}})
    atomic_json(stage / "release.json", manifest, 0o644)
    with output.open("wb") as raw, gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=epoch) as gz, tarfile.open(fileobj=gz, mode="w") as tar:
        for path in sorted(p for p in stage.rglob("*") if p.is_file()):
            item = tar.gettarinfo(str(path), arcname=str(path.relative_to(stage)))
            item.uid = item.gid = 0
            item.uname = item.gname = ""
            item.mtime = epoch
            item.mode = 0o755 if item.name.startswith("bin/") else 0o644
            with path.open("rb") as f:
                tar.addfile(item, f)
    return manifest


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--config", default="deploy/linux/build.json")
    p.add_argument("--version", required=True)
    p.add_argument("--output", default="dist-release")
    args = p.parse_args()
    require(platform.system() == "Linux" and platform.machine() == "x86_64", "build on Ubuntu 22.04 amd64")
    require('VERSION_ID="22.04"' in Path("/etc/os-release").read_text(), "Ubuntu 22.04 build baseline required")
    import project
    project_cfg = project.load()
    project.validate_repository(project_cfg, project.configured_origin(), os.environ.get("GITHUB_REPOSITORY"))
    require(Path(args.config).resolve() == (Path.cwd() / project_cfg["build_config"]).resolve(), "build config must match project configuration")
    cfg = project.build_config(project_cfg, Path.cwd())
    if cfg.get("pre_build_hook"):
        from fetch_runtime_assets import verify_destinations
        verify_destinations(Path.cwd(), cfg["pre_build_hook"]["config"], project.build_web_apps(cfg, Path.cwd()))
    name(cfg["project"])
    version(args.version)
    require(cfg["schema"] == 1, "unsupported build contract")
    root = Path.cwd()
    normalized_apps = project.build_web_apps(cfg, root)
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    epoch = int(subprocess.check_output(["git", "show", "-s", "--format=%ct", "HEAD"], text=True))
    from datetime import datetime, timezone
    build_time = datetime.fromtimestamp(epoch, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    identity = dict(schema=1, project=cfg["project"], version=args.version, commit=commit, build_time=build_time, platform="ubuntu-22.04-amd64")
    env = dict(os.environ, CGO_ENABLED="1", GOOS="linux", GOARCH="amd64", AINP_REQUIRE_PDF_CJK="1")
    require(set(cfg["commands"]) == {"api", "worker", "migrate"}, "build requires api/worker/migrate entrypoints")
    require(env.get("CGO_CFLAGS") and env.get("CGO_LDFLAGS"), "source the pinned MuPDF environment first")
    go_dir = root / cfg["go_dir"]
    cjk_tests = ("TestKnowledgeSafeParserHandlesUniGBReceipt", "TestKnowledgeSafeParserDecodesPDFToUnicodeMapping", "TestKnowledgeSafeParserExtractsEnglishPDF")
    result = subprocess.run(["go", "test", "-tags", "extlib,static", cfg["cjk_test_package"], "-run", "^(" + "|".join(cjk_tests) + ")$", "-count=1", "-v"], cwd=go_dir, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    print(result.stdout, end="")
    require(result.returncode == 0 and all(f"--- PASS: {test} (" in result.stdout for test in cjk_tests), "all three CJK/PDF tests must execute and pass (no skip/no-tests)")
    output = root / args.output
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory() as d:
        stage = Path(d)
        (stage / "bin").mkdir()
        flags = "-s -w " + " ".join(f"-X {cfg['buildinfo_package']}.{k}={v}" for k, v in (("version", args.version), ("commit", commit), ("buildTime", build_time)))
        for role in ("api", "worker", "migrate"):
            binary = stage / "bin" / role
            run(["go", "build", "-tags", "extlib,static", "-trimpath", "-ldflags", flags, "-o", str(binary), cfg["commands"][role]], go_dir, env)
            run([str(binary), "-h"], root, env)
            libraries = subprocess.check_output(["ldd", str(binary)], text=True)
            print(libraries)
            require("not found" not in libraries, "unresolved shared library in native binary")
        for app in normalized_apps:
            build_web(root / app["source_dir"], stage, env, identity, cfg, app["output_dir"], app["build_command"], app["build_env"], app["health_path"])
        if cfg.get("pre_build_hook"):
            from fetch_runtime_assets import verify_staged
            verify_staged(root, cfg["pre_build_hook"]["config"], normalized_apps, stage)
        identity["web_apps"] = [{k: app[k] for k in ("name", "mount", "health_path", "output_dir")} for app in normalized_apps]
        archive = output / "release.tar.gz"
        manifest = pack(stage, archive, identity, epoch)
        atomic_json(output / "release.json", manifest, 0o644)
        atomic_json(output / "reference.json", {**identity, "sha256": sha256(archive)}, 0o644)
        (output / "SHA256SUMS").write_text(f"{sha256(archive)}  release.tar.gz\n")


if __name__ == "__main__":
    main()
