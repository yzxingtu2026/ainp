import json
import os
import re
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
NGINX_CONF = ROOT / "docker" / "nginx.conf"
PROXY_PARAMS = ROOT / "docker" / "proxy_params"
NGINX_IMAGE = "nginx:1.27.5-alpine"

PROTOCOL_PATHS = (
    "/.well-known/oauth-protected-resource",
    "/.well-known/oauth-authorization-server",
    "/oauth/register",
    "/oauth/token",
    "/oauth/revoke",
    "/mcp",
)
SPA_PATHS = ("/oauth/authorize", "/mcp/confirm", "/setup")
FORBIDDEN_LOCATION_PATTERNS = (
    r"location\s+~\s+\^/oauth/",
    r"location\s+/oauth/",
    r"location\s+/oauth\s*\{",
    r"location\s+~\s+\^/\\\.well-known/",
    r"location\s+~\s+\^/\.well-known/",
)


def read_nginx_conf() -> str:
    return NGINX_CONF.read_text(encoding="utf-8")


def location_block(conf: str, path: str) -> str:
    pattern = rf"location\s+=\s+{re.escape(path)}\s*\{{(.*?)\n    \}}"
    match = re.search(pattern, conf, flags=re.S)
    if match is None:
        raise AssertionError(f"missing exact location {path}")
    return match.group(1)


class TestWebNginxConfig(unittest.TestCase):
    def test_protocol_endpoints_use_exact_locations(self):
        conf = read_nginx_conf()
        for path in PROTOCOL_PATHS:
            block = location_block(conf, path)
            self.assertIn("proxy_pass http://api:8080;", block)
            self.assertNotRegex(block, r"proxy_pass\s+http://api:8080/")

    def test_forbids_broad_oauth_or_well_known_proxy(self):
        conf = read_nginx_conf()
        for pattern in FORBIDDEN_LOCATION_PATTERNS:
            self.assertIsNone(re.search(pattern, conf), f"forbidden location survived: {pattern}")
        for path in SPA_PATHS:
            self.assertIsNone(
                re.search(rf"location\s+.+\s+{re.escape(path)}\s*\{{", conf),
                f"{path} must use SPA fallback instead of a proxy location",
            )

    def test_spa_fallback_and_mcp_streamable_settings(self):
        conf = read_nginx_conf()
        self.assertRegex(conf, r"location\s+/\s*\{\s*try_files\s+\$uri\s+\$uri/\s+/index.html;")
        mcp = location_block(conf, "/mcp")
        self.assertIn("proxy_http_version 1.1;", mcp)
        self.assertIn("proxy_buffering off;", mcp)
        self.assertIn("proxy_cache off;", mcp)
        self.assertIn("proxy_read_timeout 3600s;", mcp)


class MockOAuthAPI(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, format, *args):
        del format, args

    def do_GET(self):
        self._handle()

    def do_POST(self):
        self._handle()

    def _handle(self):
        path = self.path.split("?", 1)[0]
        body = self._read_body()
        if path == "/.well-known/oauth-protected-resource":
            self._json(200, {
                "resource": "https://ainp.example/mcp",
                "authorization_servers": ["https://ainp.example"],
                "scopes_supported": ["actions:read", "actions:write", "actions:sensitive"],
                "bearer_methods_supported": ["header"],
            })
            return
        if path == "/.well-known/oauth-authorization-server":
            self._json(200, {
                "issuer": "https://ainp.example",
                "authorization_endpoint": "https://ainp.example/oauth/authorize",
                "token_endpoint": "https://ainp.example/oauth/token",
                "registration_endpoint": "https://ainp.example/oauth/register",
                "revocation_endpoint": "https://ainp.example/oauth/revoke",
                "code_challenge_methods_supported": ["S256"],
            })
            return
        if path in {"/oauth/register", "/oauth/token", "/oauth/revoke"}:
            if self.command != "POST":
                self._plain(405, "method not allowed")
                return
            self._json(200, {
                "reached": "api",
                "method": self.command,
                "path": path,
                "content_type": self.headers.get("Content-Type", ""),
                "x_test_header": self.headers.get("X-Test-Header", ""),
                "body": body.decode("utf-8"),
            })
            return
        if path == "/mcp":
            self.send_response(401)
            self.send_header("WWW-Authenticate", 'Bearer resource_metadata="https://ainp.example/.well-known/oauth-protected-resource"')
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if path == "/healthz":
            self._json(200, {"status": "ok"})
            return
        self._plain(404, "API 404 page not found")

    def _read_body(self) -> bytes:
        length = int(self.headers.get("Content-Length") or "0")
        if length <= 0:
            return b""
        return self.rfile.read(length)

    def _json(self, status: int, payload: dict):
        raw = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _plain(self, status: int, message: str):
        raw = message.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


def unused_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


@unittest.skipUnless(shutil.which("docker"), "docker is required for Nginx routing contract tests")
class TestWebNginxRoutingContract(unittest.TestCase):
    work = None
    mock = None
    container = None

    @classmethod
    def setUpClass(cls):
        cls.work = Path(tempfile.mkdtemp(prefix="ainp-nginx-routes-"))
        cls.html = cls.work / "html"
        cls.html.mkdir()
        (cls.html / "index.html").write_text(
            "<!DOCTYPE html><html><head><title>AINP Admin</title></head>"
            "<body><main id=\"app\">AINP_ADMIN_SPA</main></body></html>\n",
            encoding="utf-8",
        )
        shutil.copyfile(PROXY_PARAMS, cls.work / "proxy_params")
        cls.mock = ThreadingHTTPServer(("0.0.0.0", 0), MockOAuthAPI)
        cls.mock_thread = threading.Thread(target=cls.mock.serve_forever, daemon=True)
        cls.mock_thread.start()
        mock_port = cls.mock.server_address[1]
        rewritten = read_nginx_conf().replace("http://api:8080", f"http://host.docker.internal:{mock_port}")
        (cls.work / "nginx.conf").write_text(rewritten, encoding="utf-8")
        cls.web_port = unused_port()
        cls.container = f"ainp-nginx-routes-{os.getpid()}-{cls.web_port}"
        try:
            check = subprocess.run(
                [
                    "docker", "run", "--rm",
                    "-v", f"{cls.work / 'nginx.conf'}:/etc/nginx/conf.d/default.conf:ro",
                    "-v", f"{cls.work / 'proxy_params'}:/etc/nginx/proxy_params:ro",
                    NGINX_IMAGE, "nginx", "-t",
                ],
                check=False,
                capture_output=True,
                text=True,
            )
            if check.returncode != 0:
                raise AssertionError(f"nginx -t failed: {check.stderr or check.stdout}")
            run = subprocess.run(
                [
                    "docker", "run", "-d", "--rm",
                    "--name", cls.container,
                    "-p", f"127.0.0.1:{cls.web_port}:8080",
                    "--add-host", "host.docker.internal:host-gateway",
                    "-v", f"{cls.work / 'nginx.conf'}:/etc/nginx/conf.d/default.conf:ro",
                    "-v", f"{cls.work / 'proxy_params'}:/etc/nginx/proxy_params:ro",
                    "-v", f"{cls.html}:/usr/share/nginx/html:ro",
                    NGINX_IMAGE,
                ],
                check=False,
                capture_output=True,
                text=True,
            )
            if run.returncode != 0:
                raise unittest.SkipTest(f"unable to start nginx fixture: {run.stderr or run.stdout}")
            cls._wait_ready()
        except Exception:
            cls.tearDownClass()
            raise

    @classmethod
    def tearDownClass(cls):
        if cls.container:
            subprocess.run(["docker", "rm", "-f", cls.container], check=False, capture_output=True)
        if cls.mock is not None:
            cls.mock.shutdown()
            cls.mock.server_close()
            cls.mock = None
        if cls.work is not None:
            shutil.rmtree(cls.work, ignore_errors=True)
            cls.work = None

    @classmethod
    def _wait_ready(cls):
        deadline = time.time() + 20
        last_error = None
        while time.time() < deadline:
            try:
                with urllib.request.urlopen(cls._url("/healthz"), timeout=1) as response:
                    if response.status == 200:
                        return
            except (urllib.error.URLError, TimeoutError, ConnectionError) as err:
                last_error = err
                time.sleep(0.2)
        raise unittest.SkipTest(f"nginx fixture did not become ready: {last_error}")

    @classmethod
    def _url(cls, path: str) -> str:
        return f"http://127.0.0.1:{cls.web_port}{path}"

    def _request(self, method, path, data=None, headers=None):
        request = urllib.request.Request(self._url(path), data=data, method=method, headers=headers or {})
        try:
            with urllib.request.urlopen(request, timeout=5) as response:
                return response.status, dict(response.headers), response.read()
        except urllib.error.HTTPError as err:
            body = err.read()
            headers = dict(err.headers)
            err.close()
            return err.code, headers, body

    def test_oauth_metadata_is_json_from_api(self):
        status, headers, body = self._request("GET", "/.well-known/oauth-protected-resource")
        self.assertEqual(status, 200)
        self.assertIn("application/json", headers.get("Content-Type", ""))
        payload = json.loads(body)
        self.assertEqual(payload["resource"], "https://ainp.example/mcp")

        status, headers, body = self._request("GET", "/.well-known/oauth-authorization-server")
        self.assertEqual(status, 200)
        payload = json.loads(body)
        self.assertEqual(payload["authorization_endpoint"], "https://ainp.example/oauth/authorize")
        self.assertEqual(payload["token_endpoint"], "https://ainp.example/oauth/token")
        self.assertEqual(payload["registration_endpoint"], "https://ainp.example/oauth/register")
        self.assertEqual(payload["revocation_endpoint"], "https://ainp.example/oauth/revoke")
        self.assertIn("S256", payload["code_challenge_methods_supported"])

    def test_spa_pages_keep_query_and_are_not_proxied(self):
        cases = (
            "/oauth/authorize?client_id=demo+client&redirect_uri=https%3A%2F%2Fapp.example%2Fcb%3Fx%3D1&code_challenge_method=S256",
            "/mcp/confirm?token=opaque-recovery",
            "/setup",
        )
        for path in cases:
            with self.subTest(path=path):
                status, headers, body = self._request("GET", path)
                text = body.decode("utf-8")
                self.assertEqual(status, 200)
                self.assertIn("text/html", headers.get("Content-Type", ""))
                self.assertIn("AINP_ADMIN_SPA", text)
                self.assertNotIn("API 404 page not found", text)

    def test_oauth_protocol_posts_preserve_method_body_and_headers(self):
        cases = (
            ("/oauth/register", b'{"client_name":"codex","redirect_uris":["https://app.example/cb"]}', "application/json"),
            ("/oauth/token", b"grant_type=authorization_code&code=demo", "application/x-www-form-urlencoded"),
            ("/oauth/revoke", b"token=demo-token", "application/x-www-form-urlencoded"),
        )
        for path, data, content_type in cases:
            with self.subTest(path=path):
                status, _, body = self._request(
                    "POST",
                    path,
                    data=data,
                    headers={"Content-Type": content_type, "X-Test-Header": "keep-me"},
                )
                self.assertEqual(status, 200)
                payload = json.loads(body)
                self.assertEqual(payload["reached"], "api")
                self.assertEqual(payload["method"], "POST")
                self.assertEqual(payload["path"], path)
                self.assertEqual(payload["content_type"], content_type)
                self.assertEqual(payload["x_test_header"], "keep-me")
                self.assertEqual(payload["body"], data.decode("utf-8"))

    def test_mcp_unauthenticated_challenge_stays_on_exact_path(self):
        status, headers, _ = self._request("GET", "/mcp")
        self.assertEqual(status, 401)
        self.assertIn(
            'Bearer resource_metadata="https://ainp.example/.well-known/oauth-protected-resource"',
            headers.get("WWW-Authenticate", ""),
        )
        status, _, body = self._request("GET", "/mcp/")
        self.assertEqual(status, 200)
        self.assertIn("AINP_ADMIN_SPA", body.decode("utf-8"))


if __name__ == "__main__":
    unittest.main()
