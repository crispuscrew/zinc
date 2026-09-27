package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

const SupervisorCommand = "__supervise"

type LaunchRequest struct {
	Config          schema.AppConfig
	Options         vmoptions.Config
	Paths           paths.Paths
	Generation      string
	AudioRuntimeDir string
}

type LaunchReady struct {
	Error string
	PID   int
}

func (svc Service) Run(cfg schema.AppConfig) error {
	if _, err := svc.preparationBudget(); err != nil {
		return err
	}
	// Perform read-only checks before a helper, directory or audio context exists.
	if _, err := svc.launchPlan(cfg); err != nil {
		return err
	}
	if err := svc.Paths.EnsureDirs(); err != nil {
		return err
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return err
	}
	generation := hex.EncodeToString(entropy[:])
	request := LaunchRequest{cfg, svc.Options, svc.Paths, generation, svc.AudioRuntimeDir}
	log, err := os.OpenFile(svc.Paths.Log(cfg.AppNameID), os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	for _, warning := range qemu.Warnings(cfg, svc.Options, false) {
		fmt.Fprintln(os.Stderr, "warning: "+warning)
	}
	child, err := ipc.Start([]string{SupervisorCommand, cfg.AppNameID, generation}, request, log)
	if err != nil {
		return err
	}
	child.Grace = DefaultStopTimeout + 15*time.Second
	return svc.awaitSupervisor(child)
}
