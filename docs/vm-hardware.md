# VM hardware and installation

Resources/boot/display intent live in YAML; pins/profiles in [VM options](virtualization.md).
QEMU uses `-nodefaults`, `-sandbox on` except Vulkan. Raw argv can override typed choices.

## Profiles and display

`ResourcesMeta.MaxRamMiB` must be positive; `MaxCPUCores` a positive integer.
`Devices: Virtio` uses paravirtual hardware; `Compatible` supplies AHCI, USB input
and e1000e only when networking is declared. Neither invents a NIC.

| Runtime `Display` | Hardware |
| --- | --- |
| `None` | Headless, serial supported |
| `Window` | Unaccelerated virtio GPU, GTK |
| `Accelerated` | virtio-gpu-gl, accelerated local GTK |
| `Compatible` | Firmware-friendly unaccelerated VGA/bochs |
| Empty | None for Terminal; else Compatible for `DisableGpuAccess`; else Accelerated |

GPU denial/Vulkan contradictions error. Acceleration requires compatible guest drivers.
`DisplayMeta.Vulkan` selects Venus/blob on compatible accelerated hardware and **disables
QEMU seccomp** for its renderer helper, with a validation warning. `hostmem` is address
space, not VRAM quota; Zinc enforces no GPU memory cap.

Compatible UEFI uses validated framebuffer/EDID geometry; BIOS/plain VGA cannot express
all modes. Driverless guests usually retain boot resolution despite window resizing.
Valid emulated geometry still faces guest/driver ceilings; observed driverless Windows
high-resolution shearing is neither a universal limit nor a driver-support guarantee.

## Firmware, Secure Boot and TPM

`StartConditions.LoaderBIOS: false` selects UEFI. Secure Boot needs UEFI, trusted
matching firmware/variable stores and SMM; boot success does not prove signature enforcement.
TPM needs swtpm/control socket and compatible firmware. Modern supported OVMF is preferred;
incompatible variable-store shapes are refused, never silently substituted. Legacy
firmware may enumerate TPM without required support; check the guest's requirements.

UUIDs stay stable per app; explicit/provisioned MACs persist, omitted MACs use the
network contract. Identity changes can affect enrollment/activation. `MinimizeFingerprint`
requests neutral SMBIOS and locally administered generated MACs, not reliable concealment.

## Installing a Windows-class guest

- Setup may need Compatible devices or a driver disc for storage. Secure Boot/TPM
  depend on OS/version: choose boot fields/trusted OVMF; no profile is a complete preset.
- Compatible media may stage virtio-win via `zinc-setup.cmd`; installation needs guest
  user/admin action, not a guest agent. Disabling cloud-init does not remove other setup media.
- `zvr install` creates a base; `--app` selects app hardware defaults, `--resume` is
  required to write an existing target. Review the completed digest and authorize it
  in VM options before launch.
- Disk hashes do not authenticate installed NVRAM. Secure Boot refuses untrusted adoption;
  TPM-sealed state is not automatically cloned. A disk is not a full machine backup:
  resets/adoption/migration must account for firmware variables and TPM identity.
