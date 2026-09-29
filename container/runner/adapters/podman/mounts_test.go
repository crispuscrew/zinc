package podman

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func TestResourceLimitsAndUser(t *testing.T) {
	cfg := validCfg()
	cfg.ResourcesMeta = schema.ResourcesMeta{MaxCPUCores: 0.5, MaxRamMiB: 2048, PIDsLimit: 100}
	cfg.InternalUserMeta = schema.InternalUserMeta{UseNonRootUser: true, NonRootUserName: "app", KeepUserID: true}
	cfg.Keys = []schema.Key{{Type: schema.SSH, Path: "/keys/id_ed25519"}, {Type: schema.GPG, Path: "/keys/pubring.kbx"}}
	args := appArgs(t, cfg, options.HostOptions{}, netNone())
	for _, pair := range [][2]string{{"--cpus", "0.5"}, {"--memory", "2048m"}, {"--pids-limit", "100"}, {"--user", "app"}, {"-v", "/keys/id_ed25519:/home/app/.ssh/id_ed25519:ro"}, {"-v", "/keys/pubring.kbx:/home/app/.gnupg/pubring.kbx:ro"}} {
		assertContainsSeq(t, args, pair[0], pair[1])
	}
	assertContains(t, args, "--userns=keep-id")
	mustNotContain(t, args, "--memory-swap")
	mustNotContain(t, appArgs(t, cfg, options.HostOptions{}, []string{"--pod", "app-pod"}), "--userns=keep-id")
	for _, pair := range []struct {
		cores float64
		text  string
	}{{2, "2"}, {1.25, "1.25"}} {
		cfg.ResourcesMeta.MaxCPUCores = pair.cores
		assertContainsSeq(t, appArgs(t, cfg, options.HostOptions{}, nil), "--cpus", pair.text)
	}
}

func TestConfigsAndVolumes(t *testing.T) {
	cfg := validCfg()
	cfg.AppNameID = "demo.work"
	cfg.Configs = []schema.ConfigFile{{BundlePath: "settings.json", InnerMount: "/etc/app.json"}, {BundlePath: "state.ini", InnerMount: "/etc/state.ini", Writable: true}}
	if _, err := (Runtime{}).AppRunArgs(cfg, options.HostOptions{}, nil); err == nil {
		t.Fatal("config accepted without bundle")
	}
	cfg.Volumes = []schema.Volume{
		{InnerMount: "/data", Writable: true, SizeLimited: true, SizeLimitMiB: 256},
		{InnerMount: "/readonly"}, {InnerMount: "/scratch", Writable: true, Executable: true},
		{HostMounted: true, HostMount: "/host", InnerMount: "/work", Writable: true},
	}
	cfg.HostTheme = true
	opt := baseOpts()
	opt.BundleDir = "/apps/demo/configs"
	args := appArgs(t, cfg, opt, nil)
	for _, mount := range []string{"/apps/demo/configs/settings.json:/etc/app.json:ro,noexec", "/apps/demo/configs/state.ini:/etc/state.ini:rw,noexec", "/host:/work:rw,noexec", "/theme:/etc/zinc/theme:ro"} {
		assertContainsSeq(t, args, "-v", mount)
	}
	for _, mount := range []string{"type=tmpfs,destination=/data,nosuid,nodev,noexec,tmpfs-size=256m", "type=tmpfs,destination=/readonly,nosuid,nodev,ro,noexec", "type=tmpfs,destination=/scratch,nosuid,nodev"} {
		assertContainsSeq(t, args, "--mount", mount)
	}
}
