#!/usr/bin/env python3
"""Build all app processes once; deterministic, checksummed native release archive."""
import argparse
import gzip
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile

from common import atomic_json, name, require, sha256, validate_manifest, version


def run(args, cwd, env):
    subprocess.run(args, cwd=cwd, env=env, check=True)


def build_web(web_dir, stage, env, identity):
    # Release Web is served behind the same Nginx origin in every environment.
    # Override inherited shell values as well as Vite's .env files.
    web_env = dict(env, VITE_BUILD_VERSION=identity["version"],
                   VITE_BUILD_COMMIT=identity["commit"], VITE_BUILD_TIME=identity["build_time"],
                   VITE_SERVICE_BASE_URL="/", VITE_OTHER_SERVICE_BASE_URL='{"demo":"/"}')
    run(["pnpm", "install", "--frozen-lockfile"], web_dir, web_env)
    run(["pnpm", "build"], web_dir, web_env)
    shutil.copytree(web_dir / "dist", stage / "web")
    atomic_json(stage / "web/build.json", identity, 0o644)


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
    project.validate_repository(project_cfg, subprocess.check_output(["git", "remote", "get-url", "origin"], text=True).strip(), os.environ.get("GITHUB_REPOSITORY"))
    require(Path(args.config).resolve() == (Path.cwd() / project_cfg["build_config"]).resolve(), "build config must match project configuration")
    cfg = project.build_config(project_cfg, Path.cwd())
    name(cfg["project"])
    version(args.version)
    require(cfg["schema"] == 1, "unsupported build contract")
    root = Path.cwd()
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
        build_web(root / cfg["frontend_dir"], stage, env, identity)
        archive = output / "release.tar.gz"
        manifest = pack(stage, archive, identity, epoch)
        atomic_json(output / "release.json", manifest, 0o644)
        atomic_json(output / "reference.json", {**identity, "sha256": sha256(archive)}, 0o644)
        (output / "SHA256SUMS").write_text(f"{sha256(archive)}  release.tar.gz\n")


if __name__ == "__main__":
    main()
