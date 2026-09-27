package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	shared "github.com/crispuscrew/zinc/common/adapters/audio"
	audio "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/audioholder"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/disk"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/firmware"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/machine"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

type execution struct {
	Process *machine.Process
	Audio   *audioholder.Handle
	Cleanup func() error
}

func (svc Service) startOnce(ctx context.Context, cfg schema.AppConfig, generation string, diagnostic io.Writer) (result execution, failure error) {
	lock, err := svc.lock(cfg.AppNameID)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	if svc.stopRequested(cfg.AppNameID, generation) {
		return result, fmt.Errorf("launch canceled by manual stop")
	}
	state, err := svc.Runtime.State(cfg.AppNameID)
	if err != nil {
		return result, err
	}
	if state.Alive {
		return result, fmt.Errorf("%s is already running", cfg.AppNameID)
	}
	plan, err := svc.launchPlan(cfg)
	if err != nil {
		return result, err
	}
	if err := svc.freezeNetwork(&plan); err != nil {
		return result, err
	}
	terminal, err := terminalCommand(cfg)
	if err != nil {
		return result, err
	}
	if err := audioholder.CheckALSA(plan.Layout.Audio.ALSA); err != nil {
		return result, err
	}
	for _, media := range svc.Options.InstallMedia {
		info, err := os.Stat(media)
		if err != nil {
			return result, err
		}
		if !info.Mode().IsRegular() {
			return result, fmt.Errorf("media %s is not a regular file", media)
		}
	}
	if err := disk.VerifyBase(cfg.ImageMeta.Image, svc.Options.BaseDigest); err != nil {
		return result, err
	}
	if err := svc.startDependencies(cfg); err != nil {
		return result, err
	}
	if err := disk.EnsureOverlay(cfg.ImageMeta.Image, svc.Options.BaseDigest, plan.Layout.Overlay, svc.Options.DiskSizeGiB); err != nil {
		return result, err
	}
	if plan.Layout.Seed != "" {
		if err := disk.WriteSeed(plan.Layout.Seed, cfg, svc.Options.Devices); err != nil {
			return result, err
		}
	}
	if _, err := firmware.Prepare(cfg.StartConditions, svc.Paths.UEFIVars(cfg.AppNameID), cfg.ImageMeta.Image); err != nil {
		return result, err
	}
	if err := svc.saveManifest(cfg); err != nil {
		return result, err
	}
	var holder *audioholder.Handle
	cleanup := func() error {
		firmware.StopTPM(svc.Paths.TPMSocket(cfg.AppNameID), svc.Paths.TPMPID(cfg.AppNameID))
		if holder != nil {
			return holder.Close()
		}
		return nil
	}
	defer func() {
		if failure != nil {
			failure = errors.Join(failure, cleanup())
		}
	}()
	if cfg.StartConditions.TPM {
		firmware.StopTPM(svc.Paths.TPMSocket(cfg.AppNameID), svc.Paths.TPMPID(cfg.AppNameID))
		if _, err := firmware.StartTPM(svc.Paths.TPMState(cfg.AppNameID), plan.Layout.TPMSocket, svc.Paths.TPMPID(cfg.AppNameID)); err != nil {
			return result, err
		}
	}
	var environment []string
	if cfg.DisplayMeta.Vulkan {
		environment, err = svc.Paths.VenusEnv()
		if err != nil {
			return result, err
		}
	}
	plan.Layout.Audio.Planning = false
	if audio.UsesPipeWire(cfg.AudioMeta) {
		holder, err = audioholder.Start(shared.Request{RuntimeDir: svc.AudioRuntimeDir, AppID: cfg.AppNameID,
			InstanceID: cfg.AppNameID + "-" + generation, Audio: cfg.AudioMeta}, diagnostic)
		if err != nil {
			return result, err
		}
		plan.Layout.Audio = holder.Ready.Audio
		environment = append(environment, holder.Ready.Environment...)
	}
	if err := qemu.Validate(plan.RuntimeConfig, plan.Layout); err != nil {
		return result, err
	}
	argv, ruleset, err := svc.networkCommand(plan, qemu.Args(plan.RuntimeConfig, plan.Layout))
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if svc.stopRequested(cfg.AppNameID, generation) {
		return result, fmt.Errorf("launch canceled by manual stop")
	}
	if holder != nil {
		if err := holder.Check(); err != nil {
			return result, err
		}
	}
	process, err := svc.Runtime.Launch(cfg.AppNameID, argv, environment, ruleset)
	if err != nil {
		return result, err
	}
	if err := startTerminal(cfg, terminal); err != nil {
		return result, errors.Join(err, svc.Runtime.Stop(cfg.AppNameID, false, DefaultStopTimeout))
	}
	if err := removeIfPresent(svc.controlPath(cfg.AppNameID, "fault")); err != nil {
		fmt.Fprintln(diagnostic, "clear stale VM fault:", err)
	}
	return execution{Process: process, Audio: holder, Cleanup: cleanup}, nil
}
