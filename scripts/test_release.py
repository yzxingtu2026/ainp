import importlib.util
import json
import unittest
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location("release", Path(__file__).with_name("release.py"))
release = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
SPEC.loader.exec_module(release)


class ReleaseValidationTest(unittest.TestCase):
    TEST_PROJECT = {
        "project": "example-app",
        "repository": "example/application",
        "default_branch": "main",
        "required_checks": ["backend", "frontend"],
        "tag_strategy": "platform-pair",
    }

    def setUp(self):
        self.project = dict(self.TEST_PROJECT)
        self.repository = self.project["repository"]
        self.patcher = mock.patch.multiple(
            release,
            PROJECT=self.project,
            REPOSITORY=self.repository,
            DEFAULT_BRANCH=self.project["default_branch"],
            REQUIRED_CHECKS=set(self.project["required_checks"]),
        )
        self.patcher.start()
        self.env_patcher = mock.patch.dict(release.os.environ, {"GITHUB_REPOSITORY": self.repository})
        self.env_patcher.start()
        self.addCleanup(self.env_patcher.stop)
        self.addCleanup(self.patcher.stop)

    def test_version_is_strict_stable_semver(self):
        self.assertEqual(release.validate_version("v1.2.3"), "v1.2.3")
        self.assertEqual(release.platform_tag("v1.2.3"), "platform/v1.2.3")
        for invalid in ("1.2.3", "v1.2", "v1.2.3-rc.1", "v01.2.3"):
            with self.assertRaises(release.ReleaseError):
                release.validate_version(invalid)

    def test_remote_is_exact_repository(self):
        release.validate_remote(f"https://github.com/{self.repository}.git")
        with self.assertRaises(release.ReleaseError):
            release.validate_remote("https://github.com/example/other.git")

    def test_successful_checks_require_named_successes(self):
        payload = {"check_runs": [
            {"name": "backend", "conclusion": "failure"},
            {"name": "backend", "conclusion": "success"},
            {"name": "frontend", "conclusion": "success"},
            {"name": "unrelated", "conclusion": "success"},
        ]}
        self.assertEqual(release.successful_checks(payload), set(self.project["required_checks"]))

    def test_merged_pull_request_head_sha_resolves_squash_merge(self):
        payload = [{
            "number": 354,
            "base": {"ref": "main"},
            "head": {"sha": "pr-head"},
            "merge_commit_sha": "merge-sha",
            "merged_at": "2026-09-26T06:23:01Z",
        }]
        with mock.patch.object(
            release,
            "run",
            return_value=json.dumps(payload),
        ) as run:
            self.assertEqual(release.merged_pull_request_head_sha("merge-sha"), "pr-head")
        run.assert_called_once_with(
            "gh",
            "api",
            f"repos/{self.repository}/commits/merge-sha/pulls?per_page=100",
        )

    def test_merged_pull_request_head_sha_fails_closed(self):
        payload = [{
            "number": 354,
            "base": {"ref": "develop"},
            "head": {"sha": "pr-head"},
            "merge_commit_sha": "merge-sha",
            "merged_at": "2026-09-26T06:23:01Z",
        }]
        with mock.patch.object(release, "run", return_value=json.dumps(payload)):
            with self.assertRaisesRegex(release.ReleaseError, "exactly one merged main PR"):
                release.merged_pull_request_head_sha("merge-sha")

    def successful_preflight_run(self, overrides=None):
        overrides = overrides or {}
        root = str(Path.cwd().resolve())
        successes = {"check_runs": [{"name": name, "conclusion": "success"} for name in release.REQUIRED_CHECKS]}
        repository = self.repository
        default_branch = self.project["default_branch"]
        responses = {
            ("git", "rev-parse", "--show-toplevel"): root,
            ("git", "branch", "--show-current"): "main",
            ("git", "status", "--porcelain=v1", "--untracked-files=all"): "",
            ("git", "config", "--get", "remote.origin.url"): f"https://github.com/{repository}.git",
            ("gh", "auth", "status", "--hostname", "github.com"): "",
            ("git", "fetch", "origin", default_branch, "--tags", "--prune"): "",
            ("git", "rev-parse", "HEAD"): "abc",
            ("git", "rev-parse", f"origin/{default_branch}"): "abc",
            ("git", "tag", "--list", "v1.2.3"): "",
            ("git", "ls-remote", "--tags", "origin", "refs/tags/v1.2.3"): "",
            ("git", "tag", "--list", "platform/v1.2.3"): "",
            ("git", "ls-remote", "--tags", "origin", "refs/tags/platform/v1.2.3"): "",
            ("gh", "api", f"repos/{repository}/commits/abc/pulls?per_page=100"): json.dumps([{
                "base": {"ref": "main"},
                "head": {"sha": "pr-head"},
                "merge_commit_sha": "abc",
                "merged_at": "2026-09-26T06:23:01Z",
            }]),
            ("gh", "api", f"repos/{repository}/commits/pr-head/check-runs?per_page=100"): json.dumps(successes),
            ("gh", "variable", "get", "OSS_BUCKET", "--env", "release"): "example-release-artifacts",
        }
        responses.update(overrides)

        def fake_run(*args, check=True):
            del check
            return responses[args]

        return fake_run

    def test_preflight_accepts_only_fully_synchronized_main(self):
        with mock.patch.object(release, "run", side_effect=self.successful_preflight_run()):
            self.assertEqual(release.preflight("v1.2.3"), ("abc", "example-release-artifacts"))

    def test_preflight_rejects_dirty_diverged_duplicate_and_missing_ci(self):
        repository = self.repository
        default_branch = self.project["default_branch"]
        cases = [
            ({("git", "branch", "--show-current"): "feature"}, "from main"),
            ({("git", "status", "--porcelain=v1", "--untracked-files=all"): "?? secret.env"}, "not clean"),
            ({("git", "rev-parse", f"origin/{default_branch}"): "def"}, "exactly match"),
            ({("git", "tag", "--list", "v1.2.3"): "v1.2.3"}, "already exists"),
            ({("gh", "api", f"repos/{repository}/commits/pr-head/check-runs?per_page=100"): '{"check_runs":[]}'}, "CI is not successful"),
            ({("gh", "variable", "get", "OSS_BUCKET", "--env", "release"): ""}, "OSS_BUCKET is not configured"),
        ]
        for overrides, message in cases:
            with self.subTest(message=message), mock.patch.object(release, "run", side_effect=self.successful_preflight_run(overrides)):
                with self.assertRaisesRegex(release.ReleaseError, message):
                    release.preflight("v1.2.3")


if __name__ == "__main__":
    unittest.main()
