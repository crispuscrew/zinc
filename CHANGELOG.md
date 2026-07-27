# Changelog

All notable changes to Zinc are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Zinc uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). The version line is
tracked in [RELEASES.md](RELEASES.md).

## [Unreleased]

### Added

- **Windows-class guests.** A guest can now be given the machine Windows requires: UEFI
  firmware with a per-app writable variable store, Secure Boot, an emulated TPM 2.0 backed
  by swtpm, and `Devices: Compatible` - an AHCI disk, an Intel NIC and a USB tablet, which
  every mainstream OS drives out of the box. Windows Setup has drivers for none of the
  virtio hardware and reports finding no drives at all when pointed at a virtio disk, which
  is why the device profile is a field rather than an assumption. `Display: Compatible` adds
  a plain-VGA mode for guests with no virtio-gpu driver.
  A TPM guest needs the current 4 MB OVMF build (`edk2-ovmf` on Fedora 41+, `ovmf` on
  Debian 12+), and Zinc prefers it wherever both generations are installed. The legacy 2 MB
  build does not hand the TPM to the guest, and the failure is close to invisible: qemu
  publishes the TPM's ACPI device itself, so the guest enumerates it and binds a driver to
  it, and only Windows notices there is nothing behind it. It reads TPM version 0 and says
  no more than "This PC doesn't currently meet Windows 11 system requirements". Zinc warns
  when a TPM guest can only get the legacy build. Variable stores are not interchangeable
  between the two generations, so one written by the other build is refused by name rather
  than handed to qemu, which would boot it with a quietly wrong Secure Boot state.
- **Every VM app gets its own machine identity.** qemu with no `-uuid` reports the SMBIOS UUID
  `00000000-0000-0000-0000-000000000000` and gives the NIC the MAC `52:54:00:12:34:56`, both
  shared with every other default qemu VM. Windows Autopilot identifies a device by a hash
  over exactly those fields, so a guest with the defaults can match a stranger's corporate
  enrolment: a fresh Windows 11 install here reached OOBE demanding a sign-in to SAP's tenant,
  branded with their logo. Both are now derived from the app name, so they are unique per app
  and stable across restarts and resets - a changing UUID would make Windows think the
  hardware had been swapped and ask to be reactivated. An install has no app yet and runs
  under a fixed placeholder name, so it seeds its identity from the disk's own path instead:
  deriving from the placeholder would have given every install on every host the same UUID,
  which is the collision this exists to prevent, at the one moment it matters most - OOBE runs
  during an install, and OOBE is what reads it.
- **A fixed guest screen size** - `DisplayWidth`/`DisplayHeight`, `zc new --resolution WxH`,
  `zvr install --resolution WxH`. A guest with no display driver takes whatever mode the
  firmware gave it at boot and cannot change it, so `Display: Compatible` was always exactly
  1280x800 and resizing the window only scaled those pixels. That number comes from plain
  VGA's built-in EDID; the device has no resolution property at all. Asking for a size
  switches the guest to `bochs-display`, which the firmware does honour (measured: Windows
  Setup rendering at a true 1920x1080). `virtio-vga` and `qxl-vga` accept the same properties
  but are not used, because OVMF drives their VGA-compatible half and falls back to 1280x800.
  The one cost is that `bochs-display` has no VGA-compatible mode, so a fixed size requires
  UEFI; validation refuses the pairing rather than letting a BIOS guest boot to a blank
  window, and a guest that asks for nothing keeps plain VGA exactly as before.
  Sizes up to 3840x2400 work, 4K included. Getting there needed two more things, because the
  display's own defaults break down above about 3200x1800 and both failures are silent - the
  guest simply comes up at 1280x800 with nothing logged. Its framebuffer is now sized to the
  screen (the 16 MiB default is less memory than a 4K screen needs), and the generated EDID's
  refresh rate is lowered just enough for the mode to exist: that rate multiplies a pixel
  clock stored in 16 bits, and at QEMU's default 75 Hz a 4K clock overflows the field. The
  rate is close to cosmetic for a guest, whose framebuffer is virtual and whose presentation
  the host compositor drives. Above that, the EDID's active-pixel fields are 12 bits wide, so
  neither side may exceed 4095: 3840x2400 is fine and 4096x2160 is not. Validation refuses
  what cannot work rather than letting it fall back without a word.
- **`MacAddress` / `zc new --mac`** to set the guest NIC's address. The derived default sits
  under QEMU's own `52:54:00` prefix, which says plainly that the machine is a QEMU guest; an
  app that should not announce that can supply its own. A locally-administered address (first
  octet `02`, `06`, `0a` or `0e`) belongs to no vendor and so identifies nothing. The value is
  screened before it reaches qemu, where a comma would start a new device property.
  `--mac random` draws one instead of making you invent it. It is drawn once, at authoring
  time, and the literal address is written into the config: a config that said "random" would
  draw a new address on every run, and a guest whose NIC changes underneath it loses its DHCP
  lease and looks to Windows like swapped hardware.
- **`zvr install`** - runs an OS installer to produce a base disk, for guests that have no
  cloud image. It takes flags rather than an app name deliberately: an app pins its base by
  digest and a disk that does not exist yet has no digest, so requiring one would be a
  chicken and egg. When the install finishes it prints the disk's digest and the `zc new`
  line to author an app against it; from then on every run is an overlay, so `zvr reset`
  returns the app to its freshly installed state.
- `zc new --firmware/--secure-boot/--tpm/--devices` to author all of the above.
- **`InstallMedia` discs are attached on every run**, not only during the install, and
  **`zc new --media`** authors them. An OS is only the first thing a guest wants from a CD:
  the drivers its installer had no room for arrive the same way, and a guest whose network is
  not up yet has no other route in - which is exactly the position a fresh Windows guest is
  in, since the driver that would fix its display is on a disc it could no longer be handed.
  Only the boot order stays the install's own, so an ordinary run boots the disk with the disc
  simply present. The discs are read-only, so leaving one in a config costs a drive letter.
- **`make -C virtualization/runner windows-demo WIN_ISO=...`** - the whole Windows flow from
  one argument. The Windows ISO is Microsoft's and cannot be fetched for you; everything
  else is handled, including downloading the virtio-win driver disc (resumable, since it is
  ~700 MB) and defaulting the machine to what Windows 11 requires. When Setup finishes it
  pins the installed disk and authors an app against it.

### Known limitations

- **Windows guests get no 3D acceleration.** There is no virtio-gpu driver for Windows, so
  they run on an unaccelerated framebuffer; passthrough, the usual answer, needs a second GPU.
  Windows guests are for software that must run on Windows, not for games.
- **A guest's screen size is fixed at boot.** Resizing the window scales those pixels rather
  than changing the guest's resolution, because a guest with no display driver cannot be told
  about a new mode. Making the window resize the guest needs a real driver inside it
  (`viogpudo` from virtio-win, or QXL).
- **An installed Windows desktop caps its width at 1728**, whatever it is given. Measured on a
  Windows 11 24H2 guest with no display driver: asked for 1920x1080, 1920x1200 and 1920x1024
  it painted 1728 columns and left the rest black, and at 1728x1080 it filled the screen
  exactly. Height is honoured throughout, and the cap is on width rather than framebuffer
  bytes - 1920x1024 is smaller than 1728x1080 and still got clipped. Windows Setup itself is
  not affected and uses whatever mode it is given, so this is the installed desktop's own
  behaviour. Until a guest display driver is installed, a Windows app is best authored at
  `--resolution 1728x1080`, which is the same usable desktop with no wasted black.

## [0.5.0] - 2026-07-26

Guest GPU access. A VM app's 3D now reaches the host GPU for both OpenGL and Vulkan.

### Added

- **Guest Vulkan** (`VirtualizationMeta.Vulkan`, `zc new --vulkan`) - passes the guest's
  Vulkan through to the host GPU via qemu's venus. This is the half that matters for games,
  since Proton, DXVK and vkd3d are all Vulkan; without it a guest's Vulkan runs on the CPU.
  Measured on a host with the proprietary NVIDIA driver, rendering real frames: the guest
  reports `Virtio-GPU Venus (NVIDIA GeForce RTX 5080)` and scores **2243** in vkmark, while
  OpenGL reports `virgl (NVIDIA GeForce RTX 5080/PCIe/SSE2)` and scores **3947** in glmark2
  against **931** for the software renderer.
- **`make -C virtualization/runner virgl-venus`** - builds the venus-capable virglrenderer
  that guest Vulkan requires, into the user's own data directory, never touching the system
  copy. Distributions ship virglrenderer built *without* venus, so one has to be built;
  `zvr` finds it automatically and `ZVR_VIRGL_PREFIX` overrides the location.

### Changed

- **Vulkan is opt-in, and says what it costs.** Enabling it disables qemu's seccomp sandbox
  **for that app only**: venus runs in a helper process that virglrenderer forks, and the
  sandbox both forbids the fork and kills the child that inherits its filter - silently,
  surfacing only as a generic "virgl could not be initialized". `zc validate` warns that the
  guest gains GPU Vulkan while the qemu process loses its syscall filter. An app that does
  not ask keeps the sandbox and still gets accelerated OpenGL.
- `zvr` refuses to launch a Vulkan app when no venus-capable virglrenderer is present,
  printing how to build one, rather than starting qemu and letting the guest quietly fall
  back to software rendering.

### Known limitations

- Guest Vulkan and qemu's seccomp sandbox are mutually exclusive; there is no way around it
  today, because the venus helper process cannot survive the filter.
- The venus virglrenderer has to be built locally until distributions ship one.
- Everything else from 0.4 stands: no egress filtering for VM apps, no host directory
  sharing, no snapshots, x86_64 guests only.

## [0.4.0] - 2026-07-26

Adds VM apps. Guests boot, display and are managed; making their 3D actually reach the GPU
is 0.5 work, and the limits below say exactly where that line falls.

### Added

- **`zvr` (zinc-virtualization-runner)** - boots VM apps as qemu guests, the sibling of
  `zcr`. One store and one schema hold both app kinds, split by `Type`, and each runner
  refuses the other's apps by name rather than half-running them. Disks are copy-on-write
  over a digest-pinned base image that is never written to, so `zvr reset` returns an app to
  the image it was authored against. A cloud-init seed gives a fresh guest its hostname,
  user and SSH key. Supervision is a pidfile plus QMP, and the same socket carries the ACPI
  power button, so `zvr stop` lets the guest's own OS flush and unmount rather than being
  killed mid-write. `zvr run --dry-run` prints the exact qemu command line, as `zcr` does for
  podman.
- **VM apps in the schema** - `Type: ZincVirtualization` plus a `VirtualizationMeta` group:
  the guest's sizing, an explicit display mode, user-mode port forwards and cloud-init
  identity. `ImageMeta` is shared rather than duplicated - `Image` is a container reference
  or a base disk path depending on `Type`, and `Install` steps become a derived image's RUN
  layer or the guest's cloud-init `runcmd`. The two kinds **reject each other's fields**
  rather than ignoring them: a VM app carrying `Capabilities` or `NetworkLists` does not
  save, and each message says what supporting it would take. A field that looks configured
  while doing nothing is worse than one that refuses, because the author believes in a
  boundary that is not there.
- **Accelerated guest display** - `Display: Accelerated` attaches `virtio-gpu-gl` and a local
  window, so guest 3D runs on the host GPU and frames reach the compositor without leaving
  the machine or being encoded. This is **OpenGL only**: a guest's Vulkan falls back to
  llvmpipe, software rasterisation on the CPU, which matters because Proton, DXVK and vkd3d
  are all Vulkan. See the known limitations below.
- **`make -C virtualization/runner demo`** - downloads a Fedora Cloud image (verified against
  the digest Fedora publishes), authors a VM app with an accelerated display and boots it, so
  the whole path can be tried in one command.
- **`make -C virtualization/e2e e2e`** - end-to-end tests that boot a real guest under qemu
  and assert what unit tests cannot: that it boots and is reachable, that a base image
  altered after authoring refuses to run, and that a graceful stop is graceful. Not a CI
  gate - GitHub's runners have no `/dev/kvm` - so it is a local pre-release gate that skips
  cleanly when qemu or KVM is missing.

### Changed

- **BREAKING: `zcc` is now `zc` (`zinc-creator`)**, and `container/creator/` moved to
  `creator/`. The creator no longer creates only containers, so the kind letter in its name
  was wrong. There is no separate `zvc`: a VM app is the same YAML with a different `Type`,
  and `zc` already had the form, keybinds, validation and CLI for authoring it, so the
  non-compiling `virtualization/creator/` skeleton was deleted rather than migrated. The
  runners keep their kind letter, because `zcr` and `zvr` really are different things.
  Reinstall the binary under its new name; app files are unaffected.
- **`zc` authors both kinds.** `zc new --vm` writes a VM app, and the form grows a type row
  that rebuilds the rest of itself. Runtime commands route by the app's `Type` - `zcr` for
  containers, `zvr` for guests - with commands that have no counterpart refused by name
  rather than silently doing nothing.
- **VMs are run over qemu directly, not libvirt**, a deliberate departure from the
  architecture doc. libvirtd spawns the qemu process outside the user's session, so it cannot
  open an accelerated window on their compositor, and SPICE plus a viewer adds a hop that
  defeats the point for anything interactive. The cost, accepted knowingly, is that `zvr`
  owns supervision and has no snapshots or managed save.

### Known limitations

- VM apps have **no egress filtering**: the nftables model lives inside a container's own
  network namespace and does not reach a guest, so a VM app gets user-mode networking with
  explicit port forwards, each bound to 127.0.0.1. `NetworkMeta` on a VM app is an error.
- **No host directory sharing** into a guest (that needs virtiofs), no snapshots, x86_64
  guests only, and `zvr console` prints the console socket rather than attaching to it.
- **OpenGL is hardware-accelerated, confirmed by measurement.** A Fedora guest reports
  `OpenGL core profile renderer: virgl (NVIDIA GeForce RTX 5080/PCIe/SSE2)`, GL 4.2 core -
  the guest's GL is running on the host GPU, not in software.
- **Guest Vulkan is NOT accelerated.** It falls back to llvmpipe, which is the CPU, so
  anything using Proton, DXVK or vkd3d renders in software. Vulkan needs qemu's `venus=on`,
  which needs a host `virglrenderer` built with venus support; Fedora 43's is not, and
  enabling venus without one fails the whole renderer and leaves the guest with no display
  at all rather than degrading to OpenGL. Building a venus-capable virglrenderer is not by
  itself sufficient either: venus runs in a separate render-server process, and its
  handshake still fails on this host. Tracked for 0.5.

## [0.3.1] - 2026-07-25

Fixes the two things that made the 0.3 overlay look broken: a frozen window during a launch,
and a grid of thumbnails that never appeared.

### Changed

- **`zlg` no longer freezes while an app starts.** `menu`'s activate callback now runs off the
  Wayland event loop, so the overlay keeps drawing - with a `launching <app>...` banner - while
  `zcr` works, instead of sitting frozen on screen holding an exclusive keyboard grab for the
  whole launch (a second or two normally, minutes when a derived image has to be rebuilt).
  Esc dismisses the window immediately; the launch carries on in the background and `menu.Run`
  returns once it finishes, so no activation outlives the call. A second Enter while one launch
  is in flight is ignored rather than starting a duplicate. Callers name the banner's verb with
  the new `Options.BusyVerb`.
- **Thumbnail and icon decoding is bounded in bytes, not pixels.** The old guard capped declared
  pixels, which is not a memory bound: the same dimensions cost one byte per pixel as a paletted
  image and eight at 16 bits per channel, so a deep-colour file crafted to sit just under the
  pixel cap still decoded to several times what the cap implied. The budget is now the decoded
  size the file's own header implies, which cuts the grid's adversarial worst case from about
  1.7 GiB to 480 MiB while still accepting every plausible wallpaper (an 8K photograph decodes
  to about 132 MiB).

### Fixed

- **Grid thumbnails never appeared unless they decoded within the first 160ms.** The frame
  callback that paces the decode poll is double-buffered Wayland state, applied only when the
  surface commits, and the poll's "nothing landed yet, ask again" path asked without committing.
  The callback therefore never fired and the poll died silently, leaving every tile on its
  placeholder until the next keypress. It went unnoticed because the entrance fade commits a
  frame per refresh while it runs, which is long enough for the small sample images but not for
  a directory of real photographs.

### Known limitations

Unchanged from 0.3.0: the keymap is US-QWERTY, and grid thumbnails decode without
prioritisation or cancellation. The launch freeze is gone, but a launch still cannot be
**cancelled**: Esc dismisses the overlay and the launch runs to completion behind it.

## [0.3.0] - 2026-07-25

Adds the GUI launcher.

### Added

- **`zlg` (zinc-launcher-gui)** - the graphical sibling of `zlt`: the same quick
  picker over the defined apps, as a floating `wlr-layer-shell` overlay. Type to
  filter, up/down (or ctrl+p/n) to move, enter launches the selected app through
  `zcr` and quits, and a dot marks apps already running. `zlg <app>` launches one
  directly, for a desktop hotkey. The picker window itself is the new `menu` module
  (below), so `zlg` is a thin consumer that just supplies the app list and an activate
  callback; it inherits a **static, `CGO_ENABLED=0`, runs-anywhere, byte-reproducible**
  binary with no cgo and no graphics libraries. Like `zcc` and `zlt` it shells out to
  the `zcr` binary and never imports the runtime.
- **`launcher/common`** - a shared library holding the read-side app store, the `zcr`
  delegate, and the fuzzy matcher, so `zlt` and `zlg` share one copy of the
  list / launch / match logic (and its security guards).
- **`menu`** - a standalone, reusable overlay-menu module (repo-root `menu/`, module
  path `github.com/crispuscrew/zinc/menu`) extracted out of `zlg`: a pure-Go Wayland
  `wlr-layer-shell` overlay, a software renderer, a keymap, a theme resolver, and a
  fuzzy-filter picker view-model, all behind one call - `menu.Run(items, activate,
  opts)`. It speaks layer-shell directly through a hand-written binding and reads the
  system light/dark theme from the XDG portal, and it depends on **no** Zinc sibling
  module (Go `replace` directives are not transitive), so `zde` and a future wofi-like
  picker can import it too. The fuzzy matcher is copied in as `menu/internal/match`.
- **App grouping** - a new optional `Group` field on the app config (schema v2, additive)
  files apps into sections. The launcher shows a section header per group when idle and
  flattens to a plain ranked list as soon as you type. The demo apps ship with groups.
- **App icons** - the launcher now draws each app's icon in a left column. It resolves the
  existing `Icon` field (a freedesktop icon name or an absolute image path) by looking the
  name up in the icon-theme directories, decoding and scaling in pure Go. It is raster-only
  (PNG/JPEG/GIF); an SVG-only or missing icon just leaves that row's slot blank.
- **Font** - text is antialiased (rendered with `x/image/opentype`), not the old bitmap face.
  It auto-detects an installed system Nerd Font (monospace, so it matches your terminal),
  falling back to the bundled Go Mono when none is found. `ZLG_FONT=/path/to/font.ttf` pins a
  specific font.
- **`menu` grid layout** - the `menu` module can lay items out as a thumbnail grid
  (`Options.Grid`) instead of the default list: each item's new `Preview` image is drawn as a
  tile with its label beneath, arrow keys navigate in two dimensions, and typing still
  fuzzy-filters. Thumbnails decode in the background (bounded, cgo-free, aspect-preserving) and
  appear as they land, so a directory of large images stays responsive. The decode/scale core
  is shared with the icon path in the new `internal/imgutil`, and WebP is now decodable too.
- **`wallpaper` example** - a thumbnail wallpaper chooser built on the grid layout
  (`menu/example/wallpaper`), alongside the `dmenu` example. It scans a directory, shows the
  images as a filterable grid, and on Enter sets the chosen one via `$WALLPAPER_CMD`
  (`swww`/`swaybg`/`hyprpaper`) or prints the path when that is unset - proving the grid is
  reusable by an ordinary program and staying compositor-agnostic.
- **`make -C menu wallpaper-demo`** - the grid's counterpart to the launchers' `demo` targets:
  generates sample wallpapers, builds the example, and opens the grid over them, so the layout
  can be tried in one command. `WALLPAPER_DIR=...` points it at your own images instead. The
  samples are generated deterministically (`menu/example/wallpaper/gen`) rather than committed,
  so the repository carries no binary image assets.
- **Screenshots** - the READMEs now show the launcher and the grid (`docs/media`), captured
  headlessly under sway so they are reproducible and contain nobody's desktop.

### Fixed

- **Double launch in `zlt`** - a second enter while a launch was still in flight started a
  second `zcr run` of the same app. The two raced on one pod, and the loser's fail-closed
  teardown removed the pod the winner had just created, killing the app while the picker
  still reported success. Enter is now ignored until the launch resolves.
- **App names containing `..` were unusable** - the store rejected any name with `..` as a
  substring, not just the traversal segment, so an app called `my..app` was listed by both
  launchers but could never be loaded or launched. The guard now checks path segments; the
  separator check that actually confines a name to the apps directory is unchanged.
- **App names ending in `.yaml` ran the wrong file** - `zcr` reads such an argument as a
  filesystem path, so launching the app `notes.yaml` ran whatever `./notes.yaml` happened to
  be in the launcher's working directory (for a hotkey, `$HOME`) instead of the stored app.
  The launchers now reject that name and point at the explicit `./notes.yaml` path form.

### Known limitations

- `zlg`'s keymap is US-QWERTY; full keyboard-layout (xkb) support is future work.
- Like `zlt`, `zlg` lists and launches; managing an app (stop, logs, edit) stays in
  `zcc`.
- **`zlg` freezes while an app starts.** `menu`'s activate callback is synchronous, so `zlg`
  runs `zcr run` on the Wayland event loop: the overlay stops redrawing and keeps its
  exclusive keyboard grab until `zcr` returns. That is a second or two for a normal launch,
  but minutes for an app whose derived image has to be rebuilt. An asynchronous activate is
  0.4 work.
- The grid decodes thumbnails for every cell it has drawn, with no prioritisation or
  cancellation, so scrolling fast through a very large directory makes the visible tiles
  queue behind ones already scrolled past.
- The thumbnail decode guard caps declared pixels, which bounds a plausible image but not a
  crafted one: a deep-colour file at the pixel limit still decodes to far more memory than
  the cap implies.

## [0.2.1] - 2026-07-22

Adds a runtime volume flag to `zcr`.

### Added

- **`zcr run --volume` / `-v`** - mount an extra host directory or file into an app
  for one run only, without editing its config:
  `zcr run <app> -v HOST:CONTAINER[:OPTIONS]` (repeatable). `OPTIONS` defaults to
  `ro,noexec`; `rw` makes it writable and `exec` executable. The runtime mount is
  appended to the loaded config in memory (never written back to the YAML) and passes
  through the same field-shift / injection validation and the same arg-builder as a
  configured `Volumes` entry. This delivers the "can also be added temporarily at
  runtime" behavior the `Volumes` schema field always described.

## [0.2.0] - 2026-07-19

Adds the launcher.

### Added

- **`zlt` (zinc-launcher-tui)** - a fast, keyboard-first fuzzy picker over the
  defined apps. Type to filter (an in-house subsequence matcher that favours
  matches at the start of a name and at word boundaries), up/down (or ctrl+p/n) to
  move, enter launches the selected app through `zcr` and quits (dmenu-style), and
  a `●` marks apps already running (from `zcr ps`). `zlt <app>` launches one
  directly, for a desktop hotkey. Like `zcc` it depends only on `common` and
  shells out to the `zcr` binary, so it never imports the runtime - dependency
  auto-start, the network lock-down, and derived-image builds stay `zcr`'s job. It
  lives at `launcher/tui`, with `launcher/gui` reserved for the planned `zlg`.

### Known limitations

- `zlt` lists and launches; managing an app (stop, logs, edit) stays in `zcc`. The
  GUI launcher (`zlg`) and richer rows are on the roadmap.
- `virtualization/creator/` is still a non-compiling skeleton (0.7), excluded from
  the build and CI.

## [0.1.0] - 2026-07-16

First release. Ships the container tools: author an app once, run it sandboxed.

### Added

- **`common`** - the shared library: schema version 2 (a flat `AppConfig` with
  grouped `*Meta` structs) and pure validation, run identically at author time
  and at launch time. Third-party images must be digest-pinned.
- **`zcc` (zinc-container-creator)** - a keyboard-first Bubbletea TUI and a
  scriptable CLI to create, validate, edit, rename, and delete app files, with
  selectable keybind schemes (`default`, `vim`, or a custom one) and
  image search / digest-resolve. It depends only on `common` and shells out to
  the `zcr` binary to run apps.
- **`zcr` (zinc-container-runner)** - the rootless-podman runtime:
  `run` / `build` / `validate` / `stop` / `restart` / `inspect` / `logs` /
  `term` / `ps` / `image`. Derived images from `ImageMeta.Install`, terminal and
  multiterminal apps, dependency auto-start, and a dry run that prints the exact
  podman commands and nft ruleset.
- **Fail-closed network lock-down** - applied by nftables in the app's own
  network namespace before the app process starts, with no unfiltered window:
  isolated (localhost only), egress whitelist, LAN publish filtered by source,
  and per-port sibling links over a private internal bridge. A small
  digest-pinned netfilter helper image applies the ruleset.
- **Reproducible, podman-only builds** - a digest-pinned Go toolchain, vendored
  deps, and byte-identical output (`make repro`); a `version` stamped from
  `git describe`; a Go end-to-end suite against real podman; and CI.

### Known limitations

- The network model **rejects at launch** (fail-closed, never mis-enforced):
  host-scoped egress, gateway / multi-homing, and combining a sibling link with
  any other networking on one app.
- Schema-defined but not yet wired into the launch: `ResourcesMeta`,
  `InternalUserMeta`, `NotificationMeta`, and bundle-relative `Configs` mounts.
- `launcher/` and `virtualization/creator/` are skeletons that do not compile
  yet; they are on the roadmap and excluded from the build and CI.

[0.5.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.5.0
[0.4.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.4.0
[0.3.1]: https://github.com/crispuscrew/zinc/releases/tag/v0.3.1
[0.3.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.3.0
[0.2.1]: https://github.com/crispuscrew/zinc/releases/tag/v0.2.1
[0.2.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.2.0
[0.1.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.1.0
