# zcr - Zinc Container Runner

`zcr` is the Zinc container runtime. It reads an app file
(`~/.config/zinc/apps/<name>.yaml`) and runs it as a rootless podman container, applying
the network lock-down before the app starts. It is the binary `zc` (the creator) shells
out to; you can also drive it directly.

## Commands

```
zcr run <app> [--exec]      print the launch plan, or launch it (--exec)
zcr build <app>             (re)build the app's derived image (ImageMeta.Install)
zcr validate <app>          parse + validate; report problems and warnings
zcr stop|restart|inspect <app>
zcr logs <app> [-f]
zcr term <app> [--shell]    open a terminal for a multiterminal app
zcr ps                      running apps, one per line
zcr image search <term> | resolve <ref>
```

`<app>` is a store name (`~/.config/zinc/apps`) or a path (contains `/` or ends in
`.yaml`). Without `--exec`, `run` is a dry run: it validates and prints the exact podman
command(s) and any nft ruleset that would be enforced, so what will happen is visible
before anything runs.

## Network lock-down

An app's `NetworkMeta.NetworkLists` drive a fail-closed firewall applied in the app's own
network namespace before it starts:

- No lists: the app reaches only its own localhost (isolated).
- Egress list: default-drop, allow only the listed destination CIDRs/ports.
- Ingress + Host: publish the app's own ports to the LAN, filtered by source.
- Sibling link (an egress list naming another app): a private internal bridge between the
  two apps, gated per-port by interface.
- Routing through a sibling (`Via` on an egress list naming an app): that list's destinations
  leave through the sibling instead of this app's own egress - how an app is put behind a VPN
  container. The sibling must agree with `Forward: true` on its own link ingress list.
- `Domains` on an egress list: destinations named rather than numbered, resolved at launch
  into addresses. An address allowlist taken at that moment, not hostname filtering, and not
  refreshed while the app runs.
- `NetworkMeta.DNSServers`: the resolvers the app is handed, and the only ones it may reach.

The runtime is fail-closed: anything it does not yet support is rejected, not run.
Not supported in this build yet: host-scoped egress and gateway/multi-homing. (A sibling link
may now coexist with other networking on one app - that is what routing is built on.)

## Build

Podman-only, reproducible in a pinned container:

```
make build            # produces ./bin/zcr
make check            # gofmt + vet + test, in-container
make netfilter-image  # build the nft helper image the lock-down applies rules with
```

## Layout

Hexagonal: `domain` (schema-derived types, pure), `ports` (interfaces), `app`
(orchestration), `adapters/*` (podman, netenforce, fs, host), `wire` (composition), and
`main.go` (the CLI). It depends only on the shared `common` library for the app schema
and validation.
