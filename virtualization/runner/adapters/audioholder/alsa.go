package audioholder

import (
	"fmt"
	"os"
	"syscall"

	plan "github.com/crispuscrew/zinc/common/domain/audio"
)

// CheckALSA performs launch-time device verification; planning only parses the
// paths. QEMU's hw:C,D selector must correspond to kernel ALSA character nodes.
func CheckALSA(devices []plan.PCM) error {
	for _, device := range devices {
		for _, path := range []string{device.Path, device.Control} {
			if path == "" {
				continue
			}
			info, err := os.Lstat(path)
			if err != nil {
				return fmt.Errorf("ALSA %s: %w", path, err)
			}
			if !alsaNode(info) {
				return fmt.Errorf("ALSA %s: require a kernel ALSA character device, not a symlink or regular file", path)
			}
		}
	}
	return nil
}

func alsaNode(info os.FileInfo) bool {
	stat, valid := info.Sys().(*syscall.Stat_t)
	if !valid || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	major := (stat.Rdev>>8)&0xfff | (stat.Rdev>>32)&0xfffff000
	return major == 116
}
