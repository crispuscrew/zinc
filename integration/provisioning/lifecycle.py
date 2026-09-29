"""Observe the actual runner, containers, supervisor, and resulting nft policy."""

import json

from fixture import check_policy, write_fixture
from support import eventually, run, save


def absent(root, name, kind):
    result = run(root, "podman", kind, "exists", name, check=False)
    assert result.returncode in (0, 1), result.stderr
    return result.returncode == 1


def exercise(root, binary, env, name, holder, namespace, image):
    manifest = write_fixture(root, name, image, namespace)
    run(root, binary, "validate", str(root / "app.yaml"), env=env)
    plan = run(root, binary, "run", str(root / "app.yaml"), env=env).stdout
    (root / "plan.txt").write_text(plan)
    for fragment in ("--network ns:" + manifest["network_namespace"],
                     "--userns host", "--cap-add NET_ADMIN",
                     "--dns none", "--cap-drop all", "--pull never"):
        assert fragment in plan, fragment
    assert not any("table" in entry for entry in holder.request("nft")["nftables"]), "namespace not initially empty"
    print("PASS: explicit app path and real manifest LoadFile/Resolve", flush=True)
    for attempt in range(1, 4):
        duration = 120 if attempt == 3 else 10
        write_fixture(root, name, image, namespace, duration)
        run(root, binary, "run", str(root / "app.yaml"), "--exec", env=env)
        eventually(lambda: running(root, name), "app startup")
        inspect = json.loads(run(root, "podman", "inspect", name).stdout)[0]
        save(root, f"app-{attempt}.json", inspect)
        pod = json.loads(run(root, "podman", "pod", "inspect", name + "-pod").stdout)[0]
        save(root, f"pod-{attempt}.json", pod)
        assert not inspect["HostConfig"]["PortBindings"], "app published ports"
        assert not pod["InfraConfig"]["PortBindings"], "pod published ports"
        eventually(lambda: "NoNewPrivs:" in run(root, "podman", "logs", name).stdout,
                   "app namespace and capability report")
        logs = run(root, "podman", "logs", name).stdout
        assert "PROVISION_READY" in logs, logs
        assert f"net:[{namespace['net']}]" in logs, logs
        assert f"user:[{namespace['user']}]" in logs, logs
        assert "uid=65534(nobody)" in logs, logs
        for field in ("CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"):
            assert field + ":\t0000000000000000" in logs, logs
        assert "NoNewPrivs:\t1" in logs, logs
        active = holder.request("nft")
        save(root, f"active-{attempt}.json", active)
        check_policy(active)
        counters = json.loads(run(root, binary, "net", str(root / "app.yaml"), "--json", env=env).stdout)
        save(root, f"counters-{attempt}.json", counters)
        assert counters["posture"] == "filtered" and counters["counters"], counters
        print(f"PASS: startup {attempt}, joined net/user inodes, zero caps, nft installed", flush=True)
        if attempt == 3:
            run(root, binary, "stop", str(root / "app.yaml"), env=env)
        else:
            assert run(root, "podman", "wait", name, timeout=20).stdout.strip() == "0"
        eventually(lambda: absent(root, name, "container") and
                   absent(root, name + "-pod", "pod"), "supervisor cleanup")
        eventually(lambda: closed(holder), "closed nft policy")
        save(root, f"closed-{attempt}.json", holder.request("nft"))
        assert holder.request("snapshot") == namespace, "provisioned topology changed"
        print(f"PASS: {'stop' if attempt == 3 else 'natural-exit'} cleanup {attempt}", flush=True)


def running(root, name):
    result = run(root, "podman", "inspect", "--format", "{{.State.Running}}", name, check=False)
    return result.returncode == 0 and result.stdout.strip() == "true"


def closed(holder):
    try:
        check_policy(holder.request("nft"), closed=True)
        return True
    except AssertionError:
        return False
