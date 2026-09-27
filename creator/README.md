# zc - Zinc app creator

`zc` authors container and VM definitions in
`$XDG_CONFIG_HOME/zinc/apps/<name>.yaml` (default `~/.config`).
Execution goes to `zcr` for containers and `zvr` for VMs, in both the CLI and TUI.
Authoring works without either runtime installed.

## Authoring

```sh
zc tui
zc new shell --image localhost/shell:local --entrypoint /bin/sh --terminal
zc new shell --help
zc init
zc list
zc validate shell --resolved
zc delete shell
```

New definitions refuse existing destinations, including an existing VM options
file. `zc init` preserves existing examples unless `--force` is supplied.

Presentation lives in `LauncherMeta`: `--desc`, `--icon`, and `--group`.
Lifecycle options include `--terminal`, `--attached`, `--attached-entrypoint`,
`--read-only-rootfs`, `--keep-alive`, `--background`, and `--autorestart`.
Autorestart is stored under `StopConditions`.

The primary and attached environments are separate maps:

```sh
zc new work --image localhost/work:local --entrypoint /bin/sh --terminal \
  --attached --env 'MODE=primary value' --attached-env 'MODE=attached value'
```

Each `--env`/`--attached-env` takes one literal `NAME=VALUE`; repeat the option for
more entries. Values are not shell-expanded. Duplicate names are rejected.

Raw backend flags are **ordered argv arrays**, with a warning whenever nonempty:

```sh
zc new demo --image localhost/demo:local \
  --runner-flag=--label --runner-flag='purpose=two words'
```

`--creator-flags` and `--runner-flags` also accept YAML/JSON string arrays.
No shell splitting or expansion is performed. Raw flags may override Zinc's
isolation, network, device, or lifecycle settings; do not infer the effective
protections solely from the structured fields when raw flags are present.

Audio is directional: each of `playback`, `microphone`, and `monitor` offers
`--<direction>-default`, repeatable `--<direction>-pipewire` exact names, and
repeatable `--<direction>-alsa` device paths. The default flag writes
`PipeWireDefault: true` in that direction's mapping.

`--network` accepts a complete `NetworkMeta` mapping. Alternatively use repeatable
`--interface ID[=MAC]`, `--network-rule` mappings, and `--dns-resolver` mappings.
Rules and resolvers retain their authored priority. Empty `Interfaces` means no
NIC; rules are ordered, first-match, default-deny, with stateful replies.
`Internet` denotes public internet destinations, not host or sibling access.

`--volumes`, `--configs`, `--keys`, and `--notifications` accept their canonical
YAML/JSON structures. `--dbus-talk` and `--dbus-own` take comma-separated bus names
and imply `KeepUserID` when granting a bus. Backend applicability is validated by
the shared validator and runtime; changing the app type does not erase fields.

## VM options

```sh
zc new guest --vm --image /images/base.qcow2 --base-digest sha256:<64-hex> \
  --memory 4096 --vcpus 2 --disk 40 --display Accelerated \
  --firmware UEFI --ci-user guest --ci-ssh-key /keys/id.pub
```

One action creates both the app YAML and
`$XDG_CONFIG_HOME/zinc/runtime/vm/<name>.json`. Image identity is checked in both.
The JSON file holds the base digest, disk size, display/hardware profile, media,
and port forwards. Boot/security flags, resources, display dimensions/Vulkan,
cloud-init, user, public key, and network interfaces remain in the app schema.

An empty runtime display/profile selection stays automatic when editing.
`--mac random` draws and stores a locally administered MAC once; it grants no
traffic. `--forward HOST:GUEST` stores a loopback TCP forward and writes its
explicit Host -> Self rule and NIC, without adding outbound access. Multiple-NIC
forwarding is edited through the VM options form/file.

Deleting an app YAML retains VM options and disks. VM rename is refused because
disk/firmware/runtime identity cannot be moved safely by the creator alone.

## Editing and conversion

The scrolling TUI exposes shared scalar fields, both environment maps, literal
argv arrays, and per-direction audio device lists. Network policy and mounts have
summaries and are editable through the advanced YAML action. VM media/forwards
use explicit JSON arrays in the VM section. Editor reload rebinds every control.
No-op edits preserve selections; stale forms detect changes made by another writer.

Inherited apps require sparse YAML file editing. A decoded form cannot preserve
which fields were omitted, so saving an inherited app from a form is refused.

```sh
zc compose export app -o compose.yaml
zc compose import compose.yaml --dry-run
```

Conversion reports losses. Imports never infer raw privilege flags. Removed
capability/readiness/tunnel settings are reported or rejected. Port translation
and restricted bindings that cannot be represented are dropped explicitly;
representable ports retain TCP/UDP/SCTP and host/sibling/any-peer distinctions.
Exports explain lost ordered network enforcement and raw/attached/audio settings.

## Runtime and checks

`zc run app` prints a plan; `--exec` launches. `term` opens an Attached container
session or reports the VM console. VM `build`/`logs`/`restart` report their runtime
limitations. The TUI retains successful backend warnings.

```sh
make check  # formatting, vet, unit and process-boundary tests
make build  # pinned Podman build to bin/zc
```

Module builds use `vendor/`. After shared API changes, refresh first-party vendor
copies before using the ordinary build/check entry points.
