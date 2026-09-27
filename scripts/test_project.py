"""Project identity / release contract tests, including an application-only downstream."""
import json
import subprocess
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

from scripts.deployment import project
from scripts import release


class ProjectTest(unittest.TestCase):
    def config(self):
        return json.loads(Path("deploy/baseline/examples/project.json").read_text())

    def test_application_tags_and_repository_boundaries(self):
        cfg = self.config()
        self.assertEqual(project.tags(cfg, "v1.2.3"), ["v1.2.3"])
        project.validate_repository(cfg, "git@github.com:example/application.git", "example/application")
        for remote, workflow in (("https://github.com/yzxingtu2026/ainp.git", None), (None, "yzxingtu2026/ainp")):
            with self.assertRaises(project.ProjectError):
                project.validate_repository(cfg, remote, workflow)

    def test_origin_validation_uses_raw_remote_with_private_module_rewrite(self):
        cfg = self.config()
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            subprocess.run(["git", "-C", str(root), "remote", "add", "origin",
                            f"https://github.com/{cfg['repository']}.git"], check=True)
            rewrite = "url.https://x-access-token:FAKE_TOKEN@github.com/.insteadOf=https://github.com/"
            expanded = subprocess.check_output(
                ["git", "-C", str(root), "-c", rewrite, "remote", "get-url", "origin"], text=True).strip()
            self.assertIn("FAKE_TOKEN", expanded)
            remote = project.configured_origin(root)
            self.assertEqual(remote, f"https://github.com/{cfg['repository']}.git")
            project.validate_repository(cfg, remote, cfg["repository"])
            with self.assertRaises(project.ProjectError):
                project.validate_repository(cfg, expanded, cfg["repository"])
            with self.assertRaises(project.ProjectError):
                project.validate_repository(cfg, remote, "wrong/project")
            subprocess.run(["git", "-C", str(root), "remote", "set-url", "origin",
                            "https://github.com/wrong/project.git"], check=True)
            with self.assertRaises(project.ProjectError):
                project.validate_repository(cfg, project.configured_origin(root), cfg["repository"])

    def test_invalid_config_is_rejected(self):
        for field, value in (("platform_version", "latest"), ("tag_strategy", "guess"), ("required_checks", []), ("build_config", "../secrets.json"), ("repository", "example/.."), ("default_branch", "main\nvalue=bad")):
            with self.subTest(field=field), tempfile.TemporaryDirectory() as temp:
                cfg = self.config()
                cfg[field] = value
                path = Path(temp) / "project.json"
                path.write_text(json.dumps(cfg))
                with self.assertRaises(project.ProjectError):
                    project.load(path)

    def test_build_contract_exposes_downstream_toolchain_and_composition(self):
        cfg = self.config()
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "deploy/linux").mkdir(parents=True)
            (root / "backend/cmd/api").mkdir(parents=True)
            (root / "backend/cmd/worker").mkdir(parents=True)
            (root / "backend/cmd/migrate").mkdir(parents=True)
            (root / "admin/src/composition").mkdir(parents=True)
            (root / "backend/go.mod").write_text("module example/application\n")
            (root / "backend/go.sum").write_text("")
            (root / "admin/src/composition/application.ts").write_text("")
            build_fixture = {
                "schema": 1,
                "project": cfg["project"],
                "go_dir": "backend",
                "frontend_dir": "admin",
                "commands": {
                    "api": "./cmd/api",
                    "worker": "./cmd/worker",
                    "migrate": "./cmd/migrate",
                },
                "buildinfo_package": "github.com/yzxingtu2026/ainp/platform/framework/buildinfo",
                "cjk_test_package": "github.com/yzxingtu2026/ainp/platform/contracts/pdfcjk",
                "node_version": "22.22.0",
                "pnpm_version": "10.26.2",
                "admin_application": "./src/composition/application.ts",
                "admin_modules": ["core", "system", "operations", "ai", "demo"],
                "go_private": [],
            }
            (root / cfg["build_config"]).write_text(json.dumps(build_fixture))
            build = project.build_config(cfg, root)
        self.assertEqual(build["node_version"], "22.22.0")
        self.assertEqual(build["pnpm_version"], "10.26.2")
        self.assertEqual(build["admin_application"], "./src/composition/application.ts")
        self.assertEqual(build["admin_modules"], ["core", "system", "operations", "ai", "demo"])
        self.assertEqual(build["go_private"], [])

    def test_build_contract_rejects_admin_entry_outside_frontend(self):
        cfg = self.config()
        path = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(path, ignore_errors=True))
        (path / "deploy/linux").mkdir(parents=True)
        (path / "admin/src").mkdir(parents=True)
        (path / "backend/cmd/api").mkdir(parents=True)
        (path / "backend/cmd/worker").mkdir(parents=True)
        (path / "backend/cmd/migrate").mkdir(parents=True)
        for name in ("go.mod", "go.sum"):
            (path / "backend" / name).write_text("")
        (path / "admin/src/application.ts").write_text("")
        (path / "backend/go.mod").write_text("module example\n")
        bad = dict(cfg, build_config="deploy/linux/build.json")
        config = {"schema": 1, "project": cfg["project"], "go_dir": "backend", "frontend_dir": "admin", "commands": {"api": "./cmd/api", "worker": "./cmd/worker", "migrate": "./cmd/migrate"}, "buildinfo_package": "github.com/yzxingtu2026/ainp/platform/framework/buildinfo", "cjk_test_package": "github.com/yzxingtu2026/ainp/platform/contracts/pdfcjk", "node_version": "22.22.0", "pnpm_version": "10.26.2", "admin_application": "../outside.ts", "admin_modules": ["core"], "go_private": []}
        (path / "deploy/linux/build.json").write_text(json.dumps(config))
        with self.assertRaises(project.ProjectError):
            project.build_config(bad, path)

    def test_downstream_preflight_queries_only_its_own_repository(self):
        cfg = self.config()
        cfg["default_branch"] = "trunk"
        cfg["required_checks"] = ["application-ci"]
        calls = []
        def run(*args, **kwargs):
            calls.append(args)
            responses = {
                ("git", "rev-parse", "--show-toplevel"): str(Path.cwd()),
                ("git", "branch", "--show-current"): "trunk",
                ("git", "config", "--get", "remote.origin.url"): "https://github.com/example/application.git",
                ("git", "rev-parse", "HEAD"): "abc",
                ("git", "rev-parse", "origin/trunk"): "abc",
                ("gh", "api", "repos/example/application/commits/abc/pulls?per_page=100"): json.dumps([dict(base={"ref": "trunk"}, merged_at="now", merge_commit_sha="abc", head={"sha": "head"})]),
                ("gh", "api", "repos/example/application/commits/head/check-runs?per_page=100"): json.dumps({"check_runs": [{"name": "application-ci", "conclusion": "success"}]}),
                ("gh", "variable", "get", "OSS_BUCKET", "--env", "release"): "private-bucket",
            }
            return responses.get(args, "")
        with mock.patch.dict(release.os.environ, {"GITHUB_REPOSITORY": cfg["repository"]}), mock.patch.multiple(release, PROJECT=cfg, REPOSITORY=cfg["repository"], DEFAULT_BRANCH="trunk", REQUIRED_CHECKS={"application-ci"}), mock.patch.object(release, "run", side_effect=run):
            self.assertEqual(release.preflight("v1.2.3"), ("abc", "private-bucket"))
        flattened = " ".join(" ".join(args) for args in calls)
        self.assertNotIn("yzxingtu2026", flattened)
        self.assertNotIn("platform/v", flattened)
        self.assertIn("fetch origin trunk", flattened)

    def test_application_push_is_single_tag_atomic(self):
        cfg = self.config()
        with mock.patch.object(release, "PROJECT", cfg), mock.patch.object(release, "preflight", return_value=("abc", "bucket")), mock.patch.object(release, "run") as run, mock.patch.object(sys, "argv", ["release", "v1.2.3", "--yes"]):
            self.assertEqual(release.main(), 0)
        self.assertEqual(run.call_args_list, [mock.call("git", "tag", "--annotate", "v1.2.3", "--message", "Release v1.2.3"), mock.call("git", "push", "--atomic", "origin", "refs/tags/v1.2.3")])


if __name__ == "__main__":
    unittest.main()
