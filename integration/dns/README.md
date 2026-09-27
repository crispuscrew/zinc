# Shared DNS adapter

Package: `github.com/crispuscrew/zinc/common/adapters/dnsproxy` (Linux).
All DNS parsing and serialization use `miekg/dns`; QUIC uses `quic-go`.
Endpoint interpretation is shared with `common/domain/network.Upstreams`.

## Integration APIs

```go
func Lookup(meta schema.DNSMeta, name string) ([]netip.Addr, error)
func CheckReady(controlSocket string, meta schema.DNSMeta, addresses []string) error
func Serve(ctx context.Context, meta schema.DNSMeta, addresses []string, controlSocket string) error
func Run(ctx context.Context, args []string) error

func New(meta schema.DNSMeta) (*Resolver, error)
func (resolver *Resolver) Lookup(ctx context.Context, name string) ([]netip.Addr, error)
func (resolver *Resolver) Exchange(ctx context.Context, query *dns.Msg) (*dns.Msg, error)
```

Pass `dnsproxy.Lookup` directly as `common/adapters/network.Lookup`. A lookup queries
A and AAAA, follows up to eight CNAME links per family, and returns no partial
addresses if either family fails. NXDOMAIN and successful NODATA return an empty
result without an error. Inconsistent NXDOMAIN across families is an error.

Both runners route `dns-proxy` to `dnsproxy.Command`, which supplies a
signal-cancelled context to `Run` and reports the returned error. Flags are:

```text
dns-proxy --config /absolute/dns.json \
  --control-socket /private/provisioned/dns/control.sock \
  --listen 192.0.2.53 --listen '[2001:db8::53]:53'
```

`--config` is strict JSON encoding of **DNSMeta itself**, not the whole app or
network configuration. `sample.json` demonstrates all five transports using
documentation-only endpoints; replace these with provisioned upstreams.

For embedded configuration, call `Serve` instead. Each numeric listener address
gets both UDP and TCP; a bare IP uses port 53. Wildcards, names, multicast, scoped
addresses, and duplicates are rejected. Explicit port zero is supported for
isolated tests; readiness reports the actual selected port. There is no default
listener, background daemon, namespace creation, or service installation.

The runner's manifest integration calls:

```go
dnsproxy.CheckReady(manifest.DNSControlSocket, cfg.NetworkMeta.DNS, manifest.DNSProxyAddresses)
```

The manifest binds this path as `dns_control_socket`; readiness is checked
before constructing the application network launch.

## Transport and failure semantics

- UDP, TCP, authenticated TLS, HTTPS POST (DoH), and QUIC (DoQ/ALPN `doq`).
- DoQ uses zero IDs, one length-prefixed message plus FIN, and 128-byte EDNS
  query padding. TCP keepalive options are removed before DoQ forwarding and
  rejected in DoQ answers. Port 53 is forbidden for DoQ by RFC 9250.
- Configured entries and their bootstrap addresses are tried sequentially.
- Numeric addresses are dialed directly. Hostnames require `BootstrapIPs`;
  TLS SNI/certificate verification and HTTP authority retain the endpoint name.
- TLS 1.2 or newer uses system trust; QUIC requires TLS 1.3. No insecure TLS mode.
- HTTP environment proxies and redirects are disabled.
- No host resolver, search suffixes, implicit TCP retry, or protocol downgrade.
- NOERROR and NXDOMAIN stop fallback. Transport, certificate, framing, header,
  question, truncation, and other RCODE failures move to configured next entries.
  The sample's TCP/UDP entries therefore explicitly allow plaintext fallback.
- Queries are single-question IN/QUERY messages. Transfers and TSIG are rejected.
- Each attempt has a 2-second bound; each exchange a 10-second total bound;
  each dual-stack lookup a 30-second total bound. Context cancellation interrupts
  in-flight reads, handshakes, HTTP requests, and QUIC streams.
- Wire messages are bounded to 65535 bytes, DoH response headers to 8 KiB,
  configuration input to 128 KiB, and control status to 8 KiB.
- At most 16 resolvers, 16 bootstrap addresses per resolver, 16 listener addresses,
  and 64 DNS handlers. TCP sessions have a 12-second per-query deadline and a
  64-query limit. Saturated UDP requests are dropped; excess TCP sessions close.

## Readiness and lifecycle

The control parent directory must already exist, be owned by root or the current
effective UID, and have private permissions (0700). Ancestors must be trusted and
not writable by others, except sticky directories such as `/tmp`. Symlink
components are rejected. The socket must not already exist, including stale
sockets; startup never unlinks someone else's path. The created socket is 0600.

All UDP/TCP listeners bind before status becomes ready. Both control endpoints
check Linux `SO_PEERCRED` for root/current effective UID. `CheckReady` validates
socket ownership/permissions, peer credentials, readiness, exact listener set,
and SHA256(JSON(DNSMeta)) from the immutable running configuration. The digest
matches `common/adapters/network.DNSDigest` without importing that adapter.

Status JSON version 1 contains `version`, `digest`, `addresses` (IP:port, each
bound on both UDP and TCP), and `ready`. Connect and read one JSON document until
EOF; no request body is needed. Readiness attests local configuration and bound
listeners, not availability of remote DNS services.

Cancellation drains bounded handlers and closes owned resources. Listener failure
withdraws readiness and stops the whole proxy. Cleanup unlinks the control socket
only if its identity still matches the socket created by this process.

## Pinned checks

From this directory:

```sh
make prepare
make test lint build
make race GO_IMAGE=docker.io/library/golang:1.26.6@sha256:23fdfd3a6abc97c81e32a724cdd1cf541c06c416eb04d717815f4ed7c75623d0
```

`prepare` snapshots common sources and creates vendor under
`/tmp/opencode/zinc-dns-check` only. It downloads checksum-verified pinned modules.
Checks run rootless/non-root with no network or capabilities. Tests use private
loopback/ephemeral sockets. The race command uses the already pinned Debian image
for its C compiler. Ordinary repository checks use the refreshed vendor trees.

Dependencies verified against tagged upstream manifests and the Go module proxy:

- [miekg/dns v1.1.73](https://github.com/miekg/dns/blob/v1.1.73/go.mod).
- [quic-go v0.63.0](https://github.com/quic-go/quic-go/blob/v0.63.0/go.mod), requiring Go 1.26.
- Build/check image: `docker.io/library/golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83`.
- Both `common/go.mod` and `go.work` declare `go 1.26.0`; actual build toolchain is Go 1.26.6.
