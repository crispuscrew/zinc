package firmware

import (
	"os"
	"path/filepath"
	"testing"
)

// An OS installed under UEFI records its boot entry in NVRAM, not only on disk: Windows
// Setup writes a "Windows Boot Manager" entry pointing at bootmgfw.efi. `zvr install`
// leaves those variables beside the disk it produced, and the app's first run must be
// seeded from them - copying the pristine OVMF template instead throws the boot entry away
// and leaves a freshly installed guest at the UEFI shell with no bootable device.
func TestInstalledVars_FoundBesideTheBaseImage(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "win11.qcow2")
	if err := os.WriteFile(base, []byte("disk"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := InstalledVars(base); got != "" {
		t.Errorf("with no variables beside the disk, want empty, got %q", got)
	}

	vars := base + ".uefi-vars.fd"
	if err := os.WriteFile(vars, []byte("nvram"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := InstalledVars(base); got != vars {
		t.Errorf("InstalledVars = %q, want the variables the install left at %q", got, vars)
	}
	if got := InstalledVars(""); got != "" {
		t.Errorf("an empty base image has no variables, got %q", got)
	}
}
