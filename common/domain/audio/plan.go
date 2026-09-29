// Package audio describes audio grants without consulting the host.
package audio

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

type Direction string

const (
	Playback     Direction = "playback"
	Microphone   Direction = "microphone"
	Monitor      Direction = "monitor"
	MaxEndpoints           = 32
)

// Selection names a node.name, or the direction's session default at prepare time.
type Selection struct {
	Direction Direction `json:"direction"`
	Name      string    `json:"name,omitempty"`
	Default   bool      `json:"default,omitempty"`
}

type Plan struct {
	PipeWire []Selection `json:"pipewire"`
	ALSA     []PCM       `json:"alsa"`
}

func UsesPipeWire(meta schema.AudioMeta) bool {
	for _, device := range []schema.AudioDevice{meta.Playback, meta.Microphone, meta.Monitor} {
		if device.PipeWireDefault || len(device.PipeWireDevices) != 0 {
			return true
		}
	}
	return false
}

// Build treats the three transport fields as additive, never as fallback order.
func Build(meta schema.AudioMeta) (Plan, error) {
	var plan Plan
	for _, group := range []struct {
		direction Direction
		device    schema.AudioDevice
	}{
		{Playback, meta.Playback}, {Microphone, meta.Microphone}, {Monitor, meta.Monitor},
	} {
		if group.device.PipeWireDefault {
			plan.PipeWire = append(plan.PipeWire, Selection{Direction: group.direction, Default: true})
		}
		seen := map[string]bool{}
		for _, name := range group.device.PipeWireDevices {
			if name == "" || len(name) > 1024 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
				return Plan{}, fmt.Errorf("audio %s: invalid exact node.name %q", group.direction, name)
			}
			if !seen[name] {
				plan.PipeWire = append(plan.PipeWire, Selection{Direction: group.direction, Name: name})
				seen[name] = true
			}
		}
		devices, err := parseALSA(group.direction, group.device.ALSADevices)
		if err != nil {
			return Plan{}, err
		}
		plan.ALSA = append(plan.ALSA, devices...)
	}
	if len(plan.PipeWire)+len(plan.ALSA) > MaxEndpoints {
		return Plan{}, fmt.Errorf("audio: at most %d endpoints per instance", MaxEndpoints)
	}
	return plan, nil
}
