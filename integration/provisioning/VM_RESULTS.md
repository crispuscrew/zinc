# Positive production VM TAP provisioning: PASS

## Run and evidence

- Date: 2026-09-28; ordinary rootless UID 1000.
- Kernel `7.1.5-100.fc43.x86_64`; Podman `5.8.4`; QEMU `10.1.5`.
- Evidence: `/tmp/opencode/zinc-vm-provisioning-zirbgk7s/`.
- App: `vm-tap-98ad1883a710`; holder PID `2768068`.
- Provisioned namespace inodes: net `4026533636`, user `4026533112`.
- Fresh `virtualization/runner/bin/zvr` SHA-256:
  `6a4830445b4ee2c7c917933794f1eae69e401f2c93905c521fb152b627b71ac6`.
- Blank base disk verified pin:
  `sha256:a868962514df6e6678042c72d4eebca7b842a2e4edd1df7e00bdf626a6fac15c`.
- Free space before build: workspace 33 GiB, `/tmp` 24 GiB.

`make -C integration/provisioning lint test-vm` exited zero. The final
`result.json` records `status: PASS`, `failure: null`, `cleanup_errors: []`,
and `inventory_changed: []`. `vm-lifecycle.json` records three launches,
three exits, and no counter failures.

## Production boundary actually exercised

The test uses an explicit app filepath and explicit runtime-options filepath.
Owner-only network manifests point to `/proc/<holder>/ns/{net,user}` and bind
both observed inode identities. The canonical schema matches the runner's
current vendor copy. No renderer substitution or mocked process is involved.

| Check | Actual result |
| --- | --- |
| Real LoadFile, manifest binding, ConfigureResolved and TAP argv | PASS |
| Production nsenter/RunScript nft load before QEMU | PASS |
| setpriv, zero Inh/Prm/Eff/Bnd/Amb capabilities, NoNewPrivs=1 | PASS, all launches |
| QEMU net/user inodes equal the manifest | PASS, all launches |
| QMP live TAP netdev and virtio NIC with exact MAC | PASS, all launches |
| Headless BIOS, 128 MiB, TCG, read-only qcow2 root | PASS |
| `zvr net <absolute-filepath> --json` reports filtered live counters | PASS, all launches |
| QMP quit followed by VM/wrapper/supervisor exit and policy closure | PASS |
| Manual force stop, same-TAP relaunch, second force stop | PASS |
| Guest PID/QMP/supervisor state removed; base checksum unchanged | PASS, all exits |
| TAP identity/address/configured topology preserved between launches | PASS |
| Outer host/Podman inventories match; cleanup has no errors | PASS |

QMP `info network` in the first live VM reported:

```text
virtio-net-pci.0: index=0,type=nic,model=virtio-net-pci,macaddr=02:00:00:00:00:31
 \ net0: index=0,type=tap,ifname=tap0,script=no,downscript=no
```

`info qtree` independently reported the virtio network device, `netdev = "net0"`,
and `mac = "02:00:00:00:00:31"`. QMP reported `running: true`,
`base-memory: 134217728`, `kvm.enabled: false`, and root block `ro: true`.

The observed QEMU command used:

```text
-netdev tap,id=net0,ifname=tap0,script=no,downscript=no
-device virtio-net-pci,netdev=net0,mac=02:00:00:00:00:31
-display none
-machine q35,accel=tcg -cpu max
```

The TCG pair is the exact trailing override used by
`virtualization/e2e/offline_test.go`; production's earlier KVM defaults remain
visible in argv, while QMP confirms KVM is disabled. Production dry-run output
deliberately redacts the four raw RunnerFlags, so TCG is verified against the
live argv/QMP observations, not inferred from the redacted plan.

## Namespace and lifecycle details

`ip tuntap add dev tap0 mode tap user 0` ran only inside the holder's netns.
Rtnetlink confirmed TAP type, persistence, ownership, and exact MAC. ARP and
IPv6 address generation were disabled, and the private TAP carried only the
test address `10.203.0.1/32`. There are no peers or dynamic neighbors; the
manifest's static-neighbor inventory is empty. No guest OS/IP setup is claimed.

The active ruleset contained `owner_forward` and the TAP netdev ingress guard.
After every exit, `inet zinc` contained exactly the three default-drop hooks
and their drop rules. The existing `netdev zinc_link` MAC/L2 ingress guard
remained, as production's closure script replaces only the inet table. The
entire ruleset disappears with the test network namespace at final cleanup.

TAP queue attachment changes carrier/operstate and causes the kernel to add a
namespace-local IPv6 multicast route. These operational differences are saved
in raw snapshots and excluded only from the private TAP topology comparison.
Namespace inodes, ifindex, MAC, assigned address, administrative flags, other
routes, and persistence remain checked. Outer inventories are not normalized
for these TAP-specific changes and matched exactly (apart from the existing
DHCP lifetime countdown exclusion).

## Checks, cleanup, and scope

- Existing Makefile offline/no-cache `zvr` build: PASS; no image pull.
- Python syntax check and live `test-vm`: PASS.
- No VM production, schema, common, vendor, dependency, or commit changes.
- No sudo, host links/routes/firewall changes, real uplink, outward ports,
  peer app, guest OS installation, cloud-init, TPM, or audio deployment.
- All test descendants exited/reaped; the test network namespace and TAP were
  released. The temporary build image was removed. Private disks and logs are
  retained as evidence, with no running VM resources.
- Host nft reads require privileges and were denied; host firewall contents
  were not independently compared. The test issued no host firewall mutation.
- This verifies production provisioning/attachment and lifecycle. It does not
  replace the separate 256-assertion packet-policy suite or prove guest OS boot.

Earlier harness-only failures are retained separately: `ip tuntap show` reads
the caller's sysfs mount and could not inspect the private TAP (replaced by
rtnetlink inspection); the dry-run TCG assertion ignored deliberate redaction;
and initial byte-identical TAP snapshots incorrectly included carrier and
kernel multicast-route changes. Those attempts failed nonzero and are excluded
from the positive result. No VM production bug or TUN permission blocker was
encountered in the final run.
