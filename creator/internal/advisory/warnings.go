// Package advisory includes authoring notices that must also be visible in the TUI.
package advisory

import (
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
)

const RawFlags = "Raw backend flags (CreatorFlags/RunnerFlags) are passed as argv, without a shell. They may override Zinc's isolation, networking, devices, and lifecycle settings."

func Warnings(cfg schema.AppConfig) []string {
	warnings := validate.Warnings(cfg)
	if len(cfg.CreatorFlags)+len(cfg.RunnerFlags) > 0 {
		warnings = append(warnings, RawFlags)
	}
	return warnings
}

func Summary(cfg schema.AppConfig) string { return strings.Join(Warnings(cfg), "\nwarning: ") }
