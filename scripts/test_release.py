import importlib.util
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location("release", Path(__file__).with_name("release.py"))
release = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
SPEC.loader.exec_module(release)


class ReleaseValidationTest(unittest.TestCase):
    def test_version_is_strict_stable_semver(self):
        self.assertEqual(release.validate_version("v1.2.3"), "v1.2.3")
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


if __name__ == "__main__":
    unittest.main()
