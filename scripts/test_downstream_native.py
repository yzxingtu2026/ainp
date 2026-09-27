#!/usr/bin/env python3
"""Build an isolated backend/admin application with unchanged public deployment code.

PR-only integration fixture uses a local platform replacement to test unpublished code;
production baseline plan/apply never supports that source mode.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts/deployment"))
import common
import verify_release


def run(argv, cwd, env=None):
    subprocess.run(argv, cwd=cwd, env=env, check=True)


def prepare(root):
    ignore = shutil.ignore_patterns("node_modules", "__pycache__", "dist", "dist-*", ".git", ".env.local")
    for directory in ("scripts", "deploy/baseline", ".github/prompts", "docker"):
        shutil.copytree(ROOT / directory, root / directory, ignore=ignore)
    shutil.copytree(ROOT / "frontend", root / "admin", ignore=ignore)
    shutil.copytree(ROOT / "platform/examples/downstream", root / "backend", ignore=ignore)
    shutil.copytree(ROOT / "scripts/fixtures/deployment/backend/cmd", root / "backend/cmd")
    go_mod = root / "backend/go.mod"
    go_mod.write_text(go_mod.read_text().replace("=> ../..", f"=> {ROOT / 'platform'}").replace("platform v0.0.0", "platform v0.4.0"))
    config_dir = root / "deploy/linux"
    config_dir.mkdir(parents=True)
    for name in ("project.json", "build.json"):
        shutil.copy2(ROOT / "deploy/baseline/examples" / name, config_dir / name)
    package = root / "admin/package.json"
    data = json.loads(package.read_text())
    data["scripts"]["build"] = data["scripts"]["build:downstream-example"]
    package.write_text(json.dumps(data, indent=2) + "\n")
    run(["git", "init", "-b", "main"], root)
    run(["git", "remote", "add", "origin", "https://github.com/example/application.git"], root)
    run(["git", "add", "."], root)
    run(["git", "-c", "user.name=Deployment fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "Independent application composition"], root)


def check_artifacts(root, env):
    ref = verify_release.verify_local(root / "dist-release", "v0.0.0-ci")
    common.require(ref["project"] == "example-app", "fixture built wrong project")
    stage = root / "extracted"
    common.extract_verified(root / "dist-release/release.tar.gz", stage, ref["sha256"])
    identity = {key: ref[key] for key in ("version", "commit", "build_time")}
    for role in ("api", "worker", "migrate"):
        built = json.loads(subprocess.check_output([str(stage / "bin" / role), "-version"], env=env))
        common.require(built == identity, f"{role} build identity mismatch")
        metadata = subprocess.check_output(["go", "version", "-m", str(stage / "bin" / role)], text=True)
        common.require(f"example.com/ainp-downstream/cmd/{role}" in metadata, f"wrong {role} composition")
    web = common.read_json(stage / "web/build.json")
    common.require(all(web[key] == value for key, value in identity.items()), "Web identity mismatch")
    js_files = list((stage / "web").rglob("*.js"))
    common.require(js_files, "static Web missing")
    text = "\n".join(path.read_text() for path in js_files)
    common.require("injected-evil.invalid" not in text, "Web inherited cross-origin API")
    common.require("demo-task-1" not in text and "/api/v1/demo/hello" not in text, "downstream Web contains Demo")
    print("PASS: independent backend/admin; three actual PDF/CJK tests; API/Worker/migrate/Web identities; same-origin Web")


def main():
    with tempfile.TemporaryDirectory(prefix="ainp-independent-app-") as temp:
        root = Path(temp)
        prepare(root)
        env = dict(os.environ, GITHUB_REPOSITORY="example/application", VITE_SERVICE_BASE_URL="https://injected-evil.invalid", VITE_OTHER_SERVICE_BASE_URL='{"demo":"https://injected-evil.invalid"}')
        env.pop("GITHUB_OUTPUT", None)
        run(["python3", "scripts/deployment/project.py"], root, env)
        run(["python3", "scripts/deployment/package.py", "--version", "v0.0.0-ci"], root, env)
        check_artifacts(root, env)


if __name__ == "__main__":
    main()
