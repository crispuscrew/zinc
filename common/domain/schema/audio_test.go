package schema

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAudioDeviceStructuredRoundTrip(t *testing.T) {
	for _, device := range []AudioDevice{
		{}, {PipeWireDefault: true}, {PipeWireDevices: []string{"speaker"}},
		{ALSADevices: []string{"/dev/snd/pcmC0D0c"}},
		{PipeWireDefault: true, PipeWireDevices: []string{"speaker"}, ALSADevices: []string{"/dev/snd/controlC0"}},
	} {
		encoded, err := yaml.Marshal(device)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), "PipeWireDefault:") || !strings.Contains(string(encoded), "ALSADevices:") {
			t.Fatalf("audio was not serialized as a structured mapping: %s", encoded)
		}
		var decoded AudioDevice
		decoder := yaml.NewDecoder(bytes.NewReader(encoded))
		decoder.KnownFields(true)
		if err := decoder.Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.PipeWireDefault != device.PipeWireDefault ||
			strings.Join(decoded.PipeWireDevices, ",") != strings.Join(device.PipeWireDevices, ",") ||
			strings.Join(decoded.ALSADevices, ",") != strings.Join(device.ALSADevices, ",") {
			t.Fatalf("round trip changed grants: %+v -> %+v", device, decoded)
		}
		if device.IsZero() != reflect.DeepEqual(device, AudioDevice{}) {
			t.Fatalf("wrong IsZero result for %+v", device)
		}
	}
}

func TestAudioDeviceStrictDecode(t *testing.T) {
	for _, input := range []string{"default", "none", "[/dev/snd/controlC0]", "{PipeWireDefualt: true}", "{Devices: []}"} {
		var device AudioDevice
		decoder := yaml.NewDecoder(strings.NewReader(input))
		decoder.KnownFields(true)
		if err := decoder.Decode(&device); err == nil {
			t.Fatalf("strict decode accepted %q without migration", input)
		}
	}
	var audio AudioMeta
	if err := yaml.Unmarshal([]byte("Playback: {}\nMicrophone: null\n"), &audio); err != nil {
		t.Fatal(err)
	}
	if !audio.Playback.IsZero() || !audio.Microphone.IsZero() || !audio.Monitor.IsZero() {
		t.Fatalf("empty audio granted access: %+v", audio)
	}
}

func TestDBusIsZero(t *testing.T) {
	if !(DBusMeta{}).IsZero() || (DBusMeta{Talk: []string{"org.example.Service"}}).IsZero() ||
		(DBusMeta{Own: []string{"org.example.App"}}).IsZero() {
		t.Fatal("IsZero lost a bus grant")
	}
}
