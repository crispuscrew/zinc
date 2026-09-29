# Historical failure before the namespace-selection fix

This records the original failure. See [RESULTS.md](RESULTS.md) for the verified
positive result after the adapter fixes.

## Environment and build

- Date: 2026-09-28; rootless UID 1000.
- Kernel: `7.1.5-100.fc43.x86_64`; Podman `5.8.4`; crun `1.28`
  (`54f16ffbefcd022bf032af768b5c5ce075c18bfc`).
- Existing Makefile build succeeded with `--pull=never --no-cache` and
  `--network=none`; no dependency, vendor, schema, or production source edits.
- Tested `container/runner/bin/zcr` SHA-256:
  `c2bdaac5bbfe01b6b62c300f562cd9d31db1a70aa1eaa411de50d32b8c7e8412`.
- Local app image digest:
  `sha256:b5b2293b5922c15cba9ddbc418da93c08f546dec135f9a14e649abcb68999d03`.
- Local helper image digest:
  `sha256:048b8208d9dd02f369befc08d233a0854e4770607f891b5b46c3f4dbda4fe567`.
- Free space before build: workspace 34 GiB, `/tmp` 24 GiB.

## Production failure

The explicit filepath validates, and `zcr run <filepath>` successfully loads
the real owner-only manifest and renders the namespace-bound helper/pod plan.
Executing `zcr run <filepath> --exec` fails before nft installation or pod/app
startup. Exact error from the reproduced production attempt:

```text
zcr: launch provision-1a849edce933 (verify provisioned namespace and lock nft before app): verify provisioned namespace and lock nft before app: exit status 126: Error: OCI runtime error: crun: setns to target user namespace `/proc/2443108/ns/user`: Invalid argument
close provisioned namespace; preserve provisioner topology: exit status 126: Error: OCI runtime error: crun: setns to target user namespace `/proc/2443108/ns/user`: Invalid argument
```

`zcr` exits 1; `make test` exits 2. The expected no-DNS warning also appears.
Natural-exit supervisor cleanup, rerun, and explicit stop remain **unverified**.
The harness contains those assertions, but execution never reaches them.

Evidence for the production failure and successful diagnostic control:
`/tmp/opencode/zinc-provisioning-5kvf8yj0/`.

## Isolated diagnosis

The holder's user inode equals `podman unshare readlink /proc/self/ns/user`:
`user:[4026533112]`. Its private net inode is `net:[4026533635]`.

Using the same pinned helper, private netns, no-new-privileges, dropped caps,
and NET_ADMIN only:

| Direct diagnostic | Result |
| --- | --- |
| `--userns ns:/proc/2443108/ns/user` | Exit 126, same crun `Invalid argument` |
| `--userns host` in rootless Podman | Exit 0, both expected namespace inodes and nft read verified |

The second command does not use the initial host user namespace: its script
asserts the provisioned user/net inodes before reading nft. It changes no
rules. The namespace remains an empty nft ruleset after the production failure.
This isolates the explicit userns re-entry as the blocker, rather than lack
of rootless network namespace or nft capability support.

### Suggested production fix (not applied)

Review `container/runner/adapters/netenforce/pasta.go:67-84` and `:87-96`:
helper startup, pod creation, and teardown all request an explicit userns join.
When the provisioned userns is already Podman's own rootless userns, avoid
attempting to re-enter that same namespace. A possible adapter fix is to select
Podman's current-userns mode only after verifying that inode equality, retaining
the explicit network join and in-container inode preflight. Keep explicit joins
for distinct namespaces and fail closed on mismatches. Do not globally replace
user namespace joins or weaken `LoadFile`'s initial-host-namespace rejection.

The successful read-only helper control supports this direction; it does not
prove pod creation or the complete lifecycle with that change. Apply a reviewed
fix and rerun this positive test before claiming working production consumers.

## Cleanup and limitations

- Test holder and descendants exited; no test containers/pods remained.
- The temporary test build image was removed after testing; the recorded binary
  remains at `container/runner/bin/zcr` with the same SHA-256.
- Before/after inventories matched, including host links, addresses, routes,
  routing rules, namespace identities, and Podman images/resources.
- Host nft read was denied with `Operation not permitted (you must be root)`;
  host firewall contents were not independently compared. No host mutation
  command, sudo, service change, image pull, or audio operation was issued.
- Python syntax checks passed. The production integration intentionally fails.
- An early harness-only setup attempt rejected the kernel's private multicast
  route and mistakenly printed PASS for an empty `AssertionError`. The harness
  now accepts that route, stores `repr(error)`, rejects optimized assertions,
  and exits nonzero. That setup attempt is excluded from all test claims.
- A diagnostic iteration used unsupported multi-argument BusyBox `readlink`;
  separate invocations fixed the control. It is not counted as a passing control.
