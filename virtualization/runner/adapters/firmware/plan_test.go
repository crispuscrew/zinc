package firmware

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestResolveUEFIIsReadOnlyAndPrepareIsExclusive(t *testing.T) {
	root := t.TempDir()
	code, template := filepath.Join(root, "code.fd"), filepath.Join(root, "template.fd")
	for _, path := range []string{code, template} {
		if err := os.WriteFile(path, make([]byte, 128), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	previous := ovmfSearch
	ovmfSearch = []ovmfBuild{{code, template, "raw", true}}
	t.Cleanup(func() { ovmfSearch = previous })
	target := filepath.Join(root, "absent", "vars.fd")
	start := schema.StartConditions{TPM: true}
	plan, err := Resolve(start, target, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Firmware.CodePath != code || plan.Template != template {
		t.Fatal(plan)
	}
	if _, err := os.Stat(filepath.Dir(target)); !os.IsNotExist(err) {
		t.Fatal("resolve wrote state")
	}
	if _, err := Prepare(start, target, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, make([]byte, 128), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(start, target, ""); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(target)
	if err != nil || !os.SameFile(info, after) {
		t.Fatal("existing NVRAM was replaced")
	}
}
