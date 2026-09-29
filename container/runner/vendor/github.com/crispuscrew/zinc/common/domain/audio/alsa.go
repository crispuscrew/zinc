package audio

import (
	"fmt"
	"regexp"
)

var pcmNode = regexp.MustCompile(`^/dev/snd/pcmC([0-9]+)D([0-9]+)([pc])$`)
var controlNode = regexp.MustCompile(`^/dev/snd/controlC([0-9]+)$`)

// PCM is a host PCM selection, not guest device passthrough. Controls are auxiliary.
type PCM struct {
	Direction Direction `json:"direction"`
	Path      string    `json:"path"`
	Device    string    `json:"device"`
	Control   string    `json:"control,omitempty"`
}

func parseALSA(direction Direction, paths []string) ([]PCM, error) {
	if direction == Monitor && len(paths) != 0 {
		return nil, fmt.Errorf("audio monitor ALSA: loopback capture semantics are undefined; select a PipeWire sink monitor")
	}
	controls, cards, seen := map[string]string{}, map[string]bool{}, map[string]bool{}
	var devices []PCM
	for _, path := range paths {
		if match := controlNode.FindStringSubmatch(path); match != nil {
			controls[match[1]] = path
			continue
		}
		match := pcmNode.FindStringSubmatch(path)
		if match == nil {
			return nil, fmt.Errorf("audio %s: %q is not an exact ALSA PCM/control node", direction, path)
		}
		if (direction == Playback) != (match[3] == "p") {
			return nil, fmt.Errorf("audio %s: wrong PCM direction: %s", direction, path)
		}
		cards[match[1]] = true
		if !seen[path] {
			devices = append(devices, PCM{Direction: direction, Path: path, Device: "hw:" + match[1] + "," + match[2]})
			seen[path] = true
		}
	}
	for card, path := range controls {
		if !cards[card] {
			return nil, fmt.Errorf("audio %s: control %s has no selected PCM on its card", direction, path)
		}
	}
	for index := range devices {
		match := pcmNode.FindStringSubmatch(devices[index].Path)
		devices[index].Control = controls[match[1]]
	}
	return devices, nil
}
