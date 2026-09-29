package pipewirectx

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	shared "github.com/crispuscrew/zinc/common/adapters/audio"
	plan "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

type Broker struct{}

var _ ports.AudioBroker = Broker{}

const readyTimeout = 15 * time.Second

// Establish returns empty only when no PipeWire grant exists. Any failure to
// establish the policy is an error; the host session socket is never returned.
func (Broker) Establish(addr paths.Address, cfg schema.AppConfig, opt options.HostOptions) (string, error) {
	if _, err := plan.Build(cfg.AudioMeta); err != nil {
		return "", err
	}
	if !Applies(cfg) {
		return "", nil
	}
	if opt.RuntimeDir == "" {
		return "", fmt.Errorf("audio: no runtime directory")
	}
	request := shared.Request{RuntimeDir: opt.RuntimeDir, AppID: addr.App,
		InstanceID: addr.Runtime(), Audio: cfg.AudioMeta}
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	if len(body) > maxRequest {
		return "", fmt.Errorf("audio: holder request too large")
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	statusRead, statusWrite, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer statusRead.Close()
	defer statusWrite.Close()
	requestRead, requestWrite, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer requestRead.Close()
	defer requestWrite.Close()
	process := exec.Command(self, HoldCommand, addr.String())
	process.ExtraFiles = []*os.File{statusWrite, requestRead}
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	process.Stderr = os.Stderr
	if err := process.Start(); err != nil {
		return "", err
	}
	go process.Wait()
	success := false
	defer func() {
		if !success {
			_ = process.Process.Kill()
		}
	}()
	statusWrite.Close()
	requestRead.Close()
	if err := requestWrite.SetWriteDeadline(time.Now().Add(readyTimeout)); err != nil {
		return "", err
	}
	if _, err := requestWrite.Write(body); err != nil {
		return "", err
	}
	requestWrite.Close()
	socket, err := readStatus(statusRead)
	if err != nil {
		return "", fmt.Errorf("%s: audio preparation: %w", addr, err)
	}
	success = true
	return socket, nil
}
