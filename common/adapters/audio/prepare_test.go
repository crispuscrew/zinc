package audio

import (
	"context"
	"strings"
	"testing"

	plan "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestNoPipeWireDoesNotNeedDaemon(t *testing.T) {
	session, err := Prepare(context.Background(), Request{Audio: schema.AudioMeta{
		Playback: schema.AudioDevice{ALSADevices: []string{"/dev/snd/pcmC0D0p"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if session.Socket != "" || len(session.ALSA) != 1 {
		t.Fatal(session)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMissingRuntimeCannotFallback(t *testing.T) {
	_, err := Prepare(context.Background(), Request{AppID: "test", InstanceID: "test",
		Audio: schema.AudioMeta{Monitor: schema.AudioDevice{PipeWireDefault: true}}})
	if err == nil {
		t.Fatal("missing runtime silently accepted")
	}
}

func TestEndpointReplyMustCoverEveryGrant(t *testing.T) {
	token := strings.Repeat("a", 32)
	selections := []plan.Selection{{Direction: plan.Playback, Name: "sink"}, {Direction: plan.Monitor, Name: "sink"}}
	endpoints := []Endpoint{{Direction: plan.Playback, Name: "za." + token + ".1", Target: "sink"}}
	if err := checkEndpoints(endpoints, token, selections); err == nil {
		t.Fatal("partial grants accepted")
	}
	endpoints = append(endpoints, Endpoint{Direction: plan.Monitor, Name: "za." + token + ".2", Target: "sink"})
	if err := checkEndpoints(endpoints, token, selections); err != nil {
		t.Fatal(err)
	}
	endpoints[1].Target = "another-sink"
	if err := checkEndpoints(endpoints, token, selections); err == nil {
		t.Fatal("fallback accepted")
	}
}
