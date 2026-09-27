"""Safe deployment stages; raw command output stays in a root-only host log."""
from contextlib import contextmanager
import os
import subprocess
import sys
import threading
import time

INTERVAL = 15
LOG_PATH = "/var/log/ainp-bootstrap.log"


def report(message):
    print(f"AINP_PROGRESS {time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())} {message}", flush=True)


@contextmanager
def stage(label):
    # Labels are fixed source-code literals, never config, argv, URLs or exceptions.
    started = time.monotonic()
    done = threading.Event()

    def heartbeat():
        while not done.wait(INTERVAL):
            report(f"{label}: running ({int(time.monotonic() - started)}s)")

    report(f"{label}: started")
    thread = threading.Thread(target=heartbeat, daemon=True)
    thread.start()
    try:
        yield
    except BaseException as error:
        done.set()
        thread.join()
        report(f"{label}: failed ({type(error).__name__})")
        raise
    else:
        done.set()
        thread.join()
        report(f"{label}: done ({int(time.monotonic() - started)}s)")


def run(label, argv):
    with stage(label):
        fd = os.open(LOG_PATH, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
        with os.fdopen(fd, "wb") as output:
            os.fchmod(output.fileno(), 0o600)
            output.write((label + "\n").encode())
            output.flush()
            # The bootstrap shell keeps fd 9 open for the lifetime of its lock.
            result = subprocess.run(argv, stdout=output, stderr=subprocess.STDOUT,
                                    pass_fds=(9,) if label == "bootstrap-lock" else ())
        if result.returncode:
            report(f"{label}: exit {result.returncode}. Inspect protected bootstrap log.")
            raise SystemExit(result.returncode if result.returncode > 0 else 128 - result.returncode)


if __name__ == "__main__":
    run(sys.argv[1], sys.argv[2:])
