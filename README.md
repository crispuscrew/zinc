# Zinc

> Stable, Secure then beautiful

Run Linux apps in rootless Podman containers or QEMU VMs, with shared YAML
definitions, authoring tools, and terminal/Wayland launchers.

[Quickstart](docs/quickstart.md) | [Examples](common/examples/README.md) |
[Architecture](docs/architecture.md) | [Changelog](CHANGELOG.md)

## Tools

| Command | Purpose |
| --- | --- |
| `zc` | Create and edit app definitions (CLI + TUI) |
| `zcr` | Run containers |
| `zvr` | Run VMs |
| `zlt` | Terminal app picker |
| `zlg` | Wayland app picker |

## Get started

Build the creator and container runner using rootless Podman:

```sh
make -C creator build
make -C container/runner build
export PATH="$PWD/creator/bin:$PWD/container/runner/bin:$PATH"
zc init
zc tui
```

Follow the [quickstart](docs/quickstart.md) for image, terminal, and runtime setup.
Networking requires [provisioning](docs/network-provisioning.md); PipeWire audio
requires the [Zinc WirePlumber policy](integration/wireplumber/README.md).

## Documentation

- [Schema and behavior](docs/architecture.md) | [VM support](docs/virtualization.md)
- [Builds, Nix, and checks](docs/build-and-checks.md) | [Contributing](CONTRIBUTING.md)
- [Release plan](RELEASES.md) | [Roadmap](ROADMAP.md)

<details>
<summary>Launcher preview</summary>

![The zlg launcher overlay](docs/media/zlg-launcher.png)

</details>
