package validate

import (
	"regexp"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Only exact control and direction-bearing PCM nodes are grants. Raw MIDI,
// sequencer, hwdep and directory grants do not describe playback/capture policy.
var alsaNodeRE = regexp.MustCompile(`^/dev/snd/(controlC[0-9]+|pcmC[0-9]+D[0-9]+[pc])$`)

func checkAudio(cfg schema.AppConfig, add addFunc) {
	checkAudioDevice("Playback", cfg.AudioMeta.Playback, add)
	checkAudioDevice("Microphone", cfg.AudioMeta.Microphone, add)
	checkAudioDevice("Monitor", cfg.AudioMeta.Monitor, add)
	if len(cfg.AudioMeta.Monitor.ALSADevices) > 0 {
		add("AudioMeta.Monitor.ALSADevices: requires an explicitly resolved ALSA loopback capture route; loopback resolution is not supported, so these nodes cannot be granted as a monitor")
	}
}

func checkAudioDevice(field string, device schema.AudioDevice, add addFunc) {
	// PipeWireDefault and both lists are additive grants, not exclusive forms.
	for index, selector := range device.PipeWireDevices {
		if strings.TrimSpace(selector) == "" || hasControl(selector) || strings.TrimSpace(selector) != selector ||
			strings.ContainsAny(selector, "*?[]{}") || strings.HasPrefix(selector, "~") {
			add("AudioMeta.%s.PipeWireDevices[%d] %q: must be an exact node.name, not an empty, wildcard or regex selector", field, index, selector)
		}
	}
	for index, node := range device.ALSADevices {
		if !alsaNodeRE.MatchString(node) {
			add("AudioMeta.%s.ALSADevices[%d] %q: must be an exact ALSA device node /dev/snd/controlC<number> or /dev/snd/pcmC<number>D<number>[pc]", field, index, node)
			continue
		}
		if strings.HasSuffix(node, "c") && field == "Playback" {
			add("AudioMeta.Playback.ALSADevices[%d] %q: CAPTURE device grants a microphone; move it to Microphone", index, node)
		}
		if strings.HasSuffix(node, "p") && field != "Playback" {
			add("AudioMeta.%s.ALSADevices[%d] %q: PLAYBACK device cannot capture; move it to Playback", field, index, node)
		}
	}
}
