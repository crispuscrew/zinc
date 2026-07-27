package app

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// The provisioning disc carries two unrelated things to two unrelated guests: a cloud-init
// guest's identity, and zinc-setup.cmd for a guest that has never heard of cloud-init. The
// trap is the second riding on the first's switch. A Windows guest turning cloud-init off is
// not an odd config - it is the sensible one, since nothing in Windows reads it - and if that
// took the disc away it would also take away the only thing Zinc can hand such a guest.
func TestNeedsProvisioningDisc(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		devices  schema.VMDevices
		disabled bool
		want     bool
	}{
		{"a cloud-init guest reads its identity from it", schema.VMDevicesVirtio, false, true},
		{"a virtio guest with cloud-init off needs nothing on it", schema.VMDevicesVirtio, true, false},
		{"a compatible guest gets the setup script", schema.VMDevicesCompatible, false, true},
		{"and keeps it when cloud-init is off", schema.VMDevicesCompatible, true, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			virt := schema.VirtualizationMeta{
				Devices:   testCase.devices,
				CloudInit: schema.CloudInit{Disabled: testCase.disabled},
			}
			if got := needsProvisioningDisc(virt); got != testCase.want {
				t.Errorf("needsProvisioningDisc = %v, want %v", got, testCase.want)
			}
		})
	}
}
