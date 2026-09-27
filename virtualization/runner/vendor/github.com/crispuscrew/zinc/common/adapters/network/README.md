# Provisioned packet network manifest, version 1

No adapter creates namespaces, routes, TAPs, bridges, host listeners or host
firewall rules. A provisioner must create one exclusive enforcement namespace
per app and a user namespace accessible to the invoking user. It supplies a
strict JSON file at `$ZINC_NETWORK_MANIFEST_DIR/<AppNameID>.json`, defaulting to
`$XDG_RUNTIME_DIR/zinc/network/<AppNameID>.json`. Files and parent directories
must be root/user-owned and protected from other writers; symlinks are refused
for the manifest and its parent directories. Duplicate and unknown JSON keys
are errors. Namespace paths are checked against nsfs and pinned inode numbers;
the running initializer checks both identities again after entering them.

The `Manifest` Go type is the wire format. `policy` is the exact NetworkMeta
snapshot (its keys use the schema's Go field names). `topology.mode` is
`container` or `tap`. All declared interfaces must map exactly once. MACs are
canonical unicast addresses; a provisioner assigns generated MACs when the
schema omits one. Assigned addresses are literal IPv4/IPv6 addresses, not CIDRs.

Container devices live in the application namespace. TAP devices live in the
guest's routing/enforcement namespace; QEMU opens each pre-created TAP there.
There must be no bridge or alternate path that bypasses the inet forward hook.
`complete_inventory` attests the manifest enumerates every reachable Zinc app
and every host address, so Any cannot bypass an omitted peer's endpoint policy.
The provisioner supplies static addressing/neighbors, enables forwarding for
TAP topology, and prevents alternate guest MAC/IP identities or outside source
spoofing. The adapter additionally enforces exact endpoint sources, routes,
MACs and ARP sender identity. DHCP/RA services are not inferred or started.

Peer interface `device` means the path to that peer *as seen in this app's
enforcement namespace*. Each peer includes its authoritative NetworkMeta.
Every packet to/from a peer must pass both ordered policies. Host mappings
identify all host addresses (including public ones); external interfaces
identify packet-preserving uplinks. Internet excludes those addresses, every
app address, and conservative special/private/link-local ranges. AnyApp includes
Self and the manifest's registered peers; identity is exact AppNameID (instances
must be provisioned under their runtime identities).

DNS upstream transport descriptions do not install firewall exceptions.
`dns_proxy_addresses` refer only to an already-provisioned plaintext local
forwarder whose configuration hashes to `dns_config_digest` (DNSDigest).
`dns_control_socket` names its private Unix status socket; both runners verify
the live configuration and exact UDP/TCP listener set before preparing a launch.
Its address must be routable in the provisioned topology, not namespace loopback.
App policy must allow access to that address/port. Encrypted upstream endpoints
are never placed directly in resolv.conf. The separately integrated DNS worker
owns readiness and authenticated upstream transport. Domain lookups require
an explicitly injected Lookup receiving the owner's configured DNSMeta; no
host fallback exists. Results are a frozen launch snapshot, not a live DNS
hostname firewall. A failed/empty lookup aborts even a deny rule.

Changing a policy, address or namespace requires reprovisioning and relaunch.
The manifest is authoritative, not an auto-discovery cache. Peer registration
must remain valid for the namespace lifetime. Namespace reuse while an old
process is still running is forbidden. Application network-admin privileges
must never be granted. KeepUserID requires a matching provisioned user mapping.

VM callers validate runtime port mappings with `CheckForwards`; `publications`
must match protocol, bind address, both ports, and logical NIC exactly. Raw
slirp hostfwd arguments are refused, not widened or silently copied onto TAPs.
VM callers use `netns.Configure` for a runtime-only MAC-bound config and the
QEMU attachment list, then call `CommandResolved` with their approved Lookup.
`nsenter`, `nft`, `ip`, and util-linux `setpriv` must be installed in the launch
environment. BusyBox `setpriv` lacks the required capability-bounding options.
The VM supervisor retains only the provisioning authority; QEMU is started by
setpriv with all capability bounding/inheritable/ambient sets dropped.

The adapter changes only Zinc tables inside the dedicated namespace. It loads
default deny before the full policy; rollback restores default deny rather than
reopening an old policy. Stateful admissions carry a hash of the resolved policy
so replacing policy does not automatically trust old conntrack entries.
