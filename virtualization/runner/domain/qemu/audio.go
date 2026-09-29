package qemu

import (
	"fmt"
	"strconv"
	"strings"

	audio "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

type AudioEndpoint struct {
	Direction audio.Direction
	Name      string
}

type AudioLayout struct {
	Socket    string
	Endpoints []AudioEndpoint
	ALSA      []audio.PCM
	Planning  bool
}

// Four codecs keep each HDA controller within its simultaneous stream budget.
const codecsPerController = 4

// PlanAudio never connects to PipeWire or examines host devices. The endpoint
// labels are placeholders; neither host selections nor private session tokens
// are printed as QEMU routing targets.
func PlanAudio(meta schema.AudioMeta) (AudioLayout, error) {
	grants, err := audio.Build(meta)
	if err != nil {
		return AudioLayout{}, err
	}
	result := AudioLayout{ALSA: grants.ALSA, Planning: true}
	for index, selection := range grants.PipeWire {
		result.Endpoints = append(result.Endpoints, AudioEndpoint{selection.Direction, "za.plan." + strconv.Itoa(index)})
	}
	return result, nil
}

func audioArgs(layout AudioLayout) []string {
	var args []string
	index := 0
	appendEndpoint := func(backend string, direction audio.Direction, property, target string) {
		identifier := "audio" + strconv.Itoa(index)
		controller := "sound" + strconv.Itoa(index/codecsPerController)
		if index%codecsPerController == 0 {
			args = append(args, "-device", "intel-hda,id="+controller)
		}
		codec, stream, disabled := "hda-output", "out", "in"
		if direction != audio.Playback {
			codec, stream, disabled = "hda-micro", "in", "out"
		}
		backend += ",id=" + identifier + "," + disabled + ".voices=0," + stream + "." + property + "=" + strings.ReplaceAll(target, ",", ",,")
		args = append(args, "-audiodev", backend,
			"-device", codec+",bus="+controller+".0,cad="+strconv.Itoa(index%codecsPerController)+",audiodev="+identifier)
		index++
	}
	for _, endpoint := range layout.Endpoints {
		appendEndpoint("pipewire", endpoint.Direction, "name", endpoint.Name)
	}
	for _, device := range layout.ALSA {
		appendEndpoint("alsa", device.Direction, "dev", device.Device)
	}
	return args
}

func validateAudio(meta schema.AudioMeta, layout AudioLayout) error {
	grants, err := audio.Build(meta)
	if err != nil {
		return err
	}
	if len(layout.ALSA) != len(grants.ALSA) {
		return fmt.Errorf("QEMU ALSA layout differs from requested devices")
	}
	for index, device := range grants.ALSA {
		if device != layout.ALSA[index] {
			return fmt.Errorf("QEMU ALSA layout differs from requested devices")
		}
	}
	if len(layout.Endpoints) > len(grants.PipeWire) {
		return fmt.Errorf("unrequested private audio endpoint")
	}
	if len(grants.PipeWire) > 0 && len(layout.Endpoints) == 0 {
		return fmt.Errorf("audio requires prepared private broker endpoints, never naked host node routing")
	}
	if len(layout.Endpoints) > 0 && !layout.Planning && layout.Socket == "" {
		return fmt.Errorf("private audio endpoints require the broker socket environment")
	}
	for _, endpoint := range layout.Endpoints {
		if !strings.HasPrefix(endpoint.Name, "za.") || strings.ContainsAny(endpoint.Name, "\x00\r\n") {
			return fmt.Errorf("audio endpoint must be a private broker node")
		}
		requested := false
		for _, grant := range grants.PipeWire {
			requested = requested || grant.Direction == endpoint.Direction
		}
		if !requested {
			return fmt.Errorf("unrequested private audio direction")
		}
		switch endpoint.Direction {
		case audio.Playback, audio.Microphone, audio.Monitor:
		default:
			return fmt.Errorf("invalid broker audio direction")
		}
	}
	for _, grant := range grants.PipeWire {
		present := false
		for _, endpoint := range layout.Endpoints {
			present = present || grant.Direction == endpoint.Direction
		}
		if !present {
			return fmt.Errorf("missing private audio direction")
		}
	}
	return nil
}
