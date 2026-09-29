"""Failure cleanup is restricted to identities reserved by this test."""

import json
import os
from pathlib import Path

from processes import finish
from support import run


def cleanup(root, name, holder, namespace, reserved):
    errors = []

    def attempt(action):
        try:
            action()
        except Exception as error:
            errors.append(repr(error))

    if reserved:
        for argv in (("pod", "rm", "-f", "--ignore", name + "-pod"),
                     ("rm", "-f", "--ignore", name),
                     ("rm", "-f", "--ignore", name + "-probe")):
            attempt(lambda argv=argv: run(root, "podman", *argv))
        if namespace:
            attempt(lambda: remove_helpers(root, namespace))
    if holder:
        attempt(holder.close)
    attempt(finish)
    if reserved:
        lock = Path(os.environ["XDG_RUNTIME_DIR"]) / "zinc/run" / ("launch-" + name + ".lock")
        attempt(lambda: lock.unlink(missing_ok=True))
    return errors


def remove_helpers(root, namespace):
    identifiers = run(root, "podman", "ps", "-a", "--no-trunc", "--format", "{{.ID}}").stdout.split()
    if not identifiers:
        return
    containers = json.loads(run(root, "podman", "inspect", *identifiers).stdout)
    target = f"ns:/proc/{namespace['pid']}/ns/net"
    for container in containers:
        # Anonymous --rm helpers can survive a timed-out CLI. Never remove by image.
        if container["HostConfig"]["NetworkMode"] == target:
            run(root, "podman", "rm", "-f", "--ignore", container["Id"])
