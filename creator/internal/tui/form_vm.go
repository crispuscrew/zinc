package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func (frm *formModel) vmFields() []formField {
	frm.baseDigest = newInput(frm.vm.BaseDigest, "sha256:... (zvr pin)")
	frm.diskSize = newInput(numText(frm.vm.DiskSizeGiB, 0), "overlay GiB")
	frm.ciKey = newInput(frm.draft.ImageMeta.PublicSSHKeyPath, "public SSH key path")
	media, _ := json.Marshal(frm.vm.InstallMedia)
	forwards, _ := json.Marshal(frm.vm.ForwardPorts)
	frm.vmMedia = newArea(string(media), "JSON string array")
	frm.vmForwards = newArea(string(forwards), "JSON PortForward array")
	return []formField{
		{label: "base digest", kind: kindText, input: &frm.baseDigest},
		{label: "disk (GiB)", kind: kindText, input: &frm.diskSize},
		{label: "display", kind: kindEnum, values: []string{"", "Accelerated", "Window", "Compatible", "None"}, get: func() string { return string(frm.vm.Display) }, set: func(value string) { frm.vm.Display = vmoptions.Display(value) }},
		{label: "devices", kind: kindEnum, values: []string{"", "Virtio", "Compatible"}, get: func() string { return string(frm.vm.Devices) }, set: func(value string) { frm.vm.Devices = vmoptions.Devices(value) }},
		toggle("loader BIOS", func() bool { return frm.draft.StartConditions.LoaderBIOS }, func(value bool) { frm.draft.StartConditions.LoaderBIOS = value }),
		toggle("secure boot", func() bool { return frm.draft.StartConditions.SecureBoot }, func(value bool) { frm.draft.StartConditions.SecureBoot = value }),
		toggle("TPM", func() bool { return frm.draft.StartConditions.TPM }, func(value bool) { frm.draft.StartConditions.TPM = value }),
		toggle("cloud-init", func() bool { return frm.draft.ImageMeta.CloudInit }, func(value bool) { frm.draft.ImageMeta.CloudInit = value }),
		{label: "cloud-init ssh key", kind: kindText, input: &frm.ciKey},
		{label: "VM media (JSON array)", kind: kindMultiline, area: &frm.vmMedia},
		{label: "VM forwards (JSON array)", kind: kindMultiline, area: &frm.vmForwards},
	}
}

func (frm *formModel) options(cfg schema.AppConfig) (*vmoptions.Config, error) {
	options, err := frm.draftOptions(cfg)
	if err != nil || options == nil {
		return options, err
	}
	return options, vmoptions.Validate(*options)
}

func (frm *formModel) draftOptions(cfg schema.AppConfig) (*vmoptions.Config, error) {
	if cfg.Type != schema.ZincVirtualization {
		return nil, nil
	}
	options := frm.vm
	options.AppNameID, options.Image = cfg.AppNameID, cfg.ImageMeta.Image
	options.BaseDigest = strings.TrimSpace(frm.baseDigest.Value())
	var err error
	options.DiskSizeGiB, err = wholeNumber(frm.diskSize.Value())
	if err != nil {
		return nil, fmt.Errorf("disk (GiB): %w", err)
	}
	options.InstallMedia, options.ForwardPorts = nil, nil
	if err := decodeJSON(frm.vmMedia.Value(), &options.InstallMedia); err != nil {
		return nil, fmt.Errorf("VM media: %w", err)
	}
	if err := decodeJSON(frm.vmForwards.Value(), &options.ForwardPorts); err != nil {
		return nil, fmt.Errorf("VM forwards: %w", err)
	}
	return &options, nil
}

func decodeJSON(text string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}

func numText(value, fallback int64) string {
	if value == 0 {
		value = fallback
	}
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func cpuText(value, fallback float64) string {
	if value == 0 {
		value = fallback
	}
	if value == 0 {
		return ""
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func wholeNumber(value string) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("want a non-negative whole number")
	}
	return number, nil
}

func cpuNumber(value string) (float64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0, fmt.Errorf("want a finite, non-negative CPU count")
	}
	return number, nil
}
