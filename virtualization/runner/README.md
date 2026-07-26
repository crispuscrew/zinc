# zvr - the Zinc virtualization runner

`zvr` boots VM apps as qemu guests. It is the sibling of `zcr`: one app store, one schema,
split by `Type`, with each runner taking the apps it owns and refusing the other's by name.

```sh
zvr run <app>            boot a guest (detached; the window, if any, is qemu's)
zvr run <app> --dry-run  print the exact qemu command line, change nothing
zvr stop <app> [--force] shut it down (ACPI power button, or signal the process)
zvr ps                   running guests
zvr status <app>         one guest's state
zvr validate <app>       check a config, run nothing
zvr reset <app>          delete the guest's disk, back to the pinned base
zvr pin <image.qcow2>    print the sha256 pin to put in a config
zvr console <app>        where to attach for the serial console
```

Apps are authored with `zc` (`zc new <name> --vm ...`, or the `tui` form), never here: a
runner that could rewrite a config could quietly change what it is about to run.

## Why qemu directly, and not libvirt

libvirtd spawns the qemu process, so that process is not in your session and cannot open a
window on your compositor. libvirt's answer is SPICE plus a separate viewer, which puts a hop
between the guest's frames and the screen - and for a VM that exists to run something
interactive, that hop is the whole problem.

`zvr` starts qemu as a child of your session instead, so a guest gets a local,
GPU-accelerated window whose frames never leave the machine. It also matches the container
side exactly: build an argv from validated config, exec a binary, and print it with
`--dry-run` before anything runs.

The cost is paid knowingly: **zvr owns supervision.** Starting, finding and stopping guests
is its job, and there are no snapshots or managed save, both of which libvirt would have
provided.

## What a guest gets

Exactly what its config asked for. qemu is started with `-nodefaults`, so no device arrives
merely because it was compiled in, and the host process runs inside qemu's own seccomp jail
(`-sandbox on`, denying privilege elevation, helper spawning and scheduling changes).

**Disks are copy-on-write.** The base image is never opened for writing; each app has its own
overlay backed by it. `zvr reset` deletes that overlay and the app is back to its authored
image - the VM reading of a disposable container.

**The base is pinned.** `VirtualizationMeta.BaseDigest` is the sha256 of the file's bytes,
because a file digest cannot ride inside a path the way a container digest rides inside a
reference. `zvr pin` prints it. The image is hashed in full on first use and then whenever
its identity moves - device, inode, size and both timestamps at nanosecond resolution - which
catches a base that was replaced, rebuilt or restored. It is not a defence against someone
who can already write to your image directory, since they can rewrite the cache beside it.

**Stopping is graceful by default.** `zvr stop` presses the guest's ACPI power button over
QMP and waits, so the guest's own OS flushes and unmounts; `--force` signals the process
instead, for a guest that has stopped answering.

## Display

`VirtualizationMeta.Display` is explicit, never inferred - whether a guest gets an
accelerated window is the difference between a playable game and a slideshow:

| Value | What it does |
| --- | --- |
| `Accelerated` | `virtio-gpu-gl` plus a local window: guest 3D runs on the host GPU and reaches the compositor as a dmabuf |
| `Window` | a local window with no 3D acceleration |
| `None` | headless; the way in is the serial console |

Accelerated 3D needs a guest with the virtio-gpu driver, which in practice means Linux.

## Known limits

- **No egress filtering.** The container network model is nftables inside a container's own
  netns and does not reach a guest, so rather than mis-enforce it a VM app gets user-mode
  networking with explicit `ForwardPorts`, each bound to 127.0.0.1. `NetworkMeta` on a VM app
  is a validation error.
- **No host directory sharing.** That needs virtiofs, which this build does not implement, so
  `Volumes`, `Configs`, `Keys` and `HostTheme` are refused on a VM app.
- **x86_64 guests only.** A foreign architecture would run without KVM and be far too slow
  for interactive use.
- **`zvr console` prints the socket rather than attaching.** This build does not put the
  terminal into raw mode, and a console that mangled keys would be worse than pointing at one
  that works.
- **No snapshots or managed save**, the things libvirt would have given us.

## Try it

```sh
make -C virtualization/runner demo
```

Downloads a Fedora Cloud image once (~600 MB), verifies it against the digest Fedora
publishes - refusing to boot if it does not match - authors a demo VM app with an
accelerated display, and starts it. A qemu window opens on your compositor. To check the
display path is really accelerated rather than falling back to software:

```sh
ssh -p 2222 -o StrictHostKeyChecking=no zinc@127.0.0.1 glxinfo -B
```

Look for `virgl` or `virtio_gpu` in the renderer line; `llvmpipe` means the guest fell back
to software rendering.

## Build

```sh
make build        # reproducible static build -> ./bin/zvr
make check        # gofmt + vet + test in the pinned container
make repro        # prove the build is byte-identical
```

Running guests needs `qemu-system-x86_64`, `qemu-img` and `xorriso` on the host, plus
`/dev/kvm`.
