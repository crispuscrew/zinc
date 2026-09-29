package network

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func TestPublicationMatchesProtocolInterfaceAndHostBinding(t *testing.T) {
	_, manifest := manifestFixture()
	manifest.Publications = []Publication{{Protocol: "TCP", BindAddress: "127.0.0.1", HostPort: 2222, GuestPort: 22, InterfaceID: "main"}}
	forward := vmoptions.PortForward{HostPort: 2222, GuestPort: 22}
	if err := CheckForwards(manifest, []vmoptions.PortForward{forward}); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*vmoptions.PortForward){
		func(value *vmoptions.PortForward) { value.BindAddress = "0.0.0.0" },
		func(value *vmoptions.PortForward) { value.Protocol = "UDP" },
		func(value *vmoptions.PortForward) { value.Interface = "other" },
		func(value *vmoptions.PortForward) { value.GuestPort = 80 },
	} {
		changed := forward
		mutate(&changed)
		if err := CheckForwards(manifest, []vmoptions.PortForward{changed}); err == nil {
			t.Fatal("publication scope widened")
		}
	}
	if err := CheckForwards(manifest, nil); err == nil {
		t.Fatal("unrequested publication ignored")
	}
}
