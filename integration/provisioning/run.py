"""Run only as the ordinary rootless Podman user, outside podman unshare."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import tempfile
import traceback
import uuid

from lifecycle import exercise
from diagnose import diagnose
from cleanup import cleanup
from processes import supervise
from support import Holder, inventory, run, save


def interrupted(signum, frame):
    raise RuntimeError(f"interrupted by signal {signum}")


def main():
    if not __debug__:
        raise RuntimeError("Python optimization disables assertions; refusing the test")
    assert os.getuid() != 0, "run as the ordinary rootless Podman user"
    supervise()
    os.umask(0o077)
    root = Path(tempfile.mkdtemp(prefix="zinc-provisioning-", dir="/tmp/opencode"))
    print("Evidence: " + str(root), flush=True)
    repo = Path(__file__).resolve().parents[2]
    binary = str(repo / "container/runner/bin/zcr")
    name = "provision-" + uuid.uuid4().hex[:12]
    for location in (repo, root, Path(os.environ["XDG_RUNTIME_DIR"])):
        assert shutil.disk_usage(location).free > 1024 ** 3, "need at least 1 GiB free"
    assert run(root, "podman", "info", "--format", "{{.Host.Security.Rootless}}").stdout.strip() == "true"
    digest = hashlib.sha256(Path(binary).read_bytes()).hexdigest()
    schema = "github.com/crispuscrew/zinc/common/domain/schema/schema.go"
    assert (repo / "common/domain/schema/schema.go").read_bytes() == (
        repo / "container/runner/vendor" / schema).read_bytes(), "vendored schema mismatch"
    image_info = json.loads(run(root, "podman", "image", "inspect", "localhost/zinc/e2e-app:local").stdout)[0]
    image = "localhost/zinc/e2e-app@" + image_info["Digest"]
    run(root, "podman", "image", "exists", image)
    helper = json.loads(run(root, "podman", "image", "inspect", "localhost/zinc/netfilter:local").stdout)[0]
    save(root, "inputs.json", {"name": name, "binary_sha256": digest, "image": image,
                              "image_id": image_info["Id"], "helper_id": helper["Id"]})
    before = inventory(root)
    save(root, "before.json", before)
    holder = None
    namespace = None
    failure = None
    reserved = False
    for signum in (signal.SIGINT, signal.SIGTERM):
        signal.signal(signum, interrupted)
    try:
        assert run(root, "podman", "container", "exists", name, check=False).returncode == 1
        assert run(root, "podman", "container", "exists", name + "-probe", check=False).returncode == 1
        assert run(root, "podman", "pod", "exists", name + "-pod", check=False).returncode == 1
        reserved = True
        holder = Holder(root)
        initial = holder.receive()
        save(root, "namespace-initial.json", initial)
        assert initial["net"] != before["namespaces"]["net"]
        assert initial["user"] != before["namespaces"]["user"]
        assert [link["ifname"] for link in initial["links"]] == ["lo"]
        namespace = holder.request("provision")
        save(root, "namespace.json", namespace)
        assert {link["ifname"] for link in namespace["links"]} == {"lo", "dummy0"}
        assert all(route.get("type") in ("local", "multicast") and
                   route.get("dev") == "dummy0" for route in namespace["routes"]), namespace["routes"]
        env = dict(os.environ, ZINC_NETWORK_MANIFEST_DIR=str(root), WAYLAND_DISPLAY="",
                   XDG_CONFIG_HOME=str(root / "config"), XDG_STATE_HOME=str(root / "state"),
                   ZINC_NETFILTER_IMAGE="localhost/zinc/netfilter@" + helper["Digest"])
        exercise(root, binary, env, name, holder, namespace, image)
    except Exception as error:
        failure = repr(error)
        (root / "failure.txt").write_text(traceback.format_exc())
        print("FAIL: " + failure, flush=True)
        if namespace and "setns to target user namespace" in failure:
            diagnose(root, name, namespace, "localhost/zinc/netfilter@" + helper["Digest"])
    finally:
        cleanup_errors = cleanup(root, name, holder, namespace, reserved)
        after = inventory(root)
        save(root, "after.json", after)
        changed = [key for key in before if before[key] != after[key]]
        print("Inventory changed: " + str(changed), flush=True)
        if cleanup_errors or changed:
            failure = failure or "cleanup/inventory assertion failed"
        save(root, "result.json", {"status": "FAIL" if failure is not None else "PASS",
                                   "failure": failure, "cleanup_errors": cleanup_errors,
                                   "inventory_changed": changed})
    if failure is not None:
        raise SystemExit(1)
    print("PASS: provisioning, rerun, stop, and outer inventory", flush=True)


if __name__ == "__main__":
    main()
