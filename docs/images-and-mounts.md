# Images, bundles and filesystem grants

## Container image trust

- `ImageMeta.Image`: third-party references require `@sha256:<64 hex>`; only
  `localhost/` permits mutable tags. Clean single lines are required for `FROM`/argv.
  Digests identify bytes, not trustworthy publishers/code.
- Fetch approved bases separately: launch uses `--pull never`. `zc image search`
  and `zc image resolve` delegate to `zcr`; resolve prints a canonical pin.
  `ImageMeta.SourceTag` is provenance, not a launch pin.
- `ImageMeta.Install` joins nonempty shell lines with `&&` in **one RUN layer**
  after `FROM <Image>`. Result: `zinc/app-<name>:local`, never implicitly pulled/pushed.
- `zinc.build` fingerprints base/script/ordered `CreatorFlags`. Missing/stale builds
  rebuild on launch; `zcr build APP` forces it. Flags alone build without empty RUN.
- Creator package-manager hints are advisory. Pins do not pin repositories/downloads;
  derived builds may need networking, unlike [vendored tool compilation](build-and-checks.md).

## Config bundles

`Configs`: `{BundlePath, InnerMount, Writable}`; source root
`$XDG_CONFIG_HOME/zinc/apps/<app>/configs/`, shared by instances.
Bundle-relative paths only: no absolute/traversal paths or runtime placeholders.
Read-only unless `Writable` opts in:

```yaml
Configs:
  - BundlePath: settings.json
    InnerMount: /etc/app/settings.json
    Writable: false
```

## Volumes and one-run mounts

| `Volumes` choice | Meaning |
| --- | --- |
| `HostMounted: true` | Absolute `HostMount` bound to `InnerMount`; no automatic home grant |
| Non-host | tmpfs, `nosuid,nodev`; optional `SizeLimited`/positive `SizeLimitMiB`, kernel-enforced |
| `Writable`, `Executable` | Independent opt-ins, neither implied |

Unsized tmpfs uses backend defaults, not unlimited/persistent storage.
See [scratch-space example](../common/examples/apps/attached-shell.yaml).
Repeatable `zcr run APP -v HOST:CONTAINER[:OPTIONS]` adds one-run binds, default
`ro,noexec`, same validation, no YAML edit. Absolute host paths must contain no `..`;
delimiter/whitespace guards prevent option-field shifting.

Typed sources cannot grant `/run/user`, `/proc`, `/sys`, `/dev` to bypass brokers
or device policy; resolution checks actual filesystem paths. Raw flags remain a
warned escape hatch.

## Keys and themes

`Keys`: `Type: SSH`/`GPG`, absolute protected host path, no `~` expansion;
read-only under the app user's `.ssh`/`.gnupg`.
`HostTheme: true` plus `ZINC_THEME_BUNDLE` grants curated read-only `/etc/zinc/theme`,
not whole host config. Desktop integration supplies GTK/Qt config/icons/cursors/fonts;
omitted HostTheme grants nothing.
[VMs](virtualization.md) use copy-on-write disks/explicit cloud-init and reject
volumes, config bundles, private-key mounts and theme sharing.
