package qemu

import (
	"strconv"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func diskArgs(cfg schema.AppConfig, layout Layout) []string {
	drive := "file=" + layout.Overlay + ",format=qcow2"
	if cfg.StartConditions.ReadOnlyRootfs {
		drive += ",readonly=on"
	} else {
		drive += ",discard=unmap"
	}
	var args []string
	if layout.Runtime.Devices == vmoptions.DevicesCompatible {
		args = []string{"-drive", drive + ",if=none,id=disk0", "-device", "ahci,id=ahci",
			"-device", "ide-hd,drive=disk0,bus=ahci.0"}
		if layout.Seed != "" {
			args = append(args, "-drive", "file="+layout.Seed+",if=none,id=seed,format=raw,readonly=on",
				"-device", "ide-cd,drive=seed,bus=ahci.1")
		}
	} else {
		args = []string{"-drive", drive + ",if=virtio"}
		if layout.Seed != "" {
			args = append(args, "-drive", "file="+layout.Seed+",if=virtio,format=raw,readonly=on")
		}
	}
	return args
}

func mediaArgs(media []string) []string {
	if len(media) == 0 {
		return nil
	}
	args := []string{"-device", "ahci,id=media"}
	for index, path := range media {
		identifier := "cd" + strconv.Itoa(index)
		args = append(args, "-drive", "file="+path+",if=none,id="+identifier+",format=raw,media=cdrom,readonly=on",
			"-device", "ide-cd,drive="+identifier+",bus=media."+strconv.Itoa(index))
	}
	return args
}
