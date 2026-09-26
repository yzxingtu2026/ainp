"""Offline release/controller/security contracts; never calls a cloud or production host."""
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
from unittest import mock

DEPLOY = Path(__file__).parent / "deployment"
sys.path.insert(0, str(DEPLOY))
import common
import cloud
import host
import launcher
import storage
import check_oss
import verify_release
spec = importlib.util.spec_from_file_location("native_package", DEPLOY / "package.py")
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)


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
        command = cloud.bootstrap(request)
        self.assertLessEqual(len(command.encode()), 16384)
        self.assertNotIn("PAYLOAD_LITERAL", command)
        self.assertNotIn("DATABASE_DSN=", command)
        self.assertLessEqual(len(base64.b64encode(command.encode())), 24576)
        self.assertIn("chmod 644 /usr/share/postgresql-common/pgdg", command)
        self.assertIn("mirrors.cloud.aliyuncs.com", command)
        self.assertIn("/etc/apt/sources.list", command)

    def test_bootstrap_retries_only_apt_lock_failures_with_bounded_timeout(self):
        command = cloud.bootstrap(dict(project="ainp", ram_role="ainp-production-runtime", action="init", operation_id="a" * 36, release=self.reference(), config_oss=self.target()["config_oss"], config_key=self.target()["config_key"], config_sha256="a" * 64))
        self.assertIn("AINP_APT_LOCK_TIMEOUT: waited %s seconds", command)
        self.assertIn("apt_get update -qq", command)
        self.assertIn("AINP_APT_LOCK_TIMEOUT", command)
        self.assertIn("locks remain; reconcile unattended-upgrades", command)
        self.assertNotIn("rm -f /var/lib/dpkg/lock", command)
        self.assertNotIn("kill ", command)

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
            with mock.patch.object(cloud.Alibaba, "call", side_effect=call), mock.patch.object(cloud, "poll"), mock.patch.object(cloud, "bootstrap", return_value="echo safe"), contextlib.redirect_stdout(io.StringIO()):
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
            with mock.patch.object(cloud.Alibaba, "call", side_effect=common.DeployError("timeout")) as call:
                with self.assertRaises(common.DeployError):
                    cloud.execute(self.target(), self.reference(), "init", path)
                self.assertEqual(common.read_json(path)["status"], "creating")
                self.assertTrue(common.read_json(path)["client_token"])
                with self.assertRaisesRegex(common.DeployError, "outcome unknown"):
                    cloud.execute(self.target(), self.reference(), "init", path)
                self.assertEqual(call.call_count, 1)

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
            with mock.patch.object(cloud.Alibaba, "call", side_effect=call), mock.patch.object(cloud, "poll"), mock.patch.object(cloud, "bootstrap", return_value="echo safe") as bootstrap, contextlib.redirect_stdout(io.StringIO()):
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


class NativeNginxSyntaxTest(unittest.TestCase):
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
                pid = {"ainp-api.service": 901, "ainp-worker.service": 902, "ainp-ai-worker.service": 903}[argv[2]]
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
        identity = dict(schema=1, project="ainp", version=version, commit="b" * 40, platform="ubuntu-22.04-amd64", build_time="2026-09-24T00:00:00Z")
        archive = self.fs / (version + ".tar.gz")
        package.pack(stage, archive, identity, 1)
        self.archives[version] = archive
        return dict(project="ainp", ram_role="runtime", action=action, operation_id=action + version, config_oss={}, config_key="config.json", config_sha256=common.sha256(self.config_source), release={**identity, "sha256": common.sha256(archive), "key": version, "oss": {}}, confirm_schema_compatible=True)

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
