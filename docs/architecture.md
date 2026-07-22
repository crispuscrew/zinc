# Zinc - Architecture

> **Priority order: Stable, then Secure, then Beautiful.**
> Keyboard-first, no mouse required. Zinc is a security-focused sandboxing core: it runs
> user-facing apps in rootless Podman containers, each walled off from the rest of the
> desktop, on any Linux distribution (Fedora is the primary development target).

This document is the single source of truth for what Zinc actually ships. Section numbers
are cited from the code (for example "architecture.md 5.3" points at the network model), so
the numbering is kept stable. It targets the **0.1 release**, which ships two tools: `zcc`
(the container creator) and `zcr` (the container runner). Everything marked *planned* is on
the roadmap and does not ship yet.

---

## 1. Design Principles

- **Stable first** - declarative config, pinned versions, reproducible from scratch on any
  machine.
- **Secure by default** - minimum host surface; a container gets nothing unless the config
  explicitly grants it. Fail-closed: anything the runtime cannot enforce is rejected, not
  run.
- **Beautiful always** - a consistent visual language, but never at the cost of the two
  priorities above.
- **Keyboard sovereign** - every interaction is reachable without a mouse.
- **No magic** - every decision is explicit, documented, and reversible. A dry run prints
  the exact `podman` commands and firewall rules before anything executes.
- **Honest about limits** - where a security mechanism is partial (see 5.2), the doc says so
  loudly rather than marketing it as more than it is.

---

## 2. Stack Overview

Zinc is the sandboxing core. It is compositor-agnostic and installs cleanly on top of an
existing system; **ZDE** (the Zinc Desktop Environment) is a separate project layered on top
and is out of scope here.

```
+-------------------------------------------------------------+
|  Any Wayland compositor                                     |
|  + wayland-security-context (label applied; enforcement is  |
|    the compositor's, see 5.2)                               |
+-----------------------------+-------------------------------+
|  zcc - zinc-container-creator (CLI + Bubbletea TUI, Go)     |
|    authors app files, forwards run/manage to zcr            |
|                          | shells out over $PATH            |
|  zcr - zinc-container-runner (runtime, Go)                  |
|    reads an app file, runs it, locks the network down       |
+-----------------------------+-------------------------------+
|  Config store (YAML)                                        |
|    ~/.config/zinc/apps/<name>.yaml   (app definitions)      |
|    ~/.config/zinc/zcc                (zcc's TUI keybinds)   |
+-----------------------------+-------------------------------+
|  Rootless Podman + pasta networking                         |
|    per-app network namespace (a pod), locked by an nftables |
|    ruleset before the app starts (5.3); the ruleset is      |
|    applied by a small digest-pinned netfilter helper image  |
+-----------------------------+-------------------------------+
|  Any Linux distribution (Fedora is the primary dev target)  |
+-------------------------------------------------------------+
```

There is no daemon, no host firewall change, and no persistent background service. `zcc` and
`zcr` are two static binaries you put on `$PATH`.

---

## 3. App Config (YAML)

One YAML file per app: `~/.config/zinc/apps/<name>.yaml`. The format is **schema version 2**.
The same file is validated identically at author time (in `zcc`, on save) and at launch time
(in `zcr`, before anything runs), because the validation is pure and shared - so a manual
edit or drift cannot slip an invalid config past launch.

`AppConfig` is a flat struct with grouped `*Meta` sub-structs. There are no presets and no
network "modes"; behavior is exactly the fields below.

```yaml
SchemaVersion: 2                 # must be 2 (the only version this build understands)
Type: ZincContainer              # ZincContainer today (ZincVirtualization is planned)

AppNameID: firefox               # also the container/pod name; [a-z0-9._-], starts alphanumeric
Icon: firefox
Description: Web browser

StartConditions:
  DependsOn: []                  # other apps that must be up first (auto-started, see 6.6)
  Autorestart: false             # restart only on failure (a clean exit / manual stop is final)
  Entrypoint: firefox            # process to run; empty = the image's default command
  Terminal: false                # CLI/TUI app: launch in a host terminal-emulator window
  Multiterminal: false           # many terminals attach to one shared container (see 9.1)
  MultiterminalEntrypoint: ""    # per-terminal command; empty = Entrypoint

StopConditions:
  KeepAlive: false               # keep the container after the entrypoint exits (no --rm)
  Background: false              # stay alive after the window closes

ResourcesMeta:                   # validated (>= 0; 0 = unlimited). Runtime enforcement: roadmap
  MaxCPUCores: 2                 # fractional allowed (0.5)
  MaxRamMiB: 2048
  MaxSwapMiB: 0
  PIDsLimit: 512                 # fork-bomb guard

InternalUserMeta:                # validated. Runtime wiring (--user): roadmap
  UseNonRootUser: true
  NonRootUserName: app
  KeepUserID: false

ImageMeta:
  Image: docker.io/library/alpine@sha256:...  # third-party images MUST be digest-pinned (5.5)
  Install:                       # optional shell lines; if set, Zinc builds a derived image (5.5, 7)
    - apk add --no-cache firefox font-dejavu

DisplayMeta:
  DisableSecurityContext: false  # false = the wp_security_context_v1 label is applied (5.2)
  DisableGpuAccess: true         # true = no /dev/dri (default off; GPU weakens isolation, 5.4)

NetworkMeta:
  NetworkLists: []               # empty = isolated (own localhost only). See 5.3 and section 6.

NotificationMeta:                # schema-defined; runtime wiring: roadmap
  Disabled: false
  Silenced: false
  UseCustomPrefix: false
  CustomPrefix: ""
  AllowedActions: false
  AllowedProlonged: false
  AllowedLinks: false

Configs: []                      # bundle-relative config mounts; DEFERRED (not wired yet)
Volumes: []                      # explicit host bind mounts are wired; see below
Keys: []                         # SSH/GPG convenience mounts; see below
HostTheme: true                  # mount the curated host theme bundle read-only (5.6)
AudioMeta:
  Pipewire: false                # pass the Pipewire socket in
  LegacyALSA: false              # mount /dev/snd for ALSA-only apps (rare)
Capabilities: []                 # extra `--cap-add` entries, on top of the drop-all baseline
```

**Volumes.** Each `Volume` is explicit; there is no implicit home access. The runner wires
only **explicit host bind mounts** today (`HostMounted: true` with a `HostMount` path): it
maps `HostMount:InnerMount` with `ro`/`rw` from `Writable` and `noexec`/`exec` from
`Executable`. Anonymous and `SizeLimited` volumes are schema-defined but not wired yet.

```yaml
Volumes:
  - HostMounted: true
    HostMount: /home/user/Downloads
    InnerMount: /home/user/Downloads
    Writable: true
    Executable: false
```

**Keys.** A convenience layer for SSH/GPG only: unlike a plain volume, a `Key` mounts the key
read-only into the container home (`.ssh` for `SSH`, `.gnupg` for `GPG`). Per-key explicit
opt-in.

```yaml
Keys:
  - Type: SSH
    Path: /home/user/.ssh/id_ed25519
```

**Wired at runtime in 0.1:** identity/image, the network attach and lock-down, the
capability drop-all baseline plus `Capabilities`, Wayland socket + security-context label,
GPU device, the theme bundle, audio (Pipewire socket / `/dev/snd`), explicit host bind
mounts, SSH/GPG key mounts, the entrypoint override, and the terminal / multiterminal /
background / keep-alive lifecycle. **Schema-defined but not yet wired into the
launch:** `ResourcesMeta`, `InternalUserMeta`, `NotificationMeta`, and `Configs`. They are on
the roadmap and are called out here so the doc does not overclaim.

---

## 4. Tools and the creator / runner split

Every Zinc tool is named `zinc-<kind>-<role>`, where `<kind>` is `container` or
`virtualization` and `<role>` is `creator` or `runner`, plus a `zinc-launcher-<ui>` picker.
The short code is the initials.

| Short | Tool                          | Role                                   | Status  |
|-------|-------------------------------|----------------------------------------|---------|
| `zcc` | `zinc-container-creator`      | define container apps (write configs)  | 0.1     |
| `zcr` | `zinc-container-runner`       | launch + supervise a container app     | 0.1     |
| `zvc` | `zinc-virtualization-creator` | define VM apps                         | planned |
| `zvr` | `zinc-virtualization-runner`  | launch + supervise a VM app            | planned |
| `zlg` | `zinc-launcher-gui`           | fast app launcher (GUI)                | 0.3     |
| `zlt` | `zinc-launcher-tui`           | fast app launcher (TUI)                | 0.2     |

**The split is architectural, not cosmetic.** A *creator* authors an app and writes its
config; a *runner* actually starts that app and owns its lifecycle. `zcc` (the creator)
depends only on the shared `common` library and **knows nothing about podman**. To run what
it authors, it shells out to the `zcr` binary on `$PATH`. The two meet only at the on-disk
YAML format and at that process boundary; `zcc` never imports `zcr`.

```
   author / manage                        run / supervise
   +-----------------+   shells out to    +------------------+
   |  zcc (creator)  |  ---- $PATH ---->  |  zcr (runner)    |
   |  depends: common|   run/build/stop   |  depends: common |
   |  no podman code |   logs/term/image  |  drives podman   |
   +-----------------+                    +------------------+
             \                                   /
              \______ on-disk YAML app file ____/
                 ~/.config/zinc/apps/<name>.yaml
```

`zcc` authoring commands (`new`, `list`, `validate`, `delete`, `tui`, `keys`) work without
`zcr`. The runtime commands (`run`, `build`, `stop`, `restart`, `inspect`, `logs`, `term`,
`image`) are forwarded verbatim to `zcr`, streaming its output and preserving its exit
status; if `zcr` is missing, those commands fail with an actionable message while authoring
keeps working. Details in section 9.

---

## 5. Security Model - what each layer actually gives you

### 5.1 Container isolation (rootless Podman)

**Strong. This is the real security boundary.** Namespaces isolate PID, network, mounts,
IPC, and UTS. The user namespace maps container root to your unprivileged host user, so
"root in the container" is not root on the host. An app cannot see the host filesystem
outside its explicit mounts, cannot see other containers, and cannot escalate.

Every app container starts from a least-privilege baseline: `--security-opt
no-new-privileges --cap-drop all`. Anything the app genuinely needs is re-added explicitly
from `Capabilities`, and each capability is validated against a safe charset. The launch is
hermetic - `--pull never` - so a run never triggers a surprise registry pull; the image must
already be in local storage (resolved at author time or built as a derived image).

### 5.2 Wayland isolation (wayland-security-context)

**Partial in practice - be honest.** The `wp_security_context_v1` protocol lets the
compositor tag a client so it can be treated as untrusted. Zinc applies the marker: when
`DisplayMeta.DisableSecurityContext` is false (the default), the app container is labelled
`zinc.wayland=security-context` and the Wayland socket is passed in read-only.

But enforcement is the compositor's and the toolkit's job, not Zinc's. An app that ignores
the protocol still talks to the compositor. Treat the security-context label as a hint that
improves as compositors and toolkits adopt it, not as a wall. **The real isolation boundary
for a container is the container itself** (5.1). For genuinely untrusted GUI apps the future
answer is a VM (section 10), not a nested compositor.

### 5.3 Network isolation (per-app netns, fail-closed)

**Strong, and the crown jewel of the security model.** An app's `NetworkMeta.NetworkLists`
drive a fail-closed firewall applied in the app's **own network namespace before the app
process starts**. There is never an unfiltered window.

Mechanics:

- A filtered app (one with any `NetworkLists`) runs inside a **pod**. The pod's infra
  container owns the network namespace and provides connectivity via **pasta** (userspace,
  no host root, no host firewall change).
- Before the app container joins the pod, an **nftables ruleset is loaded into the pod's
  netns** by a one-shot init step: `podman run --pod <pod> --pull never --cap-drop all
  --cap-add NET_ADMIN --security-opt no-new-privileges <netfilter-image> nft -f -`, with the
  ruleset piped in on stdin. `CAP_NET_ADMIN` is namespaced to the pod's user namespace, so it
  grants nothing on the host.
- Only then is the app container started with `--pod <pod>`. The lock is in place first, so
  the app never sees an open network.
- An app with **no** `NetworkLists` gets `--network none`: it reaches only its own localhost,
  fully isolated.
- The whole thing is fail-closed. If any prepare step fails, the half-built pod is torn down.
  If the app dies after fork, a reaping goroutine tears the pod (and its netns) down too, so
  no rule-less netns is ever left behind.

Enforcement is a **port** in the runner hexagon (`NetEnforcer`, see section 13): the
pasta-plus-nft implementation is one adapter. Swapping the traffic-control mechanism later (a
different firewall, an eBPF egress filter, an external controller) is a new adapter, with the
launch path unchanged. The tiers and the exact ruleset are in section 6.

### 5.4 GPU passthrough

**Weak isolation when enabled.** Granting `/dev/dri` (via `DisplayMeta.DisableGpuAccess:
false`) exposes GPU rendering state more broadly than process boundaries suggest, and Linux
GPU sandboxing is immature. GPU access is **off by default**. Rule: never enable it for
untrusted code.

### 5.5 Image trust (digest pinning + derived images)

**Pin by digest, not by tag, for third-party images.** Validation enforces this: an
`ImageMeta.Image` that is not a `localhost/` reference must be a canonical digest pin
(`...@sha256:` followed by exactly 64 hex characters). Only `localhost/` images - which
resolve to local storage and can never pull something remote - may use a mutable tag. The
image reference must also be a single clean line (no whitespace or control characters),
because it is interpolated into a `FROM` line and into podman arguments.

To make pinning painless without a browser, `zcc image search <term>` and `zcc image resolve
<ref>` (forwarded to `zcr`) find an image and print its digest-pinned form to paste into
`ImageMeta.Image`; the TUI resolves the image field in place.

**Derived images** are the "quick setup" path and are covered in section 7. Their base
inherits the digest-pin rule above, so a derived build always starts from a known base.

### 5.6 Theme passthrough

**Default on, security impact minimal.** When `HostTheme` is true and a theme bundle is
available, the runner mounts a single curated directory read-only at `/etc/zinc/theme` inside
the container - not the host's real `~/.config` or `~/.themes`. The bundle path comes from
`ZINC_THEME_BUNDLE`; generating that bundle (GTK/Qt configs, icon and cursor themes, fonts)
is a job for the desktop layer (ZDE), out of scope for Zinc 0.1. The point of default-on is
that containerized apps look like part of the system; set `HostTheme: false` to deny it.

---

## 6. Networking model and startup ordering

This section expands 5.3: the scopes, the four tiers, the exact ruleset, and, at the end, how
dependent apps are started.

### 6.1 Scopes and direction

Each `NetworkList` entry is one directional rule. The fields that shape it:

- **`Host`** - scope. `false` (default) means the app's own netns. `true` means the host: a
  host-interface bind for LAN publishing (ingress), or host-scoped egress (not supported yet,
  6.5).
- **`AppName`** - for `Host: false`, which app's network. `""` (default) means this app
  itself; a name means a sibling app (a link, tier 2).
- **`Ingress`** - direction. `false` (default) is an **egress** rule: `Ports` are destination
  ports the app may reach. `true` is an **ingress** rule: `Ports` are the app's own listening
  ports, exposed to the scope.
- **`Blacklist`** - `false` (default) is a whitelist (default-drop; allow only what is
  listed). `true` is allow-all-except (default-accept with the listed entries dropped).
- **`IPv4CIDR` / `IPv6CIDR`** - destinations (egress) or allowed sources (ingress). The two
  families are emitted separately, so a v4 CIDR never gates v6 traffic.
- **`Ports`** - TCP+UDP port set for the rule.
- **`Interface`** - a specific host/app interface to bind (pasta copies its addressing).
- **`GatewayV4` / `GatewayV6`** - next-hop for multi-homing (not supported yet, 6.5).

List order is priority: the first entry wins. Blocking DNS, for example, is just an egress
blacklist for ports 53 and 853, ordered ahead of any broad allow so it wins. Validation
rejects an **egress port rule with no destination CIDRs** (it would silently no-op and leave
those ports open); name `0.0.0.0/0` and/or `::/0` for "everywhere", or drop the ports. An
ingress list needs no CIDR - empty means "any source".

### 6.2 The four tiers

| Tier | Config shape | Result |
|------|--------------|--------|
| Isolated | no `NetworkLists` | `--network none`; the app reaches only its own localhost |
| Egress   | egress list(s), self-scoped | default-drop; allow only the listed destination CIDRs and ports |
| LAN publish (tier 3) | `Ingress: true` + `Host: true` | publish the app's own ports to the LAN via pod `-p` forwards, filtered by source address |
| Sibling link (tier 2) | egress list naming another app's `AppName` | a private `--internal` bridge between the two apps, gated per-port |

**Sibling link, in detail.** A consumer's egress list that names a producer's `AppNameID`,
and the producer's own self-scoped ingress list, attach both apps to a private, internal
bridge `zinc-link-<producer>` with fixed interface names (`zlink0`, `zlink1`, ...) and a
network alias equal to each app's `AppNameID` (so a consumer connects to `<producer>:<port>`).
The producer accepts only its published `Ports` inbound on that link interface; everything
else default-drops. The consumer accepts nothing new inbound. The bridge is `--internal`, so
neither app reaches anything else through it.

### 6.3 The nftables ruleset

The ruleset is a pure function of the validated config, rendered as `table inet zinc`. A
standard (egress and/or tier-3) app builds an `output` chain (egress: `daddr`/`dport`) and,
when it publishes, an `input` chain (ingress: `saddr`/`dport`). A tier-2 (sibling-link) app
is gated by interface instead: the private `zlink*` bridges are accepted, and a producer's
published ports are accepted inbound on its own link interface, nothing else.

Every chain defaults to `policy drop` and always accepts loopback and
`established,related` traffic (so a server's reply rides the established rule). Per-direction
policy follows that direction's lists: a whitelist present means default-drop (fail-closed);
an all-blacklist direction means default-accept with the listed drops as high-priority
carve-outs. A direction with no lists stays default-drop - a pure publisher gets no egress; an
egress-only app gets no input chain at all.

A dry run (`zcr run <app>` with no `--exec`) prints the exact `podman` commands **and the nft
ruleset that would be piped in**, so what will be enforced is fully visible before anything
runs.

### 6.4 The netfilter helper image

The nft step runs inside a tiny helper image, `zinc/netfilter:local`, built once with `make
netfilter-image` (in `container/runner`). It is a digest-pinned Alpine base with a
version-pinned `nftables`. It is referenced by a local tag and run with `--pull never`, so the
privileged step is always the locally vetted build and never a registry pull; a missing image
fails fast with a clear error. It runs with `--cap-drop all --cap-add NET_ADMIN
--security-opt no-new-privileges`, reads the ruleset on stdin, and exits - the rules persist
in the pod's netns for the app that starts next.

### 6.5 Not supported yet (rejected, not run)

The runtime is fail-closed: a config it cannot enforce correctly is **rejected at launch**,
never silently mis-enforced. Rejected in this build:

- **Host-scoped egress** (`Host: true` with an egress list).
- **Gateway / multi-homing** (`GatewayV4` or `GatewayV6` set) - the fields are schema-legal,
  but a config using them is rejected.
- **An ingress list that targets an `AppName`** - contradictory (a producer publishes to any
  sibling that joins its link; the consumer names the producer).
- **Combining a sibling link with any other networking on the same app** - a tier-2 app must
  be link-only for now.

Also deferred at the mount layer: bundle-relative `Configs` mounts and anonymous/size-limited
volumes; only explicit host bind mounts are wired (section 3).

### 6.6 Dependency startup ordering

`StartConditions.DependsOn` lists apps that must be up before this one. On launch the runner
brings them up first, depth-first, so a dependency's own dependencies come up before it. An
already-running dependency is left untouched; a dependency cycle is reported as an error
rather than recursed into forever. This is why launch is a single orchestrated path (the app
layer, section 13) rather than a bare `podman run`.

---

## 7. Images: derived builds and base images

**Third-party images are pulled and pinned by digest** (5.5). **Derived images** are the
quick-setup path: instead of authoring a Containerfile, an app sets `ImageMeta.Install` to one
or more shell lines, and Zinc builds a small image `FROM <ImageMeta.Image>` plus a single
`RUN` layer carrying those lines (joined with `&&`, so a multi-step setup fails fast). The
app then runs that derived image instead of the bare base.

- The derived image is tagged locally, `zinc/app-<name>:local`, built with `-t`, run with
  `--pull never`, and never pushed.
- Its `FROM` base is `ImageMeta.Image`, which 5.5 forces to be digest-pinned (or a
  `localhost/` ref), so the build starts from a known base. Because a per-machine local build
  has no meaningful registry digest, the derived image is not itself digest-pinned; its
  guarantee is the pinned base plus the visible install lines.
- Freshness is tracked by an OCI label, `zinc.build`, holding a fingerprint of the inputs
  (the base image plus the install script). A plain `zcr run` rebuilds automatically when the
  image is missing or the fingerprint differs, so an unchanged app reuses its image and a
  re-pinned base or edited install takes effect on the next run. `zcr build <app>` (or `zcc
  build`, or the TUI build action) forces a rebuild.
- The install line runs through the image's own `/bin/sh`, so a distro package-manager
  invocation works exactly as typed. The form offers a per-distro hint (apt for
  debian/ubuntu, apk for alpine, dnf for fedora/rhel, pacman for arch, zypper for openSUSE),
  derived from the base image name; it is UI sugar only and never constrains what may go into
  `Install`.

The example `hollywood.yaml` app is a derived-image demo: a stock digest-pinned Debian base
plus `apt-get install -y hollywood`, run in a terminal window.

Locally built base images (for example a language toolchain image to layer projects on) are
referenced by a `localhost/` tag and are exempt from the digest pin; trust comes from the
vetted, version-pinned recipe rather than a global digest, and their own `FROM` base is still
digest-pinned. Shipping a curated set of such base images is on the roadmap.

---

## 8. Build and Release

**Podman-only. There is no host Go for tool builds.** Every Go command - `test`, `vet`,
`fmt`, `vendor`, `build` - runs inside a **digest-pinned `golang` container**, invoked
through `make`. There is no `go run`: `make build` produces a static binary in the container
and you run that binary.

The build logic is shared, not copied. A single repo-root `Containerfile` (the pinned Go
toolchain; it builds whichever module is the build context), a `check.mk` of containerized
checks that every module includes, and a `tool.mk` of binary targets that each tool's
three-line `Makefile` includes. "The same logic, only different paths."

From any module (`common`, `container/runner`, `container/creator`):

```sh
make check      # gofmt + go vet + go test, in the pinned container
make build      # reproducible build -> ./bin/<tool>
make repro      # build twice, assert the binary is byte-identical
make vendor     # refresh vendored deps (the only step that needs network; GOWORK=off)
```

Reproducibility is enforced by pinning every input: the Go toolchain by digest
(`GOTOOLCHAIN=local`, so it never silently downloads a different one), dependencies by each
module's `./vendor` plus `go.sum` (built with `-mod=vendor`, no proxy, no network), and the
build flags themselves (`CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, `-ldflags="-s -w
-buildid="`). Same inputs, same bytes, on any machine.

**Versioning.** Each binary carries a `version`, stamped from `git describe` via
`-ldflags "-X main.version=..."`; `zcc version` and `zcr version` print it. A plain build with
no git falls back to `dev`.

**CI** (`.github/workflows/ci.yml`) runs three jobs: `common` (gofmt / vet / test on the
schema and validation), `check` (a matrix of `container/runner` and `container/creator`, each
running `make check` and `make build`), and `e2e` (the end-to-end suite driving the real
binaries against podman).

`virtualization/creator/` still exists as a skeleton and is **intentionally not built in CI:
it still references the removed `core` hexagon and does not compile yet**. It joins the
matrix once migrated to `common`.

---

## 9. Components

### 9.1 zcc - the creator

**Stack:** Go + Bubbletea. `zcc` authors app files and manages them; it depends only on the
shared `common` library and never imports the runtime.

Authoring commands (local, no runtime needed):

```
zcc new <name> --image <img> [--desc d] [--icon i]
zcc list
zcc validate <name|app.yaml>
zcc delete <name>
zcc keys list|show|set <s>|edit|validate|path
zcc tui
```

Runtime commands (forwarded verbatim to `zcr`, see 9.2):

```
zcc run <name|app.yaml> [--exec]     zcc build <name|app.yaml>
zcc stop|restart|inspect <name>      zcc logs <name> [-f]
zcc term <name> [--shell]            zcc image search <term>|resolve <ref>
```

A bare `<name>` resolves against the store (`~/.config/zinc/apps`); an argument that looks
like a path (contains `/` or ends in `.yaml`) is read directly.

**The TUI** is the keyboard-first manager: create, edit, run, stop, logs, delete, rename. Its
form footers show only the gestures that actually apply to the focused field and the app's
current state, drawn from the active scheme, so the hints stay honest instead of a fixed wall
of keys. An **advanced** row opens the full YAML in `$EDITOR`, where capabilities, network
lists, volumes, and keys are edited directly. The TUI's run/stop/build/term/logs actions all
go through `zcr` (via the backend facade); `zcc` never runs a container itself.

**Keybind schemes.** `zcc`'s own TUI keys are not hardcoded: they resolve through a
selectable scheme. Two are built in (`default` and `vim`) and users can define their own under
`~/.config/zinc/zcc`. `zcc keys list|show|set|edit|validate|path` and an in-TUI picker (open
with `?`) choose and apply one, switching live. These are `zcc`'s interface keys only, an
implementation detail of the creator; they are distinct from any desktop hotkeys, which are a
host-level (ZDE) concern. (The scheme files happen to be TOML internally; that is a `zcc`
implementation detail and has nothing to do with the app format, which is YAML.)

Internally `zcc` is a small CLI over a backend facade: an `internal/store` YAML app store (a
mirror of the same on-disk format `zcr` reads), an `internal/runner` delegate that finds `zcr`
on `$PATH` and drives it, an `internal/backend` facade the CLI and TUI both call, the
`internal/tui` Bubbletea UI, and `internal/keys` for the keybind schemes.

### 9.2 zcr - the runner

**Stack:** Go. `zcr` is the runtime. It reads an app file and runs it as a rootless podman
container, applying the network lock-down before the app starts (5.3). `zcc` shells out to it,
but it is a first-class CLI you can drive directly:

```
zcr run <app> [--exec]      print the launch plan, or launch it (--exec)
zcr build <app>             (re)build the derived image (ImageMeta.Install)
zcr validate <app>          parse + validate; report problems and warnings
zcr stop|restart|inspect <app>
zcr logs <app> [-f]         zcr term <app> [--shell]      zcr ps
zcr image search <term> | resolve <ref>
```

Without `--exec`, `run` is a dry run: it validates and prints the exact `podman` command(s)
and any nft ruleset that would be enforced. This is the "no magic" principle in action.

The launch path is a single orchestrated sequence (validate -> gate unsupported network
shapes -> auto-start dependencies -> build the derived image if needed -> run the network
lock-down through the `NetEnforcer` -> start the app container detached), so there is exactly
one path to get right.

**Terminal apps.** A GUI app renders through the passed-in Wayland socket. A CLI/TUI app
(`StartConditions.Terminal`) is launched inside the host's terminal emulator with an
interactive TTY - a container otherwise has no terminal to attach to. The emulator argv comes
from `ZINC_TERMINAL` (else `$TERMINAL`), so both `foot` and `xterm -e` forms work; launching a
terminal app with neither set fails with a clear message. See section 11.

**Multiterminal apps.** A terminal app may also set `StartConditions.Multiterminal` to attach
many terminals to one shared instance. The container runs a detached **holder** (`sleep
infinity` under `--init`, so `podman stop` is prompt), and every terminal is a `podman exec
-it` into it, running the app's own command (or a shell with `--shell`). The app lives until
the **last terminal closes**, unless `StopConditions.Background` keeps the holder running.
Coordination is by filesystem flock under `$XDG_RUNTIME_DIR/zinc/run/<app>/`, with no daemon
and no socket: a per-app lock serializes holder start-up, each terminal's waiter flock-holds a
liveness marker for its life (auto-released on death, so a killed terminal cannot wedge the
count), and the last waiter out stops the container. Because a holder owns PID 1, a
multiterminal app needs an explicit entrypoint (the image default cannot be replayed into each
terminal), which validation enforces.

### 9.3 zlt - the launcher (TUI)

**Stack:** Go + Bubbletea. `zlt` (zinc-launcher-tui) is a fast, keyboard-first fuzzy
picker over the defined apps. It lists `~/.config/zinc/apps`, filters as you type (a small
in-house subsequence matcher that favours matches at the start of a name and at word
boundaries), and on **enter launches the selected app by shelling out to `zcr run <app>
--exec`**, then quits (dmenu-style). So `zcr` still does the real work - validation,
dependency auto-start, the derived-image build, the network lock-down - and `zlt`, like
`zcc`, depends only on `common` and never imports the runtime. A `zlt <app>` form launches
one app directly (for a desktop hotkey or a script), and a `●` marks apps already running
(best-effort, from `zcr ps`). It lives at `launcher/tui`; the read / launch / match logic it
shares with `zlg` lives in the `launcher/common` library.

### 9.4 zlg - the launcher (GUI)

**Stack:** Go, rendering in pure Go with no cgo. `zlg` (zinc-launcher-gui) is the graphical
sibling of `zlt`: the same quick picker over the defined apps, for a point-and-click or
keyboard launch. It speaks the Wayland wire protocol directly (go-wayland) and
software-renders the picker into a shared-memory buffer with a bundled bitmap font, so it
stays a **static, `CGO_ENABLED=0`, runs-anywhere, byte-reproducible** binary built from the
same minimal image as the other tools - no graphics libraries, no dynamic linking. It shares
`zlt`'s read / launch / match logic through `launcher/common` and, like it, shells out to
`zcr` and never imports the runtime. `zlg <app>` launches one app directly.

Everything but the Wayland event loop is a pure, unit-tested package (`internal/picker`,
`internal/keymap`, `internal/render`); only `internal/ui` needs a live compositor. Known
limits (0.3): the keymap is US-QWERTY (full xkb layout support is future work), and
go-wayland carries no `wlr-layer-shell`, so `zlg` is a normal window, not a dmenu-style
overlay.

### 9.5 Planned components (roadmap)

`zvc` / `zvr` (virtualization, section 10) are not built yet. They will share the same
`common` schema library, so every tool uses one config format.

---

## 10. Virtualization (planned - zvc / zvr)

For isolation needs beyond what containers provide - untrusted GUI apps, foreign OSes,
throwaway environments - Zinc will add VMs (`libvirt` + `qemu`) as a parallel runtime, with a
creator/runner split (`zvc` / `zvr`) mirroring the container tools and reusing the shared
schema. Containers remain the primary runtime; VMs are the heavy-isolation escape hatch. A VM
works for any guest (including Windows) where a nested Wayland compositor would break on
real-world software. None of this ships in 0.1; the schema reserves `Type:
ZincVirtualization` for it.

---

## 11. Host Surface (minimal)

Zinc adds almost nothing to the host. The moving parts:

| Component | Reason |
|-----------|--------|
| A Wayland compositor | owns the display; passes the socket in per app |
| A terminal emulator | drops into terminal/multiterminal apps on explicit launch |
| `zcc` / `zcr` | two static binaries on `$PATH` - author and run apps |
| Rootless Podman + pasta | the container runtime and userspace networking |
| Pipewire (optional) | audio; the socket is passed in only on explicit grant |

Everything else runs inside containers. The host-side values a launch needs (Wayland and
runtime sockets, the theme bundle, the terminal emulator, the netfilter image) are resolved
from the environment in one place (the host adapter, section 13) into an explicit options
struct, so the argv-building code stays pure and testable. The environment variables:
`XDG_RUNTIME_DIR`, `WAYLAND_DISPLAY`, `ZINC_TERMINAL` (else `$TERMINAL`), `ZINC_THEME_BUNDLE`,
and `ZINC_NETFILTER_IMAGE` (overrides the default helper image tag).

---

## 12. Desktop integration (ZDE)

Desktop hotkeys, a launcher, session profiles, and the Nix home-manager wiring belong to
**ZDE** (the Zinc Desktop Environment), a separate project layered on Zinc. They are not part
of the Zinc 0.1 core and are intentionally out of scope for this document. Zinc is
compositor-agnostic on purpose, so it can be adopted piecemeal on any existing system.

---

## 13. Repo Layout

The shared library is `common/`, which holds **only** the app schema and its validation
(`common/domain/schema` plus `common/domain/schema/validate`), pure stdlib with no I/O. Both
tools depend on it and validate identically. The runtime hexagon lives **inside**
`container/runner`; there is no separate shared runtime module.

```
zinc/
  Containerfile          generic reproducible build (any module; digest-pinned Go)
  check.mk               containerized checks (test/vet/fmt/vendor); every module includes it
  tool.mk                binary targets (build/run/repro); each tool's Makefile includes it
  go.work                ties the modules together for local dev only (the build never uses it)
  common/                shared library - the app schema + validation (pure, no I/O)
    domain/schema/                    schema.go (AppConfig, schema version 2)
    domain/schema/validate/           the hard rules + create-time warnings
    examples/apps/                    sample app YAMLs
  container/
    creator/             zcc - the creator (CLI + Bubbletea TUI)
      main.go                         CLI: authoring local; runtime forwarded to zcr
      internal/store/                 the YAML app store (~/.config/zinc/apps)
      internal/runner/                the zcr delegate (finds zcr on $PATH, drives it)
      internal/backend/               the one facade the CLI + TUI use
      internal/tui/                   the keyboard-first terminal UI
      internal/keys/                  the TUI keybind schemes (~/.config/zinc/zcc)
    runner/              zcr - the runtime (the hexagon)
      domain/                         pure model: derived-image policy, launch options
      ports/                          interfaces: Store, Runtime, ImageBuilder,
                                        ImageResolver, NetEnforcer, + the neutral Command type
      app/                            launch orchestration (the Service)
      adapters/podman/                Runtime + ImageBuilder + ImageResolver (podman argv/exec)
      adapters/netenforce/            the NetEnforcer: pasta pod + nft ruleset  <- the swap point
      adapters/fs/                    the YAML app store + codec
      adapters/host/                  environment -> host launch options
      wire/                           composition root (assembles adapters -> app.Service)
      images/netfilter/               Containerfile for the nft helper image (make netfilter-image)
      main.go                         the CLI
    e2e/                 end-to-end tests: drive the real zcc/zcr against podman
  launcher/common/       shared launcher library (store + zcr delegate + fuzzy matcher)
  launcher/tui/          zlt - the launcher TUI (fuzzy picker over the apps; shells out to zcr)
  launcher/gui/          zlg - the launcher GUI (pure-Go Wayland; shells out to zcr)
  virtualization/creator/  zvc skeleton - NOT migrated to common yet, does not compile
  docs/architecture.md   this document
```

**The runner is a hexagon** (ports and adapters), so a mechanism can be swapped by writing a
new adapter rather than editing call sites - the motivating case being network enforcement
(5.3), where "not pasta" later is one more adapter. The layers:

- **`domain/`** - pure model and policy (the derived-image policy, the launch options type).
  No I/O, no podman/nft/fs/env.
- **`ports/`** - the interfaces the application depends on: `Store`, `Runtime`,
  `ImageBuilder`, `ImageResolver`, and `NetEnforcer` (the network swap point), plus the
  neutral `Command` type a `NetEnforcer` emits and a `Runtime` executes.
- **`app/`** - the application service that orchestrates a launch through the ports. The
  single launch path.
- **`adapters/`** - the concrete edges: `podman` (Runtime/ImageBuilder/ImageResolver),
  `netenforce` (the pasta pod plus nft ruleset - the swap point), `fs` (YAML store + codec),
  `host` (environment to options).
- **`wire/`** - the composition root: the one place that imports every adapter and assembles
  them into an `app.Service`. The CLI calls it; nothing else names a concrete adapter.

The shared schema (`common`) is vendored into each tool, so every per-module container build
stays hermetic (no network, no sibling checkout at build time). The repo-root `go.work` ties
the modules together for local editing only; the build never depends on it (`make vendor`
runs with `GOWORK=off`).

Two distinct container concerns live here: the repo-root `Containerfile` reproducibly builds a
**tool binary** (pinned Go toolchain plus that module's vendored deps); the netfilter
`Containerfile` under `container/runner/images/netfilter` builds the **runtime helper image**
the network lock-down applies rules with (6.4).

---

## 14. Known Issues and Tradeoffs

| # | Issue | Mitigation |
|---|-------|------------|
| 1 | wayland-security-context enforcement is the compositor's, not Zinc's | the container boundary is the real wall (5.1); a VM (section 10) is the future path for untrusted GUI apps |
| 2 | GPU passthrough weakens isolation | off by default; never enable for untrusted images (5.4) |
| 3 | Image tags can be poisoned upstream | third-party images must be digest-pinned; launch is `--pull never` (5.5) |
| 4 | Derived images are per-machine, not digest-pinned | their guarantee is the pinned base plus the visible install lines (7) |
| 5 | Some schema fields are validated but not yet enforced at runtime (resources, internal user, notifications, config mounts) | called out explicitly in section 3; on the roadmap, fail-loud where relevant |
| 6 | Host-scoped egress, gateway/multi-homing, and mixing a sibling link with other networking are unsupported | fail-closed: rejected at launch, never mis-enforced (6.5) |
| 7 | The netfilter helper runs with namespaced `CAP_NET_ADMIN` | namespaced to the pod's userns, harmless on the host; the image is local and `--pull never` (6.4) |
| 8 | `virtualization/creator/` does not compile | intentionally excluded from CI until migrated to `common` (8) |

---

## 15. Status and Roadmap

**0.1 ships containers:** the schema and validation, the YAML app store, the full
`zcc` + `zcr` split with a keyboard-first TUI, real rootless-container lifecycle, and the
fail-closed network lock-down (isolated / egress / LAN publish / sibling link).

**0.2 ships the launcher:** `zlt`, a keyboard-first fuzzy picker over the defined apps that
shells out to `zcr` (depends only on `common`, never imports the runtime).

**Next (see `RELEASES.md`):** the GUI launcher (`zlg`, 0.3) and virtualization
(`zvc` / `zvr`, 0.4), the latter migrating the non-compiling `virtualization/` skeleton off
the removed `core` hexagon onto `common`. The ZDE desktop layer is a separate project in its
own repository (section 12), not a Zinc release. Within the container tools, the near-term
work is wiring the already-validated schema fields (resources, internal user) into the launch.
</content>
</invoke>
