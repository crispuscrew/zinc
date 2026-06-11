package config

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// Load reads and decodes an app TOML from disk. It does NOT apply semantic rules —
// call Validate on the result. Unknown keys (typos, stale fields after a hand
// edit) are reported as an error so dead config can't silently accumulate.
func Load(path string) (AppConfig, error) {
	var cfg AppConfig
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return AppConfig{}, fmt.Errorf("config: decode %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("config: %s: unknown key(s): %s", path, joinKeys(undecoded))
	}
	return cfg, nil
}

func joinKeys(keys []toml.Key) string {
	parts := make([]string, len(keys))
	for index, key := range keys {
		parts[index] = key.String()
	}
	return strings.Join(parts, ", ")
}
