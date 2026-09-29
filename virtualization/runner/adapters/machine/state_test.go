package machine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
)

func TestUnreadableStateDoesNotMeanStopped(check *testing.T) {
	runtime := Runtime{Paths: paths.Paths{RunDir: check.TempDir()}}
	if state, err := runtime.State("guest"); err != nil || state.Alive {
		check.Fatal(state, err)
	}
	for _, contents := range []string{"not a pid", "1", ""} {
		if err := os.WriteFile(runtime.Paths.PIDFile("guest"), []byte(contents), 0o600); err != nil {
			check.Fatal(err)
		}
		if _, err := runtime.State("guest"); err == nil {
			check.Fatalf("accepted corrupt pid %q", contents)
		}
	}
	if err := os.Remove(runtime.Paths.PIDFile("guest")); err != nil {
		check.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(runtime.Paths.RunDir, "elsewhere"), runtime.Paths.PIDFile("guest")); err != nil {
		check.Fatal(err)
	}
	if _, err := runtime.State("guest"); err == nil {
		check.Fatal("followed pidfile symlink")
	}
}
