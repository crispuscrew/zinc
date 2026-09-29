// Package runner delegates app actions to zcr or zvr according to the resolved config.
// The launchers share app files with the runtimes, without importing their backends.
package runner

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/launcher/common/store"
)

// safeName guards the app argument at the exec boundary, independent of how zcr parses
// its arguments: a name that is empty or begins with '-' is rejected, so a filename- or
// CLI-derived token can never land in `zcr run`'s slot as a flag instead of an app. It
// deliberately allows '/', so `zlt <path>` still reaches zcr's path form.
func safeName(name string) error {
	if name == "" {
		return fmt.Errorf("empty app name")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("app name %q cannot begin with '-'", name)
	}
	// zcr reads an argument that ends in ".yaml" as a filesystem path, resolved against the
	// caller's working directory. A store key with that suffix (the app file "notes.yaml.yaml",
	// listed as "notes.yaml") would therefore run whatever ./notes.yaml happens to be - an
	// arbitrary config that never went through the store - instead of the app the user picked.
	// A real path still reaches zcr's path form, because it carries a separator.
	if !strings.Contains(name, "/") && strings.HasSuffix(name, ".yaml") {
		return fmt.Errorf("app name %q cannot end with '.yaml' (the runtime would read it as a file path; use ./%s for that)", name, name)
	}
	return nil
}

// Match zcr's instance grammar without importing its backend. Dots belong only
// to the app key; the runtime uses a dot to separate the app from its instance.
var instanceAddressRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*@[a-z0-9][a-z0-9_-]*$`)

func appDefinition(name string) (string, bool, error) {
	if err := safeName(name); err != nil {
		return "", false, err
	}
	// A slash selects the runtime's file form, even when that path contains @.
	if strings.Contains(name, "/") {
		return name, false, nil
	}
	base, _, instanced := strings.Cut(name, "@")
	if !instanced {
		return name, false, nil
	}
	if !instanceAddressRE.MatchString(name) {
		return "", false, fmt.Errorf("invalid app instance %q: want app@instance; instance must be lowercase letters, digits, '_' or '-', starting with a letter or digit", name)
	}
	return base, true, nil
}

// appBinary reads the resolved Type, retaining the caller's key/path as the action identity.
func appBinary(name string) (string, error) {
	definition, instanced, err := appDefinition(name)
	if err != nil {
		return "", err
	}
	appStore, err := store.Default()
	if err != nil {
		return "", err
	}
	var config schema.AppConfig
	if strings.Contains(name, "/") {
		config, err = appStore.LoadFileResolved(name)
	} else {
		config, err = appStore.LoadResolved(definition)
	}
	if err != nil {
		return "", err
	}
	switch config.Type {
	case schema.ZincContainer:
		return Binary, nil
	case schema.ZincVirtualization:
		if instanced {
			return "", fmt.Errorf("app %q: VM instances are unsupported", name)
		}
		return VMBinary, nil
	default:
		return "", fmt.Errorf("app %q: unsupported Type %q; want %s or %s", name, config.Type, schema.ZincContainer, schema.ZincVirtualization)
	}
}

// Launch starts the app detached. Only zcr needs --exec; zvr detaches by default.
// Backend arguments (including RunnerFlags) remain the selected runtime's responsibility.
func Launch(name string) error {
	binary, err := appBinary(name)
	if err != nil {
		return err
	}
	args := []string{"run", name}
	if binary == Binary {
		args = append(args, "--exec")
	}
	_, err = capture(binary, args...)
	return err
}

// Stop delegates teardown to the runtime selected by the resolved app Type.
func Stop(name string) error {
	binary, err := appBinary(name)
	if err != nil {
		return err
	}
	_, err = capture(binary, "stop", name)
	return err
}
