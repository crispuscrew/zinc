# zc - Zinc app creator

`zc` writes `$XDG_CONFIG_HOME/zinc/apps/<name>.yaml` (default `~/.config`).
Authoring needs no runtime; execution delegates to `zcr` or `zvr`.

## Authoring

```sh
zc tui
zc new shell --image localhost/shell:local --entrypoint /bin/sh --terminal \
  --attached --env 'MODE=primary value' --attached-env 'MODE=attached value'
zc new shell --help
zc init
zc list
zc validate shell --resolved
zc delete shell
```

New definitions refuse existing YAML/VM-options destinations. `zc init` preserves
existing examples unless `--force` is supplied.

- Presentation: `--desc`, `--icon`, `--group` write `LauncherMeta`.
- Lifecycle: `--terminal`, `--attached`, `--attached-entrypoint`,
  `--read-only-rootfs`, `--keep-alive`, `--background`, `--autorestart`.
  Autorestart lives in `StopConditions`; see [runtime semantics](../docs/containers.md).
- Repeat `--env`/`--attached-env` for literal `NAME=VALUE` entries in separate maps;
  no shell expansion, duplicate names rejected.

Raw flags are ordered argv, never shell-expanded/split; `--creator-flags` and
`--runner-flags` accept YAML/JSON arrays. They warn because they can override containment:

```sh
zc new demo --image localhost/demo:local \
  --runner-flag=--label --runner-flag='purpose=two words'
```

- Audio: `playback`, `microphone`, `monitor` each have `--<direction>-default`
  (`PipeWireDefault: true`), repeatable `--<direction>-pipewire` exact names and
  `--<direction>-alsa` paths. PipeWire needs [WirePlumber deployment](../docs/audio.md).
- Network: `--network` takes `NetworkMeta`; alternatively repeat `--interface ID[=MAC]`,
  `--network-rule` and `--dns-resolver` mappings. Authored priority is preserved.
  Empty interfaces mean no NIC; [ordered, default-deny rules](../docs/network-policy.md)
  require [provisioning](../docs/network-provisioning.md), with no host DNS fallback.
- `--volumes`, `--configs`, `--keys`, `--notifications` take canonical YAML/JSON.
  `--dbus-talk`/`--dbus-own` take comma-separated names and imply `KeepUserID`.
  [Shared validation](../docs/schema.md) checks backend support; changing type keeps fields.

## VM options

```sh
zc new guest --vm --image /images/base.qcow2 --base-digest 'sha256:<64-hex>' \
  --memory 4096 --vcpus 2 --disk 40 --display Accelerated \
  --firmware UEFI --ci-user guest --ci-ssh-key /keys/id.pub
```

Replace the pin placeholder with an independently authorized digest. Creation writes
YAML plus `$XDG_CONFIG_HOME/zinc/runtime/vm/<name>.json`, checking image identity in both.
See [field ownership and disk state](../docs/virtualization.md#app-intent-and-external-vm-options).

- Empty display/profile stays automatic. `--mac random` stores a locally administered
  MAC once, granting no traffic. `--forward HOST:GUEST` adds loopback TCP intent and a
  Host -> Self rule/NIC, not outbound access; edit multi-NIC forwards in the options form/file.
- Deleting YAML keeps VM options/disks. VM rename is refused to protect runtime identity.

## Editing and conversion

The TUI edits scalars, env/argv/audio lists and VM media/forward JSON arrays;
advanced YAML edits network/mounts. Stale forms detect other writers; inherited apps
require sparse YAML editing because forms cannot preserve omitted fields.

```sh
zc compose export app -o compose.yaml
zc compose import compose.yaml --dry-run
```

Conversion reports losses: unsupported capabilities/readiness/tunnels, port translation
and bindings, ordered enforcement, raw/attached/audio settings. Imports never infer
raw privilege flags; representable ports retain TCP/UDP/SCTP and host/sibling/any-peer distinctions.

## Runtime and checks

`zc run app` plans; `--exec` launches. `term` opens an Attached container session or
reports the VM console. VM `build`/`logs`/`restart` report limitations; TUI warnings persist.

```sh
make check  # pinned Podman: formatting, vet, unit/process-boundary tests
make build  # bin/zc
```

Builds use `vendor/`; refresh first-party copies after shared API changes.
