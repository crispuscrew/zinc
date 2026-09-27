"""Small, dependency-free command and evidence helpers."""

import json
import os
from pathlib import Path
import select
import shlex
import signal
import subprocess
import time


def save(root, name, value):
    (root / name).write_text(json.dumps(value, indent=2) + "\n")


def run(root, *argv, env=None, check=True, timeout=30):
    result = subprocess.run(argv, env=env, text=True, capture_output=True, timeout=timeout)
    with (root / "commands.jsonl").open("a") as stream:
        stream.write(json.dumps({"argv": argv, "returncode": result.returncode,
                                 "stdout": result.stdout, "stderr": result.stderr}) + "\n")
    if check and result.returncode:
        raise RuntimeError(f"{shlex.join(argv)}\nexit {result.returncode}\n"
                           f"{result.stdout}{result.stderr}")
    return result


def inventory(root):
    commands = {
        "links": ["ip", "-j", "link", "show"],
        "addresses": ["ip", "-j", "address", "show"],
        "routes4": ["ip", "-j", "-4", "route", "show", "table", "all"],
        "routes6": ["ip", "-j", "-6", "route", "show", "table", "all"],
        "rules4": ["ip", "-j", "-4", "rule", "show"],
        "rules6": ["ip", "-j", "-6", "rule", "show"],
        "containers": ["podman", "ps", "-a", "--format", "{{.ID}} {{.Names}} {{.State}}"],
        "pods": ["podman", "pod", "ps", "--format", "{{.ID}} {{.Name}} {{.Status}}"],
        "networks": ["podman", "network", "ls", "--format", "{{.ID}} {{.Name}}"],
        "volumes": ["podman", "volume", "ls", "--format", "{{.Name}}"],
        "images": ["podman", "images", "--no-trunc", "--format", "{{.ID}} {{.Repository}}:{{.Tag}}"],
    }
    result = {key: run(root, *argv).stdout for key, argv in commands.items()}
    addresses = json.loads(result["addresses"])
    for interface in addresses:
        for address in interface["addr_info"]:
            address.pop("valid_life_time", None)
            address.pop("preferred_life_time", None)
    result["addresses"] = addresses  # DHCP lease countdown is not a configuration change.
    firewall = run(root, "nft", "-j", "list", "ruleset", check=False)
    result["firewall"] = {"returncode": firewall.returncode, "stdout": firewall.stdout,
                          "stderr": firewall.stderr}
    result["namespaces"] = {kind: os.stat("/proc/self/ns/" + kind).st_ino
                            for kind in ("net", "user")}
    return result


class Holder:
    def __init__(self, root):
        self.log = (root / "holder.stderr").open("w")
        self.process = subprocess.Popen(
            ["podman", "unshare", "unshare", "--net", "python3", "-u",
             str(Path(__file__).with_name("holder.py"))],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.log, text=True,
            start_new_session=True)

    def receive(self):
        ready, _, _ = select.select([self.process.stdout], [], [], 15)
        if not ready:
            raise RuntimeError("holder response timed out; see holder.stderr")
        line = self.process.stdout.readline()
        if not line:
            raise RuntimeError("holder exited; see holder.stderr")
        return json.loads(line)

    def request(self, operation):
        self.process.stdin.write(operation + "\n")
        self.process.stdin.flush()
        return self.receive()

    def close(self):
        self.process.stdin.close()
        try:
            self.process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(self.process.pid, signal.SIGTERM)
            self.process.wait(timeout=10)
        self.log.close()


def eventually(predicate, description, seconds=25):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if predicate():
            return
        time.sleep(0.2)
    raise AssertionError("timeout: " + description)
