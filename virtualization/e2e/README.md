# VM end-to-end tests

Drives the real `zc` and `zvr` binaries against real qemu and asserts the things unit tests
cannot prove: that a guest actually boots, that its first-boot identity reaches it, that a
base image which no longer matches its pin refuses to run, and that a graceful stop is
actually graceful.

```sh
make -C virtualization/e2e e2e
```

Black-box: everything goes through `os/exec`, nothing is imported from the tools under test.
It takes about 30 seconds, most of which is rebuilding the three binaries.

## Not a CI gate

GitHub's runners have no `/dev/kvm`, and an emulated guest would run far past the job
budget, so this is a **local pre-release gate** rather than part of the CI matrix. It skips
cleanly when `qemu-system-x86_64`, `qemu-img`, `xorriso` or `/dev/kvm` is missing.

## What it asserts

| Scenario | The guarantee |
| --- | --- |
| `authoring` | `zc new --vm` writes a config the shared validation accepts, and `zcr` refuses that app by name and points at `zvr` |
| `dry_run_changes_nothing` | the printed command is a real accelerated, sandboxed qemu line, and no disk is created |
| `pin_is_enforced` | a base image altered after authoring stops the launch, and nothing is created |
| `boots_and_is_reachable` | the guest answers on its forwarded port - kernel, user-mode networking and services all up - the base is untouched while it runs, and the forward binds loopback only |
| `graceful_stop` | the guest goes away well inside the fallback timeout, so the ACPI power button worked rather than the SIGTERM behind it |
| `reset_returns_to_the_base` | the overlay is removed, so the next run starts from the authored image |

The guest is cirros (~21 MB, boots in seconds), cached in `~/.cache/zinc-e2e` and **verified
against a pinned digest on every run**, cached copy included: a suite that booted whatever
happened to be at that path would prove nothing about the guest its assertions describe.

The binaries are rebuilt every run rather than only when missing. A binary left from an
earlier commit passes or fails for reasons that have nothing to do with the tree under test,
which is how a stale `zcr` - built before the schema grew VM fields - first surfaced here as
a YAML decode error rather than the refusal the test was looking for.

Config and data go to a temporary home, so your own apps and disks are never touched.
`XDG_RUNTIME_DIR` is deliberately left alone: control sockets live there, and a unix socket
path has 108 bytes to work with, which a temp path under `/tmp` blows straight past.
