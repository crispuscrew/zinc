// Package pipewirectx owns a detached holder for the shared fail-closed audio broker.
package pipewirectx

import (
	shared "github.com/crispuscrew/zinc/common/adapters/audio"
	plan "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

const SandboxEngine = shared.Engine
const HoldCommand = "__pipewire"

// MicrophoneFlag remains accepted by the CLI; grants arrive exclusively on FD 4.
const MicrophoneFlag = "--microphone"

func Applies(cfg schema.AppConfig) bool { return plan.UsesPipeWire(cfg.AudioMeta) }
