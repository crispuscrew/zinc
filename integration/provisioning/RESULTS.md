# Positive production provisioning: PASS

## Verified run

- Date: 2026-09-28; ordinary rootless UID 1000.
- Kernel `7.1.5-100.fc43.x86_64`, Podman `5.8.4`, crun `1.28`.
- Evidence: `/tmp/opencode/zinc-provisioning-1lb7j4vv/`.
- App: `provision-9c65d21395bc`; holder PID `2607194`.
- Namespace identities: net `4026533636`, user `4026533112`.
- Fresh `container/runner/bin/zcr` SHA-256:
  `5a9c591251aa545684b3d45a39b1533cf3210cb75e9ab79de4989503e8e5f320`.
- App image: `localhost/zinc/e2e-app` at manifest digest
  `sha256:b5b2293b5922c15cba9ddbc418da93c08f546dec135f9a14e649abcb68999d03`.
- Helper image: `localhost/zinc/netfilter` at manifest digest
  `sha256:048b8208d9dd02f369befc08d233a0854e4770607f891b5b46c3f4dbda4fe567`.

The final `result.json` contains `status: PASS`, `failure: null`,
`cleanup_errors: []`, and `inventory_changed: []`. The Make target exits zero.

| Production operation | Observed result |
| --- | --- |
| Explicit filepath, canonical schema, real manifest LoadFile/Resolve | PASS |
| NET_ADMIN helper installs default-deny policy in designated netns | PASS |
| Pod/app startup and expected net/user namespace inodes | PASS, three launches |
| UID 65534, zero Inh/Prm/Eff/Bnd/Amb capabilities, NoNewPrivs=1 | PASS |
| `zcr net <absolute-filepath> --json` reads live policy counters | PASS, all launches |
| Natural status-zero exit, supervisor removes app/pod and closes policy | PASS, twice |
| Same-name, same-manifest rerun after cleanup | PASS |
| `zcr stop <absolute-filepath>` removes app/pod and closes policy | PASS |
| Three-hook deny-all replacement and provisioned topology preservation | PASS, all exits |
| App/pod port bindings empty; host interfaces and other inventories unchanged | PASS |

`active-*.json`, `closed-*.json`, `app-*.json`, `pod-*.json`, `counters-*.json`,
and `commands.jsonl` contain the actual observations. No Podman shim or mocked
runtime participates in the live test.

## Flags and identity invariant

The adapter reads local/remote and rootless mode with `podman info`. For local
rootless Podman it reads `podman unshare readlink /proc/self/ns/user`; local
rootful Podman inherits the caller's userns, whose identity is read directly
because Podman refuses rootful `unshare`. Unknown/remote modes and probe errors
are explicit failures. Production memoizes the identity, including failures,
once per process, and resolves static argv before executing a plan.

- Exact `manifest.UserInode` match: `--userns host`.
- Different identity: retain `--userns ns:<manifest.UserNamespace>`.
- Helper startup, pod creation, teardown, and counters use the same selection.
- Every helper checks BOTH actual namespace inodes before any nft operation.
  `LoadFile`'s existing ownership, nsfs, inode, and host-namespace checks remain.

Actual pod argv in the successful run:

```text
podman pod create --name provision-9c65d21395bc-pod --network ns:/proc/2607194/ns/net --userns host --dns none
```

The helper also uses `--pull never --user 0 --security-opt no-new-privileges
--cap-drop all --cap-add NET_ADMIN`; the app uses `--pull never --user nobody
--security-opt no-new-privileges --cap-drop all --pod <name>-pod --read-only`.
Here `host` means Podman's verified current rootless userns, not the initial
host user namespace. It does not select host networking.

## Second adapter bug found and fixed

After fixing startup, the real counters path failed with:

```text
Error: container dependency bd7b4c2c46245c766ecdfd1849a365248039cd75cd75e0f30ebec1490eab8199 is part of a pod, but container is not: invalid argument
```

The counter helper used `--network container:<app>`, which Podman rejected
because the helper was outside the app's pod. It now snapshots the live app PID
with read-only `podman inspect` and joins `--network ns:/proc/<app-pid>/ns/net`.
Both manifest inode checks still precede nft, so a stale/replaced process or
namespace cannot silently redirect the counter read. Inspection errors and
nonpositive PIDs fail explicitly. Evidence of the failure is in
`/tmp/opencode/zinc-provisioning-pgtlunvm/`.

## Checks and scope

- Full `make -C container/runner check`: formatting, vet, and all unit tests PASS.
- Existing Makefile build with `--pull=never --no-cache`, offline network: PASS.
- `make -C integration/provisioning lint`: syntax PASS.
- `make -C integration/provisioning test`: live lifecycle PASS.
- Unit cases cover local rootless/rootful probing, distinct namespace retention,
  malformed/zero/failed identities, fixed plan argv, all four operation paths,
  and failed/frozen app-process inspection. Rootful custom provisioning is
  covered by unit tests, not claimed as a privileged live test.
- Only `container/runner/adapters/netenforce/**` and `integration/provisioning/**`
  were edited for this fix; no schema, common, vendor, or dependency edits.
- Test holder/descendants, containers/pods, and temporary build images are cleaned
  up. The freshly built binary and private evidence remain available.
- Host nft reads are unprivileged and denied, so host firewall contents were not
  independently compared. No host mutation, sudo, published ports, image pull,
  real uplink, or audio setup was used.

An earlier post-fix harness iteration incorrectly treated Podman's pod-inspect
array as an object. Its failed inventory also recorded a down Wi-Fi interface's
MAC changing; that attempt is not counted as a pass. The array handling was fixed,
and the final run passed the unchanged strict inventory checks. The original
pre-fix userns failure remains documented in [BLOCKED.md](BLOCKED.md).
