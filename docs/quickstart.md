# Quickstart: Zinc v0.11.1

You need a **Linux x86-64 desktop**, `curl`, working rootless Podman, and an installed terminal.

## Install and try Zinc

Paste this into your terminal, without `sudo`:

```sh
curl -fsSLO https://raw.githubusercontent.com/crispuscrew/zinc/main/scr/zinc-net-install.sh &&
sh zinc-net-install.sh
```

The [installer](../scr/zinc-net-install.sh):

- Downloads and verifies Zinc, then installs it in `~/.local/bin`.
- Sets PATH for itself and adds it to your shell's startup configuration.
- Downloads `zinc-hello.yaml` into the current folder, resolves its image SHA,
  and displays the finished file with `cat`.

When it asks **`Run this example? [y/N]`**, type `y` to open the offline Alpine shell.
Press Enter to keep the YAML without running it. Existing YAML files are kept unchanged.

Type `exit` in the new window to close it. Open a new terminal afterward to use
the automatically configured PATH; the original terminal cannot inherit changes from a child script.

For binaries only, use `sh zinc-net-install.sh --no-example`.
See [zinc-hello.yaml](../common/examples/apps/zinc-hello.yaml),
[the schema](../common/domain/schema/schema.go),
[upgrade notes](releases/0.11.0.md#upgrade-requirements), and
[network/audio/VM setup](architecture.md).
