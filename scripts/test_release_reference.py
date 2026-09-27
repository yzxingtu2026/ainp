"""Version selection and immutable reference contracts; no live cloud mutations."""
import contextlib
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent / "deployment"))
import cloud
import common
import release_reference as rr


class ReleaseReferenceTest(unittest.TestCase):
    def setUp(self):
        self.reference = dict(schema=1, project="ainp", version="v1.2.3", commit="a" * 40,
                              platform="ubuntu-22.04-amd64", sha256="b" * 64,
                              oss={"bucket": "private-release", "endpoint": "https://oss-cn-hangzhou.aliyuncs.com"},
                              key=f"releases/ainp/v1.2.3/{'a' * 40}/release.tar.gz")
        self.repo = "example/ainp"

    def resolve(self, *, reference=None, change_release=None, tag=None, raw=None):
        raw = raw if raw is not None else json.dumps(reference or self.reference).encode()
        release = dict(tag_name="v1.2.3", draft=False, prerelease=False, published_at="2026-09-27T00:00:00Z",
                       assets=[dict(name="reference.json", id=123, size=len(raw), state="uploaded",
                                    digest="sha256:" + hashlib.sha256(raw).hexdigest())])
        if change_release:
            change_release(release)
        def github(endpoint, **kwargs):
            if endpoint == f"repos/{self.repo}/releases/tags/v1.2.3":
                return json.dumps(release).encode()
            if endpoint == f"repos/{self.repo}/releases/assets/123":
                self.assertEqual(kwargs, {"accept": "application/octet-stream"})
                return raw
            self.assertEqual(endpoint, f"repos/{self.repo}/commits/refs/tags/v1.2.3")
            self.assertEqual(kwargs, {"query": ".sha"})
            return ((tag if tag is not None else self.reference["commit"]) + "\n").encode()
        with mock.patch.object(rr, "github", side_effect=github):
            return rr.resolve(self.repo, "v1.2.3", "ainp")

    def test_resolves_published_asset_by_id_and_checks_tag_commit(self):
        self.assertEqual(self.resolve(), self.reference)

    def test_asset_digest_optional_for_older_github_assets(self):
        self.assertEqual(self.resolve(change_release=lambda r: r["assets"][0].pop("digest")), self.reference)

    def test_rejects_unpublished_wrong_or_prerelease(self):
        for patch in ({"draft": True}, {"prerelease": True}, {"published_at": None}, {"tag_name": "v1.2.4"}):
            with self.subTest(patch=patch), self.assertRaisesRegex(common.DeployError, "published"):
                self.resolve(change_release=lambda r: r.update(patch))

    def test_rejects_missing_duplicate_or_bad_assets(self):
        changes = [lambda r: r.update(assets=[]), lambda r: r["assets"].append(r["assets"][0].copy()),
                   lambda r: r.update(assets=[None])]
        for patch in ({"id": "https://untrusted"}, {"id": 0}, {"size": rr.REFERENCE_LIMIT + 1},
                      {"state": "new"}, {"size": 1}, {"digest": "sha256:" + "0" * 64}):
            changes.append(lambda r, patch=patch: r["assets"][0].update(patch))
        for i, change in enumerate(changes):
            with self.subTest(case=i), self.assertRaises(common.DeployError):
                self.resolve(change_release=change)

    def test_rejects_invalid_or_mismatched_reference(self):
        patches = [{"schema": 2}, {"version": "v1.2.4"}, {"project": "other"}, {"platform": "windows-amd64"},
                   {"commit": "bad"}, {"sha256": "bad"}, {"oss": None},
                   {"oss": {"bucket": 123, "endpoint": None}}, {"key": "../secret"},
                   {"key": "releases/other/v1.2.3/release.tar.gz"}]
        for patch in patches:
            with self.subTest(patch=patch), self.assertRaises(common.DeployError):
                self.resolve(reference={**self.reference, **patch})
        for raw in (b"invalid", b"[]", b"\xff"):
            with self.subTest(raw=raw), self.assertRaises(common.DeployError):
                self.resolve(raw=raw)
        with self.assertRaisesRegex(common.DeployError, "Git tag commit"):
            self.resolve(tag="c" * 40)

    def test_invalid_repository_version_and_ci_rejected_before_network(self):
        with mock.patch.object(rr, "github") as github:
            for repo, version in [("../../evil", "v1.2.3"), ("owner/..", "v1.2.3"),
                                  ("owner/repo", "latest"), ("owner/repo", "v0.0.0-ci")]:
                with self.subTest(repo=repo, version=version), self.assertRaises(common.DeployError):
                    rr.resolve(repo, version, "ainp")
            github.assert_not_called()

    def test_github_errors_are_bounded_and_do_not_leak_output(self):
        with mock.patch.object(rr.subprocess, "run", return_value=subprocess.CompletedProcess([], 1, b"secret", b"token")):
            with self.assertRaisesRegex(common.DeployError, "check gh auth") as raised:
                rr.github("repos/example/ainp/releases/tags/v1.2.3")
            self.assertNotIn("secret", str(raised.exception))
            self.assertNotIn("token", str(raised.exception))
        for error in (FileNotFoundError(), subprocess.TimeoutExpired("gh", 60)):
            with mock.patch.object(rr.subprocess, "run", side_effect=error), self.assertRaisesRegex(common.DeployError, "unavailable"):
                rr.github("repos/example/ainp/releases/tags/v1.2.3")

    def test_github_uses_explicit_host_and_no_shell(self):
        with mock.patch.object(rr.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, b"{}", b"")) as run:
            rr.github("repos/example/ainp/releases/tags/v1.2.3")
        self.assertEqual(run.call_args.args[0][:4], ["gh", "api", "--hostname", "github.com"])
        self.assertNotIn("shell", run.call_args.kwargs)

    def test_plan_does_not_write_and_apply_pins_rejecting_changed_metadata(self):
        with tempfile.TemporaryDirectory() as d:
            state = Path(d) / "state"
            rr.pin_reference(state, self.repo, self.reference)
            self.assertFalse(state.exists())
            path = rr.pin_reference(state, self.repo, self.reference, write=True)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertEqual(common.read_json(path), self.reference)
            rr.pin_reference(state, self.repo, self.reference, write=True)
            changed = {**self.reference, "sha256": "c" * 64}
            for write in (False, True):
                with self.assertRaisesRegex(common.DeployError, "pinned reference"):
                    rr.pin_reference(state, self.repo, changed, write=write)
            self.assertEqual(common.read_json(path), self.reference)
            # Same version in another repository is a separate identity.
            rr.pin_reference(state, "other/ainp", changed, write=True)

    def run_cli(self, action, options):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        target = common.read_json("deploy/linux/target.example.json")
        target["config_sha256"] = "c" * 64
        common.atomic_json(root / "target.json", target)
        argv = ["cloud", action, "--target", str(root / "target.json"), "--state-dir", str(root / "state"), *options]
        if "--repository" in options:
            repo = options[options.index("--repository") + 1]
            cfg = common.read_json("deploy/linux/project.json")
            cfg["repository"] = repo
            common.atomic_json(root / "project.json", cfg)
            argv += ["--project-config", str(root / "project.json")]
        else:
            repo = "yzxingtu2026/ainp"
        output = io.StringIO()
        with mock.patch.object(cloud.subprocess, "check_output", return_value=f"https://github.com/{repo}.git"), mock.patch.object(sys, "argv", argv), mock.patch.object(rr, "resolve", return_value=self.reference) as resolve, \
                mock.patch.object(cloud, "execute") as execute, contextlib.redirect_stdout(output):
            cloud.main()
        return root, resolve, execute, output.getvalue()

    def test_version_plan_reads_github_but_never_dispatches_or_writes(self):
        root, resolve, execute, output = self.run_cli("init", ["--version", "v1.2.3"])
        resolve.assert_called_once_with(rr.DEFAULT_REPOSITORY, "v1.2.3", "ainp")
        execute.assert_not_called()
        self.assertFalse((root / "state").exists())
        plan = json.loads(output)
        self.assertIn("read-only GitHub", plan["mode"])
        self.assertEqual(plan["commit"], self.reference["commit"])
        self.assertEqual(plan["sha256"], self.reference["sha256"])

    def test_apply_all_actions_uses_verified_reference_and_pins(self):
        for action in ("init", "update", "rollback"):
            options = ["--version", "v1.2.3", "--repository", self.repo, "--apply"]
            if action == "rollback":
                options.append("--confirm-schema-compatible")
            with self.subTest(action=action):
                root, resolve, execute, _ = self.run_cli(action, options)
                self.assertEqual(execute.call_args.args[1], self.reference)
                self.assertEqual(execute.call_args.args[2], action)
                self.assertTrue((root / "state/releases" / self.repo / "v1.2.3/reference.json").is_file())

    def test_repository_override_cannot_query_another_projects_release(self):
        with tempfile.TemporaryDirectory() as temp:
            target = Path(temp) / "target.json"
            common.atomic_json(target, common.read_json("deploy/linux/target.example.json"))
            argv = ["cloud", "update", "--target", str(target), "--version", "v1.2.3", "--repository", "wrong/project"]
            with mock.patch.object(sys, "argv", argv), mock.patch.object(rr, "resolve") as resolve, mock.patch.object(cloud, "execute") as execute, contextlib.redirect_stdout(io.StringIO()), self.assertRaises(SystemExit):
                cloud.main()
            resolve.assert_not_called()
            execute.assert_not_called()

    def test_lookup_failure_never_writes_state_or_dispatches(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            common.atomic_json(root / "target.json", common.read_json("deploy/linux/target.example.json"))
            argv = ["cloud", "update", "--target", str(root / "target.json"), "--state-dir", str(root / "state"),
                    "--version", "v1.2.3", "--apply"]
            with mock.patch.object(sys, "argv", argv), \
                    mock.patch.object(rr, "resolve", side_effect=common.DeployError("lookup failed")), \
                    mock.patch.object(cloud, "execute") as execute, contextlib.redirect_stdout(io.StringIO()), \
                    self.assertRaises(SystemExit):
                cloud.main()
            execute.assert_not_called()
            self.assertFalse((root / "state").exists())

    def test_status_never_resolves_a_version(self):
        _, resolve, execute, _ = self.run_cli("status", [])
        resolve.assert_not_called()
        self.assertTrue(execute.call_args.args[-1])

    def test_conflicting_sources_are_rejected_by_cli(self):
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            self.run_cli("update", ["--version", "v1.2.3", "--release", "ref.json"])

    def test_rollback_requires_compatibility_before_lookup(self):
        with mock.patch.object(rr, "resolve") as resolve, self.assertRaises(SystemExit):
            # Direct parser invocation ensures the network mock remains visible on error.
            with tempfile.TemporaryDirectory() as d:
                common.atomic_json(Path(d) / "target.json", common.read_json("deploy/linux/target.example.json"))
                with mock.patch.object(sys, "argv", ["cloud", "rollback", "--target", str(Path(d) / "target.json"), "--version", "v1.2.3"]), contextlib.redirect_stdout(io.StringIO()):
                    try:
                        cloud.main()
                    finally:
                        resolve.assert_not_called()


if __name__ == "__main__":
    unittest.main()
