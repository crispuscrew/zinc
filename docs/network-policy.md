# Ordered packet policy

`NetworkMeta.Interfaces` declares logical IDs (not kernel names), optional `MacAddress`.
Empty means no managed NIC; declared NICs require [provisioning](network-provisioning.md)
even with no rules. Empty rules admit no new routed flows.

## From and To describe endpoints

`RulesByPriority`: `From`, `To`, optional `Domains`/`Protocols`, `AllowAllExcept`.
Endpoints: `Type`, optional `AppNameID`/`Interface`, `Filter` (`IPv4CIDR`, `IPv6CIDR`, `Ports`).

| Peer Type | Scope |
| --- | --- |
| `Self` | The policy owner's provisioned endpoints |
| `App` | Exact `AppNameID` from the authoritative peer inventory |
| `AnyApp` | Self and registered Zinc app endpoints |
| `Host` | Authoritatively registered host addresses, including public host IPs |
| `Internet` | Public external addresses, excluding host/apps and conservative special/private/link-local ranges |
| `Any` | Any endpoint, still subject to both registered app policies |

- `AppNameID`: only `App`. `Interface`: logical ID for `Self`/`App`, host name for
  `Host`, forbidden otherwise. CIDRs only narrow scope; private CIDRs cannot widen `Internet`.
- `From.Filter.Ports` matches source ports; `To.Filter.Ports` destination ports.
  Address families stay separate.
- Use the [paired network examples](../common/examples/README.md): reciprocal inbound
  grants and provisioned connectivity are required; rules create no DNS alias or bridge.

## Evaluation and protocols

- First match wins; default **deny**. `AllowAllExcept: false` allows, `true` denies
  only that match; allowing other traffic requires a later explicit allow rule.
- Both registered app endpoints must consent, even with `Any`.
- Stateful replies need no reverse rule. Conntrack binds to the resolved-policy hash;
  replacement policies do not automatically trust old admissions.
- Protocols: `TCP`, `UDP`, `ICMP`, `ICMPv6`, `SCTP`, `GRE`, `ESP`, `AH`; empty selects all.
  Ports require explicit TCP/UDP/SCTP, never non-port protocols. ICMP family mismatches error.

`common/domain/nftrules` renders validated/resolved topology at container input/output
or TAP forward hooks. Provisioning must prevent alternate paths/identities.

## Domains are IP snapshots

`Domains` further narrows destination scope/filters using each policy owner's explicit
`DNS.ResolversByPriority`, including peer rules. Missing/failed/empty results abort
even deny rules; no host DNS fallback.

**Not hostname enforcement:** literal IPs and other names sharing an allowed IP match.
No TLS SNI/HTTP Host inspection or live refresh occurs. Rotating names may break;
reassigned old IPs stay allowed until relaunch, so stale snapshots are not necessarily safe.
The app must explicitly allow its [local DNS proxy](dns.md); the proxy needs a
provisioned upstream path. Resolver selection grants no packet permission.

## Observability and escape hatches

`zcr net` observes running attachment; `zcr net APP --json`/`zvr net APP` report
namespace decisions/default drops by owner and canonical `rule[N]`. Counters reset
on table replacement; they are not payload accounting, lifetime totals or backend attestation.
Raw `RunnerFlags` warn and may override containment/add VM networking without typed
NICs; that run is not isolated. Provisioned VMs also check actual NIC/backend arguments.
