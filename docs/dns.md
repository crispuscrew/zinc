# DNS transports and integration status

`NetworkMeta.DNS.ResolversByPriority` contains ordered `DNSResolver` mappings:
`Protocol`, `Endpoint`, optional `Path`, and optional `BootstrapIPs`.
The shared implementation is [dnsproxy](../integration/dns/README.md).

| Protocol | Upstream transport | Default port |
| --- | --- | --- |
| `UDP` | Plain DNS datagrams | 53 |
| `TCP` | Length-framed plain DNS | 53 |
| `TLS` | Authenticated DNS over TLS | 853 |
| `HTTPS` | Authenticated HTTPS POST (DoH) | 443 |
| `QUIC` | Authenticated DNS over QUIC, ALPN `doq` | 853 |

`Endpoint` is an IP or hostname, optionally with port; bracket IPv6 when adding
a port. It is not a URL. Hostnames require explicit `BootstrapIPs`; the host
resolver is never used to bootstrap them. TLS certificate checks/SNI and HTTP
authority retain the configured name. HTTPS defaults to `/dns-query`; `Path`
is valid only for HTTPS. QUIC port 53 is forbidden by the worker.

## Configuration example

```yaml
NetworkMeta:
  DNS:
    ResolversByPriority:
      - Protocol: HTTPS
        Endpoint: resolver.example:443
        Path: /dns-query
        BootstrapIPs: [192.0.2.54]
```

These documentation addresses require replacement. A complete transport JSON
example is [transports.json](../common/examples/dns/transports.json). Adding
TCP/UDP fallback after encrypted entries deliberately permits plaintext DNS;
encryption is not an invariant across that list.

Resolvers and bootstrap addresses are tried sequentially. NOERROR and NXDOMAIN
stop fallback; transport, certificate, truncation and other response failures
advance to the next configured entry. There is no implicit TCP retry, search
suffix, protocol downgrade, HTTP environment proxy or redirect following.
Domain lookups query both A and AAAA and return no partial result when a family
fails. Empty results are valid DNS responses but fail domain-rule preparation.

## Running the provisioned worker

Both runners use `dnsproxy.Lookup` for launch-time domain resolution and expose
the same `dns-proxy` subcommand. The provisioner starts it with explicit addresses:

```text
zcr dns-proxy --config /absolute/dns.json \
  --listen 192.0.2.53 --listen '[2001:db8::53]:53' \
  --control-socket /private/provisioned/dns/control.sock
```

`zvr dns-proxy` accepts the same arguments. SIGINT or SIGTERM stops the worker
and closes its listeners and private control socket.
`--config` encodes **DNSMeta itself as strict JSON**, not app YAML or a whole
NetworkMeta. Listen flags are repeatable numeric addresses; each binds UDP and
TCP. Bare IPs use port 53; explicit ephemeral port zero is for isolated tests.
The worker creates no namespace or system service and chooses no default listener.

## Manifest binding and authenticated readiness

Network manifests already require proxy addresses and a digest matching the
owner's `DNSMeta`. The application needs explicit packet-policy permission to
reach that proxy; resolver configuration never adds firewall exceptions. Proxy
addresses must be reachable in the provisioned topology, not namespace loopback.
Guest resolver configuration also needs the provisioner's guest-side setup.

The shared readiness protocol checks a private Unix socket's ownership,
permissions, peer credentials, `ready`, exact UDP/TCP listener set and
SHA256(JSON(DNSMeta)). It attests local configuration and bound listeners, not
upstream availability. The control directory must already be trusted/private;
startup refuses an existing socket instead of unlinking it.

`network.Manifest` binds `dns_proxy_addresses`, `dns_config_digest`, and
`dns_control_socket`. Both runners call `CheckReady` before preparing the
application's network launch. A missing, stopped, differently configured, or
unauthenticated proxy aborts preparation; DNS never falls back to the host.

Cancellation closes the worker's listeners; listener failure withdraws readiness
and stops the worker. Bounds on time, message size, handlers and control status
are documented in [integration/dns](../integration/dns/README.md).

## Domain policy remains IP-level

Domain-rule lookup results are frozen at launch under each policy owner's DNS
configuration. They do not inspect hostnames in later packets, do not refresh
live, and cannot distinguish names sharing an IP. DNS encryption does not change
that guarantee. See [network policy](network-policy.md#domains-are-ip-snapshots).
