package tui

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

func (frm *formModel) text(label string, input *textinput.Model, value string, apply func(*schema.AppConfig, string) error) formField {
	*input = newInput(value, "")
	frm.bindings = append(frm.bindings, configBinding{label, input.Value(), func() string { return input.Value() }, apply})
	return formField{label: label, kind: kindText, input: input}
}

func (frm *formModel) scalar(label, value string, apply func(*schema.AppConfig, string) error) formField {
	return frm.text(label, new(textinput.Model), value, apply)
}

func (frm *formModel) multiline(label string, area *textarea.Model, value string, apply func(*schema.AppConfig, string) error) formField {
	*area = newArea(value, "")
	frm.bindings = append(frm.bindings, configBinding{label, area.Value(), func() string { return area.Value() }, apply})
	return formField{label: label, kind: kindMultiline, area: area}
}

func (frm *formModel) structured(label string, value any, apply func(*schema.AppConfig, string) error) formField {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		frm.err = fmt.Errorf("%s: %w", label, err)
	}
	return frm.multiline(label, new(textarea.Model), strings.TrimSpace(string(encoded)), apply)
}

func toggle(label string, get func() bool, set func(bool)) formField {
	return formField{label: label, kind: kindBool, bget: get, bset: set}
}

func parseStructured(value string, target any) error {
	decoder := yaml.NewDecoder(bytes.NewBufferString(value))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one YAML value, got extra data: %v", err)
	}
	return nil
}

func (frm *formModel) toConfig() schema.AppConfig {
	cfg := frm.draft
	frm.err = nil
	for _, binding := range frm.bindings {
		value := binding.value()
		if value == binding.initial {
			continue
		}
		if err := binding.apply(&cfg, value); err != nil {
			frm.err = fmt.Errorf("%s: %w", binding.label, err)
			return cfg
		}
	}
	if frm.creating {
		cfg.AppNameID = strings.TrimSpace(frm.name.Value())
	}
	if frm.ciKey.Value() != frm.draft.ImageMeta.PublicSSHKeyPath {
		cfg.ImageMeta.PublicSSHKeyPath = frm.ciKey.Value()
	}
	// Only a newly edited bus grant implies keep-id; a no-op edit never changes data.
	if !cfg.DBusMeta.IsZero() && (frm.dbusTalk.Value() != strings.Join(frm.draft.DBusMeta.Talk, ", ") || frm.dbusOwn.Value() != strings.Join(frm.draft.DBusMeta.Own, ", ")) {
		cfg.InternalUserMeta.KeepUserID = true
	}
	return cfg
}

func splitCommas(text string) []string {
	var result []string
	for _, value := range strings.Split(text, ",") {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func splitLines(text string) []string {
	var result []string
	for _, value := range strings.Split(text, "\n") {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
