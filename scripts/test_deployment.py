"""Offline release/controller/security contracts; never calls a cloud or production host."""
import ast
import base64
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
import bz2
from unittest import mock

DEPLOY = Path(__file__).parent / "deployment"
sys.path.insert(0, str(DEPLOY))
import common
import cloud
import host
import launcher
import storage
import reporting
import project
import check_oss
import verify_release
spec = importlib.util.spec_from_file_location("native_package", DEPLOY / "package.py")
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)


class WebBuildTest(unittest.TestCase):
    def test_package_entrypoint_accepts_multi_web_fixture_and_packages_both_apps(self):
        root_source = DEPLOY.parents[1]
        cfg = json.loads((root_source / "deploy/baseline/examples/project.json").read_text())
        build = json.loads((root_source / "deploy/baseline/examples/build-web-apps.json").read_text())
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "deploy/linux").mkdir(parents=True)
            (root / cfg["build_config"]).write_text(json.dumps(build))
            for command in ("api", "worker", "migrate"):
                (root / "backend/cmd" / command).mkdir(parents=True)
            for filename in ("go.mod", "go.sum"):
                (root / "backend" / filename).write_text("")
            for app in build["web_apps"]:
                dist = root / app["source_dir"] / "dist"
                dist.mkdir(parents=True)
                (dist / "index.html").write_text(app["name"])
            def fake_run(args, cwd, env):
                if args[:2] == ["go", "build"]:
                    Path(args[args.index("-o") + 1]).write_bytes(b"offline binary")
            def output(args, **kwargs):
                if args[0] == "ldd":
                    return "statically linked"
                return "1700000000" if "show" in args else "a" * 40
            cjk = "".join(f"--- PASS: {name} (0.01s)\n" for name in (
                "TestKnowledgeSafeParserHandlesUniGBReceipt", "TestKnowledgeSafeParserDecodesPDFToUnicodeMapping",
                "TestKnowledgeSafeParserExtractsEnglishPDF"))
            read_text = Path.read_text
            def read(path, *args, **kwargs):
                return 'VERSION_ID="22.04"' if str(path) == "/etc/os-release" else read_text(path, *args, **kwargs)
            with mock.patch.object(Path, "cwd", return_value=root), mock.patch.object(Path, "read_text", read), \
                 mock.patch.object(package.platform, "system", return_value="Linux"), \
                 mock.patch.object(package.platform, "machine", return_value="x86_64"), \
                 mock.patch.object(project, "load", return_value=cfg), \
                 mock.patch.object(project, "configured_origin", return_value="https://github.com/example/application.git"), \
                 mock.patch.dict(os.environ, {"CGO_CFLAGS": "fixture", "CGO_LDFLAGS": "fixture"}, clear=True), \
                 mock.patch.object(sys, "argv", ["package", "--config", str(root / cfg["build_config"]), "--version", "v1.2.3"]), \
                 mock.patch.object(package.subprocess, "check_output", side_effect=output), \
                 mock.patch.object(package.subprocess, "run", return_value=mock.Mock(returncode=0, stdout=cjk)), \
                 mock.patch.object(package, "run", side_effect=fake_run) as run, contextlib.redirect_stdout(io.StringIO()):
                package.main()
            manifest = json.loads((root / "dist-release/release.json").read_text())
            self.assertEqual([app["mount"] for app in manifest["web_apps"]], ["/", "/bigscreen/"])
            with tarfile.open(root / "dist-release/release.tar.gz") as archive:
                self.assertEqual(archive.extractfile("web/index.html").read(), b"admin")
                self.assertEqual(archive.extractfile("web/bigscreen/index.html").read(), b"screen")
                for health in ("web/build.json", "web/bigscreen/build.json"):
                    self.assertEqual(json.load(archive.extractfile(health))["commit"], "a" * 40)
            screen_call = next(call for call in run.call_args_list if call.args[0] == ["pnpm", "build:screen"])
            self.assertEqual(screen_call.args[2]["VITE_SCREEN"], "yes")
            self.assertEqual(screen_call.args[2]["VITE_BASE_URL"], "/__web/v1.2.3/bigscreen/")
            admin_call = next(call for call in run.call_args_list if call.args[0] == ["pnpm", "build"])
            self.assertEqual(admin_call.args[2]["VITE_BASE_URL"], "/__web/v1.2.3/")

    def test_release_build_uses_declared_admin_composition(self):
        identity = dict(version="v1.2.3", commit="a" * 40, build_time="2026-09-26T00:00:00Z")
        cfg = {"admin_application": "./business/application.ts", "admin_modules": ["core", "system", "ai"]}
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            web_dir = root / "frontend"
            (web_dir / "dist").mkdir(parents=True)
            (web_dir / "dist/index.html").write_text("built web")
            with mock.patch.object(package, "run") as run:
                package.build_web(web_dir, root / "stage", {}, identity, cfg)
            env = run.call_args_list[-1].args[2]
            self.assertEqual(env["AINP_ADMIN_APPLICATION"], "./business/application.ts")
            self.assertEqual(env["AINP_ADMIN_MODULES"], "core,system,ai")

    def test_release_build_overrides_inherited_api_origin_and_keeps_identity(self):
        identity = dict(version="v1.2.3", commit="a" * 40, build_time="2026-09-26T00:00:00Z")
        inherited = dict(PATH="/tools", VITE_SERVICE_BASE_URL="http://127.0.0.1:8080",
                         VITE_OTHER_SERVICE_BASE_URL='{"demo":"https://wrong.example"}',
                         VITE_BUILD_COMMIT="stale")
        original = inherited.copy()
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            web_dir = root / "frontend"
            (web_dir / "dist").mkdir(parents=True)
            (web_dir / "dist/index.html").write_text("built web")
            with mock.patch.object(package, "run") as run:
                package.build_web(web_dir, root / "stage", inherited, identity)
            self.assertEqual([call.args[0] for call in run.call_args_list],
                             [["pnpm", "install", "--frozen-lockfile"], ["pnpm", "build"]])
            for call in run.call_args_list:
                self.assertEqual(call.args[1], web_dir)
                env = call.args[2]
                self.assertEqual(env["VITE_SERVICE_BASE_URL"], "/")
                self.assertEqual(json.loads(env["VITE_OTHER_SERVICE_BASE_URL"]), {"demo": "/"})
                for key, variable in (("version", "VITE_BUILD_VERSION"), ("commit", "VITE_BUILD_COMMIT"),
                                      ("build_time", "VITE_BUILD_TIME")):
                    self.assertEqual(env[variable], identity[key])
                self.assertEqual(env["PATH"], "/tools")
                self.assertEqual(env["VITE_BASE_URL"], "/__web/v1.2.3/")
                self.assertEqual(env["VITE_APP_MOUNT"], "/")
            self.assertEqual(json.loads((root / "stage/web/build.json").read_text()), identity)
            self.assertEqual((root / "stage/web/index.html").read_text(), "built web")
        self.assertEqual(inherited, original)

    def test_multiple_web_apps_are_isolated_and_use_configured_command(self):
        identity = dict(version="v1.2.3", commit="a" * 40, build_time="2026-09-26T00:00:00Z")
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            admin = root / "admin"
            screen = root / "screen"
            (admin / "dist").mkdir(parents=True)
            (screen / "dist").mkdir(parents=True)
            (admin / "dist/index.html").write_text("admin")
            (screen / "dist/index.html").write_text("screen")
            with mock.patch.object(package, "run") as run:
                package.build_web(admin, root / "stage", {}, identity, "web", ["pnpm", "build:admin"], {"VITE_SCREEN": "no"})
                package.build_web(screen, root / "stage", {}, identity, "web/bigscreen", ["pnpm", "build:screen"], {"VITE_SCREEN": "yes"})
            self.assertEqual([call.args[0] for call in run.call_args_list], [
                ["pnpm", "install", "--frozen-lockfile"], ["pnpm", "build:admin"],
                ["pnpm", "install", "--frozen-lockfile"], ["pnpm", "build:screen"],
            ])
            self.assertEqual(run.call_args_list[1].args[2]["VITE_BASE_URL"], "/__web/v1.2.3/")
            self.assertEqual(run.call_args_list[1].args[2]["VITE_APP_MOUNT"], "/")
            self.assertEqual(run.call_args_list[3].args[2]["VITE_BASE_URL"], "/__web/v1.2.3/bigscreen/")
            self.assertEqual(run.call_args_list[3].args[2]["VITE_APP_MOUNT"], "/bigscreen/")
            self.assertEqual((root / "stage/web/index.html").read_text(), "admin")
            self.assertEqual((root / "stage/web/bigscreen/index.html").read_text(), "screen")
            self.assertEqual(json.loads((root / "stage/web/bigscreen/build.json").read_text()), identity)

    def test_web_health_path_is_written_inside_non_root_mount(self):
        identity = dict(version="v1.2.3", commit="a" * 40, build_time="2026-09-26T00:00:00Z")
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            web_dir = root / "screen"
            (web_dir / "dist").mkdir(parents=True)
            (web_dir / "dist/index.html").write_text("screen")
            with mock.patch.object(package, "run"):
                package.build_web(web_dir, root / "stage", {}, identity, "web/bigscreen", ["pnpm", "build"], {}, "/bigscreen/build.json")
            self.assertEqual(json.loads((root / "stage/web/bigscreen/build.json").read_text()), identity)


class ArchiveTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.stage = self.root / "stage"
        self.stage.mkdir()
        for filename in ("bin/api", "bin/worker", "bin/migrate", "web/index.html", "web/build.json"):
            p = self.stage / filename
            p.parent.mkdir(exist_ok=True)
            p.write_text("fixture")
        self.identity = dict(schema=1, project="ainp", version="v1.2.3", commit="a" * 40, build_time="2026-09-24T00:00:00Z", platform="ubuntu-22.04-amd64")
        self.archive = self.root / "release.tar.gz"
        self.manifest = package.pack(self.stage, self.archive, self.identity, 1)

    def extract(self, path=None):
        archive = path or self.archive
        return common.extract_verified(archive, self.root / "out", common.sha256(archive))

    def test_round_trip_and_permissions_under_root_umask(self):
        old = os.umask(0o077)
        try:
            self.assertEqual(self.extract(), self.manifest)
        finally:
            os.umask(old)
        for p in ("out", "out/bin", "out/web", "out/bin/api"):
            self.assertEqual((self.root / p).stat().st_mode & 0o777, 0o755)
        self.assertEqual((self.root / "out/web/index.html").stat().st_mode & 0o777, 0o644)

    def test_multi_web_manifest_round_trip(self):
        (self.stage / "web/bigscreen/index.html").parent.mkdir(parents=True)
        (self.stage / "web/bigscreen/index.html").write_text("screen")
        (self.stage / "web/bigscreen/build.json").write_text("fixture")
        self.stage.joinpath("release.json").unlink(missing_ok=True)
        identity = {**self.identity, "web_apps": [{"name": "admin", "mount": "/", "health_path": "/build.json", "output_dir": "web"}, {"name": "screen", "mount": "/bigscreen/", "health_path": "/bigscreen/build.json", "output_dir": "web/bigscreen"}]}
        archive = self.root / "multi.tar.gz"
        manifest = package.pack(self.stage, archive, identity, 1)
        self.assertEqual(len(common.web_apps(manifest)), 2)
        common.extract_verified(archive, self.root / "multi-out", common.sha256(archive))
        self.assertTrue((self.root / "multi-out/web/bigscreen/index.html").exists())

    def test_reproducible_archive(self):
        (self.stage / "release.json").unlink()
        second = self.root / "second.tar.gz"
        package.pack(self.stage, second, self.identity, 1)
        self.assertEqual(common.sha256(second), common.sha256(self.archive))

    def test_wrong_checksum_never_extracts(self):
        with self.assertRaisesRegex(common.DeployError, "checksum"):
            common.extract_verified(self.archive, self.root / "out", "0" * 64)
        self.assertFalse((self.root / "out").exists())

    def altered_archive(self, name, kind=tarfile.REGTYPE, content=b"bad", duplicate=False):
        path = self.root / "evil.tar.gz"
        with tarfile.open(self.archive) as src, tarfile.open(path, "w:gz") as dst:
            for member in src.getmembers():
                if member.name != name or duplicate:
                    dst.addfile(member, src.extractfile(member))
            member = tarfile.TarInfo(name)
            member.type = kind
            member.size = len(content) if kind == tarfile.REGTYPE else 0
            member.linkname = "/etc/passwd"
            dst.addfile(member, io.BytesIO(content) if member.size else None)
        return path

    def test_rejects_path_link_duplicate_extra_and_tampering(self):
        for name, kind, duplicate in (("../escape", tarfile.REGTYPE, False), ("/tmp/escape", tarfile.REGTYPE, False), ("bin/api", tarfile.SYMTYPE, False), ("bin/api", tarfile.LNKTYPE, False), ("bin/api", tarfile.REGTYPE, True), ("extra", tarfile.REGTYPE, False), ("bin/api", tarfile.REGTYPE, False)):
            with self.subTest(name=name, kind=kind, duplicate=duplicate):
                import shutil
                shutil.rmtree(self.root / "out", ignore_errors=True)
                with self.assertRaises(common.DeployError):
                    self.extract(self.altered_archive(name, kind, duplicate=duplicate))

    def test_existing_staging_directory_rejected(self):
        (self.root / "out").mkdir()
        with self.assertRaisesRegex(common.DeployError, "already exists"):
            self.extract()


class ContractTest(unittest.TestCase):
    def config(self):
        c = common.read_json("deploy/linux/private.example.json")
        c["env"].update(DATABASE_DSN="host=db sslmode=verify-full", JWT_SECRET="x" * 32, CREDENTIAL_ENCRYPTION_KEY=base64.b64encode(b"y" * 32).decode(), REDIS_PASSWORD="safe-password")
        c["bootstrap"]["password"] = "StrongTestPassword123!"
        return c

    def test_external_database_requires_explicit_ssl_choice(self):
        for mode, ca, allowed in (("disable", None, True), ("verify-full", "/ca.pem", True), ("verify-full", None, False), ("prefer", None, False), (None, None, False)):
            with self.subTest(mode=mode, ca=ca):
                params = {"host": "db.internal"}
                if mode:
                    params["sslmode"] = mode
                if ca:
                    params["sslrootcert"] = ca
                conninfo = mock.Mock(conninfo_to_dict=mock.Mock(return_value=params))
                driver = mock.MagicMock()
                conn = driver.connect.return_value.__enter__.return_value
                conn.info.server_version = 170005
                conn.execute.return_value.fetchone.side_effect = [(1,), (0,)]
                with mock.patch.dict(sys.modules, {"psycopg": driver, "psycopg.conninfo": conninfo}):
                    if allowed:
                        host.db_preflight(self.config(), True)
                        driver.connect.assert_called_once_with(self.config()["env"]["DATABASE_DSN"], connect_timeout=10)
                    else:
                        with self.assertRaisesRegex(common.DeployError, "explicit disable"):
                            host.db_preflight(self.config(), True)
                        driver.connect.assert_not_called()

    def test_backup_maps_conninfo_to_environment_without_argv_secrets(self):
        params = {"host": "db.internal", "port": "5432", "dbname": "app", "user": "app", "password": "secret' $value", "sslmode": "verify-full", "sslrootcert": "/etc/ca.pem"}
        conninfo = mock.Mock(conninfo_to_dict=mock.Mock(return_value=params))
        with mock.patch.dict(sys.modules, {"psycopg": mock.Mock(), "psycopg.conninfo": conninfo}), tempfile.TemporaryDirectory() as d:
            def dump(argv, **kwargs):
                self.assertNotIn(params["password"], " ".join(argv))
                self.assertEqual(kwargs["env"]["PGDATABASE"], "app")
                self.assertEqual(kwargs["env"]["PGPASSWORD"], params["password"])
                self.assertEqual(kwargs["env"]["PGSSLROOTCERT"], "/etc/ca.pem")
                self.assertEqual(kwargs["env"]["PGCONNECT_TIMEOUT"], "15")
                kwargs["stdout"].write(b"backup")
                return mock.Mock(returncode=0)
            with mock.patch.object(host.subprocess, "run", side_effect=dump), mock.patch.object(host, "bucket"), mock.patch.object(host, "upload_immutable") as upload:
                result = host.backup(self.config(), Path(d), "runtime-role")
            upload.assert_called_once()
            self.assertEqual(len(result["sha256"]), 64)
            self.assertEqual(list(Path(d).iterdir()), [])

    def test_backup_rejects_unmapped_options_before_running_dump(self):
        conninfo = mock.Mock(conninfo_to_dict=mock.Mock(return_value={"service": "uncontrolled"}))
        with mock.patch.dict(sys.modules, {"psycopg": mock.Mock(), "psycopg.conninfo": conninfo}), mock.patch.object(host.subprocess, "run") as run:
            with self.assertRaisesRegex(common.DeployError, "unsupported by backup"):
                host.backup_environment("service=uncontrolled")
            run.assert_not_called()

    def test_config_and_loopback_binding(self):
        c = host.validate_config(self.config(), "ainp")
        app = host.app_config(c, Path("/var/lib/ainp"))
        self.assertEqual(app["server"]["addr"], "127.0.0.1:18080")
        self.assertFalse(app["database"]["auto_migrate"])
        self.assertFalse(app["jobs"]["embedded_worker"])

    def test_redis_db_defaults_to_zero_and_validates_integer(self):
        c = self.config()
        c.pop("redis_db", None)
        self.assertEqual(host.app_config(host.validate_config(c, "ainp"), Path("/var/lib/ainp"))["redis"]["db"], 0)
        for value in (0, 1, 42):
            with self.subTest(valid=value):
                c["redis_db"] = value
                self.assertEqual(host.app_config(host.validate_config(c, "ainp"), Path("/var/lib/ainp"))["redis"]["db"], value)
        for value in (True, False, -1, "1", None, 1.5, [], {}):
            with self.subTest(invalid=value), self.assertRaisesRegex(common.DeployError, "redis_db must be a non-negative integer"):
                host.validate_config({**c, "redis_db": value}, "ainp")

    def test_redis_preflight_selects_configured_db_and_fails_closed(self):
        import types
        c = self.config()
        c["redis_db"] = 3
        client = mock.Mock()
        client.ping.return_value = True
        client.eval.return_value = b"ok"
        redis = types.SimpleNamespace(Redis=mock.Mock(return_value=client), RedisError=RuntimeError)
        with mock.patch.dict(sys.modules, {"redis": redis}):
            host.redis_preflight(c)
            self.assertEqual(redis.Redis.call_args.kwargs["db"], 3)
            client.ping.assert_called_once()
            client.eval.assert_called_once()
            client.close.assert_called_once()
            client.reset_mock()
            client.ping.side_effect = RuntimeError("ERR DB index is out of range; private details")
            with self.assertRaisesRegex(common.DeployError, "redis_db=3") as error:
                host.redis_preflight(c)
            self.assertNotIn("private details", str(error.exception))
            client.close.assert_called_once()

    def test_config_rejects_injection_and_wrong_project(self):
        for change in ({"project": "other"}, {"domain": "example.com; include /etc/passwd;"}, {"api_port": "80;touch /tmp/x"}, {"database_mode": "unknown"}):
            with self.subTest(change=change), self.assertRaises(common.DeployError):
                host.validate_config({**self.config(), **change}, "ainp")
        c = self.config()
        c["env"]["LD_PRELOAD"] = "/tmp/evil"
        with self.assertRaises(common.DeployError):
            host.validate_config(c, "ainp")

    def test_nginx_exact_oauth_sse_spa_and_tls_preserved(self):
        result = host.nginx(self.config(), Path("/opt/ainp"))
        for route in ("/healthz", "/mcp", "/oauth/register", "/oauth/token", "/oauth/revoke", "/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server"):
            self.assertIn("location = " + route + " {", result)
        self.assertNotIn("location /oauth/", result)
        self.assertIn("location ~ ^/api/v1/ai/", result)
        self.assertIn("try_files $uri $uri/ /index.html", result)
        self.assertEqual(result.count("proxy_buffering off"), 2)
        self.assertIn("/etc/ainp/tls/fullchain.pem", result)
        maintenance = host.nginx(self.config(), Path("/opt/ainp"), True)
        self.assertIn("return 503", maintenance)
        self.assertNotIn("proxy_pass", maintenance)

    def test_web_version_retention_configuration_validation(self):
        for change in ({"web_keep_releases": 1}, {"web_keep_releases": 51},
                       {"web_keep_releases": True}, {"web_pinned_versions": ["latest"]},
                       {"web_pinned_versions": ["v1.0.0", "v1.0.0"]}):
            with self.subTest(change=change), self.assertRaises(common.DeployError):
                host.validate_config({**self.config(), **change}, "ainp")

    def test_nginx_websocket_upgrade_only_on_api_routes(self):
        import re
        result = host.nginx(self.config(), Path("/opt/ainp"))
        self.assertRegex(result, r"map \$http_upgrade \$ainp_websocket_connection \{\s*default upgrade;\s*'' '';\s*\}")
        blocks = dict(re.findall(r"    location (.*?) \{\n(.*?)    \}\n", result, re.S))
        for route in ("~ ^/api/v1/ai/", "/api/"):
            with self.subTest(route=route):
                block = blocks[route]
                self.assertIn("proxy_http_version 1.1;", block)
                self.assertIn("proxy_set_header Upgrade $http_upgrade;", block)
                self.assertIn("proxy_set_header Connection $ainp_websocket_connection;", block)
                self.assertIn("proxy_set_header Host $host;", block)
                self.assertIn("proxy_set_header X-Forwarded-Proto $scheme;", block)
        for route in ("= /healthz", "= /mcp", "= /oauth/token"):
            with self.subTest(route=route):
                self.assertNotIn("proxy_set_header Upgrade", blocks[route])
                self.assertNotIn("proxy_set_header Connection", blocks[route])
        self.assertEqual(result.count("proxy_set_header Upgrade"), 2)
        maintenance = host.nginx(self.config(), Path("/opt/ainp"), True)
        self.assertNotIn("proxy_set_header Upgrade", maintenance)

    def test_systemd_non_root_and_writable_scope(self):
        result = host.unit("ainp", "ai-worker", Path("/opt/ainp"), Path("/var/lib/ainp"))
        for fragment in ("User=ainp", "ProtectSystem=strict", "ReadWritePaths=/var/lib/ainp", "TimeoutStopSec=90", "NoNewPrivileges=true"):
            self.assertIn(fragment, result)
        self.assertNotIn("Environment=", result)

    def test_launcher_preserves_secret_without_shell(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / "runtime.json"
            secret = 'a"\\$%\n$(touch /tmp/not-executed)'
            path.write_text(json.dumps({"home": "/var/lib/ainp", "env": {"REDIS_PASSWORD": secret}, "worker_concurrency": 4, "ai_concurrency": 1}))
            with mock.patch.object(sys, "argv", ["launcher", str(path), "ai-worker", "/opt/ainp/current"]), mock.patch.object(os, "execve") as execute:
                launcher.main()
            binary, argv, env = execute.call_args.args
            self.assertEqual(binary, "/opt/ainp/current/bin/worker")
            self.assertEqual(env["WORKER_QUEUES"], "ai=1")
            self.assertEqual(env["REDIS_PASSWORD"], secret)
            self.assertNotIn("LD_PRELOAD", env)

    def test_atomic_json_private_and_lock(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / "state"
            common.atomic_json(path, {"state": "creating"})
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            with common.lock(Path(d) / "lock"):
                with self.assertRaises(common.DeployError):
                    with common.lock(Path(d) / "lock"):
                        pass


class CloudTest(unittest.TestCase):
    def unpack_bootstrap(self, command):
        literal = command.split("base64.b85decode(", 1)[1].splitlines()[0].removesuffix(")))")
        return json.loads(bz2.decompress(base64.b85decode(ast.literal_eval(literal))))

    def target(self):
        c = common.read_json("deploy/linux/target.example.json")
        c["config_sha256"] = "a" * 64
        return c

    def reference(self):
        return dict(project="ainp", version="v1.2.3", commit="b" * 40, platform="ubuntu-22.04-amd64", sha256="c" * 64, oss={"bucket": "private-release", "endpoint": "https://oss-cn-hangzhou-internal.aliyuncs.com"}, key="releases/ainp/v1.2.3/release.tar.gz")

    def test_default_plan_has_no_cloud_calls(self):
        with tempfile.TemporaryDirectory() as d:
            target, release = Path(d) / "target.json", Path(d) / "release.json"
            common.atomic_json(target, self.target())
            common.atomic_json(release, self.reference())
            with mock.patch.object(sys, "argv", ["cloud", "init", "--target", str(target), "--release", str(release)]), mock.patch.object(cloud, "execute") as execute, contextlib.redirect_stdout(io.StringIO()) as output:
                cloud.main()
            execute.assert_not_called()
            self.assertIn("PLAN ONLY", output.getvalue())

    def test_ci_release_is_only_allowed_in_test_environment(self):
        ci_reference = {**self.reference(), "version": "v0.0.0-ci"}
        with self.assertRaisesRegex(common.DeployError, "only in the test environment"):
            cloud.validate_release_for_environment(self.target(), ci_reference)

        test_target = {**self.target(), "environment": "test"}
        cloud.validate_release_for_environment(test_target, ci_reference)

    def test_test_environment_ci_plan_still_makes_no_cloud_calls(self):
        with tempfile.TemporaryDirectory() as d:
            target, release = Path(d) / "target.json", Path(d) / "release.json"
            common.atomic_json(target, {**self.target(), "environment": "test"})
            common.atomic_json(release, {**self.reference(), "version": "v0.0.0-ci"})
            with mock.patch.object(
                sys,
                "argv",
                ["cloud", "init", "--target", str(target), "--release", str(release)],
            ), mock.patch.object(cloud, "execute") as execute, contextlib.redirect_stdout(io.StringIO()) as output:
                cloud.main()
            execute.assert_not_called()
            self.assertIn("PLAN ONLY", output.getvalue())

    def test_fixed_template_and_no_arbitrary_cli_profile(self):
        for change in ({"launch_template": {"id": "lt-demo", "version": "Latest"}}, {"profile": "profile;evil"}, {"environment": "../production"}, {"release_oss_endpoint": "https://attacker.example.com"}):
            with self.subTest(change=change), self.assertRaises(common.DeployError):
                cloud.validate_target({**self.target(), **change})

    def test_alibaba_json_error_exposes_only_bounded_safe_fields(self):
        response = {
            "schemaVersion": "v1",
            "message": "Balance too low\nrecharge first",
            "error_code": "InvalidAccountStatus.NotEnoughBalance",
            "status_code": 403,
            "request_id": "REQUEST-123",
            "recovery": {"command": "aliyun command --AccessKeySecret leaked-secret"},
            "AccessKeySecret": "leaked-secret",
        }
        result = cloud.subprocess.CompletedProcess([], 2, stdout="", stderr=json.dumps(response))
        with mock.patch.object(cloud.subprocess, "run", return_value=result):
            with self.assertRaises(common.DeployError) as raised:
                cloud.Alibaba(self.target()).call("RunInstances", CommandContent="private-command")
        message = str(raised.exception)
        self.assertIn("code=InvalidAccountStatus.NotEnoughBalance", message)
        self.assertIn("status=403", message)
        self.assertIn("message=Balance too low recharge first", message)
        self.assertIn("request_id=REQUEST-123", message)
        self.assertNotIn("leaked-secret", message)
        self.assertNotIn("private-command", message)
        self.assertNotIn("recovery", message)

    def test_alibaba_non_json_or_oversized_error_does_not_echo_raw_output(self):
        cases = ("raw secret output", "x" * (cloud.CLI_ERROR_JSON_LIMIT + 1))
        for raw in cases:
            with self.subTest(size=len(raw)):
                result = cloud.subprocess.CompletedProcess([], 2, stdout="", stderr=raw)
                with mock.patch.object(cloud.subprocess, "run", return_value=result):
                    with self.assertRaisesRegex(common.DeployError, "response unavailable or unrecognized") as raised:
                        cloud.Alibaba(self.target()).call("RunInstances")
                self.assertNotIn(raw[:20], str(raised.exception))

    def test_alibaba_timeout_does_not_echo_command_or_exception_output(self):
        error = cloud.subprocess.TimeoutExpired(["aliyun", "--secret", "leaked-secret"], 120, output="leaked-output")
        with mock.patch.object(cloud.subprocess, "run", side_effect=error):
            with self.assertRaisesRegex(common.DeployError, "unavailable/timeout") as raised:
                cloud.Alibaba(self.target()).call("RunInstances", CommandContent="private-command")
        message = str(raised.exception)
        self.assertNotIn("leaked-secret", message)
        self.assertNotIn("leaked-output", message)
        self.assertNotIn("private-command", message)

    def test_bootstrap_with_real_request_fits_and_has_no_credentials(self):
        request = dict(project="ainp", ram_role="ainp-production-runtime", action="init", operation_id="a" * 36, release=self.reference(), config_oss=self.target()["config_oss"], config_key=self.target()["config_key"], config_sha256="a" * 64)
        files = cloud._bootstrap_files(request)
        self.assertNotIn("DATABASE_DSN=", files["bootstrap.sh"])
        shell = files["bootstrap.sh"]
        self.assertIn("chmod 644 /usr/share/postgresql-common/pgdg", shell)
        self.assertIn("mirrors.cloud.aliyuncs.com", shell)
        self.assertIn("/etc/apt/sources.list", shell)

    def test_versioned_bootstrap_loader_is_bounded_and_uses_ram_role(self):
        import subprocess
        obj = dict(endpoint="https://oss-cn-hangzhou-internal.aliyuncs.com", bucket="private-release",
                   key="releases/ainp/v1.2.3/" + "b" * 40 + "/bootstrap/" + "a" * 64 + ".bundle.bz2",
                   sha256="a" * 64, ram_role="ainp-production-runtime")
        command = cloud.bootstrap({}, obj)
        self.assertLessEqual(len(command.encode()), 16384)
        self.assertLessEqual(len(base64.b64encode(command.encode())), 24576)
        self.assertIn('"\\n/"+bucket+"/"+object_key', command)
        self.assertIn('actual==expected', command)
        self.assertIn('metadata-token', command)
        self.assertNotIn('AccessKeySecret=', command)
        subprocess.run(["sh", "-n"], input=command, text=True, check=True)
        python = command.split("<<'AINP_BOOTSTRAP'\n", 1)[1].split("\nAINP_BOOTSTRAP", 1)[0]
        compile(python, "versioned-bootstrap", "exec")

    def test_versioned_loader_fetches_signed_oss_object_and_rejects_tampering(self):
        import hmac
        import urllib.request
        obj = dict(endpoint="https://oss-cn-hangzhou-internal.aliyuncs.com", bucket="private-release",
                   key="releases/ainp/v1.2.3/bootstrap/verified.bundle.bz2", ram_role="runtime")
        bundle = bz2.compress(json.dumps({"bootstrap.sh": "exit 0", "request.json": "{}"}).encode())
        obj["sha256"] = hashlib.sha256(bundle).hexdigest()
        command = cloud.bootstrap({}, obj)
        loader = command.split("<<'AINP_BOOTSTRAP'\n", 1)[1].split("\nAINP_BOOTSTRAP", 1)[0]
        class Reply:
            def __init__(self, body): self.body = io.BytesIO(body)
            def __enter__(self): return self
            def __exit__(self, *args): pass
            def read(self, *args): return self.body.read(*args)
        class Opener:
            def __init__(self, body): self.body = body
            def open(self, request, timeout=None):
                if request.full_url.endswith("/api/token"):
                    self_role = request.headers.get("X-aliyun-ecs-metadata-token-ttl-seconds")
                    self_test.assertEqual(self_role, "21600")
                    return Reply(b"metadata-token")
                if request.full_url.endswith("/security-credentials/runtime"):
                    return Reply(json.dumps(dict(Code="Success", AccessKeyId="access", AccessKeySecret="secret", SecurityToken="sts")).encode())
                self_test.assertEqual(request.headers["X-oss-security-token"], "sts")
                signature = base64.b64encode(hmac.new(b"secret", ("GET\n\n\n" + request.headers["Date"] + "\nx-oss-security-token:sts\n/private-release/" + obj["key"]).encode(), hashlib.sha1).digest()).decode()
                self_test.assertEqual(request.headers["Authorization"], "OSS access:" + signature)
                return Reply(self.body)
        self_test = self
        with tempfile.TemporaryDirectory() as directory:
            for body, should_pass in ((bundle, True), (bundle + b"tampered", False)):
                with self.subTest(should_pass=should_pass), mock.patch.object(sys, "argv", ["loader", directory]), mock.patch.object(urllib.request, "build_opener", return_value=Opener(body)):
                    if should_pass:
                        exec(compile(loader, "versioned-bootstrap", "exec"), {})
                        self.assertEqual((Path(directory) / "bootstrap.sh").read_text(), "exit 0")
                    else:
                        with self.assertRaisesRegex(RuntimeError, "checksum mismatch"):
                            exec(compile(loader, "versioned-bootstrap", "exec"), {})

    def test_bootstrap_upload_reads_back_exact_bytes_before_dispatch(self):
        request = dict(project="ainp", action="update", operation_id="a" * 36, ram_role="role", release=self.reference())
        target = self.target()
        def transfer(argv, **kwargs):
            self.assertEqual(argv[:3], ["aliyun", "oss", "cp"])
            self.assertIn("--cli-non-interactive", argv)
            if argv[3].startswith("oss://"):
                Path(argv[4]).write_bytes(Path(upload_path[0]).read_bytes())
            else:
                upload_path.append(argv[3])
            return subprocess.CompletedProcess(argv, 0, stdout="", stderr="")
        import subprocess
        upload_path = []
        with mock.patch.object(cloud.subprocess, "run", side_effect=transfer) as run:
            obj = cloud._upload_bootstrap_object(self.reference(), target, request)
        self.assertEqual(run.call_count, 2)
        self.assertIn(request["operation_id"], obj["key"])
        self.assertEqual(obj["endpoint"], target["release_oss_endpoint"])
        self.assertEqual(len(obj["sha256"]), 64)

    def test_poll_streams_only_complete_progress_records_once(self):
        first = "AINP_PROGRESS 2026-09-26T12:00:00Z postgresql-preflight: started\n"
        last = "AINP_PROGRESS 2026-09-26T12:00:05Z postgresql-preflight: done (5s)\n"
        def payload(output, status="Running"):
            return {"Invocation": {"InvocationResults": {"InvocationResult": [{
                "InstanceId": "i-one", "InvocationStatus": status,
                "InvokeRecordStatus": "Finished" if status == "Success" else "Running",
                "ExitCode": 0, "Output": output,
            }]}}}
        api = mock.Mock()
        api.call.side_effect = [payload(first + "PASSWORD=secret-value\n" + last[:40]), payload(first + last), payload(first + last, "Success")]
        state = {"invoke_id": "t-one", "instance_id": "i-one", "status": "running"}
        with tempfile.TemporaryDirectory() as d, mock.patch.object(cloud.time, "sleep"), contextlib.redirect_stdout(io.StringIO()) as output:
            cloud.poll(api, state, Path(d) / "state", timeout=10)
        self.assertEqual(output.getvalue().count(first), 1)
        self.assertEqual(output.getvalue().count(last), 1)
        self.assertNotIn("secret-value", output.getvalue())
        self.assertEqual(state["status"], "success")

    def test_bootstrap_loader_roundtrip_and_shell_syntax(self):
        import subprocess
        files = cloud._bootstrap_files(dict(project="ainp", ram_role="role", action="init", operation_id="a" * 36, release=self.reference()))
        command = cloud.BOOTSTRAP.replace("PAYLOAD_LITERAL", repr(base64.b85encode(bz2.compress(json.dumps(files).encode())).decode()))
        # The transport may omit standalone comments, but must preserve the exact program.
        for filename in ("host.py", "common.py", "storage.py", "reporting.py", "launcher.py"):
            self.assertEqual(ast.dump(ast.parse(files[filename])), ast.dump(ast.parse((DEPLOY / filename).read_text())))
        for script in (command, files["bootstrap.sh"], files["apt.sh"]):
            subprocess.run(["sh", "-n"], input=script, text=True, capture_output=True, check=True)
        with tempfile.TemporaryDirectory() as d:
            # Execute the actual loader with a harmless replacement inner bootstrap.
            files["bootstrap.sh"] = 'set -eu\ntest -f "$1/host.py"\ntest -f "$1/reporting.py"\nprintf "unpacked successfully\\n"\n'
            payload = base64.b85encode(bz2.compress(json.dumps(files).encode())).decode()
            safe = cloud.BOOTSTRAP.replace("PAYLOAD_LITERAL", repr(payload)).replace("/var/tmp/ainp-deploy.XXXXXXXX", d + "/payload.XXXXXXXX")
            result = subprocess.run(["sh"], input=safe, capture_output=True, text=True, check=True)
            self.assertIn("unpacked successfully", result.stdout)
            self.assertEqual(list(Path(d).iterdir()), [])

    def test_progress_is_timestamped_and_flushed(self):
        with mock.patch.object(cloud.time, "strftime", return_value="2026-09-26 20:00:00"), mock.patch("builtins.print") as output:
            cloud.progress("waiting for ECS")
        output.assert_called_once_with("[2026-09-26 20:00:00] waiting for ECS", flush=True)

    def test_poll_reports_pending_heartbeat_and_terminal_status(self):
        pending = {"Invocation": {"InvocationResults": {"InvocationResult": []}}}
        success = {"Invocation": {"InvocationResults": {"InvocationResult": [{
            "InstanceId": "i-one",
            "InvocationStatus": "Success",
            "InvokeRecordStatus": "Finished",
            "ExitCode": 0,
        }]}}}
        api = mock.Mock()
        api.call.side_effect = [pending, success]
        state = {"invoke_id": "t-one", "instance_id": "i-one", "status": "running"}
        with tempfile.TemporaryDirectory() as d, mock.patch.object(cloud.time, "sleep"), contextlib.redirect_stdout(io.StringIO()) as output:
            cloud.poll(api, state, Path(d) / "state.json", timeout=1)
        text = output.getvalue()
        self.assertIn("Cloud Assistant invocation t-one still running", text)
        self.assertIn("Cloud Assistant invocation t-one: success", text)
        self.assertIn("after", text)
        self.assertNotIn("CommandContent", text)

    def test_bootstrap_retries_only_apt_lock_failures_with_bounded_timeout(self):
        files = cloud._bootstrap_files(dict(project="ainp", ram_role="ainp-production-runtime", action="init", operation_id="a" * 36, release=self.reference(), config_oss=self.target()["config_oss"], config_key=self.target()["config_key"], config_sha256="a" * 64))
        command = files["apt.sh"]
        self.assertIn("else status=$?", command)
        self.assertIn('return "$status"', command)
        self.assertNotIn("return 100", command)
        self.assertIn('sh "$work/apt.sh" update -qq', files["bootstrap.sh"])
        self.assertIn("AINP_APT_LOCK_TIMEOUT", command)
        self.assertIn("reconcile unattended-upgrades", command)
        self.assertNotIn("rm -f /var/lib/dpkg/lock", command)
        self.assertNotIn("kill ", command)

    def test_bootstrap_uses_compact_host_release_for_real_reference_metadata(self):
        reference = {
            **self.reference(),
            "schema": 1,
            "build_time": "2026-09-26T07:24:59Z",
        }
        target = self.target()
        request = dict(
            project=target["project"],
            ram_role=target["ram_role"],
            action="init",
            operation_id="a" * 36,
            release=cloud._host_release(reference, target["release_oss_endpoint"]),
            config_oss=target["config_oss"],
            config_key=target["config_key"],
            config_sha256=target["config_sha256"],
        )
        files = cloud._bootstrap_files(request)
        remote_request = json.loads(files["request.json"])
        self.assertEqual(set(remote_request["release"]), {"project", "version", "commit", "platform", "sha256", "key", "oss"})
        self.assertEqual(remote_request["release"]["oss"]["endpoint"], target["release_oss_endpoint"])
        self.assertNotIn("schema", remote_request["release"])
        self.assertNotIn("build_time", remote_request["release"])

    def test_failed_init_recovers_same_instance_and_preserves_attempt_audit(self):
        target = self.target()
        reference = self.reference()
        target_sha256 = hashlib.sha256(json.dumps(target, sort_keys=True).encode()).hexdigest()
        def call(action, **kwargs):
            self.assertNotEqual(action, "RunInstances")
            if action == "DescribeInstances":
                return {"Instances": {"Instance": [{"InstanceId": "i-existing", "Status": "Running", "InstanceChargeType": "PostPaid", "Tags": {"Tag": [{"TagKey": "ainp-project", "TagValue": "ainp"}, {"TagKey": "ainp-environment", "TagValue": "production"}]}}]}}
            if action == "DescribeCloudAssistantStatus":
                return {"InstanceCloudAssistantStatusSet": {"InstanceCloudAssistantStatus": [{"InstanceId": "i-existing", "CloudAssistantStatus": "true"}]}}
            self.assertEqual(action, "RunCommand")
            return {"InvokeId": "t-recovery"}
        with tempfile.TemporaryDirectory() as d:
            statepath = Path(d) / "state.json"
            common.atomic_json(statepath, {
                "target_sha256": target_sha256,
                "client_token": "create-token",
                "operation_id": "failed-operation",
                "invoke_id": "t-failed",
                "command_sha256": "d" * 64,
                "action": "init",
                "status": "failed",
                "instance_id": "i-existing",
                "release": reference,
                "config_sha256": target["config_sha256"],
            })
            with mock.patch.object(cloud.Alibaba, "call", side_effect=call), mock.patch.object(cloud, "poll"), mock.patch.object(cloud, "bootstrap", return_value="echo safe"), mock.patch.object(cloud, "_upload_bootstrap_object", return_value={"endpoint": "https://oss-cn-hangzhou-internal.aliyuncs.com", "bucket": "private-release", "key": "key", "sha256": "a" * 64, "ram_role": "role"}), contextlib.redirect_stdout(io.StringIO()):
                cloud.execute(target, reference, "init", statepath)
            state = common.read_json(statepath)
            self.assertEqual(state["instance_id"], "i-existing")
            self.assertEqual(state["client_token"], "create-token")
            self.assertEqual(state["invoke_id"], "t-recovery")
            self.assertEqual(state["config_sha256"], target["config_sha256"])
            self.assertEqual(state["attempts"][0]["invoke_id"], "t-failed")
            self.assertEqual(state["attempts"][0]["status"], "failed")

    def test_invocation_fail_closed(self):
        def payload(**kw):
            return {"Invocation": {"InvocationResults": {"InvocationResult": [{"InstanceId": "i-one", **kw}]}}}
        self.assertEqual(cloud.invocation_result({}, "i-one"), "pending")
        self.assertEqual(cloud.invocation_result(payload(InvocationStatus="Success", InvokeRecordStatus="Finished", ExitCode=0), "i-one"), "success")
        self.assertEqual(cloud.invocation_result(payload(InvocationStatus="Failed", ExitCode=1), "i-one"), "failed")
        self.assertEqual(cloud.invocation_result(payload(InvocationStatus="Running"), "i-one"), "pending")
        for status in ("Aborted", "Terminated"):
            self.assertEqual(cloud.invocation_result(payload(InvocationStatus=status), "i-one"), "failed")
        with self.assertRaises(common.DeployError):
            cloud.invocation_result(payload(InvocationStatus="Success", InvokeRecordStatus="Finished", ExitCode=1), "i-one")

    def test_invocation_failure_includes_bounded_redacted_output_tail(self):
        output = "old output\n" + "x" * 5000 + "\nPASSWORD=database-secret\nhttps://user:url-secret@example.com\nE: dpkg lock busy"
        payload = {"Invocation": {"InvocationResults": {"InvocationResult": [{
            "InstanceId": "i-one",
            "InvocationStatus": "Failed",
            "InvokeRecordStatus": "Finished",
            "ExitCode": 100,
            "ErrorCode": "ExitCodeNonzero",
            "ErrorInfo": "The command execution exit code is not zero.",
            "Output": output,
        }]}}}
        message = cloud.invocation_failure(payload, "i-one")
        self.assertIn("status=Failed", message)
        self.assertIn("exit_code=100", message)
        self.assertIn("error_code=ExitCodeNonzero", message)
        self.assertIn("[output tail]", message)
        self.assertIn("E: dpkg lock busy", message)
        self.assertNotIn("database-secret", message)
        self.assertNotIn("url-secret", message)
        self.assertNotIn("old output", message)

    def test_poll_failure_surfaces_invocation_diagnostics(self):
        payload = {"Invocation": {"InvocationResults": {"InvocationResult": [{
            "InstanceId": "i-one",
            "InvocationStatus": "Failed",
            "InvokeRecordStatus": "Finished",
            "ExitCode": 100,
            "ErrorCode": "ExitCodeNonzero",
            "Output": "E: Unable to acquire the dpkg frontend lock",
        }]}}}
        state = {"invoke_id": "t-one", "instance_id": "i-one", "status": "running"}
        api = mock.Mock()
        api.call.return_value = payload
        with tempfile.TemporaryDirectory() as d:
            statepath = Path(d) / "state.json"
            with self.assertRaisesRegex(common.DeployError, "dpkg frontend lock"):
                cloud.poll(api, state, statepath, timeout=1)
            self.assertEqual(common.read_json(statepath)["status"], "failed")

    def test_ownership_required_before_remote_command(self):
        api = mock.Mock()
        api.call.return_value = {"Instances": {"Instance": [{"InstanceId": "i-one", "InstanceChargeType": "PostPaid", "Tags": {"Tag": []}}]}}
        with self.assertRaisesRegex(common.DeployError, "ownership"):
            cloud.describe_instance(api, "i-one", self.target())

    def test_uncertain_creation_never_creates_second_instance(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / "state.json"
            with mock.patch.object(cloud.Alibaba, "call", side_effect=common.DeployError("timeout")) as call, mock.patch.object(cloud, "_upload_bootstrap_object", return_value={"endpoint": "https://oss-cn-hangzhou-internal.aliyuncs.com", "bucket": "private-release", "key": "key", "sha256": "a" * 64, "ram_role": "role"}):
                with self.assertRaises(common.DeployError):
                    cloud.execute(self.target(), self.reference(), "init", path)
                self.assertEqual(common.read_json(path)["status"], "creating")
                self.assertTrue(common.read_json(path)["client_token"])
                with self.assertRaisesRegex(common.DeployError, "outcome unknown"):
                    cloud.execute(self.target(), self.reference(), "init", path)
                self.assertEqual(call.call_count, 1)

    def test_config_rotation_checks_persisted_resource_identity(self):
        original = self.target()
        original_state = {"target_sha256": hashlib.sha256(json.dumps(original, sort_keys=True).encode()).hexdigest(),
                          "target_binding": cloud.target_binding(original),
                          "config_key": original["config_key"], "config_sha256": original["config_sha256"]}
        rotated = {**original, "config_key": "config-v2.json", "config_sha256": "b" * 64}
        rotated_hash = hashlib.sha256(json.dumps(rotated, sort_keys=True).encode()).hexdigest()
        cloud.validate_target_binding(original_state, rotated, rotated_hash, "update")
        for changed in ({**rotated, "region": "cn-shanghai"},
                        {**rotated, "config_oss": {**rotated["config_oss"], "bucket": "another-private-bucket"}},
                        {**rotated, "ram_role": "other-role"}):
            with self.assertRaises(common.DeployError):
                cloud.validate_target_binding(original_state, changed, "different", "update")
        with self.assertRaises(common.DeployError):
            cloud.validate_target_binding({"target_sha256": original_state["target_sha256"]}, rotated, rotated_hash, "update")
        with self.assertRaises(common.DeployError):
            cloud.validate_target_binding(original_state, rotated, rotated_hash, "rollback")

    def test_existing_instance_dispatch_uses_private_endpoint_and_idempotence(self):
        target = {**self.target(), "instance_id": "i-existing"}
        reference = self.reference()
        reference["oss"]["endpoint"] = "https://oss-cn-hangzhou.aliyuncs.com"
        def call(action, **kwargs):
            if action == "DescribeInstances":
                return {"Instances": {"Instance": [{"InstanceId": "i-existing", "Status": "Running", "InstanceChargeType": "PostPaid", "Tags": {"Tag": [{"TagKey": "ainp-project", "TagValue": "ainp"}, {"TagKey": "ainp-environment", "TagValue": "production"}]}}]}}
            if action == "DescribeCloudAssistantStatus":
                return {"InstanceCloudAssistantStatusSet": {"InstanceCloudAssistantStatus": [{"InstanceId": "i-existing", "CloudAssistantStatus": "true"}]}}
            self.assertEqual(action, "RunCommand")
            self.assertEqual(kwargs["TerminationMode"], "ProcessTree")
            self.assertEqual(kwargs["KeepCommand"], "false")
            self.assertEqual(kwargs["ClientToken"], common.read_json(statepath)["operation_id"])
            self.assertEqual(common.read_json(statepath)["status"], "dispatching")
            return {"InvokeId": "t-123"}
        with tempfile.TemporaryDirectory() as d:
            statepath = Path(d) / "state.json"
            with mock.patch.object(cloud.Alibaba, "call", side_effect=call), mock.patch.object(cloud, "poll"), mock.patch.object(cloud, "bootstrap", return_value="echo safe") as bootstrap, mock.patch.object(cloud, "_upload_bootstrap_object", return_value={"endpoint": "https://oss-cn-hangzhou-internal.aliyuncs.com", "bucket": "private-release", "key": "key", "sha256": "a" * 64, "ram_role": "role"}), contextlib.redirect_stdout(io.StringIO()):
                cloud.execute(target, reference, "init", statepath)
            request = bootstrap.call_args.args[0]
            self.assertEqual(request["release"]["oss"]["endpoint"], target["release_oss_endpoint"])
            self.assertEqual(reference["oss"]["endpoint"], "https://oss-cn-hangzhou.aliyuncs.com")
            self.assertEqual(common.read_json(statepath)["invoke_id"], "t-123")

    def test_status_timeout_does_not_mark_cancelled_or_resubmit(self):
        state = {"invoke_id": "t-123", "instance_id": "i-one", "status": "running"}
        api = mock.Mock()
        with self.assertRaisesRegex(common.DeployError, "pending/unknown"):
            cloud.poll(api, state, Path("unused"), timeout=0)
        self.assertEqual(state["status"], "running")
        api.call.assert_not_called()



class ReportingTest(unittest.TestCase):
    def test_progress_arrives_before_process_exits_and_has_heartbeat(self):
        import subprocess
        import selectors
        code = f"import sys,time; sys.path.insert(0, {str(DEPLOY.resolve())!r}); import reporting; reporting.INTERVAL=0.05\nwith reporting.stage('fixture'):\n time.sleep(0.4)\n"
        with subprocess.Popen([sys.executable, "-c", code], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True) as proc:
            try:
                with selectors.DefaultSelector() as selector:
                    selector.register(proc.stdout, selectors.EVENT_READ)
                    self.assertTrue(selector.select(timeout=5), "no flushed start record")
                first = proc.stdout.readline()
                self.assertIn("fixture: started", first)
                self.assertIsNone(proc.poll(), "output must arrive while running")
                output, error = proc.communicate(timeout=5)
                self.assertEqual(proc.returncode, 0, error)
                self.assertIn("fixture: running", output)
                self.assertIn("fixture: done", output)
            finally:
                if proc.poll() is None:
                    proc.kill()
                    proc.communicate()

    def test_failed_command_preserves_exit_code_and_protects_output(self):
        with tempfile.TemporaryDirectory() as d, mock.patch.object(reporting, "LOG_PATH", str(Path(d) / "log")), contextlib.redirect_stdout(io.StringIO()) as output:
            with self.assertRaises(SystemExit) as raised:
                reporting.run("fixture", [sys.executable, "-c", "import sys; print('sensitive-raw-output'); sys.exit(7)"])
            self.assertEqual(raised.exception.code, 7)
            self.assertIn("fixture: failed", output.getvalue())
            self.assertNotIn("sensitive-raw-output", output.getvalue())
            log = Path(d) / "log"
            self.assertIn("sensitive-raw-output", log.read_text())
            self.assertEqual(log.stat().st_mode & 0o777, 0o600)

    def test_stage_failure_does_not_print_exception_secrets(self):
        with contextlib.redirect_stdout(io.StringIO()) as output:
            with self.assertRaises(ValueError):
                with reporting.stage("postgresql-preflight"):
                    raise ValueError("password=hidden-value")
        self.assertIn("postgresql-preflight: failed (ValueError)", output.getvalue())
        self.assertNotIn("hidden-value", output.getvalue())

    def test_bootstrap_lock_descriptor_is_inherited(self):
        with tempfile.TemporaryDirectory() as d, mock.patch.object(reporting, "LOG_PATH", str(Path(d) / "log")), mock.patch.object(reporting.subprocess, "run", return_value=mock.Mock(returncode=0)) as run, contextlib.redirect_stdout(io.StringIO()):
            reporting.run("bootstrap-lock", ["flock", "-w", "120", "9"])
        self.assertEqual(run.call_args.kwargs["pass_fds"], (9,))

    def test_apt_retries_locks_only_and_preserves_exit_status(self):
        import subprocess
        for scenario, expected, count in (("recover", 0, 3), ("locked", 75, 121), ("error", 42, 1)):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as d:
                tmp = Path(d)
                (tmp / "count").write_text("0")
                apt = tmp / "apt-get"
                apt.write_text('''#!/bin/sh
i=$(cat "$COUNT"); i=$((i+1)); echo "$i" >"$COUNT"
if [ "$SCENARIO" = error ]; then echo fatal; exit 42; fi
if [ "$SCENARIO" = recover ] && [ "$i" -ge 3 ]; then echo ready; exit 0; fi
echo 'Could not get lock /var/lib/dpkg/lock-frontend'; exit 100
''')
                apt.chmod(0o755)
                sleep = tmp / "sleep"
                sleep.write_text("#!/bin/sh\nexit 0\n")
                sleep.chmod(0o755)
                env = {**os.environ, "COUNT": str(tmp / "count"), "SCENARIO": scenario, "PATH": d + os.pathsep + os.environ.get("PATH", "/usr/bin:/bin")}
                result = subprocess.run(["sh", str(DEPLOY / "apt.sh"), "update", "-qq"], env=env, capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, expected, result.stderr)
                self.assertEqual(int((tmp / "count").read_text()), count)
                if scenario == "locked":
                    self.assertIn("AINP_APT_LOCK_TIMEOUT", result.stderr)


class HTTPSProbeTest(unittest.TestCase):
    def test_real_curl_retries_transient_maintenance_but_not_permanent_errors(self):
        import http.server
        import threading
        import subprocess
        import shutil
        if not shutil.which("curl"):
            self.skipTest("curl unavailable")
        for scenario, expected_requests in (("recover", 2), ("unavailable", 6), ("missing", 1)):
            with self.subTest(scenario=scenario):
                requests = []
                class Handler(http.server.BaseHTTPRequestHandler):
                    def do_GET(self):
                        requests.append(self.path)
                        status = 404 if scenario == "missing" else (200 if scenario == "recover" and len(requests) > 1 else 503)
                        self.send_response(status)
                        self.end_headers()
                        self.wfile.write(b'{"data":{"status":"ok"}}' if status == 200 else b'maintenance')
                    def log_message(self, *args):
                        pass
                server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
                thread = threading.Thread(target=server.serve_forever, daemon=True)
                thread.start()
                def run(argv):
                    # Exercise actual curl retry semantics with a local HTTP fixture.
                    # Production retains HTTPS certificate validation and --resolve.
                    argv = [*argv[:-1], f"http://127.0.0.1:{server.server_port}/healthz"]
                    result = subprocess.run(argv, capture_output=True, timeout=20)
                    common.require(result.returncode == 0, "probe failed")
                    return result.stdout
                try:
                    with mock.patch.object(host, "command", side_effect=run):
                        if scenario == "recover":
                            self.assertEqual(host.https_json({"domain": "fixture.invalid"}, "/healthz")["data"]["status"], "ok")
                        else:
                            with self.assertRaises(common.DeployError):
                                host.https_json({"domain": "fixture.invalid"}, "/healthz")
                    self.assertEqual(len(requests), expected_requests)
                finally:
                    server.shutdown()
                    server.server_close()
                    thread.join()


class NativeNginxSyntaxTest(unittest.TestCase):
    def test_generated_multi_web_routes_are_isolated(self):
        apps = [
            {"name": "admin", "mount": "/", "health_path": "/build.json", "output_dir": "web"},
            {"name": "screen", "mount": "/bigscreen/", "health_path": "/bigscreen/build.json", "output_dir": "web/bigscreen"},
        ]
        conf = host.nginx(ContractTest().config(), Path("/opt/ainp"), manifest_apps=apps)
        self.assertIn("location = /bigscreen { return 301 /bigscreen/; }", conf)
        self.assertIn("location ^~ /bigscreen/ { try_files $uri $uri/ /bigscreen/index.html; }", conf)
        self.assertIn("location / { try_files $uri $uri/ /index.html; }", conf)

    def test_https_versioned_multi_web_assets_survive_switch_and_rollback(self):
        import shutil
        import socket
        import ssl
        import subprocess
        import time
        import uuid

        if not shutil.which("docker") or not shutil.which("openssl"):
            self.skipTest("Docker/OpenSSL unavailable for versioned Web test")
        if subprocess.run(["docker", "info"], capture_output=True).returncode:
            self.skipTest("Docker daemon unavailable for versioned Web test")
        with tempfile.TemporaryDirectory() as d:
            base = Path(d)
            root = base / "ainp"
            root.mkdir(mode=0o755)
            root.chmod(0o755)
            releases = root / "releases"
            for version in ("v1.0.0", "v1.1.0"):
                for app in ("", "bigscreen/"):
                    web = releases / version / "web" / app
                    (web / "assets").mkdir(parents=True)
                    (web / "index.html").write_text(f"{version}-{app} <script src='/__web/{version}/{app}assets/app-{version}.js'></script>")
                    (web / "assets" / f"app-{version}.js").write_text(f"{version}-{app}-chunk")
            versions = root / "web-versions"
            versions.mkdir()
            for version in ("v1.0.0", "v1.1.0"):
                (versions / version).symlink_to(f"../releases/{version}/web")
            (root / "current").symlink_to("releases/v1.0.0")
            # Pre-versioned clients still use /assets/... until the last one refreshes.
            (root / "legacy-web").symlink_to("releases/v1.0.0/web")
            subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=admin.example.com", "-keyout", str(base / "privkey.pem"), "-out", str(base / "fullchain.pem")], check=True, capture_output=True)
            apps = [{"name": "admin", "mount": "/"}, {"name": "screen", "mount": "/bigscreen/"}]
            (base / "default.conf").write_text(host.nginx(ContractTest().config(), Path("/opt/ainp"), manifest_apps=apps))
            with socket.socket() as probe:
                probe.bind(("127.0.0.1", 0))
                port = probe.getsockname()[1]
            container = "ainp-web-version-" + uuid.uuid4().hex[:12]
            result = subprocess.run(["docker", "run", "-d", "--rm", "--name", container,
                "-p", f"127.0.0.1:{port}:443",
                "-v", f"{base / 'default.conf'}:/etc/nginx/conf.d/default.conf:ro",
                "-v", f"{base}:/etc/ainp/tls:ro", "nginx:1.27.5-alpine"], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            subprocess.run(["docker", "cp", str(root), f"{container}:/opt/ainp"], check=True, capture_output=True)

            def fetch(path):
                # This test exercises routing and immutable assets with a local test certificate.
                import http.client
                conn = http.client.HTTPSConnection("127.0.0.1", port, context=ssl._create_unverified_context(), timeout=3)
                try:
                    conn.request("GET", path, headers={"Host": "admin.example.com"})
                    response = conn.getresponse()
                    return response.status, response.read().decode()
                finally:
                    conn.close()

            try:
                for attempt in range(40):
                    try:
                        fetch("/")
                        break
                    except OSError:
                        if attempt == 39:
                            self.fail("versioned Web Nginx endpoint did not start")
                        time.sleep(0.2)
                for current in ("v1.0.0", "v1.1.0", "v1.0.0"):
                    subprocess.run(["docker", "exec", container, "ln", "-sfn", f"releases/{current}", "/opt/ainp/current"], check=True, capture_output=True)
                    for app in ("", "bigscreen/"):
                        self.assertIn(current, fetch("/" + app)[1])
                        for old in ("v1.0.0", "v1.1.0"):
                            prefix = f"/__web/{old}/{app}"
                            self.assertEqual(fetch(prefix + "index.html")[0], 200)
                            self.assertIn(old, fetch(prefix)[1])
                            self.assertEqual(fetch(prefix + f"assets/app-{old}.js"), (200, f"{old}-{app}-chunk"))
                        self.assertEqual(fetch("/" + app + f"assets/app-{current}.js"), (200, f"{current}-{app}-chunk"))
                self.assertEqual(fetch("/assets/app-v1.0.0.js"), (200, "v1.0.0--chunk"))
                self.assertEqual(fetch("/assets/app-v1.1.0.js")[0], 404)
                self.assertEqual(fetch("/bigscreen/assets/app-v1.0.0.js"), (200, "v1.0.0-bigscreen/-chunk"))
                self.assertEqual(fetch("/__web/v1.2.0/assets/absent.js")[0], 404)
                self.assertEqual(fetch("/bigscreen/assets/absent.js")[0], 404)
                # The same release remains addressable when built as a single Web app.
                (base / "default.conf").write_text(host.nginx(ContractTest().config(), Path("/opt/ainp")))
                subprocess.run(["docker", "exec", container, "nginx", "-s", "reload"], check=True, capture_output=True)
                for attempt in range(20):
                    if fetch("/__web/v1.0.0/assets/app-v1.0.0.js") == (200, "v1.0.0--chunk"):
                        break
                    time.sleep(0.1)
                else:
                    self.fail("single-Web versioned assets unavailable after reload")
                self.assertIn("v1.0.0", fetch("/")[1])
            finally:
                subprocess.run(["docker", "stop", container], capture_output=True)

    def test_https_websocket_upgrade_echo_reconnect_and_rest(self):
        import http.server
        import shutil
        import socket
        import ssl
        import subprocess
        import threading
        import time
        import uuid

        if not shutil.which("docker") or not shutil.which("openssl"):
            self.skipTest("Docker/OpenSSL unavailable for HTTPS WebSocket test")
        if subprocess.run(["docker", "info"], capture_output=True).returncode:
            self.skipTest("Docker daemon unavailable for HTTPS WebSocket test")

        seen = []

        class API(http.server.BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *args):
                pass

            def do_GET(self):
                seen.append((self.path, self.headers.get("Upgrade"), self.headers.get("Connection"),
                             self.headers.get("Host"), self.headers.get("Origin"), self.headers.get("X-Forwarded-Proto")))
                if self.headers.get("Upgrade", "").lower() != "websocket":
                    self.send_response(200)
                    self.send_header("Content-Length", "2")
                    self.end_headers()
                    self.wfile.write(b"ok")
                    return
                self.send_response(101, "Switching Protocols")
                self.send_header("Upgrade", "websocket")
                self.send_header("Connection", "Upgrade")
                self.end_headers()
                # Read a masked browser frame, then echo its unmasked text back.
                head = self.rfile.read(2)
                length = head[1] & 127
                mask = self.rfile.read(4)
                data = self.rfile.read(length)
                payload = bytes(byte ^ mask[i % 4] for i, byte in enumerate(data))
                self.wfile.write(bytes([0x81, len(payload)]) + payload)
                self.wfile.flush()
                self.close_connection = True

        server = http.server.ThreadingHTTPServer(("0.0.0.0", 0), API)
        server.daemon_threads = True
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        container = "ainp-ws-test-" + uuid.uuid4().hex[:12]
        try:
            with tempfile.TemporaryDirectory() as d:
                d = Path(d)
                subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=admin.example.com", "-keyout", str(d / "privkey.pem"), "-out", str(d / "fullchain.pem")], check=True, capture_output=True)
                config = ContractTest().config()
                upstream = f"http://127.0.0.1:{config['api_port']}"
                conf = host.nginx(config, Path("/opt/ainp")).replace(upstream, f"http://host.docker.internal:{server.server_port}")
                (d / "default.conf").write_text(conf)
                with socket.socket() as probe:
                    probe.bind(("127.0.0.1", 0))
                    port = probe.getsockname()[1]
                result = subprocess.run(["docker", "run", "-d", "--rm", "--name", container,
                    "--add-host", "host.docker.internal:host-gateway", "-p", f"127.0.0.1:{port}:443",
                    "-v", f"{d / 'default.conf'}:/etc/nginx/conf.d/default.conf:ro",
                    "-v", f"{d}:/etc/ainp/tls:ro", "nginx:1.27.5-alpine"], capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                ctx = ssl.create_default_context(cafile=str(d / "fullchain.pem"))

                def connect():
                    raw = socket.create_connection(("127.0.0.1", port), timeout=3)
                    try:
                        return ctx.wrap_socket(raw, server_hostname="admin.example.com")
                    except BaseException:
                        raw.close()
                        raise

                for attempt in range(40):
                    try:
                        with connect():
                            break
                    except OSError:
                        if attempt == 39:
                            self.fail("Nginx TLS endpoint did not start")
                        time.sleep(0.2)

                def read_headers(sock):
                    data = b""
                    while b"\r\n\r\n" not in data:
                        data += sock.recv(4096)
                    return data

                for path in ("/api/v1/bigscreen/events", "/api/v1/ai/events", "/api/v1/bigscreen/events"):
                    with connect() as sock:
                        sock.sendall((f"GET {path} HTTP/1.1\r\nHost: admin.example.com\r\nOrigin: https://admin.example.com\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n").encode())
                        self.assertIn(b"101 Switching Protocols", read_headers(sock))
                        mask = b"abcd"
                        message = b"hello"
                        sock.sendall(b"\x81\x85" + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(message)))
                        echoed = b""
                        while len(echoed) < 7:
                            echoed += sock.recv(7 - len(echoed))
                        self.assertEqual(echoed, b"\x81\x05hello")
                with connect() as sock:
                    sock.sendall(b"GET /api/v1/bigscreen/events HTTP/1.1\r\nHost: admin.example.com\r\nConnection: keep-alive\r\n\r\n")
                    self.assertIn(b"200 OK", read_headers(sock))
                self.assertEqual(len(seen), 4)
                for path, upgrade, connection, hostname, origin, proto in seen[:3]:
                    self.assertEqual((upgrade, connection, hostname, origin, proto),
                                     ("websocket", "upgrade", "admin.example.com", "https://admin.example.com", "https"))
                self.assertEqual(seen[3][1:3], (None, None))
        finally:
            subprocess.run(["docker", "stop", container], capture_output=True)
            server.shutdown()
            server.server_close()
            thread.join()

    def test_generated_live_and_maintenance_config_with_real_nginx(self):
        import shutil
        import subprocess
        if not shutil.which("docker") or not shutil.which("openssl"):
            self.skipTest("Docker/OpenSSL unavailable for Nginx integration")
        check = subprocess.run(["docker", "info"], capture_output=True)
        if check.returncode:
            self.skipTest("Docker daemon unavailable for Nginx integration")
        with tempfile.TemporaryDirectory() as d:
            d = Path(d)
            subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "10", "-subj", "/CN=admin.example.com", "-keyout", str(d / "privkey.pem"), "-out", str(d / "fullchain.pem")], check=True, capture_output=True)
            for maintenance in (False, True):
                config = d / "default.conf"
                config.write_text(host.nginx(ContractTest().config(), Path("/opt/ainp"), maintenance))
                result = subprocess.run(["docker", "run", "--rm", "-v", f"{config}:/etc/nginx/conf.d/default.conf:ro", "-v", f"{d}:/etc/ainp/tls:ro", "nginx:1.27.5-alpine", "nginx", "-t"], capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr + result.stdout)



class HostTransactionTest(unittest.TestCase):
    """Real filesystem + mocked process/cloud boundaries, including failure injection."""
    def setUp(self):
        import types
        import shutil
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.fs = Path(self.tmp.name).resolve()
        self.root, self.home = self.fs / "opt/ainp", self.fs / "var/lib/ainp"
        self.root.mkdir(parents=True)
        (self.fs / "etc/systemd/system").mkdir(parents=True)
        self.config = ContractTest().config()
        self.config["tls"] = {"fullchain_pem": "test-cert", "private_key_pem": "test-key"}
        self.config_source = self.fs / "config.json"
        self.config_source.write_text(json.dumps(self.config))
        self.events = []
        self.fail_migration = False
        self.account_exists = False
        self.archives = {}
        def mapped_path(value):
            value = str(value)
            if value.startswith(("/etc/", "/proc/", "/var/log/")):
                return self.fs / value.lstrip("/")
            return Path(value)
        def account(_):
            if not self.account_exists:
                raise KeyError("missing")
            return types.SimpleNamespace(pw_uid=1001, pw_gid=1001, pw_dir=str(self.home), pw_shell="/usr/sbin/nologin")
        def download(config, key, destination, sha, role):
            shutil.copyfile(self.config_source if key == "config.json" else self.archives[key], destination)
            common.require(common.sha256(destination) == sha, "mock download checksum mismatch")
        def command(argv, **kw):
            self.events.append(tuple(argv))
            if argv[0] == "useradd":
                self.account_exists = True
            if argv[0] == "runuser" and self.fail_migration:
                raise common.DeployError("injected migration failure")
            if argv[:2] == ["systemctl", "show"]:
                if "NRestarts" in argv:
                    return b"0\n"
                pid = {f"{self.config['project']}-api.service": 901, f"{self.config['project']}-worker.service": 902, f"{self.config['project']}-ai-worker.service": 903}[argv[2]]
                exe = self.fs / f"proc/{pid}/exe"
                exe.parent.mkdir(parents=True, exist_ok=True)
                exe.unlink(missing_ok=True)
                exe.symlink_to(self.root / "current/bin" / ("api" if pid == 901 else "worker"))
                return str(pid).encode()
            if argv[0] == "curl":
                identity = common.read_json(self.root / "current/release.json")
                if argv[-1].endswith("/healthz"):
                    return json.dumps({"data": {**identity, "status": "ok"}}).encode()
                return json.dumps(identity).encode()
            return b""
        def json_api(url, data=None):
            if url.endswith("/status"):
                return {"installed": False}
            self.events.append(("initialize",))
            self.assertFalse(data["embedded_worker"])
            common.atomic_json(self.home / "state/state.yaml", {"installed": True})
            return {"installed": True}
        def backup(*args):
            self.events.append(("backup",))
            return {"key": "backup.dump", "sha256": "0" * 64}
        def nginx(*args):
            self.events.append(("nginx-maintenance" if args[-1] else "nginx-live",))
        patches = [mock.patch.object(host, "Path", side_effect=mapped_path), mock.patch.object(host.pwd, "getpwnam", side_effect=account), mock.patch.object(host.os, "chown"), mock.patch.object(host, "download", side_effect=download), mock.patch.object(host, "command", side_effect=command), mock.patch.object(host, "db_preflight"), mock.patch.object(host, "redis_preflight"), mock.patch.object(host, "backup", side_effect=backup), mock.patch.object(host, "install_nginx", side_effect=nginx), mock.patch.object(host, "get_json", side_effect=json_api), mock.patch.object(host, "wait_api"), mock.patch.object(host.time, "sleep"), mock.patch.object(host.subprocess, "run"), mock.patch.dict(sys.modules, {"yaml": types.SimpleNamespace(safe_load=json.loads)})]
        for patch in patches:
            patch.start()
            self.addCleanup(patch.stop)

    def request(self, version, action):
        stage = self.fs / (version + "-stage")
        stage.mkdir()
        for filename in ("bin/api", "bin/worker", "bin/migrate", "web/index.html", "web/build.json"):
            p = stage / filename
            p.parent.mkdir(exist_ok=True)
            p.write_text(version)
        identity = dict(schema=1, project=self.config["project"], version=version, commit="b" * 40, platform="ubuntu-22.04-amd64", build_time="2026-09-24T00:00:00Z")
        archive = self.fs / (version + ".tar.gz")
        package.pack(stage, archive, identity, 1)
        self.archives[version] = archive
        return dict(project=self.config["project"], ram_role="runtime", action=action, operation_id=action + version, config_oss={}, config_key="config.json", config_sha256=common.sha256(self.config_source), release={**identity, "sha256": common.sha256(archive), "key": version, "oss": {}}, confirm_schema_compatible=True)

    def test_aguo_tenant_change_is_valid_and_other_changes_rejected(self):
        baseline = ContractTest().config()
        baseline["project"] = "aguo-ai-dining"
        host.validate_config(baseline, "aguo-ai-dining")
        changed = json.loads(json.dumps(baseline))
        changed["env"]["AGUO_DEPLOYMENT_TENANT_ID"] = "15c44f83-ebfb-44a0-8294-55b35689148d"
        host.validate_config(changed, "aguo-ai-dining")
        host.require_tenant_only_change(baseline, changed)
        changed["redis_db"] = 1
        with self.assertRaisesRegex(common.DeployError, "may only change"):
            host.require_tenant_only_change(baseline, changed)

    def test_aguo_tenant_update_persists_and_rollback_reuses_config(self):
        self.config["project"] = "aguo-ai-dining"
        self.config_source.write_text(json.dumps(self.config))
        initial = self.request("v1.0.0", "init")
        host.perform(initial, self.root, self.home)
        previous = common.read_json(self.fs / "etc/aguo-ai-dining/private.json")
        changed = json.loads(json.dumps(self.config))
        changed["env"]["AGUO_DEPLOYMENT_TENANT_ID"] = "15c44f83-ebfb-44a0-8294-55b35689148d"
        self.config_source.write_text(json.dumps(changed))
        update = self.request("v1.1.0", "update")
        update["config_sha256"] = "0" * 64
        with self.assertRaisesRegex(common.DeployError, "checksum mismatch"):
            host.perform(update, self.root, self.home)
        self.assertEqual(common.read_json(self.fs / "etc/aguo-ai-dining/private.json"), previous)
        update["config_sha256"] = common.sha256(self.config_source)
        host.perform(update, self.root, self.home)
        self.assertEqual(common.read_json(self.fs / "etc/aguo-ai-dining/runtime.json")["env"]["AGUO_DEPLOYMENT_TENANT_ID"], changed["env"]["AGUO_DEPLOYMENT_TENANT_ID"])
        self.assertEqual(common.read_json(self.fs / "etc/aguo-ai-dining/private.previous.json"), previous)
        rollback = dict(initial, action="rollback", operation_id="rollback-v1.0.0")
        host.perform(rollback, self.root, self.home)
        self.assertEqual(common.read_json(self.fs / "etc/aguo-ai-dining/runtime.json")["env"]["AGUO_DEPLOYMENT_TENANT_ID"], changed["env"]["AGUO_DEPLOYMENT_TENANT_ID"])

    def test_update_applies_only_tenant_environment_change(self):
        initial = self.request("v1.0.0", "init")
        host.perform(initial, self.root, self.home)
        old = common.read_json(self.fs / "etc/ainp/private.json")
        self.config_source = self.fs / "config-v2.json"
        changed = json.loads(json.dumps(self.config))
        changed["env"]["AGUO_DEPLOYMENT_TENANT_ID"] = "15c44f83-ebfb-44a0-8294-55b35689148d"
        self.config_source.write_text(json.dumps(changed))
        update = self.request("v1.1.0", "update")
        with self.assertRaisesRegex(common.DeployError, "invalid AGUO_DEPLOYMENT_TENANT_ID"):
            host.perform(update, self.root, self.home)
        self.assertEqual(common.read_json(self.fs / "etc/ainp/private.json"), old)
        changed["env"].pop("AGUO_DEPLOYMENT_TENANT_ID")
        changed["env"]["JWT_SECRET"] = "changed-" + changed["env"]["JWT_SECRET"]
        self.config_source.write_text(json.dumps(changed))
        update["config_sha256"] = common.sha256(self.config_source)
        with self.assertRaisesRegex(common.DeployError, "may only change"):
            host.perform(update, self.root, self.home)
        self.assertEqual(common.read_json(self.fs / "etc/ainp/private.json"), old)

    def test_init_update_rollback_order_and_persistent_state(self):
        initial = self.request("v1.0.0", "init")
        host.perform(initial, self.root, self.home)
        self.assertIn(("initialize",), self.events)
        self.assertEqual(common.read_json(self.root / "deployment.json")["status"], "healthy")
        self.events.clear()
        update = self.request("v1.1.0", "update")
        host.perform(update, self.root, self.home)
        self.assertNotIn(("initialize",), self.events)
        backup_index = self.events.index(("backup",))
        stop_index = next(i for i, e in enumerate(self.events) if e[:2] == ("systemctl", "stop"))
        migration_index = next(i for i, e in enumerate(self.events) if e[0] == "runuser")
        self.assertLess(stop_index, backup_index)
        self.assertLess(backup_index, migration_index)
        self.assertEqual(common.read_json(self.root / "deployment.json")["release"]["version"], "v1.1.0")
        self.events.clear()
        rollback = {**initial, "action": "rollback", "operation_id": "rollback-1"}
        host.perform(rollback, self.root, self.home)
        self.assertFalse(any(e[0] == "runuser" for e in self.events))
        self.assertIn(("backup",), self.events)
        self.assertEqual(common.read_json(self.root / "deployment.json")["release"]["version"], "v1.0.0")

    def test_web_version_links_rollbacks_and_bounded_retention(self):
        self.config["web_keep_releases"] = 2
        self.config["web_pinned_versions"] = ["v1.0.0"]
        self.config_source.write_text(json.dumps(self.config))
        requests = {}
        for version, action in (("v1.0.0", "init"), ("v1.1.0", "update"),
                                ("v1.2.0", "update"), ("v1.3.0", "update")):
            request = requests[version] = self.request(version, action)
            host.perform(request, self.root, self.home)
            self.assertEqual((self.root / "web-versions" / version).resolve(),
                             (self.root / "current/web").resolve())
        versions = self.root / "web-versions"
        self.assertEqual(versions.stat().st_mode & 0o777, 0o755)
        self.assertTrue((versions / "v1.0.0").is_symlink())
        self.assertFalse((versions / "v1.1.0").exists())
        self.assertTrue((versions / "v1.2.0").is_symlink())
        self.assertTrue((versions / "v1.3.0").is_symlink())
        state = common.read_json(self.root / "deployment.json")
        self.assertEqual(len(state["history"]), 3)
        with self.assertRaises(common.DeployError):
            host.perform({**requests["v1.1.0"], "action": "rollback", "operation_id": "rollback-old"}, self.root, self.home)
        host.perform({**requests["v1.0.0"], "action": "rollback", "operation_id": "rollback-pinned"}, self.root, self.home)
        self.assertEqual(common.read_json(self.root / "deployment.json")["release"]["version"], "v1.0.0")
        self.assertTrue((versions / "v1.3.0").is_symlink())

    def test_versioned_web_is_checked_before_deployment_becomes_healthy(self):
        request = self.request("v1.0.0", "init")
        with mock.patch.object(host, "https_json", wraps=host.https_json) as probe:
            host.perform(request, self.root, self.home)
        self.assertIn("/__web/v1.0.0/build.json", [call.args[1] for call in probe.call_args_list])
        self.assertEqual(common.read_json(self.root / "deployment.json")["status"], "healthy")

    def test_inaccessible_versioned_web_fails_deployment(self):
        request = self.request("v1.0.0", "init")
        original = host.https_json
        def blocked(c, path):
            if path.startswith("/__web/"):
                raise common.DeployError("versioned resource returned 403")
            return original(c, path)
        with mock.patch.object(host, "https_json", side_effect=blocked):
            with self.assertRaises(common.DeployError):
                host.perform(request, self.root, self.home)
        self.assertEqual(common.read_json(self.root / "deployment.json")["status"], "failed-maintenance")
        self.assertIn(("nginx-maintenance",), self.events)

    def test_web_versions_directory_is_publicly_traversable_under_private_umask(self):
        request = self.request("v1.0.0", "init")
        old_umask = os.umask(0o077)
        try:
            host.perform(request, self.root, self.home)
        finally:
            os.umask(old_umask)
        self.assertEqual((self.root / "web-versions").stat().st_mode & 0o777, 0o755)

    def test_fresh_init_uses_same_nonzero_db_in_setup_and_app_config(self):
        self.config["redis_db"] = 3
        self.config_source.write_text(json.dumps(self.config))
        request = self.request("v1.0.0", "init")
        captured = []
        original = host.get_json
        def initialize(url, data=None):
            if url.endswith("/initialize"):
                captured.append(data["redis_db"])
            return original(url, data)
        with mock.patch.object(host, "get_json", side_effect=initialize):
            host.perform(request, self.root, self.home)
        self.assertEqual(captured, [3])
        app = common.read_json(self.fs / "etc/ainp/app.json")
        self.assertEqual(app["redis"]["db"], 3)
        runtime = common.read_json(self.fs / "etc/ainp/runtime.json")
        self.assertNotIn("REDIS_DB", runtime["env"])
        host.perform(self.request("v1.1.0", "update"), self.root, self.home)
        self.assertEqual(common.read_json(self.fs / "etc/ainp/app.json")["redis"]["db"], 3)

    def test_fresh_init_does_not_reset_unloaded_systemd_units(self):
        original = host.command
        def reject_unloaded_reset(argv, **kwargs):
            if argv[:2] == ["systemctl", "reset-failed"]:
                raise common.DeployError("Unit not loaded")
            return original(argv, **kwargs)
        with mock.patch.object(host, "command", side_effect=reject_unloaded_reset):
            host.perform(self.request("v1.0.0", "init"), self.root, self.home)
        self.assertEqual(common.read_json(self.root / "deployment.json")["status"], "healthy")

    def test_update_and_rollback_reset_only_loaded_project_units(self):
        import fnmatch
        initial = self.request("v1.0.0", "init")
        host.perform(initial, self.root, self.home)
        update = self.request("v1.1.0", "update")
        rollback = {**initial, "action": "rollback", "operation_id": "rollback-reset"}
        original = host.command
        own = {"ainp-api.service", "ainp-worker.service", "ainp-ai-worker.service"}
        unrelated = {"other-api.service", "ainp-api-helper.service", "ainp-extra.service"}
        # Exercise fully unloaded, partially retained and fully retained services.
        for retained in (set(), {"ainp-worker.service"}, own):
            loaded = retained | unrelated
            reset = set()
            def systemctl(argv, **kwargs):
                if argv[:2] == ["systemctl", "reset-failed"]:
                    for pattern in argv[2:]:
                        if not any(c in pattern for c in "*?[") and pattern not in loaded:
                            raise common.DeployError("Unit not loaded")
                        reset.update(u for u in loaded if fnmatch.fnmatchcase(u, pattern))
                return original(argv, **kwargs)
            for request in (update, rollback):
                with self.subTest(retained=sorted(retained), action=request["action"]):
                    reset.clear()
                    self.events.clear()
                    with mock.patch.object(host, "command", side_effect=systemctl):
                        host.perform(request, self.root, self.home)
                    self.assertEqual(reset, retained)
                    self.assertIn(("backup",), self.events)
                    self.assertEqual(common.read_json(self.root / "deployment.json")["status"], "healthy")

    def test_reset_failure_retains_maintenance_before_backup_and_migration(self):
        initial = self.request("v1.0.0", "init")
        host.perform(initial, self.root, self.home)
        previous = (self.root / "current").resolve()
        original = host.command
        def failed_reset(argv, **kwargs):
            if argv[:2] == ["systemctl", "reset-failed"]:
                raise common.DeployError("systemd bus unavailable")
            return original(argv, **kwargs)
        update = self.request("v1.1.0", "update")
        rollback = {**initial, "action": "rollback", "operation_id": "rollback-reset-failure"}
        for request in (update, rollback):
            with self.subTest(action=request["action"]):
                self.events.clear()
                with mock.patch.object(host, "command", side_effect=failed_reset):
                    with self.assertRaisesRegex(common.DeployError, "maintenance retained"):
                        host.perform(request, self.root, self.home)
                self.assertEqual(common.read_json(self.root / "deployment.json")["status"], "failed-maintenance")
                self.assertEqual((self.root / "current").resolve(), previous)
                self.assertNotIn(("backup",), self.events)
                self.assertNotIn(("nginx-live",), self.events)
                self.assertFalse(any(e[0] == "runuser" or e[:2] == ("systemctl", "start") for e in self.events))

    def test_migration_failure_never_opens_traffic_or_switches_to_bad_release(self):
        host.perform(self.request("v1.0.0", "init"), self.root, self.home)
        previous = (self.root / "current").resolve()
        self.events.clear()
        self.fail_migration = True
        with self.assertRaisesRegex(common.DeployError, "maintenance retained"):
            host.perform(self.request("v1.1.0", "update"), self.root, self.home)
        state = common.read_json(self.root / "deployment.json")
        self.assertEqual(state["status"], "failed-maintenance")
        self.assertEqual(state["backup"]["key"], "backup.dump")
        self.assertEqual((self.root / "current").resolve(), previous)
        self.assertNotIn(("nginx-live",), self.events)
        self.assertFalse(any(e[:2] == ("systemctl", "start") for e in self.events))

    def test_failed_or_unknown_release_is_not_a_rollback_candidate(self):
        host.perform(self.request("v1.0.0", "init"), self.root, self.home)
        with self.assertRaisesRegex(common.DeployError, "not previously deployed"):
            host.perform(self.request("v0.9.0", "rollback"), self.root, self.home)



class StorageTest(unittest.TestCase):
    def test_metadata_requires_token_and_disables_proxies(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b"token"
        credentials = mock.MagicMock()
        credentials.__enter__.return_value.read.return_value = json.dumps(dict(Code="Success", AccessKeyId="id", AccessKeySecret="secret", SecurityToken="sts")).encode()
        opener = mock.Mock()
        opener.open.side_effect = [response, credentials]
        with mock.patch.object(storage.urllib.request, "build_opener", return_value=opener):
            self.assertEqual(storage.metadata_credentials("runtime-role"), ("id", "secret", "sts"))
        first = opener.open.call_args_list[0].args[0]
        second = opener.open.call_args_list[1].args[0]
        self.assertEqual(first.method, "PUT")
        self.assertIn("X-aliyun-ecs-metadata-token", second.headers)

    def test_metadata_has_no_v1_fallback(self):
        opener = mock.Mock()
        opener.open.side_effect = OSError("IMDS token failed")
        with mock.patch.object(storage.urllib.request, "build_opener", return_value=opener), self.assertRaises(OSError):
            storage.metadata_credentials("runtime-role")
        self.assertEqual(opener.open.call_count, 1)


class ReleaseVerificationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.directory = Path(self.tmp.name) / "dist-release"
        self.directory.mkdir()
        stage = Path(self.tmp.name) / "stage"
        stage.mkdir()
        for filename in ("bin/api", "bin/worker", "bin/migrate", "web/index.html", "web/build.json"):
            path = stage / filename
            path.parent.mkdir(exist_ok=True)
            path.write_text("fixture")
        identity = dict(schema=1, project="ainp", version="v0.0.0-ci", commit="a" * 40, build_time="2026-09-26T00:00:00Z", platform="ubuntu-22.04-amd64")
        archive = self.directory / "release.tar.gz"
        manifest = package.pack(stage, archive, identity, 1)
        common.atomic_json(self.directory / "release.json", manifest, 0o644)
        common.atomic_json(self.directory / "reference.json", {**identity, "sha256": common.sha256(archive)}, 0o644)
        (self.directory / "SHA256SUMS").write_text(f"{common.sha256(archive)}  release.tar.gz\n")
        self.identity = identity

    def test_verifies_ci_package_and_safe_extraction(self):
        reference = verify_release.verify_local(self.directory, "v0.0.0-ci", self.identity["commit"])
        self.assertEqual(reference["project"], "ainp")

    def test_rejects_unexpected_commit(self):
        with self.assertRaisesRegex(common.DeployError, "commit"):
            verify_release.verify_local(self.directory, "v0.0.0-ci", "b" * 40)

    def test_rejects_archive_checksum_mismatch(self):
        archive = self.directory / "release.tar.gz"
        archive.write_bytes(archive.read_bytes() + b"tampered")
        with self.assertRaisesRegex(common.DeployError, "checksum"):
            verify_release.verify_local(self.directory)


class OSSPreflightTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.directory = Path(self.tmp.name)
        self.objects = {}
        self.headers = {}
        self.allow_overwrite = False
        self.version_id = None
        self.denied = False

        class ServerError(Exception):
            def __init__(self, status):
                self.status = status

        self.ServerError = ServerError
        fake_sdk = mock.Mock()
        fake_sdk.exceptions.ServerError = ServerError
        patcher = mock.patch.dict(sys.modules, {"oss2": fake_sdk})
        patcher.start()
        self.addCleanup(patcher.stop)
        self.b = mock.Mock()
        self.b.put_object_from_file.side_effect = self.put
        self.b.head_object.side_effect = self.head
        self.b.get_object.side_effect = lambda key: io.BytesIO(self.objects[key])

    def put(self, key, path, headers):
        if self.denied:
            raise self.ServerError(403)
        if key in self.objects and not self.allow_overwrite:
            raise self.ServerError(409)
        self.objects[key] = Path(path).read_bytes()
        self.headers[key] = headers

    def head(self, key):
        headers = dict(self.headers[key])
        if self.version_id:
            headers["x-oss-version-id"] = self.version_id
        return mock.Mock(headers=headers)

    def test_private_object_roundtrip_retry_and_conflict(self):
        check_oss.check(self.b, "releases/ainp/_checks/test/probe.txt", self.directory)
        self.assertEqual(len(self.objects), 1)
        self.assertIn(b"not a release artifact", next(iter(self.objects.values())))
        self.assertEqual(self.b.put_object_from_file.call_count, 3)

    def test_bucket_ignoring_forbid_overwrite_fails(self):
        self.allow_overwrite = True
        with self.assertRaisesRegex(common.DeployError, "allowed an overwrite"):
            check_oss.check(self.b, "probe", self.directory)

    def test_versioned_bucket_stops_before_overwrite(self):
        self.version_id = "actual-version-id"
        with self.assertRaisesRegex(common.DeployError, "versioned bucket"):
            check_oss.check(self.b, "probe", self.directory)
        self.assertEqual(self.b.put_object_from_file.call_count, 1)

    def test_readback_corruption_fails(self):
        self.b.get_object.side_effect = lambda key: io.BytesIO(b"corrupt")
        with self.assertRaisesRegex(common.DeployError, "read-back mismatch"):
            check_oss.check(self.b, "probe", self.directory)

    def test_permission_error_is_not_overwrite_protection(self):
        self.denied = True
        with self.assertRaises(self.ServerError) as raised:
            check_oss.check(self.b, "probe", self.directory)
        self.assertEqual(raised.exception.status, 403)

    def test_default_plan_never_connects(self):
        with mock.patch.dict(os.environ, {"OSS_PREFIX": "releases"}), mock.patch.object(sys, "argv", ["check_oss.py"]), mock.patch.object(check_oss, "bucket") as connect, contextlib.redirect_stdout(io.StringIO()):
            check_oss.main()
        connect.assert_not_called()

    def test_project_traversal_rejected_before_connecting(self):
        with mock.patch.dict(os.environ, {"OSS_PREFIX": "releases"}), mock.patch.object(sys, "argv", ["check_oss.py", "--project", "../other", "--apply"]), mock.patch.object(check_oss, "bucket") as connect:
            with self.assertRaises(common.DeployError):
                check_oss.main()
        connect.assert_not_called()


if __name__ == "__main__":
    unittest.main()
