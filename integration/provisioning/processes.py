"""Reap only descendants of this test, including daemonized runner supervisors."""

import ctypes
import os
from pathlib import Path
import signal
import time


def supervise():
    libc = ctypes.CDLL(None, use_errno=True)
    # PR_SET_CHILD_SUBREAPER affects this process only, not the user session.
    if libc.prctl(36, 1, 0, 0, 0) != 0:
        raise OSError(ctypes.get_errno(), "PR_SET_CHILD_SUBREAPER")


def children():
    path = Path(f"/proc/self/task/{os.getpid()}/children")
    return [int(value) for value in path.read_text().split()]


def reap():
    while True:
        try:
            child, _ = os.waitpid(-1, os.WNOHANG)
        except ChildProcessError:
            return
        if child == 0:
            return


def finish():
    deadline = time.monotonic() + 8
    while time.monotonic() < deadline:
        reap()
        pending = children()
        if not pending:
            return
        remaining = deadline - time.monotonic()
        if remaining < 5:
            for child in pending:
                try:
                    descriptor = os.pidfd_open(child)
                except ProcessLookupError:
                    continue
                try:
                    signal.pidfd_send_signal(descriptor, signal.SIGKILL if remaining < 2 else signal.SIGTERM)
                except ProcessLookupError:
                    pass
                finally:
                    os.close(descriptor)
        time.sleep(0.1)
    reap()
    if children():
        raise RuntimeError("test descendants did not terminate: " + str(children()))
