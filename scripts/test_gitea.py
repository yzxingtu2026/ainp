import json
import sys
import unittest
from unittest import mock

from scripts.deployment import gitea


class GiteaTest(unittest.TestCase):
    def test_credential_uses_server_scheme(self):
        result = mock.Mock(stdout="username=ci\npassword=secret\n")
        with mock.patch.object(gitea.subprocess, "run", return_value=result) as run:
            self.assertEqual(gitea._credential("http://gitea.example"), ("ci", "secret"))
        self.assertIn("protocol=http", run.call_args.kwargs["input"])

    def test_release_write_methods_use_gitea_endpoints(self):
        client = gitea.Client.__new__(gitea.Client)
        client.server, client.auth = "http://gitea.example", None
        release = {"id": 7, "draft": True}
        responses = [json.dumps(release).encode(), json.dumps(release).encode(), json.dumps({"id": 8}).encode(), json.dumps({"id": 7, "draft": False}).encode()]
        with mock.patch.object(client, "_request", side_effect=responses) as request:
            self.assertEqual(client.ensure_draft_release("owner/repo", "v1.2.3", "v1.2.3", "notes")["id"], 7)
            client.upload_release_asset("owner/repo", 7, "reference.json", b"{}")
            client.publish_release("owner/repo", 7)
        self.assertEqual(request.call_args_list[0].args[:2], ("GET", "/api/v1/repos/owner/repo/releases/tags/v1.2.3"))
        self.assertEqual(request.call_args_list[1].args[0], "PATCH")
        self.assertEqual(request.call_args_list[2].args[0], "POST")
        self.assertEqual(request.call_args_list[3].args[0], "PATCH")


if __name__ == "__main__":
    unittest.main()
