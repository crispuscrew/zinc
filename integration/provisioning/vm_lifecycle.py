"""Exercise the real zvr namespace wrapper and persistent VM supervisor."""

import json

from qmp import QMP
from support import eventually, run, save
from vm_checks import attachment, gone, guest, is_closed, policy, topology
from vm_fixture import disk_digest


def exercise(root, binary, env, name, holder, namespace, pin):
    argv = [binary, "run", str(root / "app.yaml"), "--runtime-options", str(root / "options.json")]
    plan = run(root, *argv, "--dry-run", env=env).stdout
    (root / "vm-plan.txt").write_text(plan)
    for fragment in ("nsenter", "--preserve-credentials", "setpriv", "--bounding-set=-all",
                     f"net:[{namespace['net']}]", f"user:[{namespace['user']}]",
                     "tap,id=net0,ifname=tap0,script=no,downscript=no", "4 raw RunnerFlags omitted from plan"):
        assert fragment in plan, fragment
    assert not (root / "data").exists(), "dry-run created VM state"
    assert not any("table" in entry for entry in holder.request("nft")["nftables"])
    print("PASS: VM LoadFile/ConfigureResolved, explicit path, pinned disk and TAP plan", flush=True)
    counter_failures = []
    runtime = root / "run/zinc/vm"
    for attempt in range(1, 4):
        run(root, *argv, env=env)
        process = guest(root, name, namespace, attempt)
        attachment(root, name, attempt)
        policy(holder)
        save(root, f"vm-active-{attempt}.json", holder.request("nft"))
        save(root, f"tap-active-{attempt}.json", holder.request("tap"))
        supervisor = json.loads((runtime / (name + ".supervisor.json")).read_text())
        save(root, f"vm-supervisor-{attempt}.json", supervisor)
        counters = run(root, binary, "net", str(root / "app.yaml"), "--json", env=env, check=False)
        if counters.returncode:
            counter_failures.append(counters.stderr)
            print("COUNTERS FAILED: " + counters.stderr, flush=True)
        else:
            report = json.loads(counters.stdout)
            assert report["posture"] == "filtered" and report["counters"], report
            save(root, f"vm-counters-{attempt}.json", report)
        print(f"PASS: VM startup {attempt}, QMP TAP/device, namespace inodes, zero caps, policy", flush=True)
        if attempt == 1:
            with QMP(runtime / (name + ".qmp")) as session:
                session.query("quit")
        else:
            run(root, binary, "stop", name, "--force", env=env)
        eventually(lambda: all(not (runtime / (name + suffix)).exists()
                   for suffix in (".pid", ".qmp", ".supervisor.json")), "VM supervisor cleanup")
        eventually(lambda: gone(process) and gone(supervisor["PID"]), "VM and supervisor exit")
        eventually(lambda: is_closed(holder), "VM wrapper closes nft")
        save(root, f"vm-closed-{attempt}.json", holder.request("nft"))
        snapshot = holder.request("snapshot")
        save(root, f"namespace-exit-{attempt}.json", snapshot)
        assert topology(snapshot) == topology(namespace), "provisioned TAP topology changed; see namespace-exit snapshot"
        details = holder.request("tap")
        save(root, f"tap-exit-{attempt}.json", details)
        assert details["details"][0]["linkinfo"]["info_data"]["persist"], "TAP was not retained"
        assert disk_digest(root / "base.qcow2") == pin, "base disk changed"
        save(root, f"vm-exit-{attempt}.json", {"guest_pid": process, "mode": "QMP quit" if attempt == 1 else "force stop"})
        print(f"PASS: VM {'QMP exit' if attempt == 1 else 'stop'} cleanup {attempt}", flush=True)
    save(root, "vm-lifecycle.json", {"launches": 3, "exits": 3, "counter_failures": counter_failures})
    if counter_failures:
        raise RuntimeError("VM lifecycle passed but production counters failed: " + counter_failures[0])
