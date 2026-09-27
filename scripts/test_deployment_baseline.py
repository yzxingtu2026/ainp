"""Real filesystem upgrades and fail-closed metadata/ownership/recovery contracts."""
import copy
import hashlib
import json
from pathlib import Path
import shutil
import sys
import tarfile
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent / "deployment"))
import baseline as b
from common import DeployError

ROOT = Path(__file__).resolve().parents[1]


class BaselineTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "downstream"
        self.root.mkdir()
        self.source = ROOT
        self.manifest = b.manifest_for(ROOT, "v1.0.0", "a" * 40)

    def install(self):
        b.apply(self.root, self.source, self.manifest)

    def snapshot(self):
        return {p.relative_to(self.root).as_posix(): (p.read_bytes(), p.stat().st_mode) for p in self.root.rglob("*") if p.is_file()}

    def test_complete_closure_and_missing_dependencies(self):
        for dependency in ("scripts/deployment/cloud.py", "docker/build-mupdf-cjk.sh", "scripts/test_deployment.py", ".github/workflows/native-build.yml"):
            manifest = copy.deepcopy(self.manifest)
            del manifest["files"][dependency]
            with self.subTest(dependency=dependency):
                if dependency in b.REQUIRED:
                    with self.assertRaises(DeployError):
                        b.validate_manifest(manifest)
                else:
                    self.assertNotEqual(manifest, b.manifest_for(ROOT, "v1.0.0", "a" * 40))

    def test_plan_readonly_repeat_apply_and_project_preservation(self):
        for name in ("deploy/linux/project.json", "deploy/linux/build.json", ".deploy-state/app.json", "backend/composition.go", "deploy/linux/production.local.json"):
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("private content\n")
        shutil.copy2(ROOT / "deploy/baseline/examples/project.json", self.root / "deploy/linux/project.json")
        private = self.snapshot()
        self.assertTrue(b.plan(self.root, self.manifest))
        self.assertEqual(self.snapshot(), private)
        self.install()
        for name, state in private.items():
            self.assertEqual(self.snapshot()[name], state)
        installed = self.snapshot()
        self.assertEqual(b.plan(self.root, self.manifest), [])
        self.assertEqual(self.snapshot(), installed)
        self.install()
        self.assertEqual(self.snapshot(), installed)

    def test_two_versions_add_modify_delete_and_keep_private_files(self):
        with tempfile.TemporaryDirectory() as temp:
            source = Path(temp)
            shutil.copytree(ROOT / "scripts", source / "scripts", ignore=shutil.ignore_patterns("__pycache__"))
            for entry in self.manifest["files"].values():
                dest = source / entry["source"]
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(ROOT / entry["source"], dest)
            old_path = source / "scripts/deployment/old_helper.py"
            old_path.write_text("old = 1\n")
            old = b.manifest_for(source, "v1.0.0", "a" * 40)
            b.apply(self.root, source, old)
            (source / "scripts/deployment/cloud.py").write_text("changed = True\n")
            old_path.unlink()
            (source / "scripts/deployment/new_helper.py").write_text("new = 1\n")
            new = b.manifest_for(source, "v1.1.0", "b" * 40)
            actions = b.plan(self.root, new)
            self.assertEqual({a["action"] for a in actions}, {"add", "update", "delete"})
            b.apply(self.root, source, new)
            self.assertFalse((self.root / "scripts/deployment/old_helper.py").exists())
            self.assertEqual((self.root / "scripts/deployment/cloud.py").read_text(), "changed = True\n")
            self.assertEqual(b.plan(self.root, new), [])

    def test_unmanaged_files_need_explicit_identical_adoption(self):
        path = self.root / "scripts/deployment/cloud.py"
        path.parent.mkdir(parents=True)
        shutil.copy2(ROOT / "scripts/deployment/cloud.py", path)
        with self.assertRaisesRegex(DeployError, "unmanaged collision"):
            b.plan(self.root, self.manifest)
        self.assertTrue(b.plan(self.root, self.manifest, ["scripts/deployment/cloud.py"]))
        path.write_text("local content")
        with self.assertRaisesRegex(DeployError, "unmanaged collision"):
            b.plan(self.root, self.manifest, ["scripts/deployment/cloud.py"])

    def test_local_bytes_modes_missing_files_block_all_writes(self):
        self.install()
        path = self.root / "scripts/deployment/cloud.py"
        original = path.read_bytes()
        for mutation in (lambda: path.write_bytes(original + b"\n"), lambda: path.chmod(0o755), lambda: path.unlink()):
            path.write_bytes(original)
            path.chmod(0o644)
            mutation()
            before = self.snapshot()
            with self.assertRaisesRegex(DeployError, "local drift"):
                b.apply(self.root, self.source, self.manifest)
            self.assertEqual(self.snapshot(), before)

    def test_unsafe_and_symlink_paths_and_project_takeover(self):
        for name in ("../escape", "/tmp/escape", "scripts//bad", "scripts/deployment/../../bad", ".deploy-state/ledger.json"):
            manifest = copy.deepcopy(self.manifest)
            manifest["files"][name] = next(iter(manifest["files"].values())).copy()
            with self.subTest(name=name), self.assertRaises((DeployError, ValueError)):
                b.plan(self.root, manifest)
        (self.root / "scripts").symlink_to(ROOT / "scripts", target_is_directory=True)
        with self.assertRaisesRegex(DeployError, "symlink"):
            b.plan(self.root, self.manifest)
        manifest = copy.deepcopy(self.manifest)
        manifest["files"]["deploy/linux/project.json"]["owner"] = "upstream"
        with self.assertRaisesRegex(DeployError, "configuration cannot"):
            b.validate_manifest(manifest)

    def test_failure_leaves_recoverable_journal_and_never_success_lock(self):
        original = b.write_bytes
        count = 0
        def fail(path, data, mode):
            nonlocal count
            count += 1
            if count == 3:
                raise OSError("simulated disk failure")
            original(path, data, mode)
        with mock.patch.object(b, "write_bytes", side_effect=fail), self.assertRaises(OSError):
            self.install()
        self.assertFalse((self.root / b.LOCK).exists())
        self.assertTrue((self.root / b.TRANSACTION).exists())
        with self.assertRaisesRegex(DeployError, "recover"):
            b.plan(self.root, self.manifest)
        b.recover(self.root)
        self.assertFalse((self.root / b.TRANSACTION).exists())
        self.assertFalse((self.root / b.LOCK).exists())
        self.assertFalse(any(p.is_file() for p in (self.root / "scripts").rglob("*")))
        self.install()

    def test_failed_upgrade_recovery_preserves_old_lock(self):
        self.install()
        old_lock = (self.root / b.LOCK).read_bytes()
        new = copy.deepcopy(self.manifest)
        new.update(version="v1.1.0", commit="b" * 40)
        original = b.write_bytes
        def fail_lock(path, content, mode):
            if path == self.root / b.LOCK:
                raise OSError("lock write failed")
            original(path, content, mode)
        with mock.patch.object(b, "write_bytes", side_effect=fail_lock), self.assertRaises(OSError):
            b.apply(self.root, self.source, new)
        self.assertEqual((self.root / b.LOCK).read_bytes(), old_lock)
        b.recover(self.root)
        self.assertEqual((self.root / b.LOCK).read_bytes(), old_lock)

    def test_changed_published_version_rejected(self):
        self.install()
        new = copy.deepcopy(self.manifest)
        new["commit"] = "b" * 40
        with self.assertRaisesRegex(DeployError, "changed"):
            b.plan(self.root, new)

    def configured_install(self):
        self.install()
        config = json.loads((ROOT / "deploy/baseline/examples/project.json").read_text())
        config["platform_version"] = "v1.0.0"
        (self.root / "deploy/linux/project.json").write_text(json.dumps(config))
        shutil.copy2(ROOT / "deploy/baseline/examples/build.json", self.root / "deploy/linux/build.json")
        for command in ("api", "worker", "migrate"):
            (self.root / "backend/cmd" / command).mkdir(parents=True)
        (self.root / "backend/go.mod").write_text("module example/application\nrequire github.com/yzxingtu2026/ainp/platform v1.0.0\n")
        (self.root / "backend/go.sum").write_text("fixture\n")
        (self.root / "admin").mkdir()
        (self.root / "admin/admin-baseline.json").write_text(json.dumps({"commit": "a" * 40}))
        (self.root / ".gitignore").write_text((ROOT / "deploy/baseline/templates/gitignore.txt").read_text())
        (self.root / ".agent/rules/main.md").write_text("binary-deployment\n")
        (self.root / ".agent/adapters").mkdir()
        (self.root / ".agent/adapters/rules.json").write_text(json.dumps({"rules": [{"source": ".agent/rules/binary-deployment.md"}]}))

    def test_verify_is_readonly_and_detects_go_admin_actual_files_and_integrations(self):
        self.configured_install()
        with mock.patch.object(b.subprocess, "check_output", return_value="https://github.com/example/application.git"), mock.patch.dict(b.os.environ, {}, clear=True):
            before = self.snapshot()
            b.verify(self.root)
            self.assertEqual(before, self.snapshot())
            for name, content in (
                ("backend/go.mod", "require github.com/yzxingtu2026/ainp/platform v1.1.0\n"),
                ("admin/admin-baseline.json", '{"commit":"other"}'),
                ("scripts/deployment/cloud.py", "local drift"),
                (".github/workflows/deployment-baseline.yml", "disabled"),
                (".gitignore", ""),
                (".agent/rules/main.md", ""),
                (".agent/adapters/rules.json", '{"rules":[]}'),
            ):
                path = self.root / name
                original = path.read_bytes()
                path.write_text(content)
                changed = self.snapshot()
                with self.subTest(name=name), self.assertRaises((DeployError, ValueError)):
                    b.verify(self.root)
                self.assertEqual(changed, self.snapshot())
                path.write_bytes(original)

    def test_existing_admin_metadata_format_is_configurable(self):
        self.configured_install()
        cfg_path = self.root / "deploy/linux/project.json"
        cfg = json.loads(cfg_path.read_text())
        cfg["admin_baseline"] = {"path": "admin/custom-baseline.json", "commit_field": "source.commit"}
        cfg_path.write_text(json.dumps(cfg))
        (self.root / "admin/custom-baseline.json").write_text(json.dumps({"source": {"commit": "a" * 40}}))
        with mock.patch.object(b.subprocess, "check_output", return_value="https://github.com/example/application.git"), mock.patch.dict(b.os.environ, {}, clear=True):
            b.verify(self.root)
            (self.root / "admin/custom-baseline.json").write_text('{"source":{}}')
            with self.assertRaisesRegex(DeployError, "commit field missing"):
                b.verify(self.root)

    def test_legacy_image_publisher_blocks_apply_before_any_write(self):
        workflow = self.root / ".github/workflows/images.yml"
        workflow.parent.mkdir(parents=True)
        workflow.write_text("uses: docker/build-push-action@v6\nwith:\n  push: true\n")
        before = self.snapshot()
        with self.assertRaisesRegex(DeployError, "legacy image"):
            b.plan(self.root, self.manifest)
        self.assertEqual(before, self.snapshot())

    def test_recovery_refuses_to_overwrite_edits_after_failure(self):
        original = b.write_bytes
        calls = 0
        written = []
        def fail(path, content, mode):
            nonlocal calls
            calls += 1
            if calls == 2:
                raise OSError("failed")
            original(path, content, mode)
            written.append(path)
        with mock.patch.object(b, "write_bytes", side_effect=fail), self.assertRaises(OSError):
            self.install()
        written[0].write_text("new user edit")
        before = self.snapshot()
        with self.assertRaisesRegex(DeployError, "recovery conflict"):
            b.recover(self.root)
        self.assertEqual(before, self.snapshot())

    def remote(self, *, patch_release=None, tag=None, asset_raw=None):
        raw = asset_raw or json.dumps(self.manifest).encode()
        release = dict(tag_name="v1.0.0", draft=False, prerelease=False, published_at="now", assets=[dict(name=b.ASSET, id=1, state="uploaded", size=len(raw), digest="sha256:" + hashlib.sha256(raw).hexdigest())])
        if patch_release:
            patch_release(release)
        def github(endpoint, **kwargs):
            if "/releases/tags/" in endpoint:
                return json.dumps(release).encode()
            if "/commits/refs/tags/" in endpoint:
                return (tag or "a" * 40).encode()
            if "/releases/assets/" in endpoint:
                return raw
            return json.dumps(dict(truncated=False, tree=[{"path": entry["source"]} for entry in self.manifest["files"].values()])).encode()
        def download(argv, stdout, **kwargs):
            with tarfile.open(fileobj=stdout, mode="w:gz") as tar:
                for entry in self.manifest["files"].values():
                    path = ROOT / entry["source"]
                    tar.add(path, arcname="source/" + entry["source"])
            return mock.Mock(returncode=0)
        with tempfile.TemporaryDirectory() as temp, mock.patch.object(b, "github", side_effect=github), mock.patch.object(b.subprocess, "run", side_effect=download):
            return b.fetch_source("v1.0.0", "a" * 40, Path(temp))[1]

    def test_published_source_hash_tag_and_dependency_validation(self):
        self.assertEqual(self.remote(), self.manifest)
        for patch in (lambda r: r.update(draft=True), lambda r: r.update(prerelease=True), lambda r: r.update(assets=[]), lambda r: r["assets"][0].pop("digest"), lambda r: r["assets"][0].update(digest="sha256:" + "0" * 64)):
            with self.assertRaises(DeployError):
                self.remote(patch_release=patch)
        with self.assertRaisesRegex(DeployError, "Tag/commit"):
            self.remote(tag="b" * 40)
        broken = copy.deepcopy(self.manifest)
        broken["files"]["scripts/deployment/cloud.py"]["sha256"] = "0" * 64
        with self.assertRaisesRegex(DeployError, "hashes"):
            self.remote(asset_raw=json.dumps(broken).encode())
        with mock.patch.object(b, "github", side_effect=DeployError("source unavailable")), self.assertRaisesRegex(DeployError, "unavailable"):
            b.fetch_source("v1.0.0", "a" * 40, self.root)


if __name__ == "__main__":
    unittest.main()
