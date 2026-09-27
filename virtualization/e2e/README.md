# VM end-to-end tests

## Offline lifecycle smoke test

```sh
ZINC_E2E_ZVR=/absolute/fresh/zvr make -C virtualization/e2e e2e \
    TEST_ARGS='-run TestVMOfflineLifecycle'
```

Uses a private 128 MiB VM and a newly created blank qcow2 base. It requires QEMU,
qemu-img and accessible KVM. Explicit `ZINC_E2E_ACCEL=tcg` instead exercises the
blank-disk scenario using raw test-only accelerator/CPU flags. It downloads no OS,
creates no NIC or host port, and requests no audio.

It proves the real QEMU block node is read-only through QMP, verifies automatic
restart after an intentional QEMU crash, stops during restart startup, checks
manual-stop suppression and supervisor cleanup, preserves the base digest, resets
the private overlay, and refuses a mismatched pin. It also checks dry-run writes
no guest state. This is a process/block-device test, not guest filesystem semantics.

Private files are cleaned after a successful stop. If cleanup fails, the test
retains its temporary directory for inspection instead of deleting live state.

## Provisioned network scenario

The network suite never substitutes slirp for full packet policy. It skips unless
the operator supplies all of:

- `ZINC_E2E_STATIC_IMAGE`: a Cirros-compatible, statically addressed guest fixture.
- `ZINC_E2E_STATIC_DIGEST`: its independently authorized sha256 pin.
- `ZINC_E2E_NETWORK_APP`: a fresh fixture name with the `zinc-e2e-` prefix.
- `ZINC_E2E_SSH_PORT`: the explicitly provisioned loopback TCP-to-guest-22 mapping.
- `ZINC_NETWORK_MANIFEST_DIR`: protected manifests for fresh, exclusive namespaces.

The fixture must match the exact `primary` NIC and Host-to-Self TCP/22 policy
authored by the suite. The provisioner owns TAPs, static addressing/neighbors,
forwarding and publication. Namespace setup is never performed by these tests.
Do not supply a namespace serving another live guest.

Assertions cover separate app/options authoring, runtime type refusal, read-only
planning, provisioned guest SSH identity, non-loopback exclusion, base integrity,
graceful stop and reset. Full transport/peer policy coverage belongs to the shared
network tests and needs independently provisioned fixtures.

By default this scenario rebuilds creator and both runners. During a coordinated
schema migration set `ZINC_E2E_ZC`, `ZINC_E2E_ZVR` and `ZINC_E2E_ZCR` to freshly
built canonical binaries. Config/data/runtime locations remain test-private.

Neither scenario changes host audio services. Audio-holder IPC and revocation
are covered by runner tests; the shared broker has isolated-daemon integration
tests. Graphics, guest OS read-only boot, Secure Boot and TPM sealing still need
their own compatible guest fixtures.
