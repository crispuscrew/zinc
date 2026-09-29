"""Reproduce a namespace join failure without substituting for the zcr test."""

import json

from support import run, save


def diagnose(root, name, namespace, image):
    save(root, "runtime.json", json.loads(run(
        root, "podman", "info", "--format", "{{json .Host.OCIRuntime}}").stdout))
    owner = run(root, "podman", "unshare", "readlink", "/proc/self/ns/user").stdout.strip()
    assert owner == f"user:[{namespace['user']}]", owner
    script = (f"set -eu; test \"$(readlink /proc/self/ns/net)\" = 'net:[{namespace['net']}]'; "
              f"test \"$(readlink /proc/self/ns/user)\" = 'user:[{namespace['user']}]'; "
              "id; readlink /proc/self/ns/net; readlink /proc/self/ns/user; nft -j list ruleset")
    results = {}
    for mode in (f"ns:/proc/{namespace['pid']}/ns/user", "host"):
        # In rootless Podman, this control's 'host' means the Podman userns.
        # The script proves both identities before using nft (read-only here).
        try:
            result = run(root, "podman", "run", "--rm", "--name", name + "-probe",
                         "--pull", "never", "--network", f"ns:/proc/{namespace['pid']}/ns/net",
                         "--userns", mode, "--user", "0", "--security-opt", "no-new-privileges",
                         "--cap-drop", "all", "--cap-add", "NET_ADMIN", image,
                         "sh", "-c", script, check=False)
            results[mode] = {"returncode": result.returncode, "stdout": result.stdout,
                             "stderr": result.stderr}
            print(f"DIAGNOSTIC userns={mode}: exit {result.returncode}\n"
                  f"{result.stdout}{result.stderr}", flush=True)
        finally:
            run(root, "podman", "rm", "-f", "--ignore", name + "-probe")
    save(root, "join-diagnostic.json", results)
