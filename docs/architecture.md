# HyprZinc — Architecture

> **Priority order: Stable → Secure → Beautiful**
> Keyboard-first. No mouse required. All user-facing apps run in rootless Podman containers on any Linux distribution (Fedora primary target).

---

## 1. Design Principles

- **Stable first** — declarative config everywhere, pinned versions, reproducible from scratch
- **Secure by default** — minimum host surface, containers get nothing unless explicitly granted
- **Beautiful always** — consistent visual language, but never at the cost of the above two
- **Keyboard sovereign** — every interaction reachable without a mouse
- **No magic** — every decision is explicit, documented, and reversible
- **Honest about limits** — when a security mechanism is partial (see §5), document it loudly

---

## 2. Stack Overview

```
┌─────────────────────────────────────────────────────────┐
│  Nix home-manager (single flake)                        │
│  outputs: packages.hzc + packages.hzl + packages.hzv    │
│           + homeManagerModules.hyprzinc                 │
├─────────────────────────────────────────────────────────┤
│  Hyprland (Wayland compositor)                          │
│  + wayland-security-context (default; label-only today, │
│    no runtime enforce — see §5.2)                       │
├──────────────┬────────────────┬─────────────────────────┤
│ hzc (TUI)    │ hzl (GUI)      │ hzv (TUI, future)       │
│ Bubbletea/Go │ gioui/Go       │ Bubbletea/Go            │
│ containers   │ launcher +     │ VMs (libvirt+qemu)      │
│              │ smart executor │                         │
├──────────────┴────────────────┴─────────────────────────┤
│  Config store                                           │
│  ~/.config/hyprzinc/apps/<name>.toml                    │
│  ~/.config/hyprzinc/profiles/<name>.toml                │
│  ~/.config/hyprzinc/commands/<name>.toml                │
│  ~/.config/hyprzinc/vms/<name>.toml                     │
│  ~/.config/hyprzinc/hzc/keys.toml   (hzc TUI keybinds)  │
├─────────────────────────────────────────────────────────┤
│  vpn-container (owns its own sing-box config)           │
│  ├── sing-box (tun, DNS, CIDR routing, outbounds)       │
│  ├── amnezia-wg instances (socks5)                      │
│  └── xray instances (socks5)                            │
├─────────────────────────────────────────────────────────┤
│  Rootless Podman + pasta networking  │  libvirt + qemu  │
│  (containers — primary runtime)      │  (VMs — heavy    │
│                                      │   isolation)     │
├─────────────────────────────────────────────────────────┤
│  Fedora Server (primary) / any Linux (supported)        │
└─────────────────────────────────────────────────────────┘
```

---

## 3. App Config (TOML)

One TOML file per app: `~/.config/hyprzinc/apps/<name>.toml`. Only `hzc` creates or modifies these. Validated at save **and** at launch (launch-time check catches manual edits or drift).

```toml
schema_version = 1

[app]
name        = "firefox"
image       = "docker.io/library/firefox@sha256:abc123..."  # pinned by digest
command     = []                 # argv appended after the image; overrides the image's
                                 # default command (CMD). [] = image default. e.g. ["htop"]
install     = ""                 # quick-setup: build-time setup run as a RUN layer atop
                                 # image to make a DERIVED image (FROM image + RUN install).
                                 # "" = run image directly. e.g. "apt-get update &&
                                 # apt-get install -y hollywood". Built on demand at run
                                 # (rebuilt when this or image changes) and via `hzc build`
                                 # (§5.5, §9.1). The FROM base inherits image's digest pin.
preset      = "strict"           # starting template, every field below can override
description = "Web browser"
icon        = "firefox"          # freedesktop name, OR a path (~ expanded). On set,
                                 # hzc copies the asset into HyprZinc's managed icon
                                 # store and caches a normalized thumbnail (see §9.2).
terminal    = false              # CLI/TUI app: launch in a terminal emulator window
                                 # with an interactive TTY (§9.1, §11). Exclusive with
                                 # background unless multiterminal is also set.
multiterminal = false            # terminal app only: run a shared keep-alive container
                                 # that many terminals attach to (each runs `command`,
                                 # or a shell). Lives until the LAST terminal closes,
                                 # unless background (then it stays). Needs an explicit
                                 # command. (§9.1)
background  = false              # run detached in background
autostart   = false              # start at session login
autostart_workspace = 0          # 0 = no preference; N = place on workspace N at login

[display]
wayland     = "security-context" # security-context | passthrough
gpu         = false

[network]
mode        = "none"             # none | pasta | container
# When mode = "pasta":
#   ipv4_cidr  = ["1.1.1.1/32"]
#   ipv6_cidr  = []               # IPv6 explicit, empty = blocks IPv6
#   ports      = [443, 80]
#   interface  = "eth0"
#   block_dns  = true             # block 53, 853, known DoH endpoints
# When mode = "container":
#   target     = "vpn-container"

[[mounts]]
# General host paths for data. Explicit, no implicit home access. Each needs a mode.
# host      = "/home/user/Downloads"
# container = "/home/user/Downloads"
# mode      = "rw"               # ro | rw

[keys]
# Convenience layer for SSH/GPG ONLY. Unlike [mounts], this also wires up the
# relevant agent socket (ssh-agent / gpg-agent) and enforces 0600 perms inside
# the container. Per-key explicit opt-in.
ssh         = []                 # ["/home/user/.ssh/id_ed25519"]
gpg         = []

[audio]
pipewire    = false
legacy_alsa = false              # mount /dev/snd for ALSA-only apps (rare)

[theme]
mode        = "host"             # host | none — default: host (consistent look)
# When mode = "host", the container gets, read-only:
#   - a single curated "theme bundle" directory (generated by the module, see §5.6),
#     NOT the host's real config dirs, containing:
#       - GTK 3/4 configs + themes
#       - Qt 5/6 configs + themes
#       - Icon themes, cursor themes
#       - Fonts
#   - plus the matching theme env vars set on the container
#     (GTK_THEME, QT_QPA_PLATFORMTHEME, XCURSOR_*, etc.)
# Set mode = "none" to deny theme access (apps fall back to their own defaults).

[capabilities]
extra       = []                 # podman --cap-add entries

[depends_on]
containers  = []                 # ["vpn-container"] — auto-started by hzl
```

**hzc UI-only (not in TOML):** delete preset.

---

## 4. Presets

Templates only — not enforced modes. Every field is independently overridable.

| Field | `strict` (new app default) | `standard` | `networked` |
|---|---|---|---|
| `network.mode` | `none` | `none` | `pasta` |
| `display.wayland` | `security-context` | `passthrough` | `passthrough` |
| `display.gpu` | `false` | `false` | `false` |
| `audio.pipewire` | `false` | `false` | `false` |
| `theme.mode` | `host` | `host` | `host` |

hzc shows the preset + each field's actual value, so user sees the truth, not just the label.

---

## 5. Security Model — What Each Layer Actually Gives You

### 5.1 Container isolation (rootless Podman)

**Strong.** This is the real security boundary. Namespaces isolate PID, network, mounts, IPC, UTS. User namespace maps container root to your unprivileged host user. An app cannot see the host filesystem outside explicit mounts, cannot see other containers, cannot escalate.

### 5.2 Wayland isolation (wayland-security-context)

**Partial in practice — be honest.**

Protocol is merged into Wayland 1.22, Hyprland supports it. But **enforcement requires toolkit cooperation**:

- GTK4 — partial
- Qt6 — partial
- Electron — basically nothing
- Chromium/Firefox — none

An app that doesn't implement the protocol silently runs with full Wayland access. wsc is a hint, not a wall, for non-compliant apps.

**Conclusion:** keep wsc as default (right direction, improves as toolkits adopt), but do not market it as isolation. Real isolation for containers is the container boundary itself. For genuinely untrusted apps where Wayland-level isolation matters, the answer is a **VM** (see §10 — hzv), not a nested compositor. Nested compositors (cage and similar) cover too narrow a slice of apps to be worth maintaining.

### 5.3 Network isolation (pasta + vpn-container)

**Strong, with explicit DNS handling required.**

pasta provides the userspace connectivity (no host root, no *host* nftables). The per-app **egress CIDR + port allowlist is enforced by nftables running *inside the container's own network namespace*** — this is rootless-safe because `CAP_NET_ADMIN` is namespaced to that userns and grants nothing on the host. Division of labour: pasta carries packets to/from the host; the in-netns nftables ruleset decides which destinations/ports are permitted. (`network.mode = "none"` gets no namespace connectivity at all; `network.mode = "container"` instead inherits vpn-container's netns, where sing-box's route rules — §6.3 — do the filtering.)

Enforcement is a **port** in the core hexagon (`NetEnforcer`, §13): each mode is an adapter that says how the app attaches to the network, what must run to lock the netns down before the app starts, and how to tear it down. pasta+nft is the adapter today; swapping the traffic-control mechanism (a different firewall, an eBPF egress filter, an external controller) is a new adapter, with the launch path and the rest of the system unchanged.

DNS leakage mitigation:
1. App container's `/etc/resolv.conf` points **only** at sing-box's internal resolver (an address inside vpn-container's namespace)
2. Pasta blocks outbound 53/853 **to the outside world** — but allows 53 to sing-box's internal resolver. The block stops apps from reaching `8.8.8.8:53` directly; it does not break legitimate resolution through sing-box
3. sing-box resolves upstream queries **through the active tunnel** (DNS-over-the-tunnel), never via raw host port 53
4. **Fail-closed:** if the tunnel is down or sing-box's resolver is unreachable, DNS fails — no resolution rather than leaking to a host resolver. This is the correct failure mode for leak prevention. Consequence the user must understand: **VPN down = no name resolution = no internet** for containers routed through it
5. Apps that hardcode DoH to a specific IP inside HTTPS can still leak via that IP — only mitigation is a tight `ipv4_cidr` allowlist

### 5.4 GPU passthrough

**Weak isolation when enabled.**

`/dev/dri` access exposes GPU rendering state more broadly than process boundaries suggest. Linux GPU sandboxing is immature.

**Rule:** never enable GPU on untrusted code. hzc warns when `gpu = true` is set.

### 5.5 Image trust

**Pin by digest, not tag — for pulled (third-party) images.**

For images pulled from a registry, the app TOML stores `image@sha256:...`. Tags shown in hzc UI for readability, but pulls use digest. Updates require explicit user action ("update image") — no silent upgrade.

**Locally-built `trusted-*` images are referenced by local tag** (e.g. `hyprzinc/trusted-go-dev:latest`). They are built on the machine from the Nix-shipped Containerfiles (§7), so their digest isn't known until after the build and differs per machine. Trust for these comes from the vetted, version-pinned Containerfile rather than a global digest — and their own `FROM` base is still pinned by digest.

**Derived images (`app.install`) are local, built from a pinned base.** When an app sets `install`, hzc builds a small derived image — `FROM <app.image>` plus one `RUN <install>` layer — tagged locally (`hyprzinc/app-<name>:local`) and run instead of the bare base. This is the quick-setup path: take a stock distro image and `apt`/`apk`/`dnf` the program you want, without authoring a Containerfile. The `FROM` base is `app.image`, which this section already forces to be digest-pinned (or a `trusted-*` local tag), so the build starts from a known base. The derived image itself carries only a local tag and is never pushed; like `trusted-*`, its trust comes from the inputs (the pinned base + the user's own install line), not a global digest. It is rebuilt automatically when `install` or `image` changes — tracked by a fingerprint label, so an unchanged app reuses its image — and on demand with `hzc build <name>` (or the TUI build action). **Tradeoff:** because a registry-style digest is meaningless for a per-machine local build, the derived image is not itself digest-pinned; the guarantee it inherits is that of its pinned base plus the visible install line.

Custom registries and Docker Hub supported with one-time warning per registry domain.

### 5.6 Theme passthrough

**Default: on. Security impact: minimal.**

The home-manager module (the single source of truth for theme, §9.3) generates **one curated theme bundle** — a dedicated directory (e.g. `~/.local/share/hyprzinc/theme-bundle/`) holding only GTK 3/4 + Qt 5/6 configs, icon themes, cursor themes, and fonts. **That single directory** is mounted read-only into themed containers (and the matching theme env vars are set on the container) — *not* the host's real `~/.config` or `~/.themes`. No write access, no inheritance of other home-manager state, no incidental exposure of non-theme config.

Apps opt out with `theme.mode = "none"` if they need isolation from the host's visual config (rare — only relevant if a theme file itself were a vector, which would be a separate compromise scenario).

The point of default-on: containerized apps look like part of the system, not foreign objects. Consistency reinforces the "beautiful" priority without compromising the "secure" one.

---

## 6. VPN Architecture

Single `vpn-container`, multiple backends running simultaneously, destination-based routing.

### 6.1 sing-box ownership

**sing-box config is vpn-container's own responsibility.** home-manager declares backend definitions and routing intent (passed in as a mount or env vars). vpn-container's entrypoint generates `config.json` on startup from that input. hzc does not write sing-box config directly.

### 6.2 Components inside vpn-container

```
vpn-container (CAP_NET_ADMIN, no other caps)
  ├── entrypoint script (renders sing-box config.json from input)
  ├── sing-box (owns tun0, DNS resolver, routing → outbounds)
  ├── amnezia-wg "personal"    → socks5 :1080
  ├── amnezia-wg "corporate"   → socks5 :1081
  └── xray instance(s)         → socks5 :1082+
```

### 6.3 Routing model

sing-box config maps destination CIDRs to outbound backends. `direct` = leaves the tunnel via the container's own egress (pasta); named backends route through their socks5:

```json
{
  "route": {
    "rules": [
      { "ip_cidr": ["10.0.0.0/8"],     "outbound": "corporate" },
      { "ip_cidr": ["1.1.1.1/32"],     "outbound": "personal" },
      { "ip_cidr": ["192.168.0.0/16"], "outbound": "direct" }
    ],
    "final": "xray-us"
  }
}
```

Routing rules can be reloaded live. **Adding/removing backends needs sing-box restart** — backend definitions are semi-static (declared in home-manager, set at vpn-container start). Routing rules change freely.

### 6.4 App container attachment

```toml
[network]
mode    = "container"
target  = "vpn-container"
```

→ Podman runs the app with `--network container:vpn-container`. Traffic goes through sing-box. Backend chosen per packet by destination CIDR.

### 6.5 DNS

sing-box runs its own internal resolver. App containers' `/etc/resolv.conf` points **only** at it. Egress to port 53/853 toward any address *other than* sing-box's resolver is blocked by pasta (`block_dns = true`); the internal resolver stays reachable. sing-box forwards queries upstream through the active tunnel, so DNS never leaks via raw host port 53.

**Fail-closed:** tunnel down → DNS fails (no resolution), never falls back to a leaking host resolver. VPN down therefore means no internet for routed containers — intended behavior for leak prevention, documented so it isn't a surprise.

### 6.6 Container startup ordering

App containers list dependencies in `depends_on.containers`. hzl/hzc checks state before launch — auto-starts dependencies first.

### 6.7 CRIU limitations

CRIU works for app containers (preserve state across network namespace changes). **Does not work for vpn-container itself** — tun devices can't be fully serialized. vpn-container restart = brief network blip for attached apps.

---

## 7. Image Layering

OCI single inheritance only. Linear chain, cache-friendly. The `trusted` prefix marks the provenance boundary — these images are built and vetted by you, distinct from third-party images pulled directly.

```
alpine@sha256:…                          # base pinned by DIGEST, not a tag — see §5.5
  └── hyprzinc/trusted-base
      (shell, common CLI tools, user setup, locale)
      └── hyprzinc/trusted-go        ← language toolchain
          └── hyprzinc/trusted-go-dev ← + nvim, LSP, plugins (nvim lives ONLY here)
              └── myproject           ← project-specific deps
```

Rules:
- `hyprzinc/trusted-base` is the common ancestor for all languages
- `trusted-` prefix = built locally and vetted; absence of prefix = third-party, treat with caution (digest-pinned, see §5.5)
- nvim config lives inside the language-dev image, NOT in home-manager
- One branch per language: `trusted-go`, `trusted-rust`, `trusted-python`
- **Image acquisition:** Nix ships the binaries, TOMLs, and Containerfiles to a new
  machine; images are then built/pulled **locally** per those Containerfiles (the `FROM`
  base images come from Docker Hub etc.). `home-manager switch` does *not* transport
  built image layers — it transports the recipes.
- **Two image classes, two trust rules:**
  - *Third-party* app images (firefox, slack, …) → pulled from a registry and
    **referenced by digest** in the app TOML (§5.5).
  - *trusted-* images → **built locally** from the shipped Containerfiles and
    **referenced by local tag** (their digest is only known post-build and differs per
    machine). Trust comes from the vetted Containerfile, not a global digest.
- **Reproducibility requires pinning the build inputs:** `FROM` the base **by digest**
  (`alpine@sha256:…`, never `alpine:latest`) and pin package versions — otherwise two
  machines diverge, breaking the Stable promise (§1).

### Podman-in-Podman (CI use case)

Supported with restrictions. Inner Podman inherits outer container's network namespace (pasta rules apply), but can mount paths inside the outer container freely.

**Mitigation:** outer CI container mounts only build context + Podman socket. No SSH/GPG keys, no source outside the project. Documented tradeoff.

---

## 8. Profiles

Curated sets of apps + workspace placement. Declarative, hand-editable or generated from current state via hzc ("save current as profile X").

`~/.config/hyprzinc/profiles/<name>.toml`:

```toml
schema_version = 1

[profile]
name        = "work"
description = "Work setup — IDE, browser, comms, VPN"

[autostart]
apps = [
  "vpn-container",   # started first via depends_on
  "firefox",
  "slack",
  "go-myproject",
]

[layout]
# Optional per-app workspace + placement
firefox      = { workspace = 1, fullscreen = false }
slack        = { workspace = 2 }
go-myproject = { workspace = 3 }
```

**Activating a profile:**
1. Stop apps currently running that are not in the new profile (**modal confirm** before stopping — never silent)
2. Start apps in the profile (respecting `depends_on`)
3. Place each app on its assigned workspace
4. Apply tiling layout if specified

Note: per-app login autostart uses `autostart_workspace` in the app TOML; profiles use the `[layout]` block here. Both feed the same placement logic.

**No session save / CRIU restore.** App-internal state (browser tabs, IDE buffers) is each app's responsibility. Profiles handle the "shell" of the session; apps handle their own internals.

Switching profiles is `Super+P` → hzl shows profile list.

---

## 9. Components

### 9.1 hzc — Container Definition TUI

**Stack:** Go + Bubbletea

**Responsibilities:**
- Create / edit / delete app definitions
- Validate TOML at save and at launch
- Find images and **pin them by digest** without a browser — `hzc image search`
  / `image resolve` (podman-only); the TUI image field resolves in place. Makes
  the §5.5 digest rule painless.
- Launch / stop / restart containers
- Build a **derived image** for an app with `app.install` (quick setup: pinned base
  + one install layer) — `hzc build`, the TUI build action, or auto on run (§5.5)
- Inspect logs
- Save current running state as a profile
- Edit running app config → "apply on next launch" by default
- Choose / customise the **TUI keybind scheme** — `hzc keys`, or the in-TUI
  picker (see "Keybind schemes" below)

**Launch behavior.** A GUI app renders through the passed-in Wayland socket (§5.2).
A CLI/TUI app sets `app.terminal = true`: hzc launches it inside the host's terminal
emulator (§11, configured via `$HYPRZINC_TERMINAL`, else `$TERMINAL`) with an
interactive TTY (`-it`) — a container otherwise has no terminal to attach to, so it
would run invisibly. The plan→exec **launch logic is shared, not hzc-specific**: hzl
auto-starts apps from the launcher (`depends_on`, §6.6) and reuses the same path, so
it lives in the shared functional core (§13), not welded into either UI.

**Quick setup (install + entrypoint).** Instead of authoring a Containerfile, an
app can set `app.install` to a one-line build-time setup command and `app.command`
to the entrypoint. hzc then builds a **derived image** — `FROM <app.image>` plus a
single `RUN <install>` layer, tagged `hyprzinc/app-<name>:local` — and runs that
instead of the bare base (§5.5). Example: base `debian@sha256:…`, install `apt-get
update && apt-get install -y hollywood`, command `["hollywood"]`. The form shows a
per-distro hint for the install line (apt for debian/ubuntu, apk for alpine, dnf for
fedora/rhel, pacman for arch, zypper for openSUSE), derived live from the base image
name. The image builds **automatically on run** when it's missing or its inputs
(`install` / `image`) changed — detected by a fingerprint label, so an unchanged app
reuses it — and can be (re)built explicitly with `hzc build <name>` or the TUI
**build** action (`b`). The `RUN` line uses the image's own `/bin/sh`, so a distro
package-manager invocation works exactly as typed; a `command` needing quoted,
multi-word argv stays editable as full argv via the advanced TOML row. *Honesty:* the
derived image is local and per-machine, so it isn't itself digest-pinned — its
guarantee is the pinned `FROM` base plus the visible install line (§5.5).

**Keyboard hints stay honest.** Footers show only the gestures that actually apply,
drawn from the active scheme — never a fixed "porridge" of every key. The form footer
shows just the focused field's gestures (an enum row advertises *change*, a bool row
*toggle*, a text row *clear*/*resolve*) plus save/cancel; the list footer shows only
state-applicable actions (run when stopped or for a multiterminal app, stop/logs when
running, shell when multiterminal, build when `install` is set); and each gesture
shows its primary key only, so a vim user sees `j`/`k` where a default user sees the
arrows. The logs and scheme-picker footers read the same scheme (only the viewport's
own scroll keys and the picker's intrinsic apply/edit/back stay literal).

**Multiterminal.** A terminal app may also set `app.multiterminal = true` to attach
many terminals to *one* instance. The container then runs a detached **holder** —
`sleep infinity` under `--init` (so `podman stop` is prompt: a process with no
SIGTERM handler running as PID 1 is exempt from default signal actions, so the
injected init owns PID 1 instead) — and every terminal is a `podman exec -it` into
it, running the app's own `command` (or a shell). The app lives **until the last
terminal closes**; `background = true` keeps the holder running after that. Each
terminal is its own detached waiter process; coordination is by filesystem flock
under `$XDG_RUNTIME_DIR/hyprzinc/run/<app>/` (no daemon, no socket): a per-app lock
serializes holder start-up, each waiter flock-holds a liveness marker for its life
(auto-released on death, so a killed terminal can't wedge the count), and the last
waiter out — finding no other marker still held — stops the container. Open more
with `hzc term <name>` (`--shell` for a shell) or, in the TUI, the **run** action
(again for another) and **shell**. *Honesty:* the holder needs `sleep` in the image,
and a shell terminal needs `/bin/sh`; all `trusted-*` images and any real terminal
app image have both. `multiterminal` requires an explicit `command` — a holder owns
PID 1, so the image's ENTRYPOINT/CMD never runs and `exec` cannot replay it.

**Keybind schemes.** hzc's *own* TUI keys (move the list, edit a field, scroll
logs) are not hardcoded: they resolve through a selectable **scheme**. Two are
built in — `default` (the historical bindings — an install with no config behaves
exactly as before) and `vim` — and users can define their own as
`~/.config/hyprzinc/hzc/schemes/<name>.toml` (a `base` to inherit from plus a
partial `[bindings.<screen>]` override table). The active scheme lives in
`~/.config/hyprzinc/hzc/keys.toml` (`scheme = "<name>"`); `hzc keys
list|show|set|edit|validate|path` and an in-TUI picker (open with `?`) choose,
author, and apply one — switching live. Bindings are scoped per screen (list /
form / logs / confirm) because the same key means different things in each.
These are hzc's interface keys only, **distinct from the Hyprland desktop hotkeys
in §12**, which are a host-level concern owned by the Nix module (M8/M10). hzc's
defaults are already keyboard/vim-friendly, so the two built-ins differ only
modestly; the point is the user-defined schemes.

### 9.2 hzl — GUI Launcher + Smart Executor

**Stack:** Go, GUI (rofi-style window, rendered icons + text, vertical list — not grid)

**Trigger:** `Super+G`

**Behavior — search line is the universal entry point:**

User types → fuzzy-matched against five sources, results merged into one ranked list with source indicators:

| Source | Indicator | Examples |
|---|---|---|
| App definitions | app icon | `firefox`, `slack` |
| Built-in commands | system icon | `brightness up`, `volume mute`, `lock`, `suspend`, `screenshot` |
| Projects | folder icon | detected from configurable paths in home-manager config |
| User custom commands | custom icon | from `~/.config/hyprzinc/commands/*.toml` |
| VMs | VM icon | from `~/.config/hyprzinc/vms/*.toml` |

**Layout (vertical list, one card per row):**

```
┌──────────────────────────────────────────────┐
│ 🔍 [ firef                              ]    │
├──────────────────────────────────────────────┤
│ 🦊  firefox                                  │
│     Web browser · strict profile             │
├──────────────────────────────────────────────┤
│ 📁  firefly-project                          │
│     ~/code/firefly-project                   │
├──────────────────────────────────────────────┤
│ ⚙️   firefox: stop                            │
│     Stop running container                   │
├──────────────────────────────────────────────┤
│ ➕  Add new app                              │
└──────────────────────────────────────────────┘
```

**Built-in commands (initial set):**

- `brightness up` / `down` / `set <value>`
- `volume up` / `down` / `mute`
- `mic mute` / `unmute`
- `lock` — invoke screen locker
- `suspend` / `hibernate` / `poweroff` / `reboot`
- `screenshot` / `screenshot region`
- `night mode toggle`
- `clipboard history`
- Profile switching: `profile <name>`

**Custom commands (`~/.config/hyprzinc/commands/<name>.toml`):**

```toml
schema_version = 1

[command]
name        = "deploy-staging"
description = "Deploy current branch to staging"
icon        = "rocket"
exec        = "ssh staging.example.com 'cd /app && git pull && systemctl restart app'"
confirm     = true   # show confirmation prompt before executing
```

Custom commands live in user's home-manager config — versioned, reproducible across machines.

**Projects:**

Configurable scan paths in home-manager:

```nix
hyprzinc.hzl.projectPaths = [ "~/code" "~/work" ];
```

hzl detects directories under those paths (with a `.git`, `Cargo.toml`, `go.mod`, etc. marker), shows them in results. Selecting one opens it in the configured IDE/terminal container.

**Icon handling:**

The `icon` field resolves in order: (1) if it's a path that exists → use that file; (2) else treat as a freedesktop icon name → look up in the active icon theme; (3) else → generic fallback icon by source type.

When a user sets a custom icon (path), hzc **copies the asset into a managed store** rather than referencing a random filesystem location:

```
~/.local/share/hyprzinc/icons/<app-name>.<ext>     # original, copied in
~/.local/share/hyprzinc/icons/cache/<app-name>.png # normalized thumbnail
```

(`~/.local/share` per XDG Base Directory spec — keeps `$HOME` clean, no scattered files.)

hzl renders from the normalized thumbnail cache (fixed size, e.g. 64×64) so list rendering is fast and visually consistent regardless of source image resolution or format.

### 9.3 home-manager Module

Single Nix flake outputs:

```
outputs = {
  packages.x86_64-linux.hzc = ...;
  packages.x86_64-linux.hzl = ...;
  packages.x86_64-linux.hzv = ...;
  homeManagerModules.hyprzinc = ./nix/module.nix;
};
```

Module generates (host-level, static):
- Hyprland config
- hzc/hzl/hzv binaries on PATH
- Terminal emulator config (terminal runs on host)
- Shell, fonts
- The curated theme bundle (see §5.6) — RO-mounted into themed containers
- **Optional first-run seed** for `~/.config/hyprzinc/` TOMLs: the `apps`/`profiles`
  attrs below are copied in on *initial activation only*. After that, **hzc owns those
  files** (§3) — Nix does not regenerate or overwrite them on subsequent
  `home-manager switch`. The config store is hzc's mutable runtime state, not a
  Nix-managed read-only symlink tree.

```nix
hyprzinc = {
  enable = true;

  theme = {
    # Single source of truth for system theme.
    # Generates GTK/Qt configs, icon/cursor theme settings.
    # Mounted read-only into containers with theme.mode = "host".
    gtk = {
      name      = "Adwaita-dark";
      iconTheme = "Papirus-Dark";
    };
    qt = {
      platformTheme = "gtk3";
    };
    cursor = {
      name = "Bibata-Modern-Classic";
      size = 24;
    };
    fonts = {
      sans      = "Inter";
      mono      = "JetBrainsMono Nerd Font";
      size      = 11;
    };
  };

  # apps/profiles below are FIRST-RUN SEED ONLY (see "Optional first-run seed" above).
  # hzc owns the live TOMLs after initial activation; edits made in hzc are not
  # clobbered by later `home-manager switch`.
  apps = {
    firefox = {
      preset = "strict";
      image  = "docker.io/library/firefox@sha256:...";
    };
    work-app = {
      preset = "networked";
      network = { mode = "container"; target = "vpn-container"; };
    };
  };

  profiles = {
    work = {
      autostart = [ "vpn-container" "firefox" "slack" ];
    };
  };

  hzl = {
    projectPaths = [ "~/code" "~/work" ];
  };

  vpn = {
    enable = true;
    backends = [
      { name = "personal";  type = "amnezia-wg"; configPath = ./vpn/personal.conf; }
      { name = "corporate"; type = "amnezia-wg"; configPath = ./vpn/corporate.conf; }
    ];
    routes = [
      { cidr = "10.0.0.0/8";   outbound = "corporate"; }
      { cidr = "1.1.1.1/32";   outbound = "personal";  }
    ];
    defaultOutbound = "xray-us";
  };
};
```

`home-manager switch` is the only command needed to bring a new workstation to your full state.

---

## 10. Virtualization (hzv)

For isolation needs beyond what containers provide — untrusted GUI apps, foreign OSes, throwaway environments — HyprZinc uses VMs as a parallel runtime to containers. **Containers remain the primary runtime; VMs are the heavy-isolation escape hatch.**

### 10.1 Why VMs (and not nested Wayland compositors)

A nested compositor like cage covers a narrow slice of apps and breaks on real-world software (multi-window apps, IME, drag-and-drop, browsers). A VM is heavier but works for any guest, including Windows. Implementation cost is comparable; coverage and reliability are much better.

### 10.2 Stack

`libvirt` + `qemu` underneath. `hzv` is a Bubbletea TUI on top — same patterns as `hzc`, separate config tree, separate runtime.

### 10.3 VM Config (TOML)

`~/.config/hyprzinc/vms/<name>.toml`:

```toml
schema_version = 1

[vm]
name        = "untrusted-firefox"
description = "Sandboxed browser for sketchy links"
destroy_on_shutdown = false      # true = wipe disk on shutdown (disposable)
autostart   = false

[hardware]
cpus        = 4
memory_mb   = 4096
disk_gb     = 32

[os]
image       = "fedora-39-cloud"  # libvirt image pool name, or path
firmware    = "uefi"             # uefi | bios

[display]
mode        = "window"           # window | wayland | passthrough
# window      → VM renders its own desktop, you see one host window with it inside (MVP)
# wayland     → virtio-gpu + waypipe/spice; guest apps appear as host windows (phase 2)
# passthrough → looking-glass + dedicated GPU passthrough (phase 3)

[network]
mode        = "user"             # user | bridge | none | container
# user      → qemu user-mode NAT (MVP)
# bridge    → libvirt bridge to host network (MVP)
# none      → no network (MVP)
# container → route through vpn-container (future — needs tap into container netns)

[mounts]
# virtiofs shares from host into guest. Explicit, opt-in.
# "/home/user/Downloads" = "/mnt/shared/Downloads"

[devices]
usb_passthrough  = []            # USB device IDs to forward
gpu_passthrough  = false         # phase 3 only
```

### 10.4 Display modes — ship order

**MVP (v1) ships `window` mode only.** Phase 2 and 3 are roadmap items, not v1 commitments.

| Mode | Phase | What it gives | Cost |
|---|---|---|---|
| `window` | **1 (MVP)** | Full guest desktop in a single host window. Works for any guest OS. | Window-in-window UX |
| `wayland` | 2 (future) | Guest apps integrate as native host windows (Linux guests only, virtio-gpu + waypipe or spice) | Needs guest cooperation |
| `passthrough` | 3 (future) | Near-bare-metal performance via looking-glass + GPU passthrough | Requires dedicated GPU, niche |

The TOML schema includes all three modes from day one — only the `window` implementation ships in MVP. `wayland` and `passthrough` modes return a clear "not implemented yet" error if selected before they ship.

### 10.5 Disposable VMs

When `destroy_on_shutdown = true`, `hzv` clones a fresh disk from a base image on each launch and destroys it on shutdown. No state persists. Useful for one-off sketchy tasks (opening a suspicious file, testing in a clean environment).

**Default is `false`** — VM state always persists unless explicitly opted out. No surprise data loss.

Base images live in libvirt's storage pool. HyprZinc ships no default images — user provides their own (cloud images, ISOs, existing qcow2 files). `hzv` registers and tracks them.

### 10.6 VM vs Container — when to use which

| Use case | Choice |
|---|---|
| Dev environment, browser, messenger, daily apps | **Container** (lighter, faster) |
| Untrusted binary, foreign-source GUI app | **VM** (`window` mode) |
| Windows-only app | **VM** (no other option) |
| Disposable "open and forget" task | **VM** (`destroy_on_shutdown = true`) |
| Heavy 3D / gaming / GPU work in isolation | **VM** (`passthrough`, phase 3) |

### 10.7 Integration with hzl

VMs appear in `hzl` alongside apps, with a VM icon. Selecting one launches via `hzv`. Same fuzzy-search UX. Profiles can include VMs in autostart lists (with a warning about boot time).

### 10.8 Host surface impact

`libvirtd` (or rootless equivalent — `qemu:///session` with libvirt user session) runs on the host. Adds one daemon. Acceptable cost for the isolation gain. Rootless libvirt session is the default; system-level libvirt only if explicitly enabled.

---

## 11. Host Surface (Minimal)

| Component | Reason |
|---|---|
| Hyprland | Compositor owns the display |
| Terminal emulator | Drops into containers on explicit launch |
| hzc / hzl / hzv | Container and VM management |
| Pipewire | Audio server; sockets passed in on explicit grant |
| Podman (rootless) | Container runtime |
| libvirt user session + qemu | VM runtime (rootless) |
| home-manager | Declarative config |
| Network manager | System-level, host only |

Everything else runs in containers or VMs.

---

## 12. Hotkeys (Secondary — see issue #5 for baseline)

> These are **Hyprland desktop** hotkeys (host-level, Nix-generated; M8/M10) — not
> the same thing as hzc's in-TUI keybind schemes, which are configured per-user
> under `~/.config/hyprzinc/hzc/` and described in §9.1 ("Keybind schemes").

HyprZinc-specific additions:

| Hotkey | Action |
|---|---|
| `Super+G` | Open hzl (launcher + smart executor) |
| `Super+Shift+G` | Open hzc (app manager TUI) |
| `Super+Ctrl+G` | Open hzv (VM manager TUI) |
| `Super+P` | Open hzl with profile filter active |

---

## 13. Repo Layout

Each tool is its own Go module — independent `go.mod` and vendored deps — sharing
**one** build pipeline: a repo-root generic `Containerfile` (digest-pinned Go
toolchain; it builds whichever module is the build context), a `check.mk` of
containerized checks every module includes, and a `tool.mk` (binary targets) that
each tool's three-line `Makefile` includes. "The same logic, only different paths."

The `core` module (`…/hyprzinc/core`) is structured as a **hexagon** (ports &
adapters), so a mechanism can be swapped by writing a new adapter rather than
editing call sites — the motivating case being egress enforcement, where "not
pasta" later is one more adapter (§5.3). The layers:

- **`domain/`** — pure model + rules: the app-config schema, validation, presets,
  and the derived-image policy. No I/O, no podman/nft/fs/env. The hexagon's center.
- **`ports/`** — the interfaces the application depends on: `Store`, `Runtime`,
  `ImageBuilder`, `ImageResolver`, and **`NetEnforcer`** (the egress swap point).
- **`app/`** — the application service that orchestrates a launch through the ports
  (validate → build derived image → run the egress lock-down via `NetEnforcer` →
  start the app). The single launch path reused by `hzc` and `hzl` (§9.1).
- **`adapters/`** — the concrete edges: `podman` (Runtime/ImageBuilder/Resolver),
  `netenforce` (the `none`/`pasta`/`container` enforcers — nft+pod lives here),
  `fs` (TOML store + codec), `host` (env → options).
- **`wire/`** — the composition root: the one place that imports every adapter and
  assembles them into an `app.Service`. Front-ends call it; nothing else names a
  concrete adapter.

Each consumer pulls core via a local `replace => ../core` and **vendors its
source**, so the per-module container build stays hermetic (no network, no sibling
checkout at build time). A repo-root `go.work` ties the modules together for local
editing; the build never depends on it (`make vendor` runs with `GOWORK=off`).

Implemented today: `core/` + `hzc/` in full; `hzl/` reuses `core/app` via `core/wire`
(CLI now, gioui UI in M7); `hzv/` is a buildable skeleton (imports no core).

```
hyprzinc/
├── Containerfile              ← generic reproducible build (any module; digest-pinned Go)
├── check.mk                   ← containerized checks (test/vet/fmt/vendor); every module includes it
├── tool.mk                    ← binary targets (build/run/repro); each tool's Makefile includes it
├── go.work                    ← ties the modules together for local dev (build never uses it)
├── .gitattributes             ← marks vendor/ as linguist-vendored (clean diffs/stats)
├── core/                      ← shared hexagon (domain/ports/adapters) — module …/hyprzinc/core
│   ├── go.mod · go.sum
│   ├── domain/                ← pure: schema, validation, presets, derived-image policy
│   ├── ports/                 ← interfaces: Store, Runtime, ImageBuilder, ImageResolver, NetEnforcer
│   ├── app/                   ← application service: launch/stop/build orchestration (reused by hzc + hzl)
│   ├── adapters/
│   │   ├── podman/            ← Runtime + ImageBuilder + ImageResolver (podman argv + exec)
│   │   ├── netenforce/        ← NetEnforcer adapters: none · pasta (pod + nft) · container  ← swap point
│   │   ├── fs/                ← app-definition store + TOML codec (~/.config/hyprzinc/apps)
│   │   └── host/              ← environment → host launch options
│   ├── wire/                  ← composition root (assembles adapters → app.Service)
│   ├── vendor/
│   └── Makefile               ← include ../check.mk (library: no binary)
├── hzc/                       ← HyprZinc Container — app manager (CLI + Bubbletea TUI)
│   ├── go.mod · go.sum        ← module …/hyprzinc/hzc; replace core => ../core
│   ├── main.go                ← imperative shell (CLI)
│   ├── examples_test.go       ← validates the shipped example apps
│   ├── internal/tui/          ← Bubbletea TUI (hzc-specific)
│   ├── examples/apps/         ← sample app TOMLs
│   ├── vendor/                ← vendored deps incl. core source → hermetic builds
│   ├── Makefile               ← TOOL := hzc + include ../tool.mk (+ validate, netfilter-image)
│   └── .containerignore
├── hzl/                       ← HyprZinc Launcher (Go) — reuses core/launch; gioui UI in M7
│   ├── go.mod · main.go · vendor/
│   ├── Makefile               ← TOOL := hzl + include ../tool.mk
│   └── .containerignore
├── hzv/                       ← HyprZinc Virtualization (Go, Bubbletea) — skeleton; UI in M9
│   ├── go.mod · main.go
│   ├── Makefile               ← TOOL := hzv + include ../tool.mk
│   └── .containerignore
├── nix/
│   ├── flake.nix
│   ├── module.nix             ← homeManagerModules.hyprzinc
│   ├── apps/                  ← Nix attrs → generates app TOML
│   └── vms/                   ← Nix attrs → generates VM TOML
├── images/                    ← runtime base images for app containers (§7)
│   ├── trusted-base/          ← Containerfile for hyprzinc/trusted-base
│   ├── trusted-go/
│   ├── trusted-go-dev/
│   ├── trusted-rust/
│   └── vpn/                   ← vpn-container image (sing-box + backends + entrypoint)
├── docs/
│   └── architecture.md        ← this document
└── .github/
```

Two distinct container concerns: the repo-root `Containerfile` reproducibly builds
a **tool binary** (pinned Go toolchain + that module's vendored deps, if any);
`images/` holds the **runtime base images** that app containers are layered on (§7).

---

## 14. Known Issues & Tradeoffs (Documented)

| # | Issue | Mitigation |
|---|---|---|
| 1 | wsc enforcement is partial — most apps ignore the protocol | Container isolation is the real boundary; use a VM (§10) for genuinely untrusted apps |
| 2 | DNS leakage potential | sing-box DNS + pasta blocks 53/853/DoH |
| 3 | GPU passthrough weakens isolation | Warn in hzc; never enable for untrusted images |
| 4 | sing-box backend changes need restart | Backends semi-static; routing rules live-reload |
| 5 | CRIU does not work for vpn-container | Brief network blip on vpn-container restart |
| 6 | Podman-in-Podman can read outer mounts | CI container mounts minimum; no keys |
| 7 | vpn-container's `CAP_NET_ADMIN` visible to attached containers | Read-only visibility; acceptable |
| 8 | Image tags can be poisoned upstream | Pin by digest; explicit update only |
| 9 | Profile activation may interrupt running apps | Modal confirm before stopping running apps |
| 10 | Project detection scans filesystem | Configurable paths, no implicit `$HOME` scan |
| 11 | VM startup is slow (10-30s) | Acceptable for isolation use case; not a daily-driver replacement for containers |
| 12 | `display.mode = "window"` is window-in-window UX | MVP tradeoff; `wayland` mode (phase 2) integrates guest apps natively |

---

## 15. Open Decisions (Next)

- [x] hzl GUI toolkit — **gioui** (pure Go, single binary, fast startup)
- [x] Theme: support GTK 3+4 and Qt 5+6 — plain config files, no meaningful surface increase
- [x] First language image to ship — **Go** (matches primary language)
- [x] hzl profile activation UX — **modal confirm** before stopping running apps
- [x] Audio passthrough — Pipewire socket by default, `legacy_alsa = true` in `[audio]` block for `/dev/snd` fallback
- [x] Cage fallback — **dropped**, replaced by VM option (§10)
- [x] hzv MVP scope — **`window` mode only** for v1
- [x] Base VM images — **user-provided**, no defaults shipped
- [x] VM disposable cleanup — **never destroy by default**; `destroy_on_shutdown = true` in TOML opts in

Clarifications (resolved 2026-05-30):

- [x] Config store ownership — **hzc owns the live TOMLs**; Nix `apps`/`profiles` attrs are first-run seed only (§3, §9.3)
- [x] Image acquisition & reproducibility — Nix ships Containerfiles; images built/pulled locally; third-party pinned by digest, `trusted-*` by local tag over a digest-pinned base (§5.5, §7)
- [x] Egress filtering mechanism — **nftables inside the container netns** enforces the CIDR/port allowlist; pasta only provides connectivity (§5.3)
- [x] Theme passthrough shape — **one curated, generated theme bundle**, RO-mounted; never the host's real config dirs (§3, §5.6)

Clarifications (resolved 2026-06-09):

- [x] Module structure — **one self-contained Go module per tool** (`…/hzc`, `…/hzl`, `…/hzv`): own `go.mod`, vendored deps, reproducible digest-pinned container build. The pure core (config + runspec) stays in `hzc/internal/` and **graduates to a shared `…/hyprzinc/core` module when `hzl`/`hzv` need it** — no multi-module plumbing before a second consumer exists (§13).
- [x] Build pipeline & dependency hosting — **monorepo with per-tool modules**; the build logic is shared, not copied: a repo-root `tool.mk` (each tool's `Makefile` is `TOOL := …` + `include ../tool.mk`) and one generic repo-root `Containerfile` ("same logic, only different paths"). Deps are **vendored** (a full copy in-tree) per module and marked `linguist-vendored`, **not git submodules** — submodules don't integrate with `go.sum` / `-mod=vendor` and add real UX cost, while GitHub already treats `vendor/` as vendored. If deps ever overlap heavily across tools, the Go-native dedupe is a workspace `go work vendor`, not a submodule (§13).

All open decisions resolved. Ready for implementation.
