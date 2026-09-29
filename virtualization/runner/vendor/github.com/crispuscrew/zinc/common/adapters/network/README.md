# Provisioned packet network manifest, version 1

The owner provisions one exclusive enforcement namespace per app and an accessible
user namespace. Zinc creates no namespaces, routes, TAPs, bridges, host listeners or
host firewall rules. See the [provisioning contract](../../../docs/network-provisioning.md).

Supply strict [`Manifest`](manifest.go) JSON at `$ZINC_NETWORK_MANIFEST_DIR/<AppNameID>.json`,
default `$XDG_RUNTIME_DIR/zinc/network/<AppNameID>.json`. Manifest/parents must be
root/user-owned, protected from other writers and symlink-free; unknown/duplicate keys fail.

Namespace paths must match nsfs/pinned inodes, rechecked after entry and before firewall
operations. Podman's current user namespace is reused on an exact identity match;
otherwise the provisioned path is entered. Probe failure aborts either path.

- `policy`: exact `NetworkMeta` snapshot using schema Go field names. Every interface
  maps once; MACs are canonical unicast (provisioner-assigned if absent), IPs literal, not CIDRs.
- `topology.mode`: `container` devices live in the app namespace; `tap` devices live in
  guest routing/enforcement namespaces. No path may bypass the inet forward hook.
- Provisioners supply static addressing/neighbors, TAP forwarding and anti-spoofing.
  The adapter checks sources/routes/MACs/ARP identity; no DHCP/RA is inferred.
- `complete_inventory` includes every reachable Zinc app, its policy and all host
  addresses. Peer `device` is the path seen from this enforcement namespace.
  Both endpoint policies apply; identities are exact AppNameID, including runtime instances.
- Host mappings include public host IPs; external interfaces are packet-preserving uplinks.
  Internet excludes host/app and special/private/link-local ranges; AnyApp includes Self/peers.

DNS grants create no firewall exceptions. `dns_proxy_addresses` name provisioned,
routable plaintext forwarders, not namespace loopback or encrypted upstreams.
`dns_config_digest` must match `DNSDigest`; policy must allow the proxy address/port.

Both runners verify live configuration and exact UDP/TCP listeners through private
`dns_control_socket`. The [DNS worker](../../../docs/dns.md) owns readiness/upstream
authentication; encrypted upstream endpoints never go directly into `resolv.conf`.

Injected `Lookup func(schema.DNSMeta, string) ([]netip.Addr, error)` receives the policy
owner's DNS configuration, with no host fallback. Results are frozen launch snapshots;
failed/empty resolution aborts even deny rules. This is not a live hostname firewall.

VM callers use `CheckForwards`: `publications` must match protocol, bind, both ports
and NIC exactly; raw slirp hostfwd is refused. `netns.Configure`/`ConfigureResolved`
return runtime-only MAC binding/attachments; `CommandResolved` takes the approved Lookup.

VM launch needs `nsenter`, `nft`, `ip` and util-linux `setpriv` (BusyBox is insufficient).
The supervisor retains provisioning authority; QEMU drops bounding/inheritable/ambient
capabilities. Apps must never get network-admin; KeepUserID needs matching user mappings.

Policy/address/namespace changes require reprovisioning and relaunch. Keep peer
registration valid throughout namespace lifetime; never reuse it while old processes run.
The manifest is authoritative, not discovery data.

Only Zinc tables change. Default-deny precedes full policy and is restored on rollback.
Stateful admissions hash resolved policy; new policy does not trust old conntrack entries.
