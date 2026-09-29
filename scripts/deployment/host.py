#!/usr/bin/env python3
"""Root-only single-host deployment. Explicit maintenance window, fail closed."""
import argparse
import base64
import json
import ipaddress
import os
from pathlib import Path
import platform
import pwd
import re
import shutil
import subprocess
import tempfile
import time
import urllib.request
import uuid

from common import DeployError, atomic_json, extract_verified, key, lock, name, oss_config, read_json, require, sha256, version, web_apps
from storage import bucket, download, upload_immutable
from reporting import stage

ENV_KEYS = {"DATABASE_DSN", "JWT_SECRET", "CREDENTIAL_ENCRYPTION_KEY", "REDIS_ADDR", "REDIS_PASSWORD", "REDIS_USERNAME", "MCP_BASE_URL", "MCP_WEB_BASE_URL", "AGUO_DEPLOYMENT_TENANT_ID", "AGUO_WEBSOCKET_ALLOWED_ORIGINS"}
DEPLOYMENT_TENANT_KEY = "AGUO_DEPLOYMENT_TENANT_ID"
TENANT_ID_PATTERN = r"[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}"


def command(argv, **kwargs):
    # Do not forward command output into Cloud Assistant logs (may contain secrets).
    result = subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)
    if result.returncode != 0:
        log = Path("/var/log/ainp-deployment-errors.log")
        fd = os.open(log, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
        with os.fdopen(fd, "ab") as f:
            os.fchmod(f.fileno(), 0o600)
            f.write((Path(argv[0]).name + " failed\n").encode() + result.stdout[-32768:] + result.stderr[-32768:] + b"\n")
    require(result.returncode == 0, f"command failed: {Path(argv[0]).name}; inspect protected host logs / service journal")
    return result.stdout


def redis_db(c):
    """Resolve the single deployment DB setting; old private configs remain DB 0."""
    value = c.get("redis_db", 0)
    require(type(value) is int and value >= 0, "redis_db must be a non-negative integer")
    return value


def validate_websocket_origins(value):
    """Accept explicit browser Origins only; never widen an absent allowlist."""
    message = "invalid AGUO_WEBSOCKET_ALLOWED_ORIGINS"
    require(value and all(32 <= ord(char) < 127 for char in value), message)
    label = r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?"
    pattern = rf"https?://(?P<host>{label}(?:\.{label})*|\[[0-9A-Fa-f:.]+\])(?::(?P<port>[1-9][0-9]{{0,4}}))?"
    for item in value.split(","):
        match = re.fullmatch(pattern, item.strip())
        require(match is not None, message)
        hostname, port = match.group("host", "port")
        require(len(hostname) <= 253 and (port is None or int(port) <= 65535), message)
        if hostname.startswith("["):
            try:
                ipaddress.IPv6Address(hostname[1:-1])
            except ValueError:
                raise DeployError(message) from None


def validate_config(c, project):
    require(c.get("schema") == 1 and c.get("project") == project, "wrong private configuration")
    redis_db(c)
    require(c.get("database_mode") in ("external", "local-container"), "database mode required")
    require(isinstance(c.get("env"), dict) and set(c["env"]) <= ENV_KEYS, "unsupported runtime environment key")
    for k in ("DATABASE_DSN", "JWT_SECRET", "CREDENTIAL_ENCRYPTION_KEY", "REDIS_ADDR", "REDIS_PASSWORD", "MCP_BASE_URL", "MCP_WEB_BASE_URL"):
        require(isinstance(c["env"].get(k), str) and c["env"][k] and "\x00" not in c["env"][k], f"missing/invalid {k}")
    require(all(isinstance(v, str) and "\x00" not in v for v in c["env"].values()), "invalid environment value")
    if DEPLOYMENT_TENANT_KEY in c["env"]:
        require(c["project"] == "aguo-ai-dining" and re.fullmatch(TENANT_ID_PATTERN, c["env"][DEPLOYMENT_TENANT_KEY]), "invalid AGUO_DEPLOYMENT_TENANT_ID")
    if "AGUO_WEBSOCKET_ALLOWED_ORIGINS" in c["env"]:
        require(c["project"] == "aguo-ai-dining", "invalid AGUO_WEBSOCKET_ALLOWED_ORIGINS")
        validate_websocket_origins(c["env"]["AGUO_WEBSOCKET_ALLOWED_ORIGINS"])
    require(not any("REPLACE" in v for v in c["env"].values()), "replace example secret placeholders")
    require(len(c["env"]["JWT_SECRET"]) >= 32, "production JWT secret too short")
    try:
        require(len(base64.b64decode(c["env"]["CREDENTIAL_ENCRYPTION_KEY"], validate=True)) == 32, "credential encryption key must decode to 32 bytes")
    except ValueError:
        raise DeployError("credential encryption key must be base64-encoded 32 bytes") from None
    require(re.fullmatch(r"[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?", c["domain"]) and "." in c["domain"], "invalid domain")
    require(type(c["api_port"]) is int and 1024 <= c["api_port"] <= 65535, "invalid loopback API port")
    for field in ("worker_concurrency", "ai_concurrency"):
        require(type(c[field]) is int and 1 <= c[field] <= 64, "invalid worker concurrency")
    for field in ("MCP_BASE_URL", "MCP_WEB_BASE_URL"):
        require(c["env"][field] == "https://" + c["domain"], "public URLs must match TLS domain")
    require(isinstance(c.get("bootstrap"), dict), "bootstrap configuration required")
    require(set(c["bootstrap"]) == {"tenant_name", "username", "password"}, "invalid bootstrap fields")
    require(all(isinstance(v, str) and v and "REPLACE" not in v for v in c["bootstrap"].values()), "replace bootstrap placeholders")
    require(type(c.get("web_keep_releases", 5)) is int and 2 <= c.get("web_keep_releases", 5) <= 50, "web_keep_releases must be 2..50")
    pins = c.get("web_pinned_versions", [])
    require(isinstance(pins, list) and len(pins) <= 50, "invalid web_pinned_versions")
    for pin in pins:
        version(pin)
    require(len(set(pins)) == len(pins), "duplicate web pinned version")
    oss_config(c["backup_oss"])
    key(c["backup_prefix"])
    return c


def require_runtime_only_change(current, replacement):
    before, after = dict(current), dict(replacement)
    before_env, after_env = dict(before.pop("env")), dict(after.pop("env"))
    for field in ("AGUO_DEPLOYMENT_TENANT_ID", "AGUO_WEBSOCKET_ALLOWED_ORIGINS"):
        before_env.pop(field, None)
        after_env.pop(field, None)
    require(before == after and before_env == after_env,
            "private configuration update may only change AGUO_DEPLOYMENT_TENANT_ID or AGUO_WEBSOCKET_ALLOWED_ORIGINS")


def preserve_deployment_tenant(current, replacement):
    """A missing OSS tenant field means retain the host's trusted binding."""
    current_id = current["env"].get(DEPLOYMENT_TENANT_KEY)
    if current_id and DEPLOYMENT_TENANT_KEY not in replacement["env"]:
        replacement["env"][DEPLOYMENT_TENANT_KEY] = current_id
    return replacement


def app_config(c, home):
    return {
        "server": {"addr": f"127.0.0.1:{c['api_port']}", "mode": "release"},
        "database": {"auto_migrate": False},
        "logging": {"level": "info", "format": "json", "outputs": ["stdout", "file"], "file": {"path": str(home / "logs/{service}.jsonl"), "max_size_mb": 100, "max_backups": 14, "max_age_days": 14, "compress": True}},
        "auth": {"refresh_ttl": "720h", "dev_token_enabled": False, "jwt": {"issuer": c["project"], "ttl": "15m"}},
        "redis": {"db": redis_db(c)}, "i18n": {"default_language": "zh-CN"},
        "storage": {"provider": "local", "local": {"root_dir": str(home / "objects")}},
        "outbox": {"enabled": True, "transport": "postgres"},
        "jobs": {"enabled": True, "embedded_worker": False},
        "state_file": str(home / "state/state.yaml"),
    }


def unit(project, role, root, home):
    return f"""[Unit]
Description={project} {role}
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0
[Service]
User={project}
Group={project}
WorkingDirectory={home}
ExecStart=/usr/bin/python3 {root}/launcher.py /etc/{project}/runtime.json {role} {root}/current
Restart=on-failure
RestartSec=5
TimeoutStopSec=90
KillSignal=SIGTERM
UMask=0077
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths={home}
[Install]
WantedBy=multi-user.target
"""


def nginx(c, root, maintenance=False, manifest_apps=None):
    proxy = f"http://127.0.0.1:{c['api_port']}"
    headers = "\n".join("        " + line for line in (
        "proxy_set_header Host $host;", "proxy_set_header X-Real-IP $remote_addr;",
        "proxy_set_header X-Forwarded-For $remote_addr;", "proxy_set_header X-Forwarded-Host $host;",
        "proxy_set_header X-Forwarded-Proto $scheme;"))
    routes = ""
    for route, streaming in (("= /healthz", False), ("= /.well-known/oauth-protected-resource", False), ("= /.well-known/oauth-authorization-server", False), ("= /oauth/register", False), ("= /oauth/token", False), ("= /oauth/revoke", False), ("= /mcp", True), ("~ ^/api/v1/ai/", True), ("/api/", False)):
        options = "proxy_buffering off; proxy_cache off; proxy_read_timeout 3600s;" if streaming else "proxy_read_timeout 300s;"
        upgrade = "        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection $ainp_websocket_connection;\n" if route in ("~ ^/api/v1/ai/", "/api/") else ""
        routes += f"    location {route} {{\n        proxy_pass {proxy};\n        proxy_http_version 1.1;\n        {options}\n{headers}\n{upgrade}    }}\n"
    apps = manifest_apps or [{"name": "admin", "mount": "/", "health_path": "/build.json", "output_dir": "web"}]
    if not maintenance:
        for app in sorted(apps, key=lambda item: len(item["mount"]), reverse=True):
            if app["mount"] == "/":
                continue
            mount = app["mount"]
            mount_root = mount.rstrip("/")
            routes += f"    location = {mount_root} {{ return 301 {mount}; }}\n"
            routes += f"    location ^~ {mount} {{ try_files $uri $uri/ {mount}index.html; }}\n"
        routes += f"    location ~ ^/__web/(?<web_version>v[0-9]+[.][0-9]+[.][0-9]+(?:-ci)?)/(?<web_file>.*)$ {{ alias {root}/web-versions/$web_version/$web_file; }}\n"
        for app in apps:
            routes += f"    location ^~ {app['mount']}assets/ {{ root {root}/current/web; try_files $uri @legacy_assets; }}\n"
        routes += f"    location @legacy_assets {{ root {root}/legacy-web; try_files $uri =404; }}\n"
        routes += "    location / { try_files $uri $uri/ /index.html; }\n"
    body = "    return 503;\n" if maintenance else routes
    return f"""# Generated application routes. TLS assets are managed separately, never by app updates.
map $http_upgrade $ainp_websocket_connection {{
    default upgrade;
    '' '';
}}
server {{
    listen 80;
    server_name {c['domain']};
    return 301 https://{c['domain']}$request_uri;
}}
server {{
    listen 443 ssl;
    server_name {c['domain']};
    ssl_certificate /etc/{c['project']}/tls/fullchain.pem;
    ssl_certificate_key /etc/{c['project']}/tls/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    root {root}/current/web;
    index index.html;
    client_max_body_size 256m;
{body}}}
"""


def install_nginx(c, root, maintenance, manifest_apps=None):
    path = Path(f"/etc/nginx/conf.d/{c['project']}.conf")
    old = path.read_bytes() if path.exists() else None
    path.write_text(nginx(c, root, maintenance, manifest_apps))
    try:
        command(["nginx", "-t"])
        command(["systemctl", "reload", "nginx"])
    except Exception:
        if old is None:
            path.unlink()
        else:
            path.write_bytes(old)
        raise


def get_json(url, data=None):
    req = urllib.request.Request(url, data=json.dumps(data).encode() if data else None, headers={"Content-Type": "application/json"})
    with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req, timeout=60) as r:
        return json.load(r)["data"]


def https_json(c, path):
    # Reload is asynchronous: a retiring maintenance worker can still return 503.
    # Retry only curl's transient errors, retaining TLS and release identity checks.
    return json.loads(command(["curl", "--fail", "--silent", "--show-error", "--max-time", "20",
                               "--retry", "5", "--retry-delay", "1", "--retry-max-time", "30",
                               "--resolve", f"{c['domain']}:443:127.0.0.1", f"https://{c['domain']}{path}"]))


def wait_api(c, manifest):
    for _ in range(60):
        try:
            health = get_json(f"http://127.0.0.1:{c['api_port']}/healthz")
            if health.get("status") == "ok" and health.get("version") == manifest["version"] and health.get("commit") == manifest["commit"]:
                return
        except (OSError, ValueError, KeyError):
            pass
        time.sleep(2)
    raise DeployError("API health/version check timed out")


def db_preflight(c, fresh):
    import psycopg
    from psycopg.conninfo import conninfo_to_dict
    params = conninfo_to_dict(c["env"]["DATABASE_DSN"])
    backup_environment(c["env"]["DATABASE_DSN"])
    if c["database_mode"] == "local-container":
        require(params.get("host") in ("127.0.0.1", "localhost", "::1"), "host process must use loopback-published PG container port")
    else:
        require(params.get("sslmode") == "disable" or (params.get("sslmode") == "verify-full" and params.get("sslrootcert")), "external PG requires explicit disable for private networking or verify-full with CA file")
    with psycopg.connect(c["env"]["DATABASE_DSN"], connect_timeout=10) as conn:
        require(conn.info.server_version >= 170000 and conn.info.server_version < 180000, "baseline requires PG17 (including matching pg_dump17)")
        require(conn.execute("select 1 from pg_available_extensions where name='vector'").fetchone(), "pgvector must be available on server")
        if fresh:
            require(conn.execute("select count(*) from information_schema.tables where table_schema='public' and table_type='BASE TABLE'").fetchone()[0] == 0, "fresh deployment requires empty DB; existing environments need explicit adoption")


def database_tenants(c):
    """Read tenant identities through the deployment DB, never from client input."""
    import psycopg

    try:
        with psycopg.connect(c["env"]["DATABASE_DSN"], connect_timeout=10) as conn:
            rows = conn.execute(
                "SELECT id, status FROM tenants ORDER BY id"
            ).fetchall()
    except Exception:
        raise DeployError("tenant lookup failed; inspect protected deployment logs") from None
    return [(str(row[0]), row[1]) for row in rows]


def resolve_deployment_tenant_id(c, configured=None):
    """Validate an explicit binding or infer exactly one existing tenant."""
    tenants = database_tenants(c)
    if configured is None:
        require(len(tenants) == 1, "cannot infer deployment tenant; expected exactly one tenant")
        configured = tenants[0][0]
    require(isinstance(configured, str) and re.fullmatch(TENANT_ID_PATTERN, configured), "invalid deployment tenant id")
    matches = [row for row in tenants if row[0] == configured]
    require(len(matches) == 1, "configured AGUO_DEPLOYMENT_TENANT_ID does not exist in the deployment database")
    require(matches[0][1] == "enabled", "configured deployment tenant is not enabled")
    return configured


def bind_deployment_tenant(c, tenant_id):
    bound = {**c, "env": {**c["env"], DEPLOYMENT_TENANT_KEY: tenant_id}}
    return validate_config(bound, c["project"])


def installation_state(home):
    """Read the setup marker used to distinguish safe binding recovery from a partial init."""
    statepath = Path(home) / "state/state.yaml"
    if not statepath.exists():
        return {}
    try:
        import yaml
        state = yaml.safe_load(statepath.read_text(encoding="utf-8"))
    except Exception:
        return {}
    return state if isinstance(state, dict) else {}


def initial_deployment_tenant(c, result, home):
    state = installation_state(home)
    require(state.get("installed") is True, "setup installation state is not complete")
    tenant_id = result.get("tenant_id")
    saved_id = state.get("initial_tenant_id")
    if tenant_id is not None and saved_id is not None:
        require(tenant_id == saved_id, "setup response and persisted initial tenant conflict")
    # Older API releases lack tenant_id: only the unique DB result is inferable.
    actual_id = resolve_deployment_tenant_id(c, tenant_id if tenant_id is not None else saved_id)
    configured = c["env"].get(DEPLOYMENT_TENANT_KEY)
    require(configured is None or configured == actual_id, "configured deployment tenant conflicts with initialized tenant")
    return actual_id


def write_runtime_config(c, etc, home, gid):
    runtime = {k: c[k] for k in ("env", "worker_concurrency", "ai_concurrency")}
    runtime["home"] = str(home)
    atomic_json(etc / "runtime.json", runtime, 0o640)
    os.chown(etc / "runtime.json", 0, gid)


def redis_preflight(c):
    import redis
    addr = c["env"]["REDIS_ADDR"]
    host, port = addr.rsplit(":", 1)
    db = redis_db(c)
    client = redis.Redis(host=host.strip("[]"), port=int(port), db=db,
                         username=c["env"].get("REDIS_USERNAME") or None,
                         password=c["env"]["REDIS_PASSWORD"], socket_timeout=10,
                         socket_connect_timeout=10)
    probe = "ainp:deploy:probe:" + uuid.uuid4().hex
    try:
        try:
            require(client.ping(), "Redis ping failed")
            require(client.eval("redis.call('SET', KEYS[1], 'ok', 'EX', 60); return redis.call('GET', KEYS[1])", 1, probe) == b"ok", "Redis Lua capability missing")
        finally:
            try:
                client.delete(probe)
            finally:
                client.close()
    except redis.RedisError:
        raise DeployError(f"Redis preflight failed for redis_db={db}; check database availability, authentication and Lua support") from None


def backup_environment(dsn):
    from psycopg.conninfo import conninfo_to_dict
    params = conninfo_to_dict(dsn)
    # libpq does not expand a conninfo string supplied through PGDATABASE.
    # Map individually into root-only subprocess environment; never DSN in argv.
    mapping = {"host": "PGHOST", "hostaddr": "PGHOSTADDR", "port": "PGPORT", "user": "PGUSER", "password": "PGPASSWORD", "dbname": "PGDATABASE", "sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY", "sslpassword": "PGSSLPASSWORD", "sslcrl": "PGSSLCRL", "sslcrldir": "PGSSLCRLDIR", "channel_binding": "PGCHANNELBINDING", "connect_timeout": "PGCONNECT_TIMEOUT", "application_name": "PGAPPNAME", "options": "PGOPTIONS", "target_session_attrs": "PGTARGETSESSIONATTRS"}
    require(set(params) <= mapping.keys(), "DSN contains an option unsupported by backup contract")
    return {"PATH": "/usr/bin:/bin", "PGCONNECT_TIMEOUT": "15", **{mapping[k]: v for k, v in params.items()}}


def backup(c, root, role):
    env = backup_environment(c["env"]["DATABASE_DSN"])
    path = root / ("backup-" + uuid.uuid4().hex + ".dump")
    try:
        with path.open("xb") as f:
            os.chmod(path, 0o600)
            result = subprocess.run(["/usr/lib/postgresql/17/bin/pg_dump", "--format=custom", "--no-owner", "--no-acl"], env=env, stdout=f, stderr=subprocess.PIPE)
        require(result.returncode == 0 and path.stat().st_size > 0, "database backup failed; services remain stopped")
        backup_key = f"{c['backup_prefix']}/{c['project']}/{path.name}"
        upload_immutable(bucket(c["backup_oss"], role), backup_key, path)
        return {"key": backup_key, "sha256": sha256(path)}
    finally:
        path.unlink(missing_ok=True)


def switch(root, release):
    tmp = root / "current.next"
    tmp.unlink(missing_ok=True)
    tmp.symlink_to(release)
    os.replace(tmp, root / "current")


def deploy(request):
    require(os.geteuid() == 0 and platform.machine() == "x86_64", "requires root on amd64")
    require('VERSION_ID="22.04"' in Path("/etc/os-release").read_text(), "requires Ubuntu 22.04")
    project = name(request["project"])
    root, home = Path(f"/opt/{project}"), Path(f"/var/lib/{project}")
    root.mkdir(mode=0o755, exist_ok=True)
    root.chmod(0o755)
    with lock(root / "deploy.lock"):
        perform(request, root, home)


def perform(r, root, home):
    project, role = r["project"], r["ram_role"]
    etc = Path(f"/etc/{project}")
    cfgpath, statepath = etc / "private.json", root / "deployment.json"
    action = r["action"]
    require(action in ("init", "update", "rollback"), "invalid action")
    existing = read_json(statepath) if statepath.exists() else {}
    # Cloud command replay is safe only after completion; interrupted work requires audit.
    if existing.get("operation_id") == r["operation_id"] and existing.get("status") == "healthy":
        print("Deployment already completed")
        return
    recovery = action == "init" and existing.get("status") == "failed-maintenance"
    fresh = action == "init" and not recovery
    require(not fresh or not existing, "initialization already attempted; inspect state, do not reinitialize")
    require(fresh or existing, "no managed installation; use init first")
    require(fresh or existing.get("status") == "healthy" or action == "rollback" or recovery,
            "previous deployment failed; inspect state then explicit rollback/recovery")
    etc.mkdir(mode=0o750, exist_ok=True)
    etc.chmod(0o750)
    if recovery:
        if existing.get("action") is None:
            require(existing.get("current") is None and not existing.get("history"),
                    "only a failed initial deployment can use init recovery")
        else:
            require(existing.get("action") == "init", "only a failed initial deployment can use init recovery")
        previous_release = existing.get("release") or {}
        for field in ("project", "version", "commit", "platform", "sha256", "key"):
            require(previous_release.get(field) == r["release"].get(field),
                    "init recovery must use the exact failed release")
        require(cfgpath.exists(), "failed init recovery requires the persisted private configuration")
        saved_config_sha256 = existing.get("config_sha256")
        require(saved_config_sha256 is None or saved_config_sha256 == r.get("config_sha256"),
                "init recovery must use the exact failed private configuration")
        require(installation_state(home).get("installed") is True,
                "failed init did not complete setup; inspect the host before recovery")
    if fresh:
        require(not cfgpath.exists(), "private configuration already exists; inspect incomplete bootstrap")
        with stage("private-config-download"):
            download(r["config_oss"], r["config_key"], cfgpath, r["config_sha256"], role)
        cfgpath.chmod(0o600)
    c = validate_config(read_json(cfgpath), project)
    if recovery:
        require(all(field in r for field in ("config_oss", "config_key", "config_sha256")),
                "init recovery requires the original pinned private configuration")
        with tempfile.TemporaryDirectory(prefix="private-recovery-", dir=etc) as directory:
            candidate = Path(directory) / "private.json"
            with stage("private-config-download"):
                download(r["config_oss"], r["config_key"], candidate, r["config_sha256"], role)
            recovery_config = validate_config(read_json(candidate), project)
        expected_recovery = preserve_deployment_tenant(c, recovery_config)
        require(expected_recovery == c, "init recovery private configuration differs from original except persisted tenant binding")
    original_config = c
    replacement = None
    if action == "update":
        require(all(field in r for field in ("config_oss", "config_key", "config_sha256")), "update requires pinned private configuration")
        with tempfile.TemporaryDirectory(prefix="private-config-", dir=etc) as directory:
            candidate = Path(directory) / "private.json"
            with stage("private-config-download"):
                download(r["config_oss"], r["config_key"], candidate, r["config_sha256"], role)
            replacement = validate_config(read_json(candidate), project)
        require_runtime_only_change(c, replacement)
        if project == "aguo-ai-dining":
            replacement = preserve_deployment_tenant(c, replacement)
            replacement = validate_config(replacement, project)
    effective_config = replacement or c
    manifest_ref = r["release"]
    require(manifest_ref["project"] == project, "release belongs to another project")
    version(manifest_ref["version"])
    release = root / "releases" / f"{manifest_ref['version']}-{manifest_ref['sha256'][:16]}"
    (root / "releases").mkdir(exist_ok=True)
    (root / "releases").chmod(0o755)
    with tempfile.TemporaryDirectory(prefix="stage-", dir=root) as temp:
        temp = Path(temp)
        archive = temp / "release.tar.gz"
        with stage("release-download"):
            download(manifest_ref["oss"], manifest_ref["key"], archive, manifest_ref["sha256"], role)
        with stage("release-verify"):
            manifest = extract_verified(archive, temp / "payload", manifest_ref["sha256"])
        require(all(manifest[k] == manifest_ref[k] for k in ("project", "version", "commit", "platform")), "release reference mismatch")
        if release.exists():
            require(read_json(release / "release.json") == manifest, "existing release identity mismatch")
            require(all(sha256(release / p) == h for p, h in manifest["files"].items()), "installed release modified")
        else:
            shutil.move(str(temp / "payload"), release)
    if action == "rollback":
        require(r.get("confirm_schema_compatible") is True, "application rollback needs explicit DB compatibility confirmation")
        require(str(release) in existing.get("history", []), "rollback target was not previously deployed on this host")
    with stage("postgresql-preflight"):
        db_preflight(effective_config, fresh)
    with stage("redis-preflight"):
        redis_preflight(effective_config)
    if project == "aguo-ai-dining" and not fresh:
        tenant_id = (initial_deployment_tenant(effective_config, {}, home) if recovery else
                     resolve_deployment_tenant_id(effective_config, effective_config["env"].get(DEPLOYMENT_TENANT_KEY)))
        effective_config = bind_deployment_tenant(effective_config, tenant_id)
    if effective_config != original_config:
        if not fresh:
            atomic_json(etc / "private.previous.json", original_config, 0o600)
        atomic_json(cfgpath, effective_config, 0o600)
    c = effective_config
    if fresh:
        try:
            pwd.getpwnam(project)
            raise DeployError("dedicated Unix account already exists; use a fresh template")
        except KeyError:
            command(["useradd", "--system", "--user-group", "--home-dir", str(home), "--shell", "/usr/sbin/nologin", project])
        tls = etc / "tls"
        tls.mkdir(mode=0o700, exist_ok=True)
        for k, filename in (("fullchain_pem", "fullchain.pem"), ("private_key_pem", "privkey.pem")):
            require(c.get("tls", {}).get(k), "initial TLS certificate/key required (renewal managed independently)")
            path = tls / filename
            path.write_text(c["tls"][k])
            path.chmod(0o600)
        with stage("https-certificate"):
            command(["openssl", "x509", "-in", str(tls / "fullchain.pem"), "-noout", "-checkend", "604800", "-checkhost", c["domain"]])
    user = pwd.getpwnam(project)
    require(user.pw_uid != 0 and user.pw_dir == str(home) and user.pw_shell == "/usr/sbin/nologin", "unexpected service user identity")
    os.chown(etc, 0, user.pw_gid)
    for sub in ("", "logs", "objects", "state"):
        path = home / sub
        path.mkdir(mode=0o750, parents=True, exist_ok=True)
        os.chown(path, user.pw_uid, user.pw_gid)
        path.chmod(0o750)
    shutil.copyfile(Path(__file__).with_name("launcher.py"), root / "launcher.py")
    os.chmod(root / "launcher.py", 0o644)
    write_runtime_config(c, etc, home, user.pw_gid)
    for filename, value in (("app.json", app_config(c, home)),):
        atomic_json(etc / filename, value, 0o640)
        os.chown(etc / filename, 0, user.pw_gid)
    units = [f"{project}-{p}.service" for p in ("api", "worker", "ai-worker")]
    for p in ("api", "worker", "ai-worker"):
        Path(f"/etc/systemd/system/{project}-{p}.service").write_text(unit(project, p, root, home))
    command(["systemctl", "daemon-reload"])
    previous = str((root / "current").resolve()) if (root / "current").exists() else None
    history = list(existing.get("history", []))
    if existing.get("status") == "healthy" and existing.get("current") not in history:
        history.append(existing["current"])
    state = {
        "operation_id": r["operation_id"],
        "action": action,
        "status": "deploying",
        "target": str(release),
        "previous": previous,
        "history": history,
        "release": manifest_ref,
        "current": existing.get("current"),
        "config_sha256": r.get("config_sha256", existing.get("config_sha256")),
        "bootstrap_status": existing.get("bootstrap_status") or ("pending" if fresh else "installed"),
    }
    atomic_json(statepath, state)
    try:
        install_nginx(c, root, True)
        command(["systemctl", "disable", *units])
        command(["systemctl", "stop", *units])
        # Stopped units may be garbage-collected. Exact globs skip unloaded units
        # without resetting other projects or hiding real systemctl errors.
        if not fresh:
            command(["systemctl", "reset-failed", *[f"[{u[0]}]{u[1:]}" for u in units]])
            with stage("database-backup"):
                state["backup"] = backup(c, root, role)
            atomic_json(statepath, state)
            if action != "rollback" and not recovery:
                with stage("database-migration"):
                    command(["runuser", "-u", project, "--", "/usr/bin/python3", str(root / "launcher.py"), str(etc / "runtime.json"), "migrate", str(release), "up"])
        switch(root, release)
        if fresh:
            # Preserve the selected initial release for setup recovery without
            # admitting it to the successfully deployed rollback history.
            state["current"] = str(release)
            atomic_json(statepath, state)
        command(["systemctl", "start", units[0]])
        with stage("api-readiness"):
            wait_api(c, manifest)
        if fresh:
            setup = get_json(f"http://127.0.0.1:{c['api_port']}/api/v1/setup/status")
            require(setup.get("installed") is False, "unexpected setup status")
            payload = {**c["bootstrap"], "dsn": "", "redis_addr": c["env"]["REDIS_ADDR"], "redis_db": redis_db(c), "jobs_enabled": True, "embedded_worker": False}
            with stage("application-initialize"):
                result = get_json(f"http://127.0.0.1:{c['api_port']}/api/v1/setup/initialize", payload)
            require(result.get("installed") is True, "setup failed")
            state["bootstrap_status"] = "installed"
            atomic_json(statepath, state)
            if project == "aguo-ai-dining":
                tenant_id = initial_deployment_tenant(c, result, home)
                c = bind_deployment_tenant(c, tenant_id)
                # setup has created the tenant; persist the trusted binding before
                # the API restart so API and both workers consume the same runtime.
                atomic_json(cfgpath, c, 0o600)
                write_runtime_config(c, etc, home, user.pw_gid)
            command(["systemctl", "restart", units[0]])
            with stage("api-readiness"):
                wait_api(c, manifest)
        import yaml
        require(yaml.safe_load((home / "state/state.yaml").read_text()).get("installed") is True, "healthz alone is not installation readiness")
        # The config check probes Asynq capabilities during setup; on updates migration/health
        # plus process stability are a baseline, not a synthetic worker job assertion.
        command(["systemctl", "start", *units[1:]])
        time.sleep(10)
        for service in units:
            command(["systemctl", "is-active", "--quiet", service])
            require(command(["systemctl", "show", service, "-p", "NRestarts", "--value"]).strip() == b"0", "service restarted during readiness window")
            pid = int(command(["systemctl", "show", service, "-p", "MainPID", "--value"]).strip())
            require(Path(f"/proc/{pid}/exe").resolve().parent == release / "bin", "service not running target binary")
        # Stable immutable URLs keep prior Web assets addressable across switches.
        versions = root / "web-versions"
        versions.mkdir(exist_ok=True)
        # Bootstrap runs under umask 077; Nginx must traverse this directory.
        versions.chmod(0o755)
        link = versions / manifest["version"]
        require(not link.is_symlink() and not link.exists() or link.is_symlink() and link.resolve() == release / "web", "web version collision")
        if not link.is_symlink():
            link.symlink_to(release / "web")
        if previous:
            legacy = root / "legacy-web"
            old_version = read_json(Path(previous) / "release.json")["version"]
            old_link = versions / old_version
            require(not old_link.is_symlink() and not old_link.exists() or old_link.is_symlink() and old_link.resolve() == Path(previous) / "web", "previous web version collision")
            if not old_link.is_symlink():
                if not legacy.is_symlink():
                    legacy.symlink_to(Path(previous) / "web")
                old_link.symlink_to(Path(previous) / "web")
        install_nginx(c, root, False, manifest.get("web_apps"))
        health = https_json(c, "/healthz")["data"]
        require(health.get("status") == "ok" and health.get("version") == manifest["version"] and health.get("commit") == manifest["commit"], "HTTPS health check failed")
        verified_web_apps = []
        for app in web_apps(manifest):
            web = https_json(c, app["health_path"])
            require(web.get("commit") == manifest["commit"] and web.get("version") == manifest["version"], f"Web release mismatch: {app['name']}")
            # The app mount may work while Nginx cannot traverse web-versions.
            versioned_path = f"/__web/{manifest['version']}{app['health_path']}"
            versioned_web = https_json(c, versioned_path)
            require(versioned_web.get("commit") == manifest["commit"] and versioned_web.get("version") == manifest["version"], f"Versioned Web release mismatch: {app['name']}")
            verified_web_apps.append({"name": app["name"], "mount": app["mount"], "health_path": app["health_path"], "version": web["version"], "commit": web["commit"], "build_time": web.get("build_time")})
        command(["systemctl", "enable", *units])
        if str(release) not in history:
            history.append(str(release))
        # Device operators pin active versions; keep a bounded rollback window.
        keep = set(history[-c.get("web_keep_releases", 5):])
        keep.add(str(release))
        keep.update(p for p in history if read_json(Path(p) / "release.json")["version"] in c.get("web_pinned_versions", []))
        legacy = root / "legacy-web"
        if legacy.is_symlink():
            keep.add(str(legacy.resolve().parent))
        for path in history[:]:
            candidate = Path(path)
            if path in keep or candidate.parent != root / "releases" or not candidate.is_dir() or candidate.is_symlink():
                continue
            old_version = read_json(candidate / "release.json")["version"]
            old_link = versions / old_version
            if old_link.is_symlink() and old_link.resolve() == candidate / "web":
                old_link.unlink()
            shutil.rmtree(candidate)
            history.remove(path)
        state.update(status="healthy", current=str(release), history=history, web_apps=verified_web_apps)
        atomic_json(statepath, state)
        print(f"Healthy: {project} {manifest['version']} {manifest['commit']}")
    except Exception:
        state["status"] = "failed-maintenance"
        atomic_json(statepath, state)
        # Never pretend a failed migration was undone. Keep public traffic closed.
        try:
            install_nginx(c, root, True)
        finally:
            subprocess.run(["systemctl", "stop", *units], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        raise DeployError("deployment failed; maintenance retained; inspect host state/journal and database compatibility before recovery") from None


def main():
    p = argparse.ArgumentParser()
    p.add_argument("request")
    args = p.parse_args()
    try:
        with stage("host-deployment"):
            deploy(read_json(args.request))
    except Exception as error:
        # Never serialize library exceptions that may contain credentials/HTTP body.
        print(str(error) if isinstance(error, DeployError) else f"deployment failed ({type(error).__name__}); inspect protected host state")
        raise SystemExit(1)


if __name__ == "__main__":
    main()
