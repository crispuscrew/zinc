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
zcr where <app[@instance]> [--json]
                            where the instance keeps its state, what its container is
                            called, and its filtered bus socket and proxy
zcr bus [--json]            the bus attribution table (see below)
zcr net [app] [--json]      running apps and whether each has a locked netns, or one
                            app's nftables counters
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
  container. The sibling must agree with `Forward: true` on its own link ingress list, and
  bounds what it carries with `ForwardPorts` (empty carries any port; the destinations were
  already fixed by the client's routes). Gateways chain: a hop can forward onward into
  another gateway rather than out to the network.
- `Domains` on an egress list: destinations named rather than numbered, resolved at launch
  into addresses. An address allowlist taken at that moment, not hostname filtering, and not
  refreshed while the app runs.
- `NetworkMeta.DNSServers`: the resolvers the app is handed, and the only ones it may reach.

The runtime is fail-closed: anything it does not yet support is rejected, not run.
Not supported in this build yet: host-scoped egress and gateway/multi-homing. (A sibling link
may now coexist with other networking on one app - that is what routing is built on.)

### Seeing what it did

```
$ zcr net
ADDRESS        POSTURE   NETNS
netprobe@work  filtered  netprobe.work-pod
quiet          isolated  -

$ zcr net netprobe@work
CHAIN   VERDICT  RULE                PACKETS  BYTES
output  drop     undeclared dns udp  1        57
output  accept   list[0] ip tcp      3        180
output  drop     default policy      9        652
```

`filtered` means the app has `NetworkLists`, so a pod netns of its own with the ruleset
locked in it. `isolated` means it has none, so `--network none`: it reaches only its own
localhost and has no netns or ruleset at all. The two are never merged - an isolated app is
not a filtered one whose counters happen to be zero.

`list[0]` is the entry's index in `NetworkMeta.NetworkLists`, so a number points at the line
of config that produced the rule. `default policy` is what the chain refused. Counters live
in the pod's netns and are created with it, so they read **since this launch** and are gone
when the pod is - `stop` and `restart` both reset them. `--json` gives either form as a
machine-readable document.

## Filtered session bus

An app gets **no D-Bus session bus** unless it asks for one. The host bus is a desktop-wide
capability - the keyring, the portal, the compositor, every other service the user runs - so
mounting it into a sandbox would undo most of the sandbox.

`DBusMeta` opens exactly what it names and nothing else:

```yaml
InternalUserMeta:
  KeepUserID: true                          # required: the proxy serves the socket as you
DBusMeta:
  Talk:                                     # names the app may call
    - org.freedesktop.portal.Desktop
  Own:                                      # names the app may claim
    - org.mpris.MediaPlayer2.notes
```

`zcr` runs `xdg-dbus-proxy` in the helper image, in a container it owns, which holds the real
socket and serves the app a filtered one. The proxy is deliberately **not** a member of the
app's pod: a pod shares the PID namespace, so a proxy inside it would be a process the app
could signal or ptrace. They share one thing, the socket. The app's socket directory is per
app, so two apps with different grants cannot reach each other's bus.

A trailing `.*` in `Talk` matches a subtree, including services that appear under it later, so
prefer an exact name. `Own` takes no wildcard: a process claims one concrete name or none.

Fail-closed here too. An app that asks for a bus when no host bus can be resolved does not
start, rather than starting with no bus and looking broken for reasons unrelated to its
config. `DBusMeta` on a VM app is a validation error - a guest cannot take a bind-mounted
unix socket.

### Attribution: which app is a connection on the host bus

Zinc creates the proxy and names it after the app, so it knows which host-bus connection
belongs to which `app@instance` without asking the app anything. It publishes that mapping
rather than having apps claim names for themselves - a name a sandboxed app claims is a
self-assertion, which is what attribution exists to stop trusting (architecture doc, 5.8):

```
zcr where <app[@instance]> [--json]   state dir, container name, bus socket, bus proxy
                                      ("none" / null when the app asked for no bus)
zcr bus [--json]                      every running proxy: app@instance, host pid, socket
```

Given something seen on the host bus, ask it for the connection's pid
(`org.freedesktop.DBus.GetConnectionUnixProcessID`, an `SO_PEERCRED` fact the peer cannot
assert) and look that pid up in `zcr bus`:

```sh
zcr bus --json | jq -r --argjson pid 12345 '.[] | select(.pid == $pid) | .address'
```

`xdg-dbus-proxy` opens one upstream connection per client, so an app with no live bus client
has no connection on the host bus at all, and an app with several has several - all with the
proxy's one pid.

## Build

Podman-only, reproducible in a pinned container:

```
make build            # produces ./bin/zcr
make check            # gofmt + vet + test, in-container
make netfilter-image  # build the helper image: the nft lock-down and the D-Bus proxy
```

## Layout

Hexagonal: `domain` (schema-derived types, pure), `ports` (interfaces), `app`
(orchestration), `adapters/*` (podman, netenforce, fs, host), `wire` (composition), and
`main.go` (the CLI). It depends only on the shared `common` library for the app schema
and validation.
