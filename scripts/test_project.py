"""Project identity / release contract tests, including an application-only downstream."""
import json
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

    def test_invalid_config_is_rejected(self):
        for field, value in (("platform_version", "latest"), ("tag_strategy", "guess"), ("required_checks", []), ("build_config", "../secrets.json"), ("repository", "example/.."), ("default_branch", "main\nvalue=bad")):
            with self.subTest(field=field), tempfile.TemporaryDirectory() as temp:
                cfg = self.config()
                cfg[field] = value
                path = Path(temp) / "project.json"
                path.write_text(json.dumps(cfg))
                with self.assertRaises(project.ProjectError):
                    project.load(path)

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
                ("git", "remote", "get-url", "origin"): "https://github.com/example/application.git",
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
