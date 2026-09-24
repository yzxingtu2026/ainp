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
    def test_version_is_strict_stable_semver(self):
        self.assertEqual(release.validate_version("v1.2.3"), "v1.2.3")
        self.assertEqual(release.platform_tag("v1.2.3"), "platform/v1.2.3")
        for invalid in ("1.2.3", "v1.2", "v1.2.3-rc.1", "v01.2.3"):
            with self.assertRaises(release.ReleaseError):
                release.validate_version(invalid)

    def test_remote_is_exact_repository(self):
        release.validate_remote("https://github.com/yzxingtu2026/ainp.git")
        with self.assertRaises(release.ReleaseError):
            release.validate_remote("https://github.com/example/ainp.git")

    def test_successful_checks_require_named_successes(self):
        payload = {"check_runs": [
            {"name": "backend", "conclusion": "failure"},
            {"name": "backend", "conclusion": "success"},
            {"name": "frontend", "conclusion": "success"},
            {"name": "unrelated", "conclusion": "success"},
        ]}
        self.assertEqual(release.successful_checks(payload), {"backend", "frontend"})

    def successful_preflight_run(self, overrides=None):
        overrides = overrides or {}
        root = str(Path.cwd().resolve())
        successes = {"check_runs": [{"name": name, "conclusion": "success"} for name in release.REQUIRED_CHECKS]}
        responses = {
            ("git", "rev-parse", "--show-toplevel"): root,
            ("git", "branch", "--show-current"): "main",
            ("git", "status", "--porcelain=v1", "--untracked-files=all"): "",
            ("git", "remote", "get-url", "origin"): "https://github.com/yzxingtu2026/ainp.git",
            ("gh", "auth", "status", "--hostname", "github.com"): "",
            ("git", "fetch", "origin", "main", "--tags", "--prune"): "",
            ("git", "rev-parse", "HEAD"): "abc",
            ("git", "rev-parse", "origin/main"): "abc",
            ("git", "tag", "--list", "v1.2.3"): "",
            ("git", "ls-remote", "--tags", "origin", "refs/tags/v1.2.3"): "",
            ("git", "tag", "--list", "platform/v1.2.3"): "",
            ("git", "ls-remote", "--tags", "origin", "refs/tags/platform/v1.2.3"): "",
            ("gh", "api", "repos/yzxingtu2026/ainp/commits/abc/check-runs?per_page=100"): json.dumps(successes),
            ("gh", "variable", "get", "OSS_BUCKET", "--env", "release"): "ainp-test-artifacts",
        }
        responses.update(overrides)

        def fake_run(*args, check=True):
            del check
            return responses[args]

        return fake_run

    def test_preflight_accepts_only_fully_synchronized_main(self):
        with mock.patch.object(release, "run", side_effect=self.successful_preflight_run()):
            self.assertEqual(release.preflight("v1.2.3"), ("abc", "ainp-test-artifacts"))

    def test_preflight_rejects_dirty_diverged_duplicate_and_missing_ci(self):
        cases = [
            ({("git", "branch", "--show-current"): "feature"}, "from main"),
            ({("git", "status", "--porcelain=v1", "--untracked-files=all"): "?? secret.env"}, "not clean"),
            ({("git", "rev-parse", "origin/main"): "def"}, "exactly match"),
            ({("git", "tag", "--list", "v1.2.3"): "v1.2.3"}, "already exists"),
            ({("gh", "api", "repos/yzxingtu2026/ainp/commits/abc/check-runs?per_page=100"): '{"check_runs":[]}'}, "CI is not successful"),
            ({("gh", "variable", "get", "OSS_BUCKET", "--env", "release"): ""}, "OSS_BUCKET is not configured"),
        ]
        for overrides, message in cases:
            with self.subTest(message=message), mock.patch.object(release, "run", side_effect=self.successful_preflight_run(overrides)):
                with self.assertRaisesRegex(release.ReleaseError, message):
                    release.preflight("v1.2.3")


if __name__ == "__main__":
    unittest.main()
