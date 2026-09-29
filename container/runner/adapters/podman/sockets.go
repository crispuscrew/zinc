package podman

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/adapters/dbusproxy"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func socketArgs(cfg schema.AppConfig, opt options.HostOptions) ([]string, error) {
	var args []string
	runtimeMounted := false
	if opt.RuntimeDir != "" && opt.WaylandDisplay != "" {
		socket, mode := filepath.Join(opt.RuntimeDir, opt.WaylandDisplay), "passthrough"
		if !cfg.DisplayMeta.DisableSecurityContext && opt.WaylandSocket != "" {
			socket, mode = opt.WaylandSocket, "security-context"
		}
		args = append(args, "-v", socket+":"+filepath.Join(ctrXDGRuntime, opt.WaylandDisplay)+":ro",
			"-e", "WAYLAND_DISPLAY="+opt.WaylandDisplay, "--label", "zinc.wayland="+mode)
		runtimeMounted = true
	}
	if opt.NotifySocket != "" {
		args = append(args, "-v", opt.NotifySocket+":"+dbusproxy.ContainerSocket+":rw",
			"-e", "DBUS_SESSION_BUS_ADDRESS=unix:path="+dbusproxy.ContainerSocket)
	}
	if audioUsesSession(cfg.AudioMeta) {
		if opt.PipeWireSocket == "" {
			return nil, fmt.Errorf("%s: PipeWire audio requires a restricted per-app socket", cfg.AppNameID)
		}
		args = append(args, "-v", opt.PipeWireSocket+":"+filepath.Join(ctrXDGRuntime, "pipewire-0")+":ro")
		runtimeMounted = true
	}
	if runtimeMounted {
		args = append(args, "-e", "XDG_RUNTIME_DIR="+ctrXDGRuntime)
	}
	if devices := audioDevices(cfg.AudioMeta); len(devices) > 0 {
		for _, device := range devices {
			args = append(args, "--device", device)
		}
		args = append(args, "--group-add", "keep-groups")
	}
	return args, nil
}

func audioDevices(audio schema.AudioMeta) []string {
	var devices []string
	for _, list := range [][]string{audio.Playback.ALSADevices, audio.Microphone.ALSADevices} {
		for _, device := range list {
			if !slices.Contains(devices, device) {
				devices = append(devices, device)
			}
		}
	}
	return devices
}

func audioUsesSession(audio schema.AudioMeta) bool {
	for _, device := range []schema.AudioDevice{audio.Playback, audio.Microphone, audio.Monitor} {
		if device.PipeWireDefault || len(device.PipeWireDevices) > 0 {
			return true
		}
	}
	return false
}
