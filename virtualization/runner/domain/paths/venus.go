package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

func (paths Paths) VirglPrefix() string {
	if override := os.Getenv("ZVR_VIRGL_PREFIX"); override != "" {
		return override
	}
	return filepath.Join(filepath.Dir(paths.StateDir), "virgl-venus")
}

func (paths Paths) VenusEnv() ([]string, error) {
	prefix := paths.VirglPrefix()
	library := filepath.Join(prefix, "lib64", "libvirglrenderer.so.1")
	server := filepath.Join(prefix, "libexec", "virgl_render_server")
	for _, path := range []string{library, server} {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("DisplayMeta.Vulkan requires a Venus-capable virglrenderer at %s; use the pinned virtualization/runner virgl-venus Make target or ZVR_VIRGL_PREFIX: %w", path, err)
		}
	}
	return []string{"LD_LIBRARY_PATH=" + filepath.Join(prefix, "lib64"), "RENDER_SERVER_EXEC_PATH=" + server}, nil
}
