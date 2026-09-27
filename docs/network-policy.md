# Ordered packet policy

`NetworkMeta.Interfaces` declares logical Zinc IDs with optional `MacAddress`.
IDs are not kernel interface names. No interfaces means no managed NIC for either
runtime. Declared interfaces require [provisioning](network-provisioning.md),
even when the rule list is empty; an empty list permits no new routed flows.

## From and To describe endpoints

Each `RulesByPriority` entry contains `From`, `To`, optional `Domains`, optional
`Protocols`, and `AllowAllExcept`. Each endpoint has `Type`, optional
`AppNameID`/`Interface`, and `Filter` with `IPv4CIDR`, `IPv6CIDR` and `Ports`.

| Peer Type | Scope |
| --- | --- |
| `Self` | The policy owner's provisioned endpoints |
| `App` | Exact `AppNameID` from the authoritative peer inventory |
| `AnyApp` | Self and registered Zinc app endpoints |
| `Host` | Authoritatively registered host addresses, including public host IPs |
| `Internet` | Public external addresses, excluding host/apps and conservative special/private/link-local ranges |
| `Any` | Any endpoint, still subject to both registered app policies |

`AppNameID` is valid only for `App`. `Interface` selects a logical ID for `Self`
or `App`, and a host interface name for `Host`. Other peer types cannot use it.
CIDRs narrow the chosen scope; adding a private CIDR to `Internet` does not grant
private access. `From.Filter.Ports` matches source ports; `To.Filter.Ports`
matches destination ports. Address families remain separate.

```yaml
NetworkMeta:
  Interfaces:
    - ID: primary
  RulesByPriority:
    - From: {Type: Self, Interface: primary}
      To:
        Type: App
        AppNameID: network-server
        Interface: service
        Filter: {Ports: [8080]}
      Protocols: [TCP]
```

This fragment needs the server's reciprocal inbound grant and provisioned
connectivity. It creates neither a DNS alias nor a bridge. Complete matching
examples are [here](../common/examples/README.md).

## Evaluation and protocols

- Rules are ordered, first match wins, and the default is **deny**.
- `AllowAllExcept: false` allows a matching packet; `true` denies that match.
  Despite the field name, it never changes the default to allow. To allow other
  traffic, author a later explicit allow rule.
- Both app endpoints must consent. An allow in one app, including an `Any`
  rule, cannot bypass a registered peer's policy.
- Stateful replies to admitted flows do not need a reverse rule. Connection
  tracking is bound to a resolved-policy hash, so a new policy does not simply
  trust old admissions.
- Supported protocols are `TCP`, `UDP`, `ICMP`, `ICMPv6`, `SCTP`, `GRE`, `ESP`,
  `AH`. Empty `Protocols` selects all supported protocols.
- Ports require explicit `TCP`, `UDP` or `SCTP`; they cannot be applied to
  non-port protocols. ICMP family mismatches are rejected.

The shared renderer in `common/domain/nftrules` consumes validated, resolved
topology. Containers enforce at input/output hooks; TAP guests are filtered at
the forwarding path. Provisioning must prevent alternate paths and identities.

## Domains are IP snapshots

`Domains` resolves destination addresses using the policy owner's explicit
`DNS.ResolversByPriority`, including when resolving a peer's rules. Resolved
addresses further constrain the destination match, alongside its endpoint
scope and filters. Missing, failed or empty results abort launch, including for
deny rules; there is no implicit host DNS fallback.

The frozen addresses are **not hostname enforcement**. Connections by literal IP
are allowed when they match; another hostname on the same CDN/shared address is
indistinguishable. No TLS SNI or HTTP Host inspection occurs. Results are not
refreshed during the run. Rotating names may stop working; an old IP remains
allowed until relaunch even if reassigned to another owner. Do not describe a
stale snapshot as necessarily safe or as a live domain firewall.

Resolver selection does not authorize packets to a DNS server. The application
needs an explicit rule permitting its provisioned local proxy, and the proxy
needs its own provisioned upstream path. See [DNS](dns.md).

## Observability and escape hatches

`zcr net` reports observed running attachment; `zcr net APP --json` and
`zvr net APP` expose namespace counters. Rule labels use canonical `rule[N]`
indexes and identify policy owners. The renderer counts policy decisions and
default drops; these are not application payload accounting or lifetime totals.
Replacing namespace tables resets counters. Observed attachment alone does not
attest every effective backend option.

Raw `RunnerFlags` can override structured containment and are warned. A VM with
no typed NICs can explicitly add raw networking; that is not an isolated run.
Provisioned VM launches additionally check the actual NIC/backend arguments.
