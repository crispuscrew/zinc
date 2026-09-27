package audioholder

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	audio "github.com/crispuscrew/zinc/common/domain/audio"
)

type deviceInfo struct {
	mode  os.FileMode
	major uint64
}

func (info deviceInfo) Name() string       { return "pcmC0D0p" }
func (info deviceInfo) Size() int64        { return 0 }
func (info deviceInfo) Mode() os.FileMode  { return info.mode }
func (info deviceInfo) ModTime() time.Time { return time.Time{} }
func (info deviceInfo) IsDir() bool        { return false }
func (info deviceInfo) Sys() any           { return &syscall.Stat_t{Rdev: info.major << 8} }

func TestALSARequiresKernelSoundCharacterDevices(check *testing.T) {
	if !alsaNode(deviceInfo{mode: os.ModeDevice | os.ModeCharDevice, major: 116}) {
		check.Fatal("ALSA node refused")
	}
	for _, info := range []deviceInfo{{mode: 0, major: 116}, {mode: os.ModeDevice | os.ModeCharDevice, major: 1}, {mode: os.ModeSymlink, major: 116}} {
		if alsaNode(info) {
			check.Fatal("non-ALSA node accepted")
		}
	}
	path := filepath.Join(check.TempDir(), "pcmC0D0p")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		check.Fatal(err)
	}
	if CheckALSA([]audio.PCM{{Path: path}}) == nil {
		check.Fatal("regular-file PCM accepted")
	}
}
