package validate

import (
	"path/filepath"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// The VM rules. They are stricter than they strictly need to be in one specific way: a
// field this build does not implement for a VM app is an ERROR rather than something
// quietly ignored. A config whose Capabilities or NetworkLists look configured but do
// nothing is worse than one that refuses to save, because the author believes a boundary
// exists that is not there. This mirrors how the container network model rejects what it
// cannot enforce instead of half-applying it.

// fileDigestRE is a bare sha256 pin: "sha256:" + 64 hex, anchored at both ends. Unlike
// digestRE (which matches the @sha256:... tail of a container reference) this pins a
// FILE, so it stands alone rather than trailing a name.
const digestPrefix = "sha256:"

// checkVirtualization screens a VM app: its hardware sizing, the pinned base disk, the
// display mode, port forwards and cloud-init identity.
func checkVirtualization(cfg schema.AppConfig, add addFunc) {
	virt := cfg.VirtualizationMeta

	checkBaseImage(cfg.ImageMeta.Image, virt.BaseDigest, add)

	if virt.MemoryMiB <= 0 {
		add("VirtualizationMeta.MemoryMiB %d: must be > 0 (a guest has no default RAM to fall back on)", virt.MemoryMiB)
	}
	if virt.VCPUs <= 0 {
		add("VirtualizationMeta.VCPUs %d: must be > 0", virt.VCPUs)
	}
	if virt.DiskSizeGiB < 0 {
		add("VirtualizationMeta.DiskSizeGiB %d: must be >= 0 (0 keeps the base image's size)", virt.DiskSizeGiB)
	}

	switch virt.Display {
	case schema.VMDisplayNone, schema.VMDisplayWindow, schema.VMDisplayAccelerated, schema.VMDisplayCompatible:
	case "":
		add("VirtualizationMeta.Display: must be set (%s, %s, %s or %s)",
			schema.VMDisplayNone, schema.VMDisplayWindow, schema.VMDisplayAccelerated, schema.VMDisplayCompatible)
	default:
		add("VirtualizationMeta.Display %q: must be one of %s, %s, %s, %s",
			virt.Display, schema.VMDisplayNone, schema.VMDisplayWindow, schema.VMDisplayAccelerated, schema.VMDisplayCompatible)
	}

	switch virt.Firmware {
	case schema.VMFirmwareBIOS, schema.VMFirmwareUEFI, "":
	default:
		add("VirtualizationMeta.Firmware %q: must be %s or %s (empty means %s)",
			virt.Firmware, schema.VMFirmwareBIOS, schema.VMFirmwareUEFI, schema.VMFirmwareBIOS)
	}
	if virt.SecureBoot && virt.Firmware != schema.VMFirmwareUEFI {
		// Secure Boot is a UEFI mechanism; there is nothing for it to attach to on BIOS.
		add("VirtualizationMeta.SecureBoot: requires Firmware %s", schema.VMFirmwareUEFI)
	}

	switch virt.Devices {
	case schema.VMDevicesVirtio, schema.VMDevicesCompatible, "":
	default:
		add("VirtualizationMeta.Devices %q: must be %s or %s (empty means %s)",
			virt.Devices, schema.VMDevicesVirtio, schema.VMDevicesCompatible, schema.VMDevicesVirtio)
	}

	for index, media := range virt.InstallMedia {
		checkInstallMedia(index, media, add)
	}

	// Vulkan rides on the accelerated display's virtio-gpu-gl device; there is nothing for
	// venus to attach to in the other modes, so asking for it there would be a setting that
	// silently does nothing.
	if virt.Vulkan && virt.Display != schema.VMDisplayAccelerated {
		add("VirtualizationMeta.Vulkan: requires Display %s (venus rides on that device); this app is %q",
			schema.VMDisplayAccelerated, virt.Display)
	}

	for index, forward := range virt.ForwardPorts {
		checkForward(index, forward, add)
	}
	checkCloudInit(virt.CloudInit, add)
}

// checkBaseImage screens the base disk path and its digest pin. The path is used to open
// a file and as the backing store recorded inside the app's overlay, so it must be
// absolute and free of the metacharacters that would shift a qemu-img argument.
func checkBaseImage(image, digest string, add addFunc) {
	switch {
	case strings.TrimSpace(image) == "":
		add("ImageMeta.Image: must not be empty (a VM app needs a base disk image)")
	case hasUnsafe(image):
		add("ImageMeta.Image %q: must be a single-line path (no whitespace or control characters)", image)
	case !filepath.IsAbs(image):
		// Resolved by whichever process happens to run zvr otherwise: a relative base would
		// mean a different disk depending on the working directory a hotkey inherited.
		add("ImageMeta.Image %q: must be an absolute path for a VM app (a relative base resolves differently depending on where the launcher was started)", image)
	case hasDotDot(image):
		add("ImageMeta.Image %q: must not contain a '..' segment", image)
	}

	switch {
	case strings.TrimSpace(digest) == "":
		add("VirtualizationMeta.BaseDigest: must be set - a VM base image is pinned by the sha256 of its bytes (sha256:<64 hex>), the same rule that digest-pins container images (section 5.5)")
	case !validFileDigest(digest):
		add("VirtualizationMeta.BaseDigest %q: must be sha256:<64 lowercase hex>", digest)
	}
}

// validFileDigest reports a canonical bare "sha256:<64 hex>" pin.
func validFileDigest(digest string) bool {
	rest, found := strings.CutPrefix(digest, digestPrefix)
	if !found || len(rest) != 64 {
		return false
	}
	for _, char := range rest {
		isHex := (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')
		if !isHex {
			return false
		}
	}
	return true
}

// checkInstallMedia screens an ISO path. It is opened and handed to qemu as a drive, so it
// carries the same rules as the base image.
func checkInstallMedia(index int, media string, add addFunc) {
	switch {
	case strings.TrimSpace(media) == "":
		add("VirtualizationMeta.InstallMedia[%d]: must not be empty", index)
	case hasUnsafe(media):
		add("VirtualizationMeta.InstallMedia[%d] %q: must be a single-line path (no whitespace or control characters)", index, media)
	case !filepath.IsAbs(media):
		add("VirtualizationMeta.InstallMedia[%d] %q: must be an absolute path", index, media)
	case hasDotDot(media):
		add("VirtualizationMeta.InstallMedia[%d] %q: must not contain a '..' segment", index, media)
	}
}

func checkForward(index int, forward schema.PortForward, add addFunc) {
	if forward.HostPort < 1 || forward.HostPort > 65535 {
		add("VirtualizationMeta.ForwardPorts[%d].HostPort %d: must be 1-65535", index, forward.HostPort)
	}
	if forward.GuestPort < 1 || forward.GuestPort > 65535 {
		add("VirtualizationMeta.ForwardPorts[%d].GuestPort %d: must be 1-65535", index, forward.GuestPort)
	}
	if forward.HostPort > 0 && forward.HostPort < 1024 {
		// Rootless qemu cannot bind a privileged port, so this would fail at launch with a
		// bind error that says nothing about the config that caused it.
		add("VirtualizationMeta.ForwardPorts[%d].HostPort %d: must be >= 1024 (zvr runs rootless and cannot bind a privileged port)", index, forward.HostPort)
	}
}

// checkCloudInit screens the first-boot identity. The user name lands in the guest's
// user-data document and the key path is read from the host, so both are screened before
// anything is written.
func checkCloudInit(cloudInit schema.CloudInit, add addFunc) {
	if cloudInit.Disabled {
		if cloudInit.UserName != "" || cloudInit.SSHKeyPath != "" {
			add("VirtualizationMeta.CloudInit: Disabled is set, so UserName and SSHKeyPath would be written nowhere - clear them or enable cloud-init")
		}
		return
	}
	if name := cloudInit.UserName; name != "" && !nameRE.MatchString(name) {
		add("VirtualizationMeta.CloudInit.UserName %q: only lowercase [a-z0-9._-] allowed, must start alphanumeric", name)
	}
	switch path := cloudInit.SSHKeyPath; {
	case path == "":
	case hasUnsafe(path):
		add("VirtualizationMeta.CloudInit.SSHKeyPath %q: must be a single-line path (no whitespace or control characters)", path)
	case !filepath.IsAbs(path):
		add("VirtualizationMeta.CloudInit.SSHKeyPath %q: must be an absolute path", path)
	case strings.HasSuffix(path, ".pub"):
	default:
		// Not fatal-by-content (we cannot read the file here, this is pure), but a path that
		// is not a .pub is overwhelmingly a private key, and the seed ISO is guest-readable.
		add("VirtualizationMeta.CloudInit.SSHKeyPath %q: must be a PUBLIC key (a .pub path) - the seed ISO is readable by the guest, so a private key placed here would be handed to it", path)
	}
}

// checkContainerOnlyFields rejects, on a VM app, the fields that only a container can
// honour. Each names what it would take to support it, so the message reads as a
// boundary rather than a mystery.
func checkContainerOnlyFields(cfg schema.AppConfig, add addFunc) {
	unsupported := []struct {
		set   bool
		field string
		why   string
	}{
		{len(cfg.Capabilities) > 0, "Capabilities",
			"Linux capabilities are a container concept; a guest kernel has its own"},
		{len(cfg.NetworkMeta.NetworkLists) > 0, "NetworkMeta.NetworkLists",
			"the egress lock-down is nftables inside a container netns and does not reach a guest; use VirtualizationMeta.ForwardPorts"},
		{len(cfg.Keys) > 0, "Keys",
			"a VM has no host filesystem to mount keys into; use VirtualizationMeta.CloudInit.SSHKeyPath"},
		{len(cfg.Volumes) > 0, "Volumes",
			"sharing a host directory into a guest needs virtiofs, which this build does not implement"},
		{len(cfg.Configs) > 0, "Configs",
			"sharing a host directory into a guest needs virtiofs, which this build does not implement"},
		{cfg.HostTheme, "HostTheme",
			"the theme bundle is a read-only bind mount, which a guest cannot take"},
		{cfg.StartConditions.Multiterminal, "StartConditions.Multiterminal",
			"a guest has one console; attach to it with the serial console instead"},
		{cfg.InternalUserMeta != (schema.InternalUserMeta{}), "InternalUserMeta",
			"the guest owns its own users; use VirtualizationMeta.CloudInit.UserName"},
		{cfg.ResourcesMeta != (schema.ResourcesMeta{}), "ResourcesMeta",
			"a guest is sized by VirtualizationMeta.MemoryMiB and VCPUs, which allocate rather than cap"},
	}
	for _, rule := range unsupported {
		if rule.set {
			add("%s: not supported for a VM app (%s)", rule.field, rule.why)
		}
	}
}

// checkVirtualizationUnset rejects VM fields on a container app, the mirror of the rule
// above: a field that looks configured must never be silently inert.
func checkVirtualizationUnset(cfg schema.AppConfig, add addFunc) {
	if cfg.VirtualizationMeta.IsZero() {
		return
	}
	add("VirtualizationMeta: only applies to a VM app (Type: %s); this app is %s",
		schema.ZincVirtualization, cfg.Type)
}
