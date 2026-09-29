package qemu

import (
	"crypto/sha256"
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func identityArgs(identity string, minimize bool) []string {
	sum := sha256.Sum256([]byte("zinc/vm/" + identity))
	sum[6] = (sum[6] & 0x0f) | 0x40
	sum[8] = (sum[8] & 0x3f) | 0x80
	args := []string{"-uuid", fmt.Sprintf("%x-%x-%x-%x-%x", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])}
	if minimize {
		args = append(args, "-smbios", "type=1,manufacturer=Generic,product=Computer,version=1")
	}
	return args
}

func macFor(identity, override string, minimize bool) string {
	if override != "" {
		return override
	}
	sum := sha256.Sum256([]byte("zinc/vm/mac/" + identity))
	if minimize {
		return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x", sum[0], sum[1], sum[2], sum[3], sum[4])
	}
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", sum[0], sum[1], sum[2])
}

func machineType(start schema.StartConditions) string {
	if start.SecureBoot {
		return "q35,accel=kvm,smm=on"
	}
	return "q35,accel=kvm"
}

func secureBootArgs(start schema.StartConditions) []string {
	if !start.SecureBoot {
		return nil
	}
	return []string{"-global", "driver=cfi.pflash01,property=secure,value=on", "-global", "ICH9-LPC.disable_s3=1"}
}

func firmwareArgs(firmware Firmware) []string {
	if firmware.CodePath == "" {
		return nil
	}
	format := firmware.Format
	if format == "" {
		format = "raw"
	}
	return []string{
		"-drive", "if=pflash,format=" + format + ",unit=0,readonly=on,file=" + firmware.CodePath,
		"-drive", "if=pflash,format=" + format + ",unit=1,file=" + firmware.VarsPath}
}

func tpmArgs(socket string) []string {
	if socket == "" {
		return nil
	}
	return []string{"-chardev", "socket,id=chrtpm,path=" + socket,
		"-tpmdev", "emulator,id=tpm0,chardev=chrtpm", "-device", "tpm-tis,tpmdev=tpm0"}
}

func rtcArgs(devices vmoptions.Devices) []string {
	if devices == vmoptions.DevicesCompatible {
		return []string{"-rtc", "base=localtime"}
	}
	return []string{"-rtc", "base=utc"}
}
