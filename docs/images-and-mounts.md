# Images, bundles and filesystem grants

## Container image trust

Third-party `ImageMeta.Image` references require `@sha256:` followed by 64 hex
digits. Only `localhost/` references may use mutable tags. A digest identifies
content; it does not establish that its publisher or code is trustworthy.
References must be single clean lines because they enter both `FROM` and argv.

Launch uses `--pull never`; fetch an approved exact base separately. `zc image
search` and `zc image resolve` delegate to `zcr`; the latter prints a canonical
digest reference. `ImageMeta.SourceTag` records provenance, not the launch pin.

`ImageMeta.Install` supplies shell commands for a derived image. Nonempty lines
are joined with `&&` in **one RUN layer** after `FROM <Image>`. The locally tagged
result is `zinc/app-<name>:local`, never implicitly pulled or pushed.

The `zinc.build` label fingerprints the base, install script and ordered
`CreatorFlags`. A missing/stale image rebuilds on launch; `zcr build APP` forces
a rebuild. Raw build flags alone also trigger a build without an empty RUN.
Package-manager hints in the creator are UI help, not constraints.

A pinned base does not pin package repositories or install-time downloads.
Derived app builds can need network access and are not automatically hermetic.
This is different from the [vendored tool binary build](build-and-checks.md).

## Config bundles

`Configs` uses `{BundlePath, InnerMount, Writable}`. Sources live beneath
`$XDG_CONFIG_HOME/zinc/apps/<app>/configs/`; paths are bundle-relative, not host
absolute paths, traversal paths or runtime placeholders. The app's instances
share its authored bundle. Mounts are read-only unless `Writable` opts in.

```yaml
Configs:
  - BundlePath: settings.json
    InnerMount: /etc/app/settings.json
    Writable: false
```

## Volumes and one-run mounts

`Volumes` with `HostMounted: true` and an absolute `HostMount` bind that source
to `InnerMount`. `Writable` and `Executable` select read/write and exec/noexec;
neither permission is implied. There is no automatic home-directory grant.

A non-host volume is tmpfs scratch space with `nosuid,nodev`, independent
writable/executable choices, and optional `SizeLimited`/`SizeLimitMiB`.
The size limit must be positive when enabled and is kernel-enforced. Without
an explicit size, backend defaults apply; it is not unlimited persistent storage.

```yaml
Volumes:
  - InnerMount: /work
    SizeLimited: true
    SizeLimitMiB: 64
    Writable: true
```

`zcr run APP -v HOST:CONTAINER[:OPTIONS]` adds repeatable one-run binds, default
`ro,noexec`. They are validated through the same path as authored volumes and
do not rewrite YAML. Delimiter/whitespace guards prevent mount-option field
shifting; host paths must be absolute and must not contain `..` segments.

Brokered host state (`/run/user`, `/proc`, `/sys`, `/dev`) cannot be granted as
ordinary typed mount sources to bypass display, bus or device policy. Runtime
source resolution also checks the actual filesystem path. Raw backend flags
remain a separate warned escape hatch.

## Keys and themes

`Keys` takes `Type: SSH` or `GPG` with an absolute host path. Each explicit key
mount is read-only under the app user's `.ssh` or `.gnupg` directory. `~` is not
expanded; the same source-path protections as other mounts apply.

When `HostTheme: true` and `ZINC_THEME_BUNDLE` is supplied, the container gets a
curated read-only directory at `/etc/zinc/theme`, not the host's whole config.
Producing GTK/Qt configuration, icons, cursors and fonts in that bundle belongs
to the desktop integration. Omission of HostTheme does not grant it.

VMs use copy-on-write disks and explicit cloud-init, not these bind mounts.
Volumes, config bundles, private-key mounts and theme sharing into guests are
unavailable and rejected. VM base pins, overlays and reset behavior are in
[virtualization.md](virtualization.md).
