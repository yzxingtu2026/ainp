#!/usr/bin/env python3
"""systemd launcher: JSON secrets are data, never shell/systemd interpolation."""
import json
import os
from pathlib import Path
import sys


def main():
    config_path, role, release = sys.argv[1:4]
    config = json.loads(Path(config_path).read_text())
    env = {"PATH": "/usr/bin:/bin", "HOME": config["home"], "LANG": "C.UTF-8"}
    env.update(config["env"])
    env["LOG_SERVICE"] = role
    env["LOG_FILE_PATH"] = config["home"] + "/logs/" + role + ".jsonl"
    if role in ("worker", "ai-worker"):
        env["WORKER_QUEUES"] = "ai=1" if role == "ai-worker" else "critical=6,default=3,file=2,notification=2"
        env["WORKER_CONCURRENCY"] = str(config["ai_concurrency"] if role == "ai-worker" else config["worker_concurrency"])
    executable = str(Path(release) / "bin" / ("worker" if role == "ai-worker" else role))
    argv = [executable, "-config", str(Path(config_path).with_name("app.json")), *sys.argv[4:]]
    os.execve(executable, argv, env)


if __name__ == "__main__":
    main()
