"""Public, credential-free project identity used by local tools and Actions."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import urlsplit

DEFAULT_CONFIG = Path(__file__).resolve().parents[2] / "deploy/linux/project.json"


class ProjectError(ValueError):
    pass


def require(ok, message):
    if not ok:
        raise ProjectError(message)


def relative_path(value):
    require(isinstance(value, str) and re.fullmatch(r"[A-Za-z0-9_./-]+", value)
            and not value.startswith("/") and all(p not in ("", ".", "..") for p in value.split("/")),
            f"unsafe project path: {value!r}")
    return value


def load(path=DEFAULT_CONFIG):
    cfg = json.loads(Path(path).read_text())
    required_fields = {"schema", "project", "repository", "default_branch", "required_checks", "tag_strategy", "build_config", "platform_version", "retired_workflows", "admin_baseline"}
    require(required_fields <= set(cfg) <= required_fields | {"forge"}, "invalid project configuration fields")
    forge = cfg.setdefault("forge", {"type": "github"})
    require(isinstance(forge, dict) and set(forge) <= {"type", "server"} and forge.get("type") in ("github", "gitea"), "invalid forge configuration")
    if forge["type"] == "github":
        require(set(forge) == {"type"} or forge.get("server") == "https://github.com", "GitHub forge server must be omitted or https://github.com")
    else:
        server = forge.get("server", "")
        parsed = urlsplit(server)
        require(parsed.scheme in ("http", "https") and parsed.netloc and not parsed.query and not parsed.fragment and server.rstrip("/") == server, "Gitea forge requires an absolute server URL")
    require(cfg["schema"] == 1, "unsupported project configuration")
    require(isinstance(cfg["project"], str) and re.fullmatch(r"[a-z][a-z0-9-]{1,30}", cfg["project"]), "invalid project name")
    require(isinstance(cfg["repository"], str) and re.fullmatch(r"[A-Za-z0-9_-]+/[A-Za-z0-9_.-]+", cfg["repository"]) and cfg["repository"].split("/")[1] not in (".", ".."), "invalid project repository")
    relative_path(cfg["default_branch"])
    require(isinstance(cfg["required_checks"], list) and cfg["required_checks"] and all(isinstance(c, str) and c.strip() == c and c for c in cfg["required_checks"]) and len(set(cfg["required_checks"])) == len(cfg["required_checks"]), "required_checks must contain unique check names")
    require(cfg["tag_strategy"] in ("application", "platform-pair"), "invalid tag strategy")
    require(cfg["platform_version"] == "self" and cfg["tag_strategy"] == "platform-pair" or isinstance(cfg["platform_version"], str) and re.fullmatch(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", cfg["platform_version"]), "platform_version must pin a stable platform version (self only for platform-pair)")
    require(isinstance(cfg["retired_workflows"], list), "retired_workflows must be a list")
    for workflow in cfg["retired_workflows"]:
        relative_path(workflow)
        require(workflow.startswith(".github/workflows/") and workflow.endswith((".yml", ".yaml")), "retired workflow must name an old workflow")
    admin = cfg["admin_baseline"]
    require(isinstance(admin, dict) and set(admin) == {"path", "commit_field"}, "admin_baseline requires path and commit_field")
    relative_path(admin["path"])
    require(isinstance(admin["commit_field"], str) and re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*", admin["commit_field"]), "invalid Admin source commit field")
    relative_path(cfg["build_config"])
    return cfg



def configured_origin(root=None):
    """Read the stored origin, before Git's insteadOf authentication rewriting."""
    args = ["git"]
    if root is not None:
        args.extend(["-C", str(root)])
    return subprocess.check_output(args + ["config", "--get", "remote.origin.url"], text=True).strip()


def validate_repository(cfg, remote=None, workflow_repository=None):
    repo = cfg["repository"]
    forge = cfg.get("forge", {"type": "github"})
    forge_url = urlsplit(forge.get("server", "https://github.com"))
    host = "github.com" if forge["type"] == "github" else forge_url.netloc
    scheme = "https" if forge["type"] == "github" else forge_url.scheme
    if remote is not None:
        require(remote in {f"{scheme}://{host}/{repo}", f"{scheme}://{host}/{repo}.git", f"git@{host}:{repo}.git", f"ssh://git@{host}/{repo}.git"}, "origin does not match project repository")
    if workflow_repository is not None:
        require(workflow_repository == repo, "workflow repository does not match project repository")


def tags(cfg, version):
    return [version, f"platform/{version}"] if cfg["tag_strategy"] == "platform-pair" else [version]


def build_web_apps(build, root):
    """Validate and normalize the shared preflight/package Web build contract."""
    root = Path(root).resolve()
    apps = build.get("web_apps")
    if "web_apps" not in build:
        require("frontend_dir" in build, "frontend_dir required when web_apps is absent")
        apps = [{"name": "admin", "source_dir": build["frontend_dir"]}]
    require(isinstance(apps, list) and apps, "at least one web application required")
    normalized = []
    names, mounts = set(), set()
    for app in apps:
        require(isinstance(app, dict) and {"name", "source_dir"} <= set(app)
                and set(app) <= {"name", "source_dir", "mount", "build_command", "build_env", "health_path"},
                "invalid web application fields")
        name, source = app["name"], app["source_dir"]
        require(isinstance(name, str) and re.fullmatch(r"[a-z][a-z0-9-]{0,63}", name)
                and name not in names, "invalid or duplicate web application name")
        relative_path(source)
        require((root / source).resolve().is_relative_to(root), "web application source_dir escapes repository")
        require((root / source).is_dir(), "web application source_dir must be a directory")
        mount = app.get("mount", "/")
        require(isinstance(mount, str) and (mount == "/" or re.fullmatch(r"/[A-Za-z0-9._~-]+(?:/[A-Za-z0-9._~-]+)*/", mount))
                and all(part not in (".", "..") for part in mount.strip("/").split("/"))
                and mount not in mounts, "invalid or duplicate web application mount")
        health = app.get("health_path", mount + "build.json")
        require(isinstance(health, str) and re.fullmatch(r"/[A-Za-z0-9._~/-]+", health)
                and all(part not in ("", ".", "..") for part in health[1:].split("/")),
                "invalid web application health_path")
        require(health == "/build.json" if mount == "/" else health.startswith(mount),
                "web application health_path must be inside its mount")
        command = app.get("build_command", ["pnpm", "build"])
        require(isinstance(command, list) and command and all(isinstance(arg, str) and arg and "\x00" not in arg for arg in command),
                "invalid web application build_command")
        env = app.get("build_env", {})
        require(isinstance(env, dict) and all(isinstance(key, str) and re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key)
                and isinstance(value, str) and "\x00" not in value for key, value in env.items()),
                "invalid web application build_env")
        normalized.append(dict(name=name, source_dir=source, mount=mount, health_path=health,
                               build_command=command, build_env=env,
                               output_dir="web" if mount == "/" else "web/" + mount.strip("/")))
        names.add(name)
        mounts.add(mount)
    require("/" in mounts, "web applications require a root mount")
    return normalized


def build_config(cfg, root):
    root = Path(root).resolve()
    path = root / cfg["build_config"]
    build = json.loads(path.read_text())
    require(build.get("schema") == 1 and build.get("project") == cfg["project"], "build/project identity mismatch")
    required = {"schema", "project", "go_dir", "commands", "buildinfo_package", "cjk_test_package", "node_version", "pnpm_version", "admin_application", "admin_modules", "go_private"}
    require(required <= set(build) <= required | {"frontend_dir", "web_apps", "pre_build_hook"}, "invalid build configuration fields")
    for field in ("go_dir", "frontend_dir"):
        if field not in build:
            continue
        relative_path(build[field])
        require((root / build[field]).resolve().is_relative_to(root), f"{field} escapes repository")
    apps = build_web_apps(build, root)
    hook = build.get("pre_build_hook")
    if hook is not None:
        require(isinstance(hook, dict) and set(hook) == {"type", "config"} and hook["type"] == "runtime-assets", "invalid pre_build_hook")
        relative_path(hook["config"])
        require(hook["config"].startswith("deploy/linux/") and hook["config"].endswith(".json"), "pre_build_hook config must be project owned")
        require((root / hook["config"]).is_file() and (root / hook["config"]).resolve().is_relative_to(root), "missing or unsafe pre_build_hook config")
        try:
            from .fetch_runtime_assets import load_contract
        except ImportError:
            from fetch_runtime_assets import load_contract
        load_contract(root, hook["config"], apps)
    require(set(build["commands"]) == {"api", "worker", "migrate"}, "three composition commands required")
    require(isinstance(build["node_version"], str) and re.fullmatch(r"\d+\.\d+\.\d+", build["node_version"]), "invalid Node version")
    require(isinstance(build["pnpm_version"], str) and re.fullmatch(r"\d+\.\d+\.\d+", build["pnpm_version"]), "invalid pnpm version")
    admin_application = build["admin_application"][2:] if build["admin_application"].startswith("./") else build["admin_application"]
    relative_path(admin_application)
    require(build["admin_application"].endswith((".ts", ".tsx", ".js", ".jsx")), "invalid Admin application entry")
    require(isinstance(build["admin_modules"], list) and build["admin_modules"] and all(isinstance(module, str) and re.fullmatch(r"[a-z][a-z0-9-]*", module) for module in build["admin_modules"]) and len(set(build["admin_modules"])) == len(build["admin_modules"]), "admin_modules must contain unique module names")
    require(isinstance(build["go_private"], list) and all(isinstance(pattern, str) and re.fullmatch(r"[A-Za-z0-9*?._/-]+", pattern) and not pattern.startswith("-") for pattern in build["go_private"]), "invalid private Go module patterns")
    for command in build["commands"].values():
        require(isinstance(command, str) and command.startswith("./"), "composition command must be local")
        relative_path(command[2:])
        entry = root / build["go_dir"] / command[2:]
        require(entry.resolve().is_relative_to(root) and entry.is_dir(), f"composition command missing: {command}")
    require(build["buildinfo_package"] == "github.com/yzxingtu2026/ainp/platform/framework/buildinfo", "unsupported build identity package")
    package = build["cjk_test_package"]
    require(package == "github.com/yzxingtu2026/ainp/platform/contracts/pdfcjk" or cfg["tag_strategy"] == "platform-pair" and package == "./tests", "use the platform public PDF/CJK contract tests")
    for suffix in ("go.mod", "go.sum"):
        require((root / build["go_dir"] / suffix).is_file(), f"missing {suffix}")
    return build


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", type=Path, default=DEFAULT_CONFIG)
    args = parser.parse_args()
    cfg = load(args.config)
    validate_repository(cfg, configured_origin(), os.environ.get("GITHUB_REPOSITORY"))
    build = build_config(cfg, Path.cwd())
    print(json.dumps(cfg, sort_keys=True))
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a") as output:
            for key, value in {
                "build-config": cfg["build_config"],
                "go-version-file": build["go_dir"] + "/go.mod",
                "go-sum-file": build["go_dir"] + "/go.sum",
                "node-version": build["node_version"],
                "pnpm-version": build["pnpm_version"],
                "admin-application": build["admin_application"],
                "admin-modules": ",".join(build["admin_modules"]),
                "go-private": ",".join(build["go_private"]),
                "pre-build-hook-config": build.get("pre_build_hook", {}).get("config", ""),
            }.items():
                output.write(f"{key}={value}\n")


if __name__ == "__main__":
    main()
