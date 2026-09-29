"""Offline BIOS/TCG fixture matching virtualization/e2e/offline_test.go."""

import hashlib
import os

from support import run, save


def environment(root):
    runtime = root / "run"
    runtime.mkdir(mode=0o700)
    return dict(os.environ, XDG_RUNTIME_DIR=str(runtime), XDG_DATA_HOME=str(root / "data"),
                XDG_CONFIG_HOME=str(root / "config"), ZINC_NETWORK_MANIFEST_DIR=str(root),
                WAYLAND_DISPLAY="", DISPLAY="")


def disk_digest(path):
    return "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()


def fixture(root, binary, env, name, namespace):
    base = root / "base.qcow2"
    run(root, "qemu-img", "create", "-f", "qcow2", str(base), "16M")
    pin = run(root, binary, "pin", str(base), env=env).stdout.strip()
    assert pin == disk_digest(base), "zvr pin differs from independent SHA-256"
    policy = {"Interfaces": [{"ID": "private", "MacAddress": "02:00:00:00:00:31"}],
              "RulesByPriority": None, "DNS": {"ResolversByPriority": None}}
    app = {"SchemaVersion": 4, "Type": "ZincVirtualization", "AppNameID": name,
           "ImageMeta": {"Image": str(base)}, "NetworkMeta": policy,
           "ResourcesMeta": {"MaxRamMiB": 128, "MaxCPUCores": 1},
           "StartConditions": {"LoaderBIOS": True, "ReadOnlyRootfs": True},
           "StopConditions": {"Autorestart": True}, "AudioMeta": {},
           "DisplayMeta": {"DisableGpuAccess": True},
           "RunnerFlags": ["-machine", "q35,accel=tcg", "-cpu", "max"]}
    options = {"Version": 1, "AppNameID": name, "Image": str(base), "BaseDigest": pin,
               "Display": "None", "Devices": "Virtio"}
    manifest = {"version": 1, "app_name_id": name, "generation": name,
                "network_namespace": f"/proc/{namespace['pid']}/ns/net",
                "user_namespace": f"/proc/{namespace['pid']}/ns/user",
                "network_inode": namespace["net"], "user_inode": namespace["user"],
                "packet_preserving": True, "exclusive": True, "static_neighbors": True,
                "complete_inventory": True, "keep_user_id": False, "policy": policy,
                "publications": [], "dns_proxy_addresses": [],
                "topology": {"mode": "tap", "interfaces": [
                    {"interface_id": "private", "device": "tap0",
                     "mac": "02:00:00:00:00:31", "addresses": ["10.203.0.1"]}],
                    "peers": [], "hosts": [], "external_interfaces": []}}
    for path, value in (("app.yaml", app), ("options.json", options), (name + ".json", manifest)):
        save(root, path, value)
        os.chmod(root / path, 0o600)
    return pin
