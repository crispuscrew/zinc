# Mandatory packet-preserving provisioning

Declared NICs require owner-provisioned topology; empty `NetworkMeta.Interfaces`
is offline. Zinc creates no pasta/namespaces/bridges/TAPs/routes/host listeners/firewall.
No universal setup command or bundled provisioner exists. Follow the authoritative
[contract](../common/adapters/network/README.md) and [wire type](../common/adapters/network/manifest.go).

## Manifest and trust

Supply v1 JSON: `$ZINC_NETWORK_MANIFEST_DIR/<AppNameID>.json`, default
`$XDG_RUNTIME_DIR/zinc/network/<AppNameID>.json`. Required contract checks:
- Root/user-owned manifest/ancestors, protected from other writers; no symlinks,
  duplicate or unknown keys. `policy` exactly snapshots `NetworkMeta` using Go field names.
- Each logical NIC maps once. `network_namespace`/`user_namespace` pin dedicated
  nsfs inodes, rechecked inside initialization; the invoking user must enter the user namespace.
- `packet_preserving`, `exclusive`, `static_neighbors`, `complete_inventory` must
  hold for the entire namespace lifetime; booleans cannot establish correct topology.
- Literal unicast IPs, not CIDRs; canonical nonzero unicast MACs, assigned if omitted.
  `KeepUserID` needs matching user mappings; instances use their runtime identities.
- Inventory every reachable app/policy, host address and packet-preserving uplink.
  Peer `device` is the path seen from this enforcement namespace, not a global name.

## Container and TAP paths

`topology.mode: container` places devices in the app namespace; join follows policy
installation. Never grant the app network-admin. Build the initializer/D-Bus helper
with `make -C container/runner netfilter-image`: `--pull never`, dropped capabilities,
only namespaced `NET_ADMIN` restored for initialization; initializer exits before launch.

`topology.mode: tap`: pre-create TAPs in the routing/enforcement namespace; QEMU uses
`script=no,downscript=no`. Provision forwarding/static addresses/neighbors matching the
guest; no inferred DHCP/RA. Every packet must cross inet forward, never a bypass bridge.
Prevent guest MAC/IP changes and outside spoofing; adapter checks sources/routes/MACs/ARP.
SCTP/GRE/ESP/AH need packet-preserving attachment too; slirp/socket forwarding is insufficient.

VM prerequisites: `nsenter`, `nft`, `ip`, util-linux `setpriv` (BusyBox lacks required
options). QEMU drops bounding/inheritable/ambient capabilities; supervisor retains only
provisioning authority.

## Publications and DNS

VM-options `ForwardPorts` must match existing `publications`: protocol, bind IP,
host/guest ports, logical NIC. Wildcard cannot satisfy loopback; no listener is created.
Provisioned topology refuses raw slirp `hostfwd`.

DNS needs routable non-loopback bare `dns_proxy_addresses` (plaintext port 53),
`dns_config_digest = DNSDigest(NetworkMeta.DNS)` and private `dns_control_socket`.
Both runners [authenticate live digest/listeners](dns.md#manifest-binding-and-authenticated-readiness)
before network preparation. App rules must allow the proxy; encrypted upstreams
cannot go in `resolv.conf`, and manifests grant no exception.

## Startup, failure and changes

Order: validate binding, resolve domains, install default-deny, install full policy, join.
Only Zinc tables in the dedicated namespace change; rollback restores default-deny.
Address/namespace/policy/publication changes require reprovision/relaunch; manifests
are authoritative. Never reuse a live namespace; retain peer registration for its lifetime.
`DependsOn` provides ordering, not topology, DNS-worker readiness or service health.
