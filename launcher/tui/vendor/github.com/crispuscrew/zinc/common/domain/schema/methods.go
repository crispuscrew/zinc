package schema

// IsZero reports whether no session bus access was requested.
func (bus DBusMeta) IsZero() bool { return len(bus.Talk) == 0 && len(bus.Own) == 0 }

// IsZero reports whether this direction grants no audio devices.
func (device AudioDevice) IsZero() bool {
	return !device.PipeWireDefault && len(device.PipeWireDevices) == 0 && len(device.ALSADevices) == 0
}
