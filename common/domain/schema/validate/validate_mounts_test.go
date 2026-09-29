package validate

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestHostMountSecurity(test *testing.T) {
	for _, source := range []string{
		"Downloads", "./data", "~/.ssh", "../secrets", "/home/user/../secrets",
		"/run/user/1000", "/run/user/1000/bus", "/run/user/1000/wayland-0",
		"/proc", "//proc", "/./proc", "/sys/fs/cgroup", "/dev/snd", "/run//user/1000",
		"/home/user/data:rw", "/home/user/data,ro", "/home/user/two words",
	} {
		test.Run(source, func(test *testing.T) {
			cfg := baseCfg()
			cfg.Volumes = []schema.Volume{{HostMounted: true, HostMount: source, InnerMount: "/data"}}
			requireError(test, cfg, "Volumes")
			cfg.Volumes = nil
			cfg.Keys = []schema.Key{{Type: schema.SSH, Path: source}}
			requireError(test, cfg, "Keys")
		})
	}
	for _, source := range []string{"/home/user/Downloads", "/sysroot/home/user", "/proc-data"} {
		cfg := baseCfg()
		cfg.Volumes = []schema.Volume{{HostMounted: true, HostMount: source, InnerMount: "/data"}}
		if err := Validate(cfg); err != nil {
			test.Fatalf("%s: %v", source, err)
		}
	}
}

func TestConfigAndMountShape(test *testing.T) {
	for _, source := range []string{"", "/etc/shadow", "../secret", "app/../../secret", "app:rw", "app,ro", "{runtime}/bus"} {
		cfg := baseCfg()
		cfg.Configs = []schema.ConfigFile{{BundlePath: source, InnerMount: "/etc/app.conf"}}
		requireError(test, cfg, "Configs")
	}
	for _, target := range []string{"", "/data:rw", "/data,ro", "/two words"} {
		cfg := baseCfg()
		cfg.Volumes = []schema.Volume{{InnerMount: target}}
		requireError(test, cfg, "InnerMount")
	}
	cfg := baseCfg()
	cfg.Volumes = []schema.Volume{{InnerMount: "/data", SizeLimited: true}}
	requireError(test, cfg, "SizeLimitMiB")
	cfg.Volumes[0].SizeLimitMiB = 64
	cfg.Configs = []schema.ConfigFile{{BundlePath: "app.conf", InnerMount: "/etc/app.conf"}}
	cfg.Keys = []schema.Key{{Type: schema.SSH, Path: "/home/user/.ssh/id_ed25519"}}
	if err := Validate(cfg); err != nil {
		test.Fatal(err)
	}
	cfg.Keys[0].Type = "Other"
	requireError(test, cfg, "Keys[0].Type")
}
