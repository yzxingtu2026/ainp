#!/usr/bin/env python3
"""Alibaba CLI controller: plan by default; persist identifiers before side effects."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import re
import subprocess
import time
import uuid
import zlib

from common import DeployError, atomic_json, digest, key, lock, name, oss_config, read_json, require, version

BOOTSTRAP = '''set -eu
umask 077
. /etc/os-release
test "$ID" = ubuntu && test "$VERSION_ID" = 22.04
test "$(uname -m)" = x86_64
export DEBIAN_FRONTEND=noninteractive
# Apt and venv bootstrap are serialized across projects as well.
exec 9>/var/lock/ainp-bootstrap.lock
flock -w 120 9
runtime=/opt/ainp-deploy-runtime-v1
if [ ! -f "$runtime/ready" ]; then
apt-get update -qq
apt-get install -y -qq python3-venv nginx curl ca-certificates gnupg tzdata libstdc++6
install -d -m 755 /usr/share/postgresql-common/pgdg
curl --fail --silent --show-error https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc
gpg --show-keys --with-colons /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc | grep -q 'B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8'
echo 'deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt jammy-pgdg main' >/etc/apt/sources.list.d/ainp-pgdg.list
chmod 644 /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc /etc/apt/sources.list.d/ainp-pgdg.list
apt-get update -qq
apt-get install -y -qq postgresql-client-17
python3 -m venv "$runtime"
"$runtime/bin/pip" --disable-pip-version-check install --quiet oss2==2.19.1 'psycopg[binary]==3.2.10' PyYAML==6.0.2 redis==6.4.0
touch "$runtime/ready"
fi
systemctl enable --now nginx
flock -u 9
work=$(mktemp -d /var/tmp/ainp-deploy.XXXXXXXX)
trap 'rm -rf "$work"' EXIT
python3 - "$work" <<'AINP_PAYLOAD'
import base64,json,pathlib,sys,zlib
files=json.loads(zlib.decompress(base64.b64decode(PAYLOAD_LITERAL)))
for filename, body in files.items():
    pathlib.Path(sys.argv[1], filename).write_text(body)
AINP_PAYLOAD
"$runtime/bin/python" "$work/host.py" "$work/request.json"
'''

TEST_ENVIRONMENT = "test"
CLI_ERROR_JSON_LIMIT = 64 * 1024
CLI_ERROR_TEXT_LIMIT = 500
REMOTE_OUTPUT_TAIL_LIMIT = 4000


def validate_release_for_environment(target, reference):
    """Allow CI artifacts only in the explicitly isolated test environment."""
    version(reference["version"])
    if reference["version"].endswith("-ci"):
        require(
            target["environment"] == TEST_ENVIRONMENT,
            "CI artifacts are allowed only in the test environment; production requires a stable release",
        )


def validate_target(c):
    require(c.get("schema") == 1, "unsupported target schema")
    name(c["project"])
    name(c["environment"])
    for field in ("region", "profile", "ram_role"):
        require(isinstance(c.get(field), str) and re.fullmatch(r"[A-Za-z0-9_.-]+", c[field]), f"invalid {field}")
    if c.get("release_oss_endpoint"):
        oss_config({"bucket": "validation-bucket", "endpoint": c["release_oss_endpoint"]})
    if c.get("instance_id"):
        require(re.fullmatch(r"i-[a-z0-9]+", c["instance_id"]), "invalid instance ID")
    else:
        template = c["launch_template"]
        require(re.fullmatch(r"lt-[a-z0-9]+", template["id"]), "invalid launch template")
        require(type(template["version"]) is int and template["version"] >= 1, "pin numeric launch template version, not Latest/Default")
    return c


def _diagnostic_text(value, limit=CLI_ERROR_TEXT_LIMIT):
    if not isinstance(value, (str, int)):
        return None
    text = re.sub(r"[\x00-\x1f\x7f]+", " ", str(value)).strip()
    if not text:
        return None
    return text[:limit]


def alibaba_failure(action, result):
    """Expose only bounded Alibaba error fields; never echo argv or raw output."""
    payload = None
    for raw in (result.stderr, result.stdout):
        if not raw or len(raw.encode()) > CLI_ERROR_JSON_LIMIT:
            continue
        try:
            candidate = json.loads(raw)
        except (json.JSONDecodeError, TypeError):
            continue
        if isinstance(candidate, dict):
            payload = candidate
            break

    fallback = f"Alibaba {action} failed (CLI exit {result.returncode}); response unavailable or unrecognized; inspect cloud audit"
    if payload is None:
        return fallback

    fields = (
        ("code", ("error_code", "Code", "code")),
        ("status", ("status_code", "HttpStatus", "statusCode")),
        ("message", ("message", "Message")),
        ("request_id", ("request_id", "RequestId", "requestId")),
    )
    diagnostics = []
    for label, keys in fields:
        value = next((_diagnostic_text(payload.get(key)) for key in keys if payload.get(key) is not None), None)
        if value:
            diagnostics.append(f"{label}={value}")
    if not diagnostics:
        return fallback
    return f"Alibaba {action} failed (CLI exit {result.returncode}): " + "; ".join(diagnostics)


def _remote_output_tail(value):
    if not isinstance(value, str) or not value.strip():
        return None
    text = re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", value)
    text = re.sub(r"[^\x09\x0a\x20-\x7e\u0080-\uffff]", " ", text)
    text = re.sub(r"(?i)\b(password|secret|token|authorization)(\s*[=:]\s*)([^\s]+)", r"\1\2[REDACTED]", text)
    text = re.sub(r"(https?://[^\s:/@]+:)([^\s@]+)(@)", r"\1[REDACTED]\3", text)
    text = text.strip()
    if not text:
        return None
    if len(text) > REMOTE_OUTPUT_TAIL_LIMIT:
        return "[output tail]\n" + text[-REMOTE_OUTPUT_TAIL_LIMIT:]
    return text


class Alibaba:
    def __init__(self, target):
        self.target = target

    def call(self, action, **parameters):
        argv = ["aliyun", "ecs", action, "--profile", self.target["profile"], "--RegionId", self.target["region"]]
        for k, v in parameters.items():
            argv += ["--" + k, str(v)]
        try:
            result = subprocess.run(argv, capture_output=True, text=True, timeout=120)
        except (OSError, subprocess.TimeoutExpired) as e:
            raise DeployError(f"Alibaba CLI {action} unavailable/timeout; inspect persisted state before retry") from e
        require(result.returncode == 0, alibaba_failure(action, result))
        return json.loads(result.stdout)


def bootstrap(request):
    files = {p: Path(__file__).with_name(p).read_text() for p in ("common.py", "storage.py", "host.py", "launcher.py")}
    files["request.json"] = json.dumps(request, sort_keys=True)
    payload = base64.b64encode(zlib.compress(json.dumps(files).encode(), 9)).decode()
    command = BOOTSTRAP.replace("PAYLOAD_LITERAL", repr(payload))
    require(len(command.encode()) <= 16384 and len(base64.b64encode(command.encode())) <= 24576, "bootstrap exceeds conservative 16KiB budget / API encoded limit; use a versioned bootstrap object")
    return command


def describe_instance(api, instance_id, target):
    instances = api.call("DescribeInstances", InstanceIds=json.dumps([instance_id]))["Instances"]["Instance"]
    require(len(instances) == 1 and instances[0]["InstanceId"] == instance_id, "target instance not found")
    instance = instances[0]
    tags = {t["TagKey"]: t["TagValue"] for t in instance.get("Tags", {}).get("Tag", [])}
    require(tags.get("ainp-project") == target["project"] and tags.get("ainp-environment") == target["environment"], "instance ownership tags do not match target")
    require(instance.get("InstanceChargeType") == "PostPaid", "baseline only manages postpaid ECS")
    return instance


def invocation_row(payload, instance_id):
    rows = payload.get("Invocation", {}).get("InvocationResults", {}).get("InvocationResult", [])
    rows = [r for r in rows if r.get("InstanceId") == instance_id]
    require(len(rows) <= 1, "ambiguous invocation result")
    return rows[0] if rows else None


def invocation_result(payload, instance_id):
    row = invocation_row(payload, instance_id)
    if row is None:
        return "pending"
    status = row.get("InvocationStatus")
    if status == "Success":
        require(row.get("ExitCode") == 0 and row.get("InvokeRecordStatus") == "Finished", "inconsistent command result")
        return "success"
    if status in ("Failed", "Error", "Timeout", "Cancelled", "Stopped", "Invalid", "PartialFailed", "Aborted", "Terminated") or row.get("InvokeRecordStatus") in ("Failed", "Stopped", "Terminated"):
        return "failed"
    return "pending"


def invocation_failure(payload, instance_id):
    row = invocation_row(payload, instance_id)
    require(row is not None, "remote deployment failed without an invocation result; inspect Cloud Assistant audit")
    fields = (
        ("status", row.get("InvocationStatus")),
        ("record_status", row.get("InvokeRecordStatus")),
        ("exit_code", row.get("ExitCode")),
        ("error_code", row.get("ErrorCode")),
        ("error_info", row.get("ErrorInfo")),
    )
    diagnostics = [f"{label}={value}" for label, raw in fields if (value := _diagnostic_text(raw))]
    message = "remote deployment failed"
    if diagnostics:
        message += ": " + "; ".join(diagnostics)
    if output := _remote_output_tail(row.get("Output")):
        message += "\n--- Cloud Assistant output tail ---\n" + output
    return message


def poll(api, state, statepath, timeout=2100):
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        payload = api.call("DescribeInvocationResults", InvokeId=state["invoke_id"], InstanceId=state["instance_id"], ContentEncoding="PlainText")
        status = invocation_result(payload, state["instance_id"])
        if status != "pending":
            state["status"] = status
            atomic_json(statepath, state)
            require(status == "success", invocation_failure(payload, state["instance_id"]))
            return
        time.sleep(5)
    raise DeployError("remote command still pending/unknown; run status, never assume cancellation or dispatch another update")


def execute(target, reference, action, statepath, confirm=False, status_only=False):
    fingerprint = hashlib.sha256(json.dumps(target, sort_keys=True).encode()).hexdigest()
    with lock(str(statepath) + ".lock"):
        state = read_json(statepath) if statepath.exists() else {}
        require(not state or state["target_sha256"] == fingerprint, "target differs from persisted binding; use reviewed new environment state")
        api = Alibaba(target)
        if status_only:
            require(state.get("invoke_id"), "no recorded invocation; inspect creation/dispatch audit, do not create again")
            poll(api, state, statepath, timeout=10)
            return
        if state.get("status") in ("dispatching", "creating"):
            raise DeployError("previous request outcome unknown; reconcile cloud audit and saved client token before any retry")
        if state.get("status") == "running":
            raise DeployError("invocation still running; use status instead of resubmitting")
        require(action == "init" or state.get("instance_id") or target.get("instance_id"), "update/rollback needs an existing instance")
        require(not (action == "init" and state.get("status") in ("success", "failed")), "init already attempted; inspect instance and recovery SOP")
        instance_id = state.get("instance_id") or target.get("instance_id")
        state = {"target_sha256": fingerprint, "client_token": str(uuid.uuid4()), "operation_id": str(uuid.uuid4()), "action": action, "status": "preparing", "instance_id": instance_id}
        request = dict(project=target["project"], ram_role=target["ram_role"], action=action, operation_id=state["operation_id"], release=reference, confirm_schema_compatible=confirm)
        if target.get("release_oss_endpoint"):
            request["release"] = {**reference, "oss": {**reference["oss"], "endpoint": target["release_oss_endpoint"]}}
        if action == "init":
            for field in ("config_oss", "config_key", "config_sha256"):
                request[field] = target[field]
            oss_config(request["config_oss"])
            key(request["config_key"])
            digest(request["config_sha256"])
        oss_config(reference["oss"])
        script = bootstrap(request)
        atomic_json(statepath, state)
        if not instance_id:
            require(action == "init", "only init can create an instance")
            template = target["launch_template"]
            state["status"] = "creating"
            atomic_json(statepath, state)
            result = api.call("RunInstances", LaunchTemplateId=template["id"], LaunchTemplateVersion=template["version"], InstanceChargeType="PostPaid", Amount=1, ClientToken=state["client_token"], RamRoleName=target["ram_role"], HttpTokens="required", HttpEndpoint="enabled", **{"Tag.1.Key": "ainp-project", "Tag.1.Value": target["project"], "Tag.2.Key": "ainp-environment", "Tag.2.Value": target["environment"]})
            ids = result["InstanceIdSets"]["InstanceIdSet"]
            require(len(ids) == 1, "unexpected instance creation result; inspect cloud audit")
            instance_id = state["instance_id"] = ids[0]
            state["status"] = "preparing"
            atomic_json(statepath, state)
        for _ in range(120):
            instance = describe_instance(api, instance_id, target)
            if instance.get("Status") == "Running":
                break
            time.sleep(5)
        else:
            raise DeployError("ECS did not reach Running; existing instance retained in state")
        for _ in range(60):
            assistants = api.call("DescribeCloudAssistantStatus", **{"InstanceId.1": instance_id})
            entries = assistants.get("InstanceCloudAssistantStatusSet", {}).get("InstanceCloudAssistantStatus", [])
            if any(e.get("InstanceId") == instance_id and e.get("CloudAssistantStatus") == "true" for e in entries):
                break
            time.sleep(5)
        else:
            raise DeployError("Cloud Assistant not online; install/repair agent in the launch template; no command dispatched")
        state.update(status="dispatching", release=reference, command_sha256=hashlib.sha256(script.encode()).hexdigest())
        atomic_json(statepath, state)
        result = api.call("RunCommand", Type="RunShellScript", ContentEncoding="PlainText", CommandContent=script, Timeout=1800, ClientToken=state["operation_id"], KeepCommand="false", TerminationMode="ProcessTree", **{"InstanceId.1": instance_id})
        state.update(invoke_id=result["InvokeId"], status="running")
        atomic_json(statepath, state)
        poll(api, state, statepath)
        print(f"{action} succeeded on {instance_id}; version {reference['version']}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=("init", "update", "rollback", "status"))
    parser.add_argument("--target", required=True, type=Path)
    parser.add_argument("--release", type=Path, help="verified reference.json from successful OSS release CI")
    parser.add_argument("--state-dir", type=Path, default=Path(".deploy-state"))
    parser.add_argument("--apply", action="store_true", help="allow billable ECS creation and remote environment modification")
    parser.add_argument("--confirm-schema-compatible", action="store_true")
    args = parser.parse_args()
    try:
        target = validate_target(read_json(args.target))
        reference = read_json(args.release) if args.release else None
        if args.action != "status":
            require(reference and reference["project"] == target["project"], "matching --release required")
            validate_release_for_environment(target, reference)
            digest(reference["sha256"])
            key(reference["key"])
            oss_config(reference["oss"])
            if args.action == "init":
                oss_config(target["config_oss"])
                key(target["config_key"])
                digest(target["config_sha256"])
            require(reference.get("platform") == "ubuntu-22.04-amd64", "unsupported platform")
            require(args.action != "rollback" or args.confirm_schema_compatible, "rollback requires --confirm-schema-compatible after reviewing migration compatibility")
        statepath = args.state_dir / f"{target['project']}-{target['environment']}.json"
        if args.action != "status" and not args.apply:
            print(json.dumps({"mode": "PLAN ONLY; no cloud calls", "action": args.action, "project": target["project"], "environment": target["environment"], "region": target["region"], "instance": target.get("instance_id", "create POSTPAID using pinned launch template, unless state already has instance"), "release": reference["version"], "state": str(statepath), "maintenance_window": True}, ensure_ascii=False, indent=2))
            return
        execute(target, reference, args.action, statepath, args.confirm_schema_compatible, args.action == "status")
    except (DeployError, KeyError, ValueError) as e:
        print(str(e) if isinstance(e, DeployError) else "invalid input/schema; review example configuration")
        raise SystemExit(1)


if __name__ == "__main__":
    main()
