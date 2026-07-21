// Package ui is zlg's Wayland front-end: it opens a window, software-renders the picker
// (via internal/render) into a shared-memory buffer, and feeds keyboard input (decoded by
// internal/keymap) into the picker model. It speaks the Wayland wire protocol directly
// through go-wayland - no cgo - so zlg stays a static binary.
//
// This is the one part of zlg that needs a live compositor, so it is kept deliberately
// thin: all the logic it drives (filter, cursor, launch, render) lives in the pure,
// unit-tested packages it calls. The event loop itself is validated by running zlg on a
// real Wayland session.
package ui

import (
	"fmt"

	"github.com/crispuscrew/zinc/launcher/gui/internal/picker"
)

// Runner is the slice of the zcr delegate the UI drives: launch the chosen app and read
// which apps are currently running (for the indicator). Keeping it an interface lets the
// composition root inject the real zcr delegate.
type Runner interface {
	Launch(name string) error
	Running() (map[string]bool, error)
}

// Run opens the picker window over apps, launching the selection through runner. It returns
// the launched app's name (empty if the user cancelled).
//
// TODO(zlg): the Wayland event loop is not wired yet. The pure pieces it will drive - the
// picker model, the keymap decoder, and the software renderer - are complete and tested;
// what remains is the go-wayland plumbing (registry bind, xdg_surface, wl_shm buffer, the
// wl_keyboard loop), which can only be exercised on a live compositor.
func Run(apps []picker.App, runner Runner) (string, error) {
	return "", fmt.Errorf("zlg: the Wayland UI is not wired up yet (use zlt for now)")
}
