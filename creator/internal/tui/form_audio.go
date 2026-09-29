package tui

import "github.com/crispuscrew/zinc/common/domain/schema"

func (frm *formModel) audioFields() []formField {
	var fields []formField
	for _, direction := range []struct {
		name   string
		device func(*schema.AppConfig) *schema.AudioDevice
	}{
		{"playback", func(cfg *schema.AppConfig) *schema.AudioDevice { return &cfg.AudioMeta.Playback }},
		{"microphone", func(cfg *schema.AppConfig) *schema.AudioDevice { return &cfg.AudioMeta.Microphone }},
		{"monitor", func(cfg *schema.AppConfig) *schema.AudioDevice { return &cfg.AudioMeta.Monitor }},
	} {
		device := direction.device(&frm.draft)
		prefix := "audio." + direction.name
		fields = append(fields,
			toggle(prefix+".PipeWireDefault", func() bool { return direction.device(&frm.draft).PipeWireDefault }, func(value bool) { direction.device(&frm.draft).PipeWireDefault = value }),
			frm.structured(prefix+".PipeWireDevices", device.PipeWireDevices, func(cfg *schema.AppConfig, value string) error {
				var devices []string
				err := parseStructured(value, &devices)
				direction.device(cfg).PipeWireDevices = devices
				return err
			}),
			frm.structured(prefix+".ALSADevices", device.ALSADevices, func(cfg *schema.AppConfig, value string) error {
				var devices []string
				err := parseStructured(value, &devices)
				direction.device(cfg).ALSADevices = devices
				return err
			}),
		)
	}
	return fields
}
