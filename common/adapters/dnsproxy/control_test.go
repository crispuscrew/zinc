package dnsproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestControlRejectsUnsafePathsAndPreservesExistingObjects(t *testing.T) {
	parent := privateDirectory(t)
	file := filepath.Join(parent, "existing")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, link, "relative.sock", filepath.Join(parent, "missing", "control.sock")} {
		if listener, err := listenControl(path); err == nil {
			listener.close()
			t.Fatalf("accepted %q", path)
		}
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "keep" {
		t.Fatal("clobbered existing object")
	}
	directoryLink := filepath.Join(parent, "directory-link")
	if err := os.Symlink(parent, directoryLink); err != nil {
		t.Fatal(err)
	}
	if _, err := listenControl(filepath.Join(directoryLink, "control.sock")); err == nil {
		t.Fatal("followed directory symlink")
	}
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := listenControl(filepath.Join(parent, "control.sock")); err == nil {
		t.Fatal("accepted public parent")
	}
}

func TestControlCleanupDoesNotRemoveReplacement(t *testing.T) {
	path := filepath.Join(privateDirectory(t), "control.sock")
	control, err := listenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := control.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "replacement" {
		t.Fatal("removed replacement")
	}
}

func TestReadyRejectsFalseStatusAndBadPermissions(t *testing.T) {
	meta := testMeta(schema.DNSUDP, "127.0.0.1:1")
	for _, mode := range []string{"not-ready", "digest", "listeners", "version", "trailing", "oversize", "permissions"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(privateDirectory(t), "control.sock")
			control, err := listenControl(path)
			if err != nil {
				t.Fatal(err)
			}
			defer control.close()
			actual := status{Version: 1, Ready: true, Digest: configDigest(meta), Addresses: []string{"127.0.0.1:53"}}
			switch mode {
			case "not-ready":
				actual.Ready = false
			case "digest":
				actual.Digest = "wrong"
			case "listeners":
				actual.Addresses = nil
			case "version":
				actual.Version = 2
			case "permissions":
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			}
			encoded, _ := json.Marshal(actual)
			if mode == "trailing" {
				encoded = append(encoded, []byte(" {}")...)
			}
			if mode == "oversize" {
				encoded = make([]byte, 8193)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				connection, err := control.listener.AcceptUnix()
				if err == nil {
					defer connection.Close()
					_ = writeStatus(connection, encoded)
				}
			}()
			if CheckReady(path, meta, []string{"127.0.0.1"}) == nil {
				t.Fatalf("accepted %s", mode)
			}
			control.listener.Close()
			<-done
		})
	}
}
