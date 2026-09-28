#!/usr/bin/env python3
"""Alibaba CLI controller: plan by default; persist identifiers before side effects."""
import argparse
import base64
import hashlib
import io
import json
import os
import tempfile
from pathlib import Path
import re
import subprocess
import time
import tokenize
import uuid
import bz2

import release_reference

from common import DeployError, atomic_json, digest, key, lock, name, oss_config, read_json, require, version

BOOTSTRAP = r'''set -eu
umask 077
work=$(mktemp -d /var/tmp/ainp-deploy.XXXXXXXX)
trap 'rm -rf "$work"' EXIT
python3 - "$work" <<'AINP_PAYLOAD'
import base64,bz2,json,pathlib,sys
files=json.loads(bz2.decompress(base64.b85decode(PAYLOAD_LITERAL)))
for filename,body in files.items():
    pathlib.Path(sys.argv[1],filename).write_text(body)
AINP_PAYLOAD
sh "$work/bootstrap.sh" "$work"
'''

# Keep the Cloud Assistant command small. The instance RAM role downloads the
# immutable, release-scoped object; no access key or signed URL is in the audit log.
VERSIONED_BOOTSTRAP = r'''set -eu
umask 077
work=$(mktemp -d /var/tmp/ainp-deploy.XXXXXXXX)
trap 'rm -rf "$work"' EXIT
python3 - "$work" <<'AINP_BOOTSTRAP'
import base64,bz2,datetime,hashlib,hmac,json,pathlib,re,sys,urllib.parse,urllib.request
work=pathlib.Path(sys.argv[1])
endpoint=ENDPOINT
bucket=BUCKET
object_key=OBJECT_KEY
expected=EXPECTED_SHA256
role=RAM_ROLE
require=lambda ok,msg: (_ for _ in ()).throw(RuntimeError(msg)) if not ok else None
opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
token_request=urllib.request.Request("http://100.100.100.200/latest/api/token",method="PUT",headers={"X-aliyun-ecs-metadata-token-ttl-seconds":"21600"})
with opener.open(token_request,timeout=10) as response:
    metadata_token=response.read().decode()
credentials_request=urllib.request.Request("http://100.100.100.200/latest/meta-data/ram/security-credentials/"+role,headers={"X-aliyun-ecs-metadata-token":metadata_token})
with opener.open(credentials_request,timeout=10) as response:
    credentials=json.load(response)
require(credentials.get("Code")=="Success","instance RAM credentials unavailable")
access_key=credentials["AccessKeyId"]
secret=credentials["AccessKeySecret"]
sts_token=credentials["SecurityToken"]
encoded_key=urllib.parse.quote(object_key,safe="/~")
parsed=urllib.parse.urlsplit(endpoint)
host=bucket+"."+parsed.netloc
url=urllib.parse.urlunsplit((parsed.scheme,host,"/"+encoded_key,"",""))
date=datetime.datetime.now(datetime.timezone.utc).strftime("%a, %d %b %Y %H:%M:%S GMT")
string_to_sign="GET\n\n\n"+date+"\nx-oss-security-token:"+sts_token+"\n/"+bucket+"/"+object_key
signature=base64.b64encode(hmac.new(secret.encode(),string_to_sign.encode(),hashlib.sha1).digest()).decode()
request=urllib.request.Request(url,headers={"Date":date,"Host":host,"x-oss-security-token":sts_token,"Authorization":"OSS "+access_key+":"+signature})
tmp=work/"bundle"
with opener.open(request,timeout=120) as response, tmp.open("wb") as output:
    total=0
    while True:
        block=response.read(1024*1024)
        if not block: break
        total+=len(block)
        require(total<=2*1024**3,"bootstrap object exceeds download limit")
        output.write(block)
with tmp.open("rb") as source:
    actual=hashlib.sha256(source.read()).hexdigest()
require(actual==expected,"bootstrap object checksum mismatch")
with bz2.open(tmp,"rt",encoding="utf-8") as source:
    files=json.load(source)
require(isinstance(files,dict) and set(files),"invalid bootstrap object")
for filename,body in files.items():
    require(isinstance(filename,str) and re.fullmatch(r"[A-Za-z0-9_.-]+",filename),"invalid bootstrap filename")
    require(isinstance(body,str),"invalid bootstrap file")
    (work/filename).write_text(body)
AINP_BOOTSTRAP
sh "$work/bootstrap.sh" "$work"
'''


TEST_ENVIRONMENT = "test"
CLI_ERROR_JSON_LIMIT = 64 * 1024
CLI_ERROR_TEXT_LIMIT = 500
REMOTE_OUTPUT_TAIL_LIMIT = 4000
PROGRESS_INTERVAL_SECONDS = 15


def progress(message):
    """Emit a timestamped, flushed progress line without secrets or raw API payloads."""
    timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{timestamp}] {message}", flush=True)


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


def _host_release(reference, endpoint=None):
    """Keep the Cloud Assistant request to the fields consumed by host.py."""
    release = {field: reference[field] for field in ("project", "version", "commit", "platform", "sha256", "key", "oss")}
    if endpoint:
        release["oss"] = {**release["oss"], "endpoint": endpoint}
    return release


def _bootstrap_files(request):
    files = {p: Path(__file__).with_name(p).read_text() for p in ("common.py", "storage.py", "host.py", "launcher.py", "reporting.py", "bootstrap.sh", "apt.sh")}
    # Keep the legacy inline loader available for unit tests and local inspection.
    for filename, body in files.items():
        if filename.endswith(".py"):
            lines = body.splitlines(keepends=True)
            comments = (token for token in tokenize.generate_tokens(io.StringIO(body).readline)
                        if token.type == tokenize.COMMENT and not lines[token.start[0] - 1][:token.start[1]].strip())
            for token in comments:
                lines[token.start[0] - 1] = ""
            files[filename] = "".join(lines)
    files["request.json"] = json.dumps(request, sort_keys=True, separators=(",", ":"))
    return files


def bootstrap(request, bootstrap_object=None):
    if bootstrap_object is None:
        files = _bootstrap_files(request)
        payload = base64.b85encode(bz2.compress(json.dumps(files, separators=(",", ":")).encode(), compresslevel=9)).decode()
        command = BOOTSTRAP.replace("PAYLOAD_LITERAL", repr(payload)).rstrip("\n")
        require(len(command.encode()) <= 16384 and len(base64.b64encode(command.encode())) <= 24576, "bootstrap exceeds conservative 16KiB budget / API encoded limit; use a versioned bootstrap object")
        return command
    fields = ("endpoint", "bucket", "key", "sha256", "ram_role")
    require(all(isinstance(bootstrap_object.get(field), str) and bootstrap_object[field] for field in fields), "invalid versioned bootstrap object")
    oss_config({"endpoint": bootstrap_object["endpoint"], "bucket": bootstrap_object["bucket"]})
    key(bootstrap_object["key"])
    digest(bootstrap_object["sha256"])
    require(re.fullmatch(r"[A-Za-z0-9_.-]+", bootstrap_object["ram_role"]), "invalid bootstrap RAM role")
    replacements = {
        "ENDPOINT": repr(bootstrap_object["endpoint"]),
        "BUCKET": repr(bootstrap_object["bucket"]),
        "OBJECT_KEY": repr(bootstrap_object["key"]),
        "EXPECTED_SHA256": repr(bootstrap_object["sha256"]),
        "RAM_ROLE": repr(bootstrap_object["ram_role"]),
    }
    command = VERSIONED_BOOTSTRAP
    for name_, value in replacements.items():
        command = command.replace(name_, value)
    command = command.rstrip("\n")
    require(len(command.encode()) <= 16384 and len(base64.b64encode(command.encode())) <= 24576, "versioned bootstrap loader exceeds Cloud Assistant command limit")
    return command


def _upload_bootstrap_object(reference, target, request):
    files = _bootstrap_files(request)
    oss_config(reference["oss"])
    name(reference["project"])
    version(reference["version"])
    require(re.fullmatch(r"[0-9a-f]{40}", reference["commit"]), "invalid release commit")
    with tempfile.TemporaryDirectory(prefix="ainp-bootstrap-") as directory:
        path = Path(directory) / "bootstrap.bundle.bz2"
        with bz2.open(path, "wt", encoding="utf-8") as output:
            json.dump(files, output, separators=(",", ":"))
        expected = hashlib.sha256(path.read_bytes()).hexdigest()
        object_key = f"releases/{reference['project']}/{reference['version']}/{reference['commit']}/bootstrap/{request['operation_id']}-{expected}.bundle.bz2"
        endpoint = reference["oss"]["endpoint"]
        argv = ["aliyun", "oss", "cp", str(path), f"oss://{reference['oss']['bucket']}/{object_key}", "--profile", target["profile"], "--endpoint", endpoint, "--cli-non-interactive"]
        try:
            result = subprocess.run(argv, capture_output=True, text=True, timeout=300)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise DeployError("Alibaba OSS bootstrap upload unavailable/timeout; inspect persisted state before retry") from error
        require(result.returncode == 0, "Alibaba OSS bootstrap upload failed; inspect OSS audit and persisted state")
        downloaded = Path(directory) / "readback.bundle.bz2"
        argv = ["aliyun", "oss", "cp", f"oss://{reference['oss']['bucket']}/{object_key}", str(downloaded), "--profile", target["profile"], "--endpoint", endpoint, "--cli-non-interactive"]
        try:
            result = subprocess.run(argv, capture_output=True, text=True, timeout=300)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise DeployError("Alibaba OSS bootstrap readback unavailable/timeout; inspect persisted state before retry") from error
        require(result.returncode == 0 and downloaded.exists() and hashlib.sha256(downloaded.read_bytes()).hexdigest() == expected,
                "Alibaba OSS bootstrap readback checksum mismatch")
    return {"endpoint": target.get("release_oss_endpoint") or endpoint, "bucket": reference["oss"]["bucket"], "key": object_key, "sha256": expected, "ram_role": request["ram_role"]}


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
    started = time.monotonic()
    until = started + timeout
    next_report = 0
    seen_progress = set()
    while time.monotonic() < until:
        payload = api.call("DescribeInvocationResults", InvokeId=state["invoke_id"], InstanceId=state["instance_id"], ContentEncoding="PlainText")
        row = invocation_row(payload, state["instance_id"])
        # Only explicit safe progress records, never arbitrary subprocess output/config.
        output = row.get("Output", "") if row else ""
        for line in output.splitlines(keepends=True) if isinstance(output, str) else []:
            if line.endswith("\n") and re.fullmatch(r"AINP_PROGRESS [0-9TZ:.-]+ [A-Za-z0-9 ()_.:-]+\n", line) and line not in seen_progress:
                print(line.rstrip(), flush=True)
                seen_progress.add(line)
        status = invocation_result(payload, state["instance_id"])
        elapsed = int(time.monotonic() - started)
        if status != "pending":
            progress(f"Cloud Assistant invocation {state['invoke_id']}: {status} after {elapsed}s")
            state["status"] = status
            atomic_json(statepath, state)
            require(status == "success", invocation_failure(payload, state["instance_id"]))
            return
        if elapsed >= next_report:
            progress(f"Cloud Assistant invocation {state['invoke_id']} still running ({elapsed}s elapsed)")
            next_report = elapsed + PROGRESS_INTERVAL_SECONDS
        time.sleep(5)
    raise DeployError("remote command still pending/unknown; run status, never assume cancellation or dispatch another update")


def _release_identity(reference):
    """Fields that must remain identical when recovering a failed init."""
    return {field: reference.get(field) for field in ("project", "version", "commit", "platform", "sha256", "key")}


def _attempt_snapshot(state):
    fields = ("action", "status", "operation_id", "invoke_id", "command_sha256", "release")
    return {field: state[field] for field in fields if field in state}


def _validate_recovery_state(state, target, reference, action):
    """Fail closed before dispatching a recovery command to an existing ECS."""
    if action != "init" or state.get("status") != "failed":
        return
    require(state.get("action") == "init" and state.get("instance_id"), "failed init recovery requires the persisted ECS instance; do not create another instance")
    require(state.get("invoke_id"), "failed init recovery requires the prior Cloud Assistant invocation audit")
    previous = state.get("release")
    require(isinstance(previous, dict), "failed init recovery is missing the persisted release identity")
    require(_release_identity(previous) == _release_identity(reference), "recovery release differs from the persisted failed init; use the exact same release SHA256")
    saved_config_sha256 = state.get("config_sha256")
    if saved_config_sha256 is not None:
        require(saved_config_sha256 == target.get("config_sha256"), "recovery configuration SHA256 differs from the persisted failed init")


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
        _validate_recovery_state(state, target, reference, action)
        require(action == "init" or state.get("instance_id") or target.get("instance_id"), "update/rollback needs an existing instance")
        require(not (action == "init" and state.get("status") == "success"), "init already succeeded; use update or inspect the host")
        if action == "init" and state.get("status") == "failed":
            require(state.get("instance_id"), "failed init recovery requires a persisted ECS instance; do not create another instance")
        instance_id = state.get("instance_id") or target.get("instance_id")
        attempts = list(state.get("attempts", []))
        if state.get("status") == "failed":
            attempts.append(_attempt_snapshot(state))
        state = {
            "target_sha256": fingerprint,
            "client_token": state.get("client_token") or str(uuid.uuid4()),
            "operation_id": str(uuid.uuid4()),
            "action": action,
            "status": "preparing",
            "instance_id": instance_id,
            "attempts": attempts,
        }
        if action == "init":
            state["config_sha256"] = target["config_sha256"]
        request = dict(project=target["project"], ram_role=target["ram_role"], action=action, operation_id=state["operation_id"], release=_host_release(reference, target.get("release_oss_endpoint")))
        if action == "rollback":
            request["confirm_schema_compatible"] = confirm
        if action == "init":
            for field in ("config_oss", "config_key", "config_sha256"):
                request[field] = target[field]
            oss_config(request["config_oss"])
            key(request["config_key"])
            digest(request["config_sha256"])
        oss_config(reference["oss"])
        # Persist the operation ID before uploading, so ambiguous OSS outcomes
        # remain auditable without dispatching or allocating an ECS.
        atomic_json(statepath, state)
        bootstrap_object = _upload_bootstrap_object(reference, target, request)
        script = bootstrap(request, bootstrap_object)
        state["bootstrap"] = bootstrap_object
        progress(f"versioned bootstrap prepared: {len(script.encode())} byte loader; persisting deployment state")
        atomic_json(statepath, state)
        if not instance_id:
            require(action == "init", "only init can create an instance")
            template = target["launch_template"]
            progress(f"creating ECS from launch template {template['id']} version {template['version']}")
            state["status"] = "creating"
            atomic_json(statepath, state)
            result = api.call("RunInstances", LaunchTemplateId=template["id"], LaunchTemplateVersion=template["version"], InstanceChargeType="PostPaid", Amount=1, ClientToken=state["client_token"], RamRoleName=target["ram_role"], HttpTokens="required", HttpEndpoint="enabled", **{"Tag.1.Key": "ainp-project", "Tag.1.Value": target["project"], "Tag.2.Key": "ainp-environment", "Tag.2.Value": target["environment"]})
            ids = result["InstanceIdSets"]["InstanceIdSet"]
            require(len(ids) == 1, "unexpected instance creation result; inspect cloud audit")
            instance_id = state["instance_id"] = ids[0]
            state["status"] = "preparing"
            atomic_json(statepath, state)
            progress(f"ECS created: {instance_id}; state persisted; waiting for Running")
        else:
            progress(f"using existing ECS: {instance_id}; waiting for Running")
        instance_started = time.monotonic()
        last_instance_status = None
        for attempt in range(120):
            instance = describe_instance(api, instance_id, target)
            instance_status = instance.get("Status", "unknown")
            if attempt == 0 or instance_status != last_instance_status or attempt % 3 == 0:
                progress(f"ECS {instance_id}: status={instance_status}; waited {int(time.monotonic() - instance_started)}s")
            last_instance_status = instance_status
            if instance_status == "Running":
                break
            time.sleep(5)
        else:
            raise DeployError("ECS did not reach Running; existing instance retained in state")
        assistant_started = time.monotonic()
        for attempt in range(60):
            assistants = api.call("DescribeCloudAssistantStatus", **{"InstanceId.1": instance_id})
            entries = assistants.get("InstanceCloudAssistantStatusSet", {}).get("InstanceCloudAssistantStatus", [])
            assistant_online = any(e.get("InstanceId") == instance_id and e.get("CloudAssistantStatus") == "true" for e in entries)
            if assistant_online:
                progress(f"Cloud Assistant online for {instance_id} after {int(time.monotonic() - assistant_started)}s")
                break
            if attempt == 0 or attempt % 3 == 0:
                progress(f"waiting for Cloud Assistant on {instance_id} ({int(time.monotonic() - assistant_started)}s elapsed)")
            time.sleep(5)
        else:
            raise DeployError("Cloud Assistant not online; install/repair agent in the launch template; no command dispatched")
        state.update(status="dispatching", release=reference, command_sha256=hashlib.sha256(script.encode()).hexdigest())
        atomic_json(statepath, state)
        progress(f"dispatching deployment command to {instance_id}")
        result = api.call("RunCommand", Type="RunShellScript", ContentEncoding="PlainText", CommandContent=script, Timeout=1800, ClientToken=state["operation_id"], KeepCommand="false", TerminationMode="ProcessTree", **{"InstanceId.1": instance_id})
        state.update(invoke_id=result["InvokeId"], status="running")
        atomic_json(statepath, state)
        progress(f"Cloud Assistant invocation started: {result['InvokeId']}; polling every 5s")
        poll(api, state, statepath)
        print(f"{action} succeeded on {instance_id}; version {reference['version']}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=("init", "update", "rollback", "status"))
    parser.add_argument("--target", required=True, type=Path)
    source = parser.add_mutually_exclusive_group()
    source.add_argument("--release", type=Path, help="verified reference.json from successful OSS release CI")
    source.add_argument("--version", help="published stable version; resolve reference automatically via gh")
    parser.add_argument("--repository", help="GitHub OWNER/REPO; must match project configuration")
    parser.add_argument("--project-config", type=Path, default=Path("deploy/linux/project.json"))
    parser.add_argument("--state-dir", type=Path, default=Path(".deploy-state"))
    parser.add_argument("--apply", action="store_true", help="allow billable ECS creation and remote environment modification")
    parser.add_argument("--confirm-schema-compatible", action="store_true")
    args = parser.parse_args()
    try:
        target = validate_target(read_json(args.target))
        require(not args.repository or args.version, "--repository requires --version")
        require(args.action != "status" or not (args.release or args.version), "status does not accept a release/version")
        require(args.action != "rollback" or args.confirm_schema_compatible, "rollback requires --confirm-schema-compatible after reviewing migration compatibility")
        repository = None
        if args.version:
            import project
            cfg = project.load(args.project_config)
            project.validate_repository(cfg, project.configured_origin(), os.environ.get("GITHUB_REPOSITORY"))
            require(cfg["project"] == target["project"], "target/project identity mismatch")
            repository = args.repository or cfg["repository"]
            require(repository == cfg["repository"], "--repository does not match project configuration")
        reference = (release_reference.resolve(repository, args.version, target["project"]) if args.version
                     else read_json(args.release) if args.release else None)
        if args.action != "status":
            require(reference and reference["project"] == target["project"], "matching --release or --version required")
            validate_release_for_environment(target, reference)
            digest(reference["sha256"])
            key(reference["key"])
            oss_config(reference["oss"])
            if args.action == "init":
                oss_config(target["config_oss"])
                key(target["config_key"])
                digest(target["config_sha256"])
            require(reference.get("platform") == "ubuntu-22.04-amd64", "unsupported platform")
        statepath = args.state_dir / f"{target['project']}-{target['environment']}.json"
        if args.version:
            release_reference.pin_reference(args.state_dir, repository, reference, write=args.apply)
        if args.action != "status" and not args.apply:
            print(json.dumps({"mode": "PLAN ONLY; read-only GitHub lookup; no Alibaba calls" if args.version else "PLAN ONLY; no cloud calls", "action": args.action, "project": target["project"], "environment": target["environment"], "region": target["region"], "instance": target.get("instance_id", "create POSTPAID using pinned launch template, unless state already has instance"), "release": reference["version"], "commit": reference["commit"], "sha256": reference["sha256"], "state": str(statepath), "maintenance_window": True}, ensure_ascii=False, indent=2))
            return
        execute(target, reference, args.action, statepath, args.confirm_schema_compatible, args.action == "status")
    except (DeployError, KeyError, ValueError, OSError) as e:
        print(str(e) if isinstance(e, DeployError) else "invalid input/schema; review example configuration")
        raise SystemExit(1)


if __name__ == "__main__":
    main()
