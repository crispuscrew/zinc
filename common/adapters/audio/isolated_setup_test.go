package audio

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type isolated struct {
	runtime string
	env     []string
	log     string
	policy  *exec.Cmd
}

func startIsolated(t *testing.T, policy bool) isolated {
	t.Helper()
	assets := os.Getenv("ZINC_AUDIO_TEST_POLICY")
	if assets == "" {
		t.Skip("use integration/wireplumber make isolated; never uses the host audio session")
	}
	for _, binary := range []string{"pipewire", "wireplumber", "dbus-run-session"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	// Unix socket limits require a short runtime path, independently of test names.
	runtime, err := os.MkdirTemp(os.Getenv("TMPDIR"), "pw-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(runtime) })
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	copyFile := func(source, target string) {
		t.Helper()
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		write(target, data)
	}
	copyFile(filepath.Join(assets, "testdata/pipewire.conf"), filepath.Join(root, "pipewire.conf"))
	copyFile("/usr/share/wireplumber/wireplumber.conf", filepath.Join(root, "wireplumber.conf"))
	copyFile(filepath.Join(assets, "testdata/99-isolated.conf"), filepath.Join(root, "wireplumber.conf.d/99-isolated.conf"))
	if policy {
		copyFile(filepath.Join(assets, "90-zinc-audio.conf"), filepath.Join(root, "wireplumber.conf.d/90-zinc-audio.conf"))
		err := filepath.WalkDir(filepath.Join(assets, "scripts"), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(assets, path)
			if err != nil {
				return err
			}
			copyFile(path, filepath.Join(root, "share/wireplumber", relative))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var environment []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "PIPEWIRE_") || strings.HasPrefix(name, "WIREPLUMBER_") || strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "DBUS_") || name == "HOME" {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, "XDG_RUNTIME_DIR="+runtime, "PIPEWIRE_RUNTIME_DIR="+runtime,
		"PIPEWIRE_REMOTE=pipewire-0", "PIPEWIRE_CONFIG_DIR="+root, "WIREPLUMBER_CONFIG_DIR="+root,
		"XDG_CONFIG_HOME="+root, "XDG_DATA_HOME="+filepath.Join(root, "share"), "HOME="+root, "GIO_USE_VFS=local")
	logPath := filepath.Join(root, "session.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		logFile.Close()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Log(string(data))
		}
	})
	start := func(binary string, args ...string) *exec.Cmd {
		t.Helper()
		command := exec.Command(binary, args...)
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		command.Env = environment
		command.Stdout, command.Stderr = logFile, logFile
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { syscall.Kill(-command.Process.Pid, syscall.SIGTERM); command.Wait() })
		return command
	}
	start("pipewire", "-c", filepath.Join(root, "pipewire.conf"))
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(runtime, "pipewire-0-manager")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("private daemon failed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	manager := start("dbus-run-session", "--", "wireplumber", "-c", filepath.Join(root, "wireplumber.conf"))
	time.Sleep(time.Second)
	return isolated{runtime: runtime, env: environment, log: logPath, policy: manager}
}
