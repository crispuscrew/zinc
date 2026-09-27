package podman

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func installCfg(install ...string) schema.AppConfig {
	cfg := validCfg()
	cfg.AppNameID = "hollywood"
	cfg.ImageMeta.Install = install
	return cfg
}

func TestRawRunnerArgvBeforeImage(t *testing.T) {
	cfg := validCfg()
	cfg.RunnerFlags = []string{"--privileged", "--env", "VALUE=a b;'$(touch /escape)'", "--future-option=anything", ""}
	args := appArgs(t, cfg, options.HostOptions{}, nil)
	want := append(slices.Clone(cfg.RunnerFlags), cfg.ImageMeta.Image)
	if !slices.Equal(args[len(args)-len(want):], want) {
		t.Fatalf("raw argv changed: %v", args)
	}
	cfg.RunnerFlags = []string{"bad\x00value"}
	if _, err := (Runtime{}).AppRunArgs(cfg, options.HostOptions{}, nil); err == nil {
		t.Fatal("accepted NUL")
	}
}

func TestCreatorArgvAndFingerprint(t *testing.T) {
	cfg := installCfg("apk add htop")
	cfg.CreatorFlags = []string{"--build-arg", "VALUE=a b;$(false)", "--network=host"}
	args := ImageBuildArgs(cfg)
	want := append(slices.Clone(cfg.CreatorFlags), "-")
	if !slices.Equal(args[len(args)-len(want):], want) {
		t.Fatalf("raw build argv changed: %v", args)
	}
	assertContainsSeq(t, args, "--label", derived.BuildLabel+"="+derived.BuildFingerprint(cfg))
	assertContainsSeq(t, args, "-t", derived.DerivedImageRef(cfg.AppNameID))
	original := derived.BuildFingerprint(cfg)
	for _, flags := range [][]string{{"--network=none"}, {"--build-arg VALUE=a b;$(false)", "--network=host"}} {
		changed := cfg
		changed.CreatorFlags = flags
		if derived.BuildFingerprint(changed) == original {
			t.Fatal("changed argv reused stale image")
		}
	}
	app := appArgs(t, cfg, options.HostOptions{}, nil)
	mustNotContain(t, app, "--build-arg")
	assertContains(t, app, derived.DerivedImageRef(cfg.AppNameID))
}

func TestCreatorFlagsWithoutInstallStillBuild(t *testing.T) {
	cfg := validCfg()
	cfg.CreatorFlags = []string{"--label", "custom=value"}
	if !derived.HasInstall(cfg) {
		t.Fatal("CreatorFlags ignored without Install")
	}
	if text := derived.DerivedContainerfile(cfg); strings.Contains(text, "RUN") {
		t.Fatal(text)
	}
	if derived.RunImage(cfg) != derived.DerivedImageRef(cfg.AppNameID) {
		t.Fatal("built image not selected")
	}
}

func TestEntrypointAndEnv(t *testing.T) {
	cfg := validCfg()
	cfg.StartConditions.Entrypoint = `printf '%s\n' "two words" | tee /tmp/log`
	cfg.StartConditions.EntrypointEnv = map[string]string{"ZED": "3", "ALPHA": "1", "MID": "2"}
	args := appArgs(t, cfg, baseOpts(), nil)
	assertContainsSeq(t, args, "--entrypoint", `["/bin/sh","-c"]`)
	if !slices.Equal(args[len(args)-2:], []string{cfg.ImageMeta.Image, cfg.StartConditions.Entrypoint}) {
		t.Fatal(args)
	}
	var values []string
	for index, arg := range args {
		if arg == "-e" {
			values = append(values, args[index+1])
		}
	}
	if !slices.Equal(values[:3], []string{"ALPHA=1", "MID=2", "ZED=3"}) {
		t.Fatal(values)
	}
	if slices.Index(values, "XDG_RUNTIME_DIR=/run/zinc") < 3 {
		t.Fatal(values)
	}
}
