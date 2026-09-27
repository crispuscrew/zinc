# Live network enforcement tests

```sh
make -C container/runner netfilter-image
make -C integration/network lint
make -C integration/network test
```

This suite installs the actual `common/domain/nftrules` output and exchanges
real packets. It does not infer enforcement from text matching or `nft --check`.

## Isolation and prerequisites

- Rootless Podman; no sudo, host networking, published ports, privileged mode,
  external uplink, image pulls, or package installation.
- Go 1.26.6 pinned to the same digest as root `check.mk`.
- Make resolves the repository-built helper to its immutable local content ID
  before invoking the suite. A different installed helper can be selected with
  `HELPER_IMAGE=sha256:<verified-image-id>`. The runner refuses mutable image
  references; absence is a hard error. Build the pinned-source helper separately.
- At least 1 GiB free in temporary storage and container storage, checked before
  compilation. Sources are mounted read-only; compilation and generated JSON
  fixtures live in temporary directories, never under the source tree.
- The runtime container is read-only, disconnected (`--network=none`), and has
  only SYS_ADMIN, NET_ADMIN and NET_RAW in its rootless user namespace. SYS_ADMIN
  is needed to bind temporary namespace handles and enter test network namespaces.
- Podman enables forwarding only in the disposable container's network namespace.
  The test never writes host sysctls. Its endpoint namespaces have only specific
  routes to documentation subnets, not default routes.

The runner refuses to execute packet tests in the recorded host network
namespace. Before and after the suite, its parent namespace must contain only
loopback: no routes, nft tables, named namespaces, or remaining veth endpoints.
Listeners are killed/reaped, links deleted explicitly, namespace handles removed,
and the container and temporary build directory removed even on failure.

Host permanent interface identity and stable IPv4/IPv6 routes from all tables
are compared before/after. Raw link differences are also printed. A current MAC
is normalized to the kernel's reported permanent MAC for this comparison because
disconnected Wi-Fi adapters can randomize their current MAC during a run. Route
expiration timers are not configuration changes and are omitted. This inventory
comparison does not claim an unreadable host firewall was inspected.

## Packet coverage

| Cases | Evidence |
| --- | --- |
| 192 TCP/UDP assertions | Both IP families, inbound/outbound, local input/output and routed forward enforcement; 12 policies per combination |
| 48 SCTP assertions | Both IP families, local and forwarded traffic; real associations and echo payloads |
| 12 ICMP assertions | IPv4 ICMP and ICMPv6 echo, allow/deny/default, local and forwarded |
| 2 anti-spoof assertions | Forged source IP and MAC blocked despite broad application allows |
| 1 failed-load assertion | Real malformed nft load leaves TCP blocked by default-deny counters |
| 1 preflight regression | A missing expected device must quarantine an existing permissive policy |

The 12 transport policy cases cover empty policy, matching/nonmatching deny-only
policy, both rule orderings, exact source/destination ports, independently wrong
source/destination ports and CIDRs, and missing/explicitly denied peer consent.

Every transport/family path has a working unfiltered positive control. Allowed
traffic must echo an exact payload and increment both admission and stateful reply
counters. Denied traffic must time out or return Linux's local EPERM/EACCES and
increment the expected nft **drop** counter. Connection refusal, missing routes,
bind failures, or an absent listener cannot substitute for that evidence.

SCTP skips only on EPROTONOSUPPORT/EAFNOSUPPORT and reports the exact kernel error.
Skipped cases are not enforcement passes. Selecting no runnable live case is an
error. SCTP was available and no cases were skipped in the recorded run below.

## Fixture boundary

The suite writes owner-only JSON manifests with actual namespace inode identities,
MAC/IP mappings, both endpoint policies and a new generation for each case. They
go through `Decode -> Resolve -> RenderResolved -> ApplyScript -> nft -f`.

The controller and endpoint namespaces share their owning user namespace.
Consequently this is not a positive test of production `LoadFile`, which correctly
refuses the caller's current user namespace. Nested `unshare -r -n` failed with
`write error: Operation not permitted` under the helper's chosen capabilities.
No runner CLI, DNS service, QEMU process, or real TAP guest is launched. Forwarded
guest behavior is modeled with real routed veth packets, including netdev ingress
MAC filtering; this does not verify TAP/QEMU attachment or guest configuration.

`ip netns exec` is deliberately avoided: it attempts a `/sys` remount rejected by
this rootless container. Explicit `nsenter --net=...` exercises the same network
namespaces without weakening mount protections.

## Recorded result and regression coverage

On kernel `7.1.5-100.fc43.x86_64`, nftables 1.1.3 and iproute2 6.15.0, all 256
packet/filter/rollback assertions passed without skips. The preflight regression
first reproduced a stale-permission bug and now confirms the corrected startup
ordering. See REVIEW.md.

Run only that regression:

```sh
make -C integration/network test TEST_PATTERN=TestLivePreflightFailureClosesPolicy
```

The selection is printed explicitly. A targeted pass must not be reported as a
full-suite pass. `make lint` checks formatting and vet without starting listeners.
