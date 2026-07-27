# Zinc - Release Plan


| Version | Focus          | Includes              |
|---------|----------------|-----------------------|
| 0.1.0   | Containers     | `zc` mvp + `zcr` mvp  |
| 0.2.0   | Launcher       | `zlt` mvp             |
| 0.3.0   | Launcher       | `zlg` mvp             |
| 0.4.0   | Virtualization | `zvr` mvp (`zc` authors VM apps too) |
| 0.5.0   | Guest GPU      | Vulkan through venus + confirmed virgl |
| 0.6.0   | Windows guests | UEFI + Secure Boot + TPM, `zvr install`, per-app machine identity |
| ...     |                |                       |

**ZDE** (the Zinc Desktop Environment, `zde-niri` / `zde-hypr`) is a separate project
layered on Zinc: it lives in its own repository with its own release plan. Only the Zinc
core and its tools (containers, launchers, virtualization) are released from here.

