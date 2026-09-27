# VM hardware and installation

Resources, boot security and display intent are shared app fields; disk pins and
hardware profiles belong in [external VM options](virtualization.md). QEMU uses
`-nodefaults` and, except for the Vulkan path, a managed `-sandbox on` baseline.
Raw argv can override typed choices and must be reviewed as such.

## Profiles and display

`ResourcesMeta.MaxRamMiB` must be positive and `MaxCPUCores` a positive integer
for VMs. `Devices: Virtio` uses paravirtual devices; `Compatible` supplies AHCI
storage, an e1000e NIC when networking is declared, and USB input for guests
without virtio drivers. An absent NIC remains absent under either profile.

Runtime `Display` choices:

- `None`: headless display, with serial console support.
- `Window`: unaccelerated virtio GPU and GTK window.
- `Accelerated`: virtio-gpu-gl and local accelerated GTK window.
- `Compatible`: firmware-friendly unaccelerated VGA/bochs display.

Empty Display is automatic: None for Terminal, Compatible for
`DisableGpuAccess`, otherwise Accelerated. Contradictory GPU denial/Vulkan
selections are rejected. Guest acceleration requires suitable guest drivers;
a local window does not mean every guest OS supports accelerated 3D.

`DisplayMeta.Vulkan` enables the Venus/blob path on compatible accelerated
hardware. The QEMU `hostmem` window is address space, not a VRAM cap. Zinc has
no GPU memory quota. Vulkan currently disables QEMU's seccomp sandbox because
the renderer needs a helper process; shared validation emits a warning.

Compatible UEFI display dimensions use validated framebuffer/EDID geometry.
BIOS/plain VGA cannot represent all the same requested modes. A guest without
a display driver generally keeps its boot mode; resizing the host window need
not change guest resolution. Guest-specific ceilings remain possible even when
the emulated mode is valid. Earlier development observed sheared high-resolution
Windows output without a display driver; that is not a universal guest limit
or a promise that every driver supports the accepted geometry.

## Firmware, Secure Boot and TPM

`StartConditions.LoaderBIOS: false` selects UEFI. Secure Boot requires UEFI,
matching trusted firmware/variable stores and SMM support; a firmware that boots
is not by itself evidence that it enforces signatures.

TPM uses swtpm and its control socket. Firmware generations differ: the runner
prefers supported modern OVMF layouts and refuses incompatible variable-store
shapes rather than silently switching an existing machine. Legacy firmware may
enumerate a TPM device without the expected firmware support. Validate the
guest's actual requirements against the chosen firmware build.

Machine UUIDs remain stable for app identity. Provisioned/explicit MACs are
preserved, and missing MACs are assigned through the network contract. Changing
identity can affect guest enrollment and activation. `MinimizeFingerprint`
requests neutral SMBIOS branding and locally administered generated MACs where
applicable; it does not hide virtualization reliably.

## Installing a Windows-class guest

Windows Setup often needs the Compatible device profile or an explicit driver
disc to see storage. Secure Boot and TPM requirements are OS/version-specific;
select the required boot fields and trusted OVMF rather than treating a profile
as a complete Windows preset.

Compatible provisioning media can include `zinc-setup.cmd` to stage virtio-win
drivers. Installing drivers still requires a user/admin action inside the guest;
there is no guest agent to do it silently. Cloud-init media and guest setup
media are distinct use cases, and disabling cloud-init does not imply that all
other setup media disappear.

`zvr install` creates the base disk before an app can pin it. It can use an
existing app's hardware defaults with `--app`; `--resume` is required to write
an existing target. After installation, review the completed disk digest and
place the authorized pin in VM options before normal launch.

Installed NVRAM is not authenticated by the disk hash. Secure Boot refuses
untrusted adoption; TPM-sealed state is not automatically cloned. A disk alone
is not a full machine backup. Keep firmware variables and TPM identity in mind
when planning resets, adoption or migration.
