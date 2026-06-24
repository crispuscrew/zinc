package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/hyprzinc/core/adapters/netenforce"
	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/core/ports"
)

// fakeRuntime records which apps were started, in order, and tracks a running set.
// StartApp marks its app running, so Running() reflects what has come up so far —
// enough to exercise depends_on ordering without a real podman.
type fakeRuntime struct {
	running map[string]bool
	started []string
}

func newFakeRuntime(alreadyRunning ...string) *fakeRuntime {
	engine := &fakeRuntime{running: map[string]bool{}}
	for _, name := range alreadyRunning {
		engine.running[name] = true
	}
	return engine
}

func (engine *fakeRuntime) AppRunArgs(cfg domain.AppConfig, opt domain.HostOptions, netFlags []string) ([]string, error) {
	return append([]string{"run", "--name", cfg.App.Name}, netFlags...), nil
}
func (engine *fakeRuntime) Exec(ports.Command) error { return nil }
func (engine *fakeRuntime) StartApp(cfg domain.AppConfig, opt domain.HostOptions, runArgs []string, onFail func()) error {
	engine.started = append(engine.started, cfg.App.Name)
	engine.running[cfg.App.Name] = true
	return nil
}
func (engine *fakeRuntime) OpenSession(string, []string, domain.HostOptions, bool) error { return nil }
func (engine *fakeRuntime) Exists(name string) bool                                      { return engine.running[name] }
func (engine *fakeRuntime) Do([]string) error                                            { return nil }
func (engine *fakeRuntime) Running() (map[string]bool, error)                            { return engine.running, nil }
func (engine *fakeRuntime) Logs(string, int) (string, error)                             { return "", nil }

// fakeStore serves app definitions from an in-memory map.
type fakeStore struct{ apps map[string]domain.AppConfig }

func (store fakeStore) Load(name string) (domain.AppConfig, error) {
	cfg, ok := store.apps[name]
	if !ok {
		return domain.AppConfig{}, fmt.Errorf("app %q not found", name)
	}
	return cfg, nil
}
func (store fakeStore) List() ([]string, error)                   { return nil, nil }
func (store fakeStore) Save(domain.AppConfig) error               { return nil }
func (store fakeStore) Delete(string) error                       { return nil }
func (store fakeStore) Exists(name string) bool                   { _, ok := store.apps[name]; return ok }
func (store fakeStore) Path(name string) string                   { return name }
func (store fakeStore) Marshal(domain.AppConfig) ([]byte, error)  { return nil, nil }
func (store fakeStore) LoadFile(string) (domain.AppConfig, error) { return domain.AppConfig{}, nil }

// depApp is a minimal valid (passes domain.Validate), no-network app with the given
// depends_on list. digestPin is defined in plan_test.go (same package).
func depApp(name string, deps ...string) domain.AppConfig {
	return domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App:           domain.App{Name: name, Image: "img" + digestPin},
		Display:       domain.Display{Wayland: domain.WaylandSecurityContext},
		Network:       domain.Network{Mode: domain.NetworkNone},
		Theme:         domain.Theme{Mode: domain.ThemeNone},
		DependsOn:     domain.DependsOn{Containers: deps},
	}
}

// containerApp is a container-mode app sharing target's netns (§6.4).
func containerApp(name, target string, deps ...string) domain.AppConfig {
	cfg := depApp(name, deps...)
	cfg.Network = domain.Network{Mode: domain.NetworkContainer, Target: target}
	return cfg
}

func depSvc(store ports.Store, engine ports.Runtime) Service {
	return New(store, engine, nil, nil, map[string]ports.NetEnforcer{
		domain.NetworkNone:      netenforce.None{},
		domain.NetworkPasta:     netenforce.Pasta{},
		domain.NetworkContainer: netenforce.Container{},
	})
}

// web → vpn → base: each dependency (and its own dependencies) must come up before
// the app that needs it, deepest first.
func TestLaunch_StartsDependenciesDepthFirst(t *testing.T) {
	store := fakeStore{apps: map[string]domain.AppConfig{
		"web":  depApp("web", "vpn"),
		"vpn":  depApp("vpn", "base"),
		"base": depApp("base"),
	}}
	engine := newFakeRuntime()
	if err := depSvc(store, engine).Launch(store.apps["web"], baseOpts()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"base", "vpn", "web"}; !slices.Equal(engine.started, want) {
		t.Fatalf("start order = %v, want %v", engine.started, want)
	}
}

// A dependency that is already running is not started again.
func TestLaunch_SkipsRunningDependency(t *testing.T) {
	store := fakeStore{apps: map[string]domain.AppConfig{
		"web": depApp("web", "vpn"),
		"vpn": depApp("vpn"),
	}}
	engine := newFakeRuntime("vpn") // vpn already up
	if err := depSvc(store, engine).Launch(store.apps["web"], baseOpts()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(engine.started, []string{"web"}) {
		t.Fatalf("only web should start, got %v", engine.started)
	}
}

// A depends_on name with no definition in the store is a clear error, and nothing launches.
func TestLaunch_MissingDependencyErrors(t *testing.T) {
	store := fakeStore{apps: map[string]domain.AppConfig{"web": depApp("web", "ghost")}}
	engine := newFakeRuntime()
	err := depSvc(store, engine).Launch(store.apps["web"], baseOpts())
	if err == nil || !strings.Contains(err.Error(), `depends on "ghost"`) {
		t.Fatalf("expected missing-dependency error, got %v", err)
	}
	if len(engine.started) != 0 {
		t.Fatalf("nothing should start, got %v", engine.started)
	}
}

// a → b → a must be reported, not recursed into forever.
func TestLaunch_DependencyCycleRejected(t *testing.T) {
	store := fakeStore{apps: map[string]domain.AppConfig{
		"a": depApp("a", "b"),
		"b": depApp("b", "a"),
	}}
	engine := newFakeRuntime()
	err := depSvc(store, engine).Launch(store.apps["a"], baseOpts())
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

// A container-mode app whose target is a declared dependency: the target auto-starts
// first, then the app attaches.
func TestLaunch_ContainerTargetAutoStarted(t *testing.T) {
	store := fakeStore{apps: map[string]domain.AppConfig{
		"app": containerApp("app", "vpn", "vpn"),
		"vpn": depApp("vpn"),
	}}
	engine := newFakeRuntime()
	if err := depSvc(store, engine).Launch(store.apps["app"], baseOpts()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"vpn", "app"}; !slices.Equal(engine.started, want) {
		t.Fatalf("start order = %v, want %v", engine.started, want)
	}
}

// Fail-closed: a container-mode target that is neither running nor a declared
// dependency must abort the launch — never attach to a missing netns.
func TestLaunch_ContainerTargetMissingFailsClosed(t *testing.T) {
	store := fakeStore{apps: map[string]domain.AppConfig{"app": containerApp("app", "vpn")}}
	engine := newFakeRuntime()
	err := depSvc(store, engine).Launch(store.apps["app"], baseOpts())
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected fail-closed target error, got %v", err)
	}
	if len(engine.started) != 0 {
		t.Fatalf("app must not start without its target, got %v", engine.started)
	}
}

// A pre-running target with no depends_on entry is accepted (the user manages it).
func TestLaunch_ContainerTargetPreRunningOK(t *testing.T) {
	store := fakeStore{apps: map[string]domain.AppConfig{"app": containerApp("app", "vpn")}}
	engine := newFakeRuntime("vpn")
	if err := depSvc(store, engine).Launch(store.apps["app"], baseOpts()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(engine.started, []string{"app"}) {
		t.Fatalf("only app should start, got %v", engine.started)
	}
}
