# DNS transports and integration status

`NetworkMeta.DNS.ResolversByPriority` orders mappings of `Protocol`, `Endpoint`,
optional `Path`/`BootstrapIPs`. Full [adapter contract](../integration/dns/README.md).

| Protocol | Upstream transport | Default port |
| --- | --- | --- |
| `UDP` | Plain DNS datagrams | 53 |
| `TCP` | Length-framed plain DNS | 53 |
| `TLS` | Authenticated DNS over TLS | 853 |
| `HTTPS` | Authenticated HTTPS POST (DoH) | 443 |
| `QUIC` | Authenticated DNS over QUIC, ALPN `doq` | 853 |

`Endpoint` is IP/hostname, optional port (bracket IPv6), never a URL. Hostnames
require `BootstrapIPs`, never host resolution; TLS checks/SNI and HTTP authority
retain the name. HTTPS-only `Path` defaults to `/dns-query`; QUIC port 53 is forbidden.

## Configuration example

Adapt [transports.json](../common/examples/dns/transports.json), replacing documentation
addresses. TCP/UDP fallback deliberately allows plaintext after encrypted entries.

Resolvers/bootstrap IPs are sequential; NOERROR/NXDOMAIN stop fallback. Other failures
try the next entry; [failure cases/bounds](../integration/dns/README.md#transport-and-failure-semantics) apply.
No implicit TCP retry, search suffix, downgrade, HTTP environment proxy or redirects.
A/AAAA is all-or-nothing; valid empty DNS results fail domain-rule preparation.

## Running the provisioned worker

Provisioners start either runner's worker; both use `dnsproxy.Lookup` at app launch:

```text
zcr dns-proxy --config /absolute/dns.json \
  --listen 192.0.2.53 --listen '[2001:db8::53]:53' \
  --control-socket /private/provisioned/dns/control.sock
```

`zvr dns-proxy` takes identical flags. `--config` is **strict DNSMeta JSON**, not
app YAML/NetworkMeta. Repeat numeric `--listen` addresses; each binds UDP/TCP.
Bare IPs use 53; explicit port zero is for isolated tests. No default listener,
namespace creation or service installation. SIGINT/SIGTERM closes listeners/control socket.

## Manifest binding and authenticated readiness

Manifest fields `dns_proxy_addresses`, `dns_config_digest`, `dns_control_socket`
bind the worker. Provision reachable non-loopback addresses, guest-side resolver
setup and explicit app permission; resolver configuration adds no firewall exception.

Before network preparation, `CheckReady` verifies private Unix socket ownership,
permissions/peer credentials, `ready`, exact UDP/TCP listeners and SHA256(JSON(DNSMeta)).
Failures abort without host fallback; readiness proves local bindings, **not upstream availability**.

Follow [control-directory and lifecycle requirements](../integration/dns/README.md#readiness-and-lifecycle):
trusted/private directory must exist; existing sockets are refused, not unlinked.
Cancellation closes listeners; listener failure withdraws readiness and stops the worker.

## Domain policy remains IP-level

[Domain rules](network-policy.md#domains-are-ip-snapshots): frozen per-owner IPs, no hostname
inspection/refresh/shared-IP distinction. DNS encryption does not change that.
