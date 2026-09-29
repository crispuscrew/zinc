"""Check actual QEMU attachment, private topology, and live/closed nft policy."""

import copy
import os
from pathlib import Path

from qmp import QMP
from support import save


def topology(snapshot):
    value = copy.deepcopy(snapshot)
    for section in ("links", "addresses"):
        for entry in value[section]:
            if entry["ifname"] == "tap0":
                # Attaching a TAP queue changes carrier, not provisioned identity.
                entry.pop("operstate", None)
                entry["flags"] = [flag for flag in entry["flags"] if flag not in ("NO-CARRIER", "LOWER_UP")]
    value["routes"] = [route for route in value["routes"] if not (
        route.get("type") == "multicast" and route.get("dst") == "ff00::/8" and
        route.get("dev") == "tap0" and route.get("table") == "local" and route.get("protocol") == "kernel")]
    return value


def gone(process):
    try:
        status = Path(f"/proc/{process}/stat").read_text()
    except FileNotFoundError:
        return True
    return status.rsplit(")", 1)[1].split()[0] == "Z"


def policy(holder, closed=False):
    entries = holder.request("nft")["nftables"]
    chains = [entry["chain"] for entry in entries if "chain" in entry and
              entry["chain"]["family"] == "inet" and entry["chain"]["table"] == "zinc"]
    hooks = {chain["hook"]: chain for chain in chains if "hook" in chain}
    assert set(hooks) == {"input", "output", "forward"}, hooks
    assert all(chain["policy"] == "drop" for chain in hooks.values()), hooks
    if closed:
        assert len(chains) == 3, "active VM policy survived exit"
        rules = [entry["rule"] for entry in entries if "rule" in entry and
                 entry["rule"]["family"] == "inet" and entry["rule"]["table"] == "zinc"]
        assert len(rules) == 3 and all(rule.get("comment") == "default" and
               {"drop": None} in rule["expr"] for rule in rules), rules
    else:
        assert any(chain["name"] == "owner_forward" for chain in chains), chains
        assert any("chain" in entry and entry["chain"].get("dev") == "tap0"
                   for entry in entries), "TAP ingress guard missing"
    return True


def is_closed(holder):
    try:
        return policy(holder, closed=True)
    except AssertionError:
        return False


def guest(root, name, namespace, attempt):
    runtime = root / "run/zinc/vm"
    process = int((runtime / (name + ".pid")).read_text().strip())
    assert process > 1, process
    proc = Path(f"/proc/{process}")
    argv = proc.joinpath("cmdline").read_bytes().rstrip(b"\0").decode().split("\0")
    status = proc.joinpath("status").read_text()
    save(root, f"vm-process-{attempt}.json", {"pid": process, "argv": argv, "status": status})
    assert Path(argv[0]).name == "qemu-system-x86_64", argv
    assert argv[argv.index("-name") + 1] == name, argv
    assert "tap,id=net0,ifname=tap0,script=no,downscript=no" in argv, argv
    assert "virtio-net-pci,netdev=net0,mac=02:00:00:00:00:31" in argv, argv
    assert "q35,accel=tcg" in argv and "max" in argv, argv
    for forbidden in ("-audiodev", "-tpmdev", "-vnc", "-spice"):
        assert forbidden not in argv, argv
    assert not any("hostfwd=" in arg or arg.startswith("user,id=") for arg in argv), argv
    for kind in ("net", "user"):
        assert os.stat(proc / "ns" / kind).st_ino == namespace[kind], kind
    for field in ("CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"):
        assert field + ":\t0000000000000000" in status, status
    assert "NoNewPrivs:\t1" in status, status
    return process


def attachment(root, name, attempt):
    with QMP(root / "run/zinc/vm" / (name + ".qmp")) as session:
        network = session.monitor("info network")
        devices = session.monitor("info qtree")
        observations = {"status": session.query("query-status"),
                        "kvm": session.query("query-kvm"),
                        "memory": session.query("query-memory-size-summary"),
                        "blocks": session.query("query-block"),
                        "network": network, "devices": devices}
    save(root, f"qmp-{attempt}.json", observations)
    assert observations["status"]["running"], observations["status"]
    assert not observations["kvm"]["enabled"], observations["kvm"]
    assert observations["memory"]["base-memory"] == 128 * 1024 * 1024
    assert len(observations["blocks"]) == 1 and observations["blocks"][0]["inserted"]["ro"]
    for fragment in ("net0", "type=tap", "ifname=tap0", "macaddr=02:00:00:00:00:31"):
        assert fragment in network, network
    for fragment in ("virtio-net-pci", 'netdev = "net0"', 'mac = "02:00:00:00:00:31"'):
        assert fragment in devices, fragment
