"""Project identity / release contract tests, including an application-only downstream."""
import copy
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

    def test_gitea_forge_is_validated_and_defaults_remain_github(self):
        cfg = self.config()
        self.assertEqual(project.load(Path("deploy/baseline/examples/project.json"))["forge"]["type"], "github")
        cfg["forge"] = {"type": "gitea", "server": "https://gitea.example.com"}
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "project.json"
            path.write_text(json.dumps(cfg))
            self.assertEqual(project.load(path)["forge"]["type"], "gitea")
        project.validate_repository(cfg, "https://gitea.example.com/example/application.git")
        cfg["forge"] = {"type": "gitea", "server": "http://gitea.example.com"}
        project.validate_repository(cfg, "http://gitea.example.com/example/application.git")

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
            self.assertNotIn("pre_build_hook", build)
            (root / "assets/runtime").mkdir(parents=True)
            (root / "assets/runtime/model.glb.dvc").write_text("outs:\n- md5: " + "a" * 32 + "\n  size: 1\n  path: model.glb\n")
            contract = {"schema": 1, "remote": "modelstore", "assets": [{
                "pointer": "assets/runtime/model.glb.dvc", "source": "assets/runtime/model.glb",
                "destination": "admin/public/ip/model.glb", "size": 1, "sha256": "0" * 64}]}
            (root / "deploy/linux/runtime-assets.json").write_text(json.dumps(contract))
            build_fixture["pre_build_hook"] = {"type": "runtime-assets", "config": "deploy/linux/runtime-assets.json"}
            (root / cfg["build_config"]).write_text(json.dumps(build_fixture))
            self.assertEqual(project.build_config(cfg, root)["pre_build_hook"], build_fixture["pre_build_hook"])
            # Hook targets the second Web app even without legacy frontend_dir.
            (root / "apps/bigscreen").mkdir(parents=True)
            multi = dict(build_fixture)
            multi.pop("frontend_dir")
            multi["web_apps"] = [{"name": "admin", "source_dir": "admin", "mount": "/"},
                                 {"name": "bigscreen", "source_dir": "apps/bigscreen", "mount": "/bigscreen/"}]
            contract["assets"][0]["destination"] = "apps/bigscreen/public/ip/model.glb"
            (root / "deploy/linux/runtime-assets.json").write_text(json.dumps(contract))
            (root / cfg["build_config"]).write_text(json.dumps(multi))
            self.assertEqual(project.build_config(cfg, root)["web_apps"][1]["name"], "bigscreen")
            build_fixture["pre_build_hook"]["config"] = "../private.json"
            (root / cfg["build_config"]).write_text(json.dumps(build_fixture))
            with self.assertRaises(project.ProjectError):
                project.build_config(cfg, root)
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


class WebBuildConfigTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.cfg = json.loads(Path("deploy/baseline/examples/project.json").read_text())
        self.build = json.loads(Path("deploy/baseline/examples/build-web-apps.json").read_text())
        (self.root / "deploy/linux").mkdir(parents=True)
        for directory in ("backend/cmd/api", "backend/cmd/worker", "backend/cmd/migrate", "admin", "apps/bigscreen"):
            (self.root / directory).mkdir(parents=True)
        for filename in ("go.mod", "go.sum"):
            (self.root / "backend" / filename).write_text("")

    def validate(self, build=None):
        (self.root / self.cfg["build_config"]).write_text(json.dumps(self.build if build is None else build))
        return project.build_config(self.cfg, self.root)

    def test_multi_web_fixture_without_frontend_and_with_legacy_field(self):
        for legacy in (False, True):
            with self.subTest(legacy=legacy):
                build = copy.deepcopy(self.build)
                if legacy:
                    build["frontend_dir"] = "admin"
                self.assertEqual(self.validate(build), build)
                apps = project.build_web_apps(build, self.root)
                self.assertEqual([app["output_dir"] for app in apps], ["web", "web/bigscreen"])
                self.assertEqual(apps[1]["build_command"], ["pnpm", "build:screen"])
                self.assertEqual(apps[1]["build_env"], {"VITE_SCREEN": "yes"})

    def test_legacy_and_optional_app_defaults(self):
        legacy = json.loads(Path("deploy/baseline/examples/build.json").read_text())
        self.validate(legacy)
        expected = dict(name="admin", source_dir="admin", mount="/", health_path="/build.json",
                        build_command=["pnpm", "build"], build_env={}, output_dir="web")
        self.assertEqual(project.build_web_apps(legacy, self.root), [expected])
        self.build["web_apps"] = [{"name": "admin", "source_dir": "admin"}]
        self.validate()
        self.assertEqual(project.build_web_apps(self.build, self.root), [expected])

    def test_invalid_apps_fail_during_preflight(self):
        cases = {
            "name": [None, "UPPER", "bad/name"],
            "source_dir": [None, "", "/tmp", "../outside", "admin/../apps", "missing", "backend/go.mod"],
            "mount": [None, "bigscreen/", "/bigscreen", "/../", "/bigscreen/../", "/bigscreen//"],
            "health_path": [None, "build.json", "/other/build.json", "/bigscreen/../build.json", "/bigscreen//build.json", "/bigscreen/"],
            "build_command": [None, "pnpm build", [], [1], [""], ["pnpm", "bad\x00arg"]],
            "build_env": [None, [], {"BAD=KEY": "value"}, {"": "value"}, {"1BAD": "value"}, {"OK": 1}, {"OK": "bad\x00value"}],
            "unknown": [True],
        }
        for field, values in cases.items():
            for value in values:
                with self.subTest(field=field, value=value):
                    build = copy.deepcopy(self.build)
                    build["web_apps"][1][field] = value
                    with self.assertRaises(project.ProjectError):
                        self.validate(build)
        for apps in (None, [], {}, [None], [{}], [self.build["web_apps"][0]] * 2):
            with self.subTest(apps=apps):
                build = dict(self.build, web_apps=apps)
                with self.assertRaises(project.ProjectError):
                    self.validate(build)
        build = copy.deepcopy(self.build)
        build["web_apps"] = [build["web_apps"][1]]
        with self.assertRaises(project.ProjectError):
            self.validate(build)
        build = copy.deepcopy(self.build)
        build["web_apps"][1]["mount"] = "/"
        build["web_apps"][1]["health_path"] = "/build.json"
        with self.assertRaises(project.ProjectError):
            self.validate(build)
        build = dict(self.build)
        del build["web_apps"]
        with self.assertRaises(project.ProjectError):
            self.validate(build)

    def test_symlink_source_and_legacy_path_cannot_escape_repository(self):
        with tempfile.TemporaryDirectory() as outside:
            (self.root / "external").symlink_to(outside, target_is_directory=True)
            build = copy.deepcopy(self.build)
            build["web_apps"][1]["source_dir"] = "external"
            with self.assertRaises(project.ProjectError):
                self.validate(build)
            build = dict(self.build, frontend_dir="external")
            with self.assertRaises(project.ProjectError):
                self.validate(build)


if __name__ == "__main__":
    unittest.main()
