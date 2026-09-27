package qemu

import (
	"strings"
	"testing"

	audio "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestAudioUsesOnlyPrivateBrokerEndpoints(check *testing.T) {
	cfg, layout := fixture()
	cfg.AudioMeta = schema.AudioMeta{
		Playback:   schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"host.sink"}},
		Microphone: schema.AudioDevice{PipeWireDevices: []string{"host.microphone"}},
		Monitor:    schema.AudioDevice{PipeWireDevices: []string{"host.sink"}},
	}
	var err error
	layout.Audio, err = PlanAudio(cfg.AudioMeta)
	if err != nil {
		check.Fatal(err)
	}
	if err := Validate(cfg, layout); err != nil {
		check.Fatal(err)
	}
	text := strings.Join(PlanArgs(cfg, layout), " ")
	if strings.Count(text, "-audiodev") != 4 || strings.Contains(text, "host.") {
		check.Fatal(text)
	}
	layout.Audio.Planning = false
	layout.Audio.Socket = "/run/user/1000/za-private/pipewire-0"
	for index := range layout.Audio.Endpoints {
		layout.Audio.Endpoints[index].Name = strings.Replace(layout.Audio.Endpoints[index].Name, "plan", "private", 1)
	}
	if err := Validate(cfg, layout); err != nil {
		check.Fatal(err)
	}
	text = strings.Join(Args(cfg, layout), " ")
	if !strings.Contains(text, "in.name=za.private.3") || !strings.Contains(text, "out.voices=0") {
		check.Fatal(text)
	}
	layout.Audio.Endpoints[0].Name = "host.sink"
	if Validate(cfg, layout) == nil {
		check.Fatal("naked host routing accepted")
	}
}

func TestALSAPlanningIsPureAndAdditive(check *testing.T) {
	cfg, layout := fixture()
	cfg.AudioMeta.Playback = schema.AudioDevice{PipeWireDefault: true, ALSADevices: []string{"/dev/snd/controlC90", "/dev/snd/pcmC90D1p"}}
	cfg.AudioMeta.Microphone.ALSADevices = []string{"/dev/snd/pcmC91D2c"}
	var err error
	layout.Audio, err = PlanAudio(cfg.AudioMeta)
	if err != nil {
		check.Fatal(err)
	}
	if err := Validate(cfg, layout); err != nil {
		check.Fatal(err)
	}
	text := strings.Join(Args(cfg, layout), " ")
	for _, want := range []string{"out.dev=hw:90,,1", "in.dev=hw:91,,2", "out.name=za.plan.0"} {
		if !strings.Contains(text, want) {
			check.Errorf("missing %s: %s", want, text)
		}
	}
	layout.Audio.ALSA[0].Device = "hw:0,0"
	if Validate(cfg, layout) == nil {
		check.Fatal("ALSA substitution accepted")
	}
}

func TestUnpreparedAndExtraAudioRefused(check *testing.T) {
	cfg, layout := fixture()
	cfg.AudioMeta.Microphone.PipeWireDefault = true
	if Validate(cfg, layout) == nil {
		check.Fatal("unprepared audio accepted")
	}
	layout.Audio = AudioLayout{Socket: "/private/pipewire-0", Endpoints: []AudioEndpoint{{audio.Playback, "za.private.output"}}}
	if Validate(cfg, layout) == nil {
		check.Fatal("wrong grant direction accepted")
	}
	layout.Audio.Endpoints[0].Direction = audio.Microphone
	if err := Validate(cfg, layout); err != nil {
		check.Fatal(err)
	}
	text := strings.Join(Args(cfg, layout), " ")
	if !strings.Contains(text, "out.voices=0") {
		check.Fatal("duplex microphone codec granted host output")
	}
}
