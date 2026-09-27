# Mandatory packet-preserving provisioning

Networked containers and VMs require an owner-provisioned topology before
launch. Zinc does **not** automatically create rootless pasta networking,
namespaces, bridges, TAPs, routes, host listeners or host firewall rules.
Empty `NetworkMeta.Interfaces` is the offline path.

The full contract and wire type are in
[common/adapters/network](../common/adapters/network/README.md) and
[manifest.go](../common/adapters/network/manifest.go). There is no universal
host-setup command or bundled provisioner promised by these docs.

## Manifest and trust

Supply version 1 JSON at `$ZINC_NETWORK_MANIFEST_DIR/<AppNameID>.json`, default
`$XDG_RUNTIME_DIR/zinc/network/<AppNameID>.json`.

- Manifest and parent directories must be root/user-owned and protected from
  other writers. Symlinks, duplicate JSON keys and unknown keys are refused.
- `policy` is the exact authoritative `NetworkMeta` snapshot, using Go field
  names as JSON keys. Logical interfaces must map exactly once.
- `network_namespace`, `user_namespace` and their pinned inodes identify the
  dedicated namespace pair. They are checked against nsfs and rechecked inside
  the initializer. The invoking user must be able to enter the user namespace.
- `packet_preserving`, `exclusive`, `static_neighbors`, `complete_inventory`
  attest requirements the provisioner must actually hold for the namespace's
  entire lifetime. A boolean is not a substitute for correct topology.
- Addresses are exact literal unicast IPs, not CIDRs. MACs are canonical,
  nonzero, unicast addresses; the provisioner assigns one if YAML omits it.
- `KeepUserID` requires matching provisioned user mappings.

The inventory includes every reachable app, its authoritative policy, all host
addresses and packet-preserving external interfaces. Peer `device` names mean
the path seen from this app's enforcement namespace, not a global host name.
Instances must be provisioned under their runtime identities.

## Container and TAP paths

`topology.mode: container` places application devices in its network namespace.
The container joins the approved namespace after default-deny and full policy
installation. The app must never have network-admin capability.

Container initialization uses the local helper built by
`make -C container/runner netfilter-image`, with `--pull never`, dropped
capabilities and only namespaced `NET_ADMIN` restored. It exits before the app
starts. The same image carries the separate D-Bus proxy; neither use creates
the required host topology.

`topology.mode: tap` provides pre-created TAPs in the guest's routing/enforcement
namespace. QEMU attaches with `script=no,downscript=no`. The provisioner enables
forwarding and supplies static addressing/neighbors; guest/network configuration
must agree. Zinc starts no inferred DHCP or router-advertisement service.

Every guest packet must traverse the inet forward hook. A bridge or alternate
path that bypasses it is invalid. Provisioning must prevent guest MAC/IP changes
and outside source spoofing; the adapter also checks endpoint sources, routes,
MACs and ARP sender identity. Packet-preserving attachment is required for SCTP,
GRE, ESP and AH as well as TCP/UDP; slirp/socket forwarding is not equivalent.

The VM launch environment needs `nsenter`, `nft`, `ip` and util-linux `setpriv`.
BusyBox's version lacks the required capability-bounding options. QEMU starts
with capability bounding, inheritable and ambient sets dropped; the supervisor
retains only its provisioning authority.

## Publications and DNS

VM `ForwardPorts` live in external VM-options JSON. The manifest's
`publications` must match protocol, bind IP, host port, guest port and logical
NIC exactly. A wildcard bind does not satisfy a loopback request. These are
checks of existing publications, not instructions to open a listener.
Raw slirp `hostfwd` arguments are refused in a provisioned topology.

Configured DNS requires routable, non-loopback `dns_proxy_addresses` and
`dns_config_digest` equal to `DNSDigest(NetworkMeta.DNS)`. These manifest
addresses are bare IPs used for plaintext DNS on port 53. Encrypted upstream
endpoints cannot be placed in `resolv.conf`. The app must allow the proxy in
its rules; the manifest creates no exception.

`dns_control_socket` binds the worker's private authenticated readiness socket.
Both runners verify its live configuration digest and exact UDP/TCP listener set
before preparing the network launch. See [DNS integration](dns.md).

## Startup, failure and changes

The adapter validates binding, resolves domain snapshots, installs default-deny
before full rules, then permits the runtime to join. It changes only Zinc tables
inside the dedicated namespace. Rollback restores default-deny rather than
reopening a previous policy.

Changing addresses, namespaces, policy or publications requires reprovisioning
and relaunch. A manifest is authoritative, not an auto-discovery cache. Do not
reuse a namespace while an old process remains, and keep peer registration
valid for its lifetime. `DependsOn` supplies runtime ordering, not topology,
DNS-worker readiness or application service health.
