# VM end-to-end tests

## Offline lifecycle smoke test

```sh
ZINC_E2E_ZVR=/absolute/fresh/zvr make -C virtualization/e2e e2e \
    TEST_ARGS='-run TestVMOfflineLifecycle'
```

Requires host Go, QEMU, qemu-img and accessible KVM. `ZINC_E2E_ACCEL=tcg` selects
test-only accelerator/CPU flags instead. The private 128 MiB blank-qcow2 VM needs
no OS download, NIC, host port or audio.

Checks QMP block read-only state, crash/restart, stop during restart, manual-stop
suppression, supervisor cleanup, base integrity, overlay reset, pin mismatch and
state-free dry-run. This verifies process/block-device behavior, not guest filesystem semantics.

Files are removed after successful stop; failed cleanup retains the directory for inspection.

## Provisioned network scenario

Skips unless the operator supplies all of:

- `ZINC_E2E_STATIC_IMAGE`: a Cirros-compatible, statically addressed guest fixture.
- `ZINC_E2E_STATIC_DIGEST`: independently authorized sha256 pin.
- `ZINC_E2E_NETWORK_APP`: a fresh fixture name with the `zinc-e2e-` prefix.
- `ZINC_E2E_SSH_PORT`: the explicitly provisioned loopback TCP-to-guest-22 mapping.
- `ZINC_NETWORK_MANIFEST_DIR`: protected manifests for fresh, exclusive namespaces.

Fixtures must match the exact `primary` NIC and Host-to-Self TCP/22 policy.
The [provisioner](../../docs/network-provisioning.md) owns TAPs, static addressing/neighbors,
forwarding and publication. Tests create no namespaces or slirp substitute; never reuse a live guest's namespace.

Checks paired YAML/VM-options authoring, runtime type refusal, read-only planning,
SSH identity, non-loopback exclusion, base integrity, graceful stop and reset.
Full transport/peer policy coverage needs the shared network tests and independent fixtures.

By default it rebuilds creator and both runners. For live-common work, set
`ZINC_E2E_ZC`, `ZINC_E2E_ZVR`, `ZINC_E2E_ZCR` to fresh canonical binaries.
Config/data/runtime locations remain private.

Neither scenario changes host audio services. Runner tests cover holder IPC/revocation;
[isolated broker tests](../../integration/wireplumber/README.md) cover audio integration.
Graphics, guest read-only boot, Secure Boot and TPM sealing need compatible guest fixtures.
