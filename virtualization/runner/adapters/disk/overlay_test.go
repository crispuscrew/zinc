package disk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingOverlayIsNeverReplacedOrResized(check *testing.T) {
	base, digest := writeBase(check, []byte("base fixture"))
	root := filepath.Dir(base)
	overlay := filepath.Join(root, "guest.qcow2")
	if err := os.WriteFile(overlay, []byte("irreplaceable guest data"), 0o600); err != nil {
		check.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		check.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$1\" = info ] || exit 99\nif [ \"$5\" = \"$ZINC_TEST_BASE\" ]; then printf '%s' \"$ZINC_BASE_INFO\"; else printf '%s' \"$ZINC_OVERLAY_INFO\"; fi\n"
	if err := os.WriteFile(filepath.Join(bin, "qemu-img"), []byte(script), 0o700); err != nil {
		check.Fatal(err)
	}
	check.Setenv("PATH", bin)
	check.Setenv("ZINC_TEST_BASE", base)
	check.Setenv("ZINC_BASE_INFO", `{"format":"qcow2","virtual-size":1073741824}`)
	info := imageInfo{Format: "qcow2", VirtualSize: 2 << 30, Backing: base, BackingType: "qcow2"}
	setInfo := func() {
		encoded, err := json.Marshal(info)
		if err != nil {
			check.Fatal(err)
		}
		check.Setenv("ZINC_OVERLAY_INFO", string(encoded))
	}
	setInfo()
	if err := EnsureOverlay(base, digest, overlay, 0); err != nil {
		check.Fatal(err)
	}
	if err := EnsureOverlay(base, digest, overlay, 3); err == nil || !strings.Contains(err.Error(), "resize") {
		check.Fatalf("got %v", err)
	}
	other := filepath.Join(root, "other.qcow2")
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		check.Fatal(err)
	}
	info.Backing = other
	setInfo()
	if err := EnsureOverlay(base, digest, overlay, 0); err == nil || !strings.Contains(err.Error(), "rebase") {
		check.Fatalf("got %v", err)
	}
	data, err := os.ReadFile(overlay)
	if err != nil || string(data) != "irreplaceable guest data" {
		check.Fatal("guest data changed")
	}
}
