"""Own a disconnected network namespace for the lifetime of the stdin pipe."""

import json
import os
import subprocess
import sys


def command(*argv):
    return subprocess.check_output(argv, text=True, timeout=10)


def snapshot():
    return {
        "pid": os.getpid(),
        "net": os.stat("/proc/self/ns/net").st_ino,
        "user": os.stat("/proc/self/ns/user").st_ino,
        "links": json.loads(command("ip", "-j", "link", "show")),
        "addresses": json.loads(command("ip", "-j", "address", "show")),
        "routes": json.loads(command("ip", "-j", "route", "show", "table", "all")),
    }


def main():
    # The outer harness checks both namespace identities before permitting setup.
    print(json.dumps(snapshot()), flush=True)
    operation = sys.stdin.readline().strip()
    if operation not in ("provision", "provision-tap"):
        return
    device = "tap0" if operation == "provision-tap" else "dummy0"
    if device == "tap0":
        command("ip", "tuntap", "add", "dev", device, "mode", "tap", "user", "0")
    else:
        command("ip", "link", "add", device, "type", "dummy")
    command("ip", "link", "set", device, "address", "02:00:00:00:00:31")
    command("ip", "link", "set", device, "arp", "off", "addrgenmode", "none")
    command("ip", "address", "add", "10.203.0.1/32", "dev", device)
    command("ip", "link", "set", device, "up")
    print(json.dumps(snapshot()), flush=True)
    for request in sys.stdin:
        operation = request.strip()
        if operation == "snapshot":
            result = snapshot()
        elif operation == "nft":
            result = json.loads(command("nft", "-j", "list", "ruleset"))
        elif operation == "tap":
            result = {"details": json.loads(command("ip", "-d", "-j", "link", "show", "dev", "tap0")),
                      "neighbors": json.loads(command("ip", "-j", "neighbor", "show"))}
        else:
            raise ValueError("unsupported holder operation: " + operation)
        print(json.dumps(result), flush=True)


if __name__ == "__main__":
    main()
