// Package paths decides two things a container app needs agreed in exactly one place: what
// an instance of an app is called, and where that instance keeps its state.
//
// Both exist because one app definition can be run more than once - a browser for work and
// one for personal are the same config and must not be the same running thing. The address
// "app@instance" is what a person types; the runtime name and the state directory are
// derived from it, here, so `zcr run` creates what `zcr stop` removes and `zcr where`
// reports.
//
// It is pure string work over the environment, so the layout is unit-testable and no caller
// has to reimplement it. That last part is the point: a desktop that hardcodes the layout
// becomes a second source of truth the moment either side changes, which is why `zcr where`
// exists rather than a documented constant.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// instanceRE is the charset for an instance name. Narrower than an app name on purpose: no
// dots, because the runtime name joins app and instance with one, and no uppercase, so the
// address a person types is the address they get back.
var instanceRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Separator joins app and instance into a runtime object name. Not "@", which is what the
// address uses, because podman rejects it:
//
//	names must match [a-zA-Z0-9][a-zA-Z0-9_.-]*
//
// So "@" is the human form and "." is the runtime form, and this package is the only place
// that knows both.
const Separator = "."

// Address identifies one running thing: an app definition, and optionally which instance of
// it. A zero Instance means the un-instanced app, which is what every config authored before
// instances existed resolves to - so those keep their current runtime name and nothing
// already running is renamed out from under itself.
type Address struct {
	App      string
	Instance string
}

// ParseAddress reads "app" or "app@instance". The app half is returned unvalidated, because
// what makes an app name legal belongs to the schema validator and duplicating it here would
// give two answers that can disagree; the instance half is validated, because nothing else
// will.
func ParseAddress(spec string) (Address, error) {
	app, instance, found := strings.Cut(strings.TrimSpace(spec), "@")
	if !found {
		return Address{App: app}, nil
	}
	switch {
	case app == "":
		return Address{}, fmt.Errorf("%q: an instance needs an app before the @", spec)
	case instance == "":
		return Address{}, fmt.Errorf("%q: the @ is there but no instance follows it; drop the @ to address the app itself", spec)
	case !instanceRE.MatchString(instance):
		return Address{}, fmt.Errorf("%q: instance %q must be lowercase letters, digits, '_' or '-', starting with a letter or digit", spec, instance)
	}
	return Address{App: app, Instance: instance}, nil
}

// String renders the address back in the form a person types.
func (addr Address) String() string {
	if addr.Instance == "" {
		return addr.App
	}
	return addr.App + "@" + addr.Instance
}

// Runtime is the name podman objects take: the container, its pod, and anything named after
// it. An un-instanced app keeps the bare app name it has always had.
func (addr Address) Runtime() string {
	if addr.Instance == "" {
		return addr.App
	}
	return addr.App + Separator + addr.Instance
}

// StateDir is where this instance's own files live, under $XDG_STATE_HOME (falling back to
// ~/.local/state, which is what the XDG spec says that variable defaults to). State rather
// than data because it is what the app accumulates by running - reproducible only in the
// sense that deleting it resets the instance.
//
// Per instance, not per app: the whole reason to have two instances is that they do not
// share what they accumulate.
func StateDir(addr Address) (string, error) {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(stateHome, "zinc", addr.App)
	if addr.Instance != "" {
		dir = filepath.Join(dir, addr.Instance)
	}
	return dir, nil
}
