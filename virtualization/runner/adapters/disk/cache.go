package disk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func identify(info os.FileInfo) string {
	stat, valid := info.Sys().(*syscall.Stat_t)
	if !valid {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d:%d.%09d:%d.%09d", stat.Dev, stat.Ino, info.Size(),
		stat.Mtim.Sec, stat.Mtim.Nsec, stat.Ctim.Sec, stat.Ctim.Nsec)
}

type sidecar struct {
	Identity string `json:"identity"`
	Digest   string `json:"digest"`
}

func sidecarPath(base string) string {
	return filepath.Join(filepath.Dir(base), "."+filepath.Base(base)+".zinc-digest")
}

func readSidecar(base string) (sidecar, bool) {
	data, err := os.ReadFile(sidecarPath(base))
	if err != nil {
		return sidecar{}, false
	}
	var cached sidecar
	err = json.Unmarshal(data, &cached)
	return cached, err == nil
}

// Cache failure merely costs a rehash. Replace the cache atomically rather than
// following an existing symlink beside a user-selected base image.
func writeSidecar(base string, entry sidecar) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	file, err := os.CreateTemp(filepath.Dir(base), ".digest-*")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr == nil && closeErr == nil {
		_ = os.Rename(file.Name(), sidecarPath(base))
	}
}
