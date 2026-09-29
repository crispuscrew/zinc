# Live network enforcement tests

From the repository root:

```sh
make -C container/runner netfilter-image
make -C integration/network lint
make -C integration/network test
```

Real packets against installed `common/domain/nftrules` output.

## Isolation and prerequisites

- Requires rootless Podman, `ip`, `df`, `mktemp`, `diff`, and locally available immutable images.
  No sudo, host networking, published ports, privileged mode, uplink, pulls, or package installation.
- Go 1.26.6: [check.mk](../../check.mk) digest. Make resolves the separately built pinned-source helper's
  local ID; override: `HELPER_IMAGE=sha256:<verified-image-id>`. Missing/mutable images fail.
  Direct [run.sh](run.sh) requires `GO_IMAGE`/`HELPER_IMAGE`; live suite timeout: 240s.
- Requires 1 GiB free in temporary/container storage. Sources: read-only; builds/fixtures: temporary.
  Runtime: read-only, `--network=none`, only
  SYS_ADMIN (namespace handles/entry), NET_ADMIN and NET_RAW in the rootless user namespace.
- Forwarding changes only the disposable netns, never host sysctls. Endpoints: documentation-subnet routes,
  no defaults. Packet tests refuse the recorded host netns.
- Parent netns must start/end loopback-only: no routes, nft tables, named namespaces or veths.
  Even on failure: kill/reap listeners, delete links/handles, remove container and temporary build directory.
- Compare host permanent interface identities/all-table IPv4/IPv6 routes; print raw link differences.
  Normalize randomized MACs to permanent MACs; omit route expiration timers. No claim to inspect unreadable host nft.

## Packet coverage

| Cases | Evidence |
| --- | --- |
| 192 TCP/UDP | IPv4/IPv6, inbound/outbound, local input/output and routed forward; 12 policies/combination |
| 48 SCTP | IPv4/IPv6, local/forwarded, real associations/echo |
| 12 ICMP | IPv4/IPv6 echo, allow/deny/default, local/forwarded |
| 2 anti-spoof | Forged IP/MAC blocked despite broad allows |
| 1 failed load | Malformed nft load leaves TCP blocked; default-deny counters |
| 1 preflight | Missing device quarantines existing permissive policy |

[12 policies](matrix_test.go): empty, matching/nonmatching deny-only, both orderings, exact ports,
independently wrong source/destination ports/CIDRs, missing/denied peer consent.
Every transport/family needs an unfiltered positive control. Allow: exact echo + admission/stateful-reply counters.
Deny: timeout/local EPERM/EACCES + expected **drop** counter, never refusal/missing routes/listeners/bind failure.
SCTP skips only on reported EPROTONOSUPPORT/EAFNOSUPPORT; skips are not passes. Selecting zero runnable live cases fails.

## Fixture boundary

Owner-only manifests: actual namespace inodes, MAC/IP mappings, both policies, fresh generation/case;
`Decode -> Resolve -> RenderResolved -> ApplyScript -> nft -f`. Shared userns excludes positive production
`LoadFile` coverage (caller userns rejected); nested `unshare -r -n` failed: `write error: Operation not permitted`.
No runner CLI/DNS/QEMU/TAP guest. Routed veth/netdev-ingress MAC tests do not verify TAP/QEMU attachment/guest setup.
Use `nsenter --net=...`; `ip netns exec` requires a rejected `/sys` remount.

## Recorded result and regression coverage

Recorded: 256 assertions, no skips, kernel `7.1.5-100.fc43.x86_64`, nftables 1.1.3, iproute2 6.15.0.
[REVIEW.md](REVIEW.md) records the stale-permission preflight bug and corrected startup ordering. Run that regression alone:

```sh
make -C integration/network test TEST_PATTERN=TestLivePreflightFailureClosesPolicy
```

Selection is printed; a targeted pass is not a full-suite pass. `make lint` checks formatting/vet without listeners.
