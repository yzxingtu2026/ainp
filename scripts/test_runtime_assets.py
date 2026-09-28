"""Offline contract and fail-closed tests for the optional native build asset hook."""
import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from scripts.deployment import fetch_runtime_assets as assets


class RuntimeAssetsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / 'deploy/linux').mkdir(parents=True)
        (self.root / 'assets/runtime').mkdir(parents=True)
        (self.root / 'assets/runtime/model.glb.dvc').write_text('outs:\n- md5: ' + 'a' * 32 + '\n  size: 13\n  path: model.glb\n')
        self.source = self.root / 'assets/runtime/model.glb'
        self.source.write_bytes(b'fixture-asset')
        self.asset = {'pointer': 'assets/runtime/model.glb.dvc', 'source': 'assets/runtime/model.glb',
                      'destination': 'apps/bigscreen/public/ip/model.glb', 'size': len(b'fixture-asset'),
                      'sha256': hashlib.sha256(b'fixture-asset').hexdigest()}
        self.contract = {'schema': 1, 'remote': 'modelstore', 'assets': [self.asset]}
        self.save()

    def save(self):
        (self.root / 'deploy/linux/runtime-assets.json').write_text(json.dumps(self.contract))

    def execute(self, **kwargs):
        return assets.execute(self.root, 'deploy/linux/runtime-assets.json', 'apps/bigscreen', **kwargs)

    def test_offline_copy_and_verify(self):
        with mock.patch.object(assets.subprocess, 'run') as run:
            self.execute(offline=True)
            run.assert_not_called()
        self.assertEqual((self.root / self.asset['destination']).read_bytes(), b'fixture-asset')
        assets.verify_destinations(self.root, 'deploy/linux/runtime-assets.json', 'apps/bigscreen')
        staged = self.root / 'stage/web/ip'
        staged.mkdir(parents=True)
        (staged / 'model.glb').write_bytes(b'fixture-asset')
        assets.verify_staged(self.root, 'deploy/linux/runtime-assets.json', 'apps/bigscreen', self.root / 'stage')
        (staged / 'model.glb').write_bytes(b'wrong')
        with self.assertRaises(ValueError):
            assets.verify_staged(self.root, 'deploy/linux/runtime-assets.json', 'apps/bigscreen', self.root / 'stage')
        self.execute(cleanup=True)
        self.assertFalse(self.source.exists())
        self.assertTrue((self.root / self.asset['destination']).is_file())

    def test_pull_is_limited_to_declared_pointer_and_hides_output(self):
        with mock.patch.dict(os.environ, {'AINP_ASSET_STORE_KEY_ID': 'SECRET_ID',
                                          'AINP_ASSET_STORE_KEY_SECRET': 'SECRET_KEY',
                                          'AINP_ASSET_STORE_ENDPOINT': 'SECRET_ENDPOINT'}), \
             mock.patch.object(assets.subprocess, 'run') as run, contextlib.redirect_stdout(io.StringIO()) as output:
            self.execute()
        args, kwargs = run.call_args
        self.assertEqual(args[0], ['dvc', 'pull', '--quiet', '--remote', 'modelstore', self.asset['pointer']])
        self.assertEqual(kwargs['env']['OSS_ACCESS_KEY_SECRET'], 'SECRET_KEY')
        self.assertEqual(kwargs['stdout'], assets.subprocess.DEVNULL)
        self.assertEqual(kwargs['stderr'], assets.subprocess.DEVNULL)
        self.assertNotIn('SECRET', output.getvalue())

    def test_fail_closed_on_missing_credentials_or_pull_failure(self):
        with mock.patch.dict(os.environ, {'AINP_ASSET_STORE_KEY_ID': '', 'AINP_ASSET_STORE_KEY_SECRET': '', 'AINP_ASSET_STORE_ENDPOINT': ''}):
            with self.assertRaises(ValueError):
                self.execute()
        with mock.patch.dict(os.environ, {'AINP_ASSET_STORE_KEY_ID': 'ID', 'AINP_ASSET_STORE_KEY_SECRET': 'KEY', 'AINP_ASSET_STORE_ENDPOINT': 'END'}), \
             mock.patch.object(assets.subprocess, 'run', side_effect=assets.subprocess.CalledProcessError(1, ['dvc'])):
            with self.assertRaisesRegex(assets.AssetError, 'DVC pull failed'):
                self.execute()
        self.assertFalse((self.root / self.asset['destination']).exists())

    def test_missing_empty_wrong_size_or_digest(self):
        for content, size, digest in ((None, self.asset['size'], self.asset['sha256']),
                                      (b'', self.asset['size'], self.asset['sha256']),
                                      (b'fixture-asset', 99, self.asset['sha256']),
                                      (b'fixture-asset', self.asset['size'], '0'*64)):
            with self.subTest(content=content, size=size, digest=digest):
                if content is None:
                    self.source.unlink(missing_ok=True)
                else:
                    self.source.write_bytes(content)
                self.asset['size'] = size
                self.asset['sha256'] = digest
                self.save()
                with self.assertRaises(ValueError):
                    self.execute(offline=True)
                self.assertFalse((self.root / self.asset['destination']).exists())
        with self.assertRaises(ValueError):
            assets.verify_destinations(self.root, 'deploy/linux/runtime-assets.json', 'apps/bigscreen')

    def test_directory_pointer_and_second_web_app(self):
        pointer = self.root / self.asset['pointer']
        pointer.unlink()
        directory = self.root / 'assets/runtime/exports'
        directory.mkdir()
        source = directory / 'model.glb'
        source.write_bytes(b'fixture-asset')
        (directory / 'review.txt').write_text('keep workspace file')
        self.asset['pointer'] = 'assets/runtime/exports.dvc'
        self.asset['source'] = 'assets/runtime/exports/model.glb'
        (self.root / self.asset['pointer']).write_text(
            'outs:\n- md5: ' + 'a' * 32 + '.dir\n  size: 100\n  nfiles: 2\n  hash: md5\n  path: exports\n')
        apps = [{'source_dir': 'admin', 'output_dir': 'web'},
                {'source_dir': 'apps/bigscreen', 'output_dir': 'web/bigscreen'}]
        self.save()
        assets.execute(self.root, 'deploy/linux/runtime-assets.json', apps, offline=True)
        stage = self.root / 'stage/web/bigscreen/ip'
        stage.mkdir(parents=True)
        (stage / 'model.glb').write_bytes(b'fixture-asset')
        assets.verify_staged(self.root, 'deploy/linux/runtime-assets.json', apps, self.root / 'stage')
        (stage / 'model.glb').write_bytes(b'bad')
        with self.assertRaises(ValueError):
            assets.verify_staged(self.root, 'deploy/linux/runtime-assets.json', apps, self.root / 'stage')
        assets.execute(self.root, 'deploy/linux/runtime-assets.json', apps, cleanup=True)
        self.assertFalse(source.exists())
        self.assertTrue((directory / 'review.txt').exists())
        self.assertTrue((self.root / self.asset['destination']).exists())
        self.asset['source'] = 'assets/runtime/review.txt'
        self.save()
        with self.assertRaises(ValueError):
            assets.load_contract(self.root, 'deploy/linux/runtime-assets.json', apps)

    def test_reject_pointer_with_directory_or_multiple_outputs(self):
        pointer = self.root / self.asset['pointer']
        for content in ('outs:\n- md5: ' + 'a' * 32 + '.dir\n  path: model.glb\n',
                        'outs:\n- md5: ' + 'a' * 32 + '\n  path: model.glb\n- md5: ' + 'b' * 32 + '\n  path: review.glb\n',
                        'outs:\n- md5: ' + 'a' * 32 + '\n  path: model.glb\n  remote: unrelated\n'):
            with self.subTest(content=content):
                pointer.write_text(content)
                with self.assertRaises(ValueError):
                    self.execute(offline=True)
        pointer.unlink()
        with self.assertRaises(ValueError):
            self.execute(offline=True)

    def test_reject_unsafe_paths_and_symlink_escape(self):
        for field, path in (('pointer', '../model.dvc'), ('source', '/tmp/model.glb'),
                            ('destination', 'apps/bigscreen/public/../../model.glb')):
            with self.subTest(field=field):
                old = self.asset[field]
                self.asset[field] = path
                self.save()
                with self.assertRaises(ValueError):
                    self.execute(offline=True)
                self.asset[field] = old
        outside = self.root.parent / (self.root.name + '-outside')
        outside.mkdir()
        self.addCleanup(outside.rmdir)
        (self.root / 'apps').symlink_to(outside, target_is_directory=True)
        self.save()
        with self.assertRaises(ValueError):
            self.execute(offline=True)


if __name__ == '__main__':
    unittest.main()
