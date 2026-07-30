package paths

import (
	"path/filepath"
	"regexp"
	"testing"
)

// SplitRuntime is Runtime() read backwards, and the reading is only decidable against the set
// of apps that exist: "media.work" is an instance of "media" only if "media" is an app. The
// consumer is the Wayland security context, where app_id must be the same string for every
// instance and instance_id must not be.
func TestSplitRuntime(t *testing.T) {
	defined := func(name string) bool {
		switch name {
		case "browser", "org.mozilla.firefox", "media.work":
			return true
		}
		return false
	}
	for _, testCase := range []struct {
		name     string
		runtime  string
		wantApp  string
		wantInst string
	}{
		{"an instance of a defined app", "browser.work", "browser", "work"},
		{"a defined app run without an instance", "browser", "browser", ""},
		{"a dotted app name is not a split", "org.mozilla.firefox", "org.mozilla.firefox", ""},
		{"an instance of a dotted app name", "org.mozilla.firefox.work", "org.mozilla.firefox", "work"},
		{"a defined name wins over splitting it", "media.work", "media.work", ""},
		{"an app nothing knows about (run from a file path)", "some.tool", "some.tool", ""},
		{"a trailing separator is not an instance", "browser.", "browser.", ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := SplitRuntime(testCase.runtime, defined)
			if got.App != testCase.wantApp || got.Instance != testCase.wantInst {
				t.Errorf("SplitRuntime(%q) = %+v, want app %q instance %q", testCase.runtime, got, testCase.wantApp, testCase.wantInst)
			}
			// However it splits, the halves have to rebuild the same runtime name - otherwise
			// the instance_id handed to the compositor is not the container's name, and the
			// whole point is that a consumer can cross-check the two.
			if got.Runtime() != testCase.runtime {
				t.Errorf("SplitRuntime(%q).Runtime() = %q; the halves must rebuild the name", testCase.runtime, got.Runtime())
			}
		})
	}
}

// With no authority to ask, the whole name is the app. That is the right answer for a config
// run straight from a file path (a path cannot carry an instance), and a safe one everywhere
// else: it reports an app_id that is too specific rather than inventing an instance.
func TestSplitRuntime_WithoutAStore(t *testing.T) {
	got := SplitRuntime("browser.work", nil)
	if got.App != "browser.work" || got.Instance != "" {
		t.Errorf("SplitRuntime with no predicate = %+v, want the whole name as the app", got)
	}
}

func TestParseAddress_Forms(t *testing.T) {
	for _, testCase := range []struct {
		spec     string
		app      string
		instance string
	}{
		{"firefox", "firefox", ""},
		{"firefox@work", "firefox", "work"},
		{"firefox@work-laptop", "firefox", "work-laptop"},
		{"firefox@w0rk_2", "firefox", "w0rk_2"},
		{"  firefox@work  ", "firefox", "work"}, // typed with a stray space
	} {
		addr, err := ParseAddress(testCase.spec)
		if err != nil {
			t.Errorf("ParseAddress(%q): %v", testCase.spec, err)
			continue
		}
		if addr.App != testCase.app || addr.Instance != testCase.instance {
			t.Errorf("ParseAddress(%q) = %+v, want app=%q instance=%q", testCase.spec, addr, testCase.app, testCase.instance)
		}
	}
}

// The instance half is the only thing this package validates, so what it refuses is the whole
// of its contract. Uppercase and spaces matter most: the address is echoed back to the user
// and joined into a podman object name, and neither survives a space.
func TestParseAddress_Rejects(t *testing.T) {
	for _, spec := range []string{
		"firefox@",            // an @ promising an instance that is not there
		"@work",               // an instance with no app
		"firefox@Work",        // uppercase would not round-trip the address
		"firefox@work laptop", // a space breaks the runtime name
		"firefox@work.local",  // a dot collides with the app/instance separator
		"firefox@-work",       // must start with a letter or digit
	} {
		if addr, err := ParseAddress(spec); err == nil {
			t.Errorf("ParseAddress(%q) = %+v, want an error", spec, addr)
		}
	}
}

// podmanName is podman's own rule, from the error it gives on a bad --name. Runtime() output
// has to satisfy it or the launch fails at container-create time with a message about
// charsets rather than about instances.
var podmanName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func TestRuntime_IsAPodmanLegalName(t *testing.T) {
	for _, spec := range []string{"firefox", "firefox@work", "firefox@work-laptop", "firefox@w_2"} {
		addr, err := ParseAddress(spec)
		if err != nil {
			t.Fatalf("ParseAddress(%q): %v", spec, err)
		}
		if got := addr.Runtime(); !podmanName.MatchString(got) {
			t.Errorf("Runtime() for %q = %q, which podman would refuse", spec, got)
		}
	}
}

// An app authored before instances existed must keep the runtime name it has always had, or
// upgrading Zinc renames every running container out from under whatever is managing it.
func TestRuntime_UninstancedIsUnchanged(t *testing.T) {
	addr, _ := ParseAddress("firefox")
	if got := addr.Runtime(); got != "firefox" {
		t.Errorf("Runtime() for an un-instanced app = %q, want %q", got, "firefox")
	}
}

// Two instances of one app must not resolve to the same runtime name, or stopping one stops
// the other.
func TestRuntime_InstancesAreDistinct(t *testing.T) {
	work, _ := ParseAddress("firefox@work")
	personal, _ := ParseAddress("firefox@personal")
	if work.Runtime() == personal.Runtime() {
		t.Errorf("two instances share a runtime name: %q", work.Runtime())
	}
}

func TestString_RoundTripsTheTypedForm(t *testing.T) {
	for _, spec := range []string{"firefox", "firefox@work"} {
		addr, _ := ParseAddress(spec)
		if got := addr.String(); got != spec {
			t.Errorf("String() = %q, want %q", got, spec)
		}
	}
}

// XDG_STATE_HOME is honoured rather than ~/.local/state being hardcoded: a machine that sets
// it expects everything to follow, and ZDE sets it.
func TestStateDir_HonoursXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	addr, _ := ParseAddress("firefox@work")
	got, err := StateDir(addr)
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if want := filepath.Join("/custom/state", "zinc", "firefox", "work"); got != want {
		t.Errorf("StateDir = %q, want %q", got, want)
	}
}

// Unset falls back to the spec's own default for that variable, not to a Zinc invention.
func TestStateDir_FallsBackToLocalState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/tester")
	addr, _ := ParseAddress("firefox@work")
	got, err := StateDir(addr)
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if want := filepath.Join("/home/tester", ".local", "state", "zinc", "firefox", "work"); got != want {
		t.Errorf("StateDir = %q, want %q", got, want)
	}
}

// The point of instances is that they do not share what they accumulate.
func TestStateDir_IsPerInstance(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	work, _ := ParseAddress("firefox@work")
	personal, _ := ParseAddress("firefox@personal")
	bare, _ := ParseAddress("firefox")

	dirs := map[string]string{}
	for name, addr := range map[string]Address{"work": work, "personal": personal, "bare": bare} {
		dir, err := StateDir(addr)
		if err != nil {
			t.Fatalf("StateDir(%s): %v", name, err)
		}
		if previous, clash := dirs[dir]; clash {
			t.Errorf("%s and %s share a state directory: %s", name, previous, dir)
		}
		dirs[dir] = name
	}
}

// One definition, many instances: the whole reason templating exists is so a per-desk mount
// does not require a per-desk copy of the app config.
func TestExpand_StateIsPerInstance(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	work, _ := ParseAddress("firefox@work")
	personal, _ := ParseAddress("firefox@personal")

	gotWork, err := work.Expand("{state}/profile")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	gotPersonal, _ := personal.Expand("{state}/profile")
	if gotWork == gotPersonal {
		t.Fatalf("two instances expanded to the same host path: %s", gotWork)
	}
	if want := filepath.Join("/custom/state", "zinc", "firefox", "work", "profile"); gotWork != want {
		t.Errorf("Expand = %q, want %q", gotWork, want)
	}
}

// {state} must agree with StateDir, or a mount and `zcr where` would report different homes
// for the same instance.
func TestExpand_StateAgreesWithWhere(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	addr, _ := ParseAddress("firefox@work")
	reported, err := StateDir(addr)
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	mounted, err := addr.Expand("{state}")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if mounted != reported {
		t.Errorf("{state} expands to %q but `zcr where` reports %q", mounted, reported)
	}
}

func TestExpand_AppAndInstance(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	addr, _ := ParseAddress("firefox@work")
	got, err := addr.Expand("/data/{app}/{instance}")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if want := "/data/firefox/work"; got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

// An unknown placeholder must fail rather than reach the filesystem: a directory literally
// named "{desk}" looks like a Zinc bug from the outside, and a mount that was meant to be
// per-instance and silently is not shares what two instances exist in order not to share.
func TestExpand_RejectsUnknownPlaceholder(t *testing.T) {
	addr, _ := ParseAddress("firefox@work")
	if got, err := addr.Expand("{desk}/profile"); err == nil {
		t.Errorf("Expand of an unknown placeholder = %q, want an error", got)
	}
}

// A path with no placeholders is returned untouched - notably NOT cleaned, so an existing
// config's mount cannot change meaning because templating was added.
func TestExpand_LeavesPlainPathsAlone(t *testing.T) {
	addr, _ := ParseAddress("firefox@work")
	for _, path := range []string{"/srv/data/", "/srv/./data"} {
		if got, err := addr.Expand(path); err != nil || got != path {
			t.Errorf("Expand(%q) = %q, %v; want it returned untouched", path, got, err)
		}
	}
}
