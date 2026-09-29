# Shared DNS adapter

Linux package: `github.com/crispuscrew/zinc/common/adapters/dnsproxy`.
Uses `miekg/dns`, `quic-go`, and shared `common/domain/network.Upstreams` endpoint interpretation.

## Integration APIs

API signatures: [`Lookup`/`Resolver.Lookup`](../../common/adapters/dnsproxy/lookup.go),
[`New`/`Resolver.Exchange`](../../common/adapters/dnsproxy/resolver.go),
[`CheckReady`](../../common/adapters/dnsproxy/ready.go), [`Serve`](../../common/adapters/dnsproxy/serve.go),
[`Run`](../../common/adapters/dnsproxy/cli.go).
Pass `dnsproxy.Lookup` as `common/adapters/network.Lookup`: A+AAAA, eight CNAME links/family maximum,
no partial results on failure. NXDOMAIN/NODATA return empty success; inconsistent NXDOMAIN fails.

Both runners use `dnsproxy.Command` for signal cancellation/error reporting:

```text
dns-proxy --config /absolute/dns.json \
  --control-socket /private/provisioned/dns/control.sock \
  --listen 192.0.2.53 --listen '[2001:db8::53]:53'
```

`--config`: strict **DNSMeta JSON**, not app/network configuration. Replace [sample.json](sample.json)'s example upstreams.
Embedded use: `Serve`. Numeric listeners bind UDP+TCP; bare IP means port 53; test port zero reports its selected port.
Reject wildcards/names/multicast/scoped addresses/duplicates. No default listener, daemonization, namespace/service setup.

Before network launch, runners check the manifest's `dns_control_socket`:
```go
dnsproxy.CheckReady(manifest.DNSControlSocket, cfg.NetworkMeta.DNS, manifest.DNSProxyAddresses)
```

## Transport and failure semantics

- Transports: UDP, TCP, authenticated TLS, HTTPS POST (DoH), QUIC (DoQ/ALPN `doq`).
- DoQ: zero IDs, one length-prefixed message + FIN, 128-byte EDNS query padding; no port 53 (RFC 9250).
  TCP keepalive options are stripped from forwarded queries and rejected in answers.
- Entries/bootstrap addresses are tried sequentially. Numeric addresses dial directly; hostnames require
  `BootstrapIPs`, retaining the endpoint name for TLS SNI/certificate verification and HTTP authority.
- System trust, TLS >=1.2 (QUIC: TLS 1.3); no insecure TLS, HTTP environment proxies/redirects,
  host resolver, search suffixes, implicit TCP retry, or protocol downgrade.
- NOERROR/NXDOMAIN stop fallback. Transport/certificate/framing/header/question/truncation/other RCODE
  failures try the next configured entry. Sample TCP/UDP entries explicitly permit plaintext fallback.
- Single-question IN/QUERY only; no transfers or TSIG.
- Bounds: 2s/attempt, 10s/exchange, 30s/dual-stack lookup. Cancellation interrupts reads, handshakes, HTTP and QUIC.
- Sizes: wire 65535 bytes; DoH response headers/control status 8 KiB each; configuration 128 KiB.
- Maxima: 16 resolvers, 16 bootstrap addresses/resolver, 16 listeners, 64 handlers.
  TCP: 12s/query, 64 queries/session. Saturation drops UDP requests and closes excess TCP sessions.

## Readiness and lifecycle

- Provision a clean absolute control path with an existing 0700 parent owned by root/current effective UID.
  Ancestors must have trusted ownership and no group/other writes except sticky directories (`/tmp`).
  No symlink components or existing socket (even stale); startup never unlinks it. New socket mode: 0600.
- Both control peers require Linux `SO_PEERCRED` root/current effective UID. `CheckReady` checks socket
  ownership/mode/identity, credentials, ready state, exact UDP+TCP listeners and immutable
  SHA256(JSON(DNSMeta)), matching `common/adapters/network.DNSDigest`.
- Connect without a request; read one JSON document through EOF: `version` (1), `digest`, `addresses`
  (IP:port, UDP+TCP), `ready`. Ready requires all binds; it does not attest remote DNS availability.
- Cancellation drains bounded handlers/closes owned resources; listener failure withdraws readiness/stops the proxy.
  Cleanup unlinks only an identity-matching owned control socket.

## Pinned checks

From this directory (rootless Podman):

```sh
make prepare
make test lint build
make race GO_IMAGE=docker.io/library/golang:1.26.6@sha256:23fdfd3a6abc97c81e32a724cdd1cf541c06c416eb04d717815f4ed7c75623d0
```

`prepare` downloads checksum-verified pinned modules into a common snapshot/vendor at
`/tmp/opencode/zinc-dns-check` (`OUTPUT` override). Checks: non-root, networkless, capability-free;
tests: private loopback/ephemeral sockets. Race requires the pinned Debian image's C compiler.
Dependencies: [miekg/dns v1.1.73](https://github.com/miekg/dns/blob/v1.1.73/go.mod),
[quic-go v0.63.0](https://github.com/quic-go/quic-go/blob/v0.63.0/go.mod) (Go 1.26 required).
[`common/go.mod`](../../common/go.mod)/[`go.work`](../../go.work): Go 1.26.0; toolchain: Go 1.26.6.
Build/check image: `docker.io/library/golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83`.
