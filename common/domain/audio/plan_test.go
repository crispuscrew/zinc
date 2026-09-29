package audio

import (
	"github.com/crispuscrew/zinc/common/domain/schema"
	"testing"
)

func TestAdditiveGrants(t *testing.T) {
	meta := schema.AudioMeta{Playback: schema.AudioDevice{PipeWireDefault: true,
		PipeWireDevices: []string{"sink.exact", "sink.exact"},
		ALSADevices:     []string{"/dev/snd/controlC2", "/dev/snd/pcmC2D3p"}},
		Monitor: schema.AudioDevice{PipeWireDevices: []string{"sink.monitor"}}}
	result, err := Build(meta)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PipeWire) != 3 || len(result.ALSA) != 1 {
		t.Fatalf("lost additive grants: %+v", result)
	}
	if result.ALSA[0].Device != "hw:2,3" || result.ALSA[0].Control != "/dev/snd/controlC2" {
		t.Fatal(result.ALSA)
	}
}

func TestALSARefusesAmbiguousOrWrongDevices(t *testing.T) {
	for _, path := range []string{"/dev/snd/", "/dev/snd/pcmC0D0c", "/dev/snd/seq", "/dev/snd/controlC1", "hw:0,0"} {
		_, err := Build(schema.AudioMeta{Playback: schema.AudioDevice{ALSADevices: []string{path}}})
		if err == nil {
			t.Errorf("accepted %s", path)
		}
	}
	_, err := Build(schema.AudioMeta{Monitor: schema.AudioDevice{ALSADevices: []string{"/dev/snd/pcmC0D0c"}}})
	if err == nil {
		t.Fatal("accepted undefined ALSA monitor semantics")
	}
}

func TestNoAudioAndExactNames(t *testing.T) {
	if UsesPipeWire(schema.AudioMeta{}) {
		t.Fatal("empty grants need no socket")
	}
	for _, name := range []string{"", "sink\x00extra", "sink\nextra"} {
		if _, err := Build(schema.AudioMeta{Playback: schema.AudioDevice{PipeWireDevices: []string{name}}}); err == nil {
			t.Fatal(name)
		}
	}
	if !UsesPipeWire(schema.AudioMeta{Monitor: schema.AudioDevice{PipeWireDevices: []string{"exact"}}}) {
		t.Fatal("named monitor lost")
	}
}
