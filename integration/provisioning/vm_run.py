"""Rootless positive TAP provisioning test; production VM code stays read-only."""

import hashlib
import os
from pathlib import Path
import shutil
import signal
import tempfile
import traceback
import uuid

from processes import finish, supervise
from run import interrupted
from support import Holder, inventory, run, save
from vm_fixture import environment, fixture
from vm_lifecycle import exercise


def cleanup(root, binary, env, name, holder):
    errors = []
    runtime = root / "run/zinc/vm"
    try:
        if any((runtime / (name + suffix)).exists() for suffix in (".pid", ".supervisor.json")):
            run(root, binary, "stop", name, "--force", env=env)
    except Exception as error:
        errors.append(repr(error))
    if holder:
        try:
            holder.close()
        except Exception as error:
            errors.append(repr(error))
    try:
        finish()
    except Exception as error:
        errors.append(repr(error))
    return errors


def main():
    if not __debug__:
        raise RuntimeError("Python optimization disables assertions; refusing the test")
    assert os.getuid() != 0, "run outside podman unshare as the ordinary rootless user"
    supervise()
    os.umask(0o077)
    root = Path(tempfile.mkdtemp(prefix="zinc-vm-provisioning-", dir="/tmp/opencode"))
    print("VM evidence: " + str(root), flush=True)
    repo = Path(__file__).resolve().parents[2]
    binary = str(repo / "virtualization/runner/bin/zvr")
    name = "vm-tap-" + uuid.uuid4().hex[:12]
    env = environment(root)
    for location in (repo, root):
        assert shutil.disk_usage(location).free > 1024 ** 3, "need at least 1 GiB free"
    assert run(root, "podman", "info", "--format", "{{.Host.Security.Rootless}}").stdout.strip() == "true"
    schema = "github.com/crispuscrew/zinc/common/domain/schema/schema.go"
    assert (repo / "common/domain/schema/schema.go").read_bytes() == (
        repo / "virtualization/runner/vendor" / schema).read_bytes(), "vendored schema mismatch"
    save(root, "vm-inputs.json", {"name": name, "binary_sha256": hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                                 "qemu": run(root, "qemu-system-x86_64", "--version").stdout})
    before = inventory(root)
    save(root, "before.json", before)
    holder, failure = None, None
    for signum in (signal.SIGINT, signal.SIGTERM):
        signal.signal(signum, interrupted)
    try:
        holder = Holder(root)
        initial = holder.receive()
        save(root, "namespace-initial.json", initial)
        assert all(initial[kind] != before["namespaces"][kind] for kind in ("net", "user"))
        assert [link["ifname"] for link in initial["links"]] == ["lo"]
        namespace = holder.request("provision-tap")
        save(root, "namespace.json", namespace)
        details = holder.request("tap")
        save(root, "tap-initial.json", details)
        attachment = details["details"][0]["linkinfo"]
        assert attachment["info_kind"] == "tun" and attachment["info_data"]["type"] == "tap", details
        assert attachment["info_data"]["persist"] and attachment["info_data"]["user"] in ("root", "0", 0), details
        assert not details["neighbors"], "unexpected peers"
        assert {link["ifname"] for link in namespace["links"]} == {"lo", "tap0"}
        assert all(route.get("type") in ("local", "multicast") and
                   route.get("dev") == "tap0" for route in namespace["routes"]), namespace["routes"]
        pin = fixture(root, binary, env, name, namespace)
        exercise(root, binary, env, name, holder, namespace, pin)
    except Exception as error:
        failure = repr(error)
        (root / "failure.txt").write_text(traceback.format_exc())
        print("FAIL: " + failure, flush=True)
        if holder:
            print((root / "holder.stderr").read_text(), flush=True)
    finally:
        cleanup_errors = cleanup(root, binary, env, name, holder)
        after = inventory(root)
        save(root, "after.json", after)
        changed = [key for key in before if before[key] != after[key]]
        if cleanup_errors or changed:
            failure = failure or "VM cleanup/inventory assertion failed"
        save(root, "result.json", {"status": "FAIL" if failure is not None else "PASS",
                                   "failure": failure, "cleanup_errors": cleanup_errors,
                                   "inventory_changed": changed})
        print("VM inventory changed: " + str(changed), flush=True)
    if failure is not None:
        raise SystemExit(1)
    print("PASS: real VM TAP attachment, exit/stop cleanup, relaunch, outer inventory", flush=True)


if __name__ == "__main__":
    main()
