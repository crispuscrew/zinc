package pipewirectx

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
)

func TestNamedDirectionsRequireBroker(t *testing.T) {
	for _, audio := range []schema.AudioMeta{
		{Playback: schema.AudioDevice{PipeWireDevices: []string{"sink"}}},
		{Microphone: schema.AudioDevice{PipeWireDevices: []string{"source"}}},
		{Monitor: schema.AudioDevice{PipeWireDevices: []string{"sink"}}},
	} {
		if !Applies(schema.AppConfig{AudioMeta: audio}) {
			t.Fatal("named grant ignored")
		}
	}
}

func TestMissingRuntimeIsError(t *testing.T) {
	cfg := schema.AppConfig{AudioMeta: schema.AudioMeta{Playback: schema.AudioDevice{PipeWireDefault: true}}}
	if socket, err := (Broker{}).Establish(paths.Address{App: "test"}, cfg, options.HostOptions{}); err == nil || socket != "" {
		t.Fatal(socket, err)
	}
}

func TestStatusNeverReturnsRawFallback(t *testing.T) {
	for _, line := range []string{"unsupported", "error missing policy", "ok", "ok relative", "ok /socket\nextra"} {
		if _, err := parseStatus(line); err == nil {
			t.Fatal(line)
		}
	}
}

func TestInheritedRequestIsRequiredAndBounded(t *testing.T) {
	for _, body := range []string{"", "invalid", strings.Repeat(" ", maxRequest+1)} {
		if _, err := decodeRequest(strings.NewReader(body)); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}
