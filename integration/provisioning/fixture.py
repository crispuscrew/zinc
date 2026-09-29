"""Canonical version-4 app plus the production provisioner manifest contract."""

import os

from support import save


def write_fixture(root, name, image, namespace, duration=10):
    policy = {"Interfaces": [{"ID": "private", "MacAddress": "02:00:00:00:00:31"}],
              "RulesByPriority": None, "DNS": {"ResolversByPriority": None}}
    script = ("printf 'PROVISION_READY\\n'; id; readlink /proc/self/ns/net; "
              "readlink /proc/self/ns/user; "
              "grep -E '^(Cap(Inh|Prm|Eff|Bnd|Amb)|NoNewPrivs):' /proc/self/status; "
              f"sleep {duration}; printf 'PROVISION_COMPLETE\\n'")
    app = {"SchemaVersion": 4, "Type": "ZincContainer", "AppNameID": name,
           "ImageMeta": {"Image": image}, "NetworkMeta": policy,
           "InternalUserMeta": {"UseNonRootUser": True, "NonRootUserName": "nobody"},
           "DisplayMeta": {"DisableGpuAccess": True}, "AudioMeta": {},
           "StartConditions": {"Entrypoint": script, "ReadOnlyRootfs": True},
           "StopConditions": {"Background": True}}
    manifest = {"version": 1, "app_name_id": name, "generation": name,
                "network_namespace": f"/proc/{namespace['pid']}/ns/net",
                "user_namespace": f"/proc/{namespace['pid']}/ns/user",
                "network_inode": namespace["net"], "user_inode": namespace["user"],
                "packet_preserving": True, "exclusive": True, "static_neighbors": True,
                "complete_inventory": True, "keep_user_id": False, "policy": policy,
                "topology": {"mode": "container", "interfaces": [
                    {"interface_id": "private", "device": "dummy0",
                     "mac": "02:00:00:00:00:31", "addresses": ["10.203.0.1"]}]}}
    save(root, "app.yaml", app)  # JSON is a YAML subset accepted by the real loader.
    save(root, name + ".json", manifest)
    os.chmod(root / "app.yaml", 0o600)
    os.chmod(root / (name + ".json"), 0o600)
    return manifest


def check_policy(value, closed=False):
    entries = value["nftables"]
    chains = [entry["chain"] for entry in entries if "chain" in entry]
    hooks = {chain["hook"]: chain for chain in chains if "hook" in chain}
    assert set(hooks) == {"input", "output", "forward"}, hooks
    assert all(chain["policy"] == "drop" for chain in hooks.values()), hooks
    if closed:
        assert len(chains) == 3, "teardown did not replace the active policy"
        rules = [entry["rule"] for entry in entries if "rule" in entry]
        assert len(rules) == 3, rules
        assert all(rule.get("comment") == "default" and
                   {"drop": None} in rule["expr"] for rule in rules), rules
    else:
        assert any(chain["name"] == "owner_output" for chain in chains), chains
