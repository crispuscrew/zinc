package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/adapters/netenforce"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

// fakeRuntime records which apps were started, in order, and tracks a running set.
// StartApp marks its app running, so Running() reflects what has come up so far -
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

func (engine *fakeRuntime) AppRunArgs(cfg schema.AppConfig, opt options.HostOptions, netFlags []string) ([]string, error) {
	return append([]string{"run", "--name", cfg.AppNameID}, netFlags...), nil
}
func (engine *fakeRuntime) Exec(ports.Command) error { return nil }
func (engine *fakeRuntime) StartApp(cfg schema.AppConfig, opt options.HostOptions, runArgs []string, onFail func()) error {
	engine.started = append(engine.started, cfg.AppNameID)
	engine.running[cfg.AppNameID] = true
	return nil
}
func (engine *fakeRuntime) OpenSession(string, []string, options.HostOptions, bool) error { return nil }
func (engine *fakeRuntime) Exists(name string) bool                                       { return engine.running[name] }
func (engine *fakeRuntime) Do([]string) error                                             { return nil }
func (engine *fakeRuntime) Running() (map[string]bool, error)                             { return engine.running, nil }
func (engine *fakeRuntime) Logs(string, int) (string, error)                              { return "", nil }

// fakeStore serves app definitions from an in-memory map.
type fakeStore struct{ apps map[string]schema.AppConfig }

func (store fakeStore) Load(name string) (schema.AppConfig, error) {
	cfg, ok := store.apps[name]
	if !ok {
		return schema.AppConfig{}, fmt.Errorf("app %q not found", name)
	}
	return cfg, nil
}
func (store fakeStore) List() ([]string, error)                   { return nil, nil }
func (store fakeStore) Save(schema.AppConfig) error               { return nil }
func (store fakeStore) Delete(string) error                       { return nil }
func (store fakeStore) Exists(name string) bool                   { _, ok := store.apps[name]; return ok }
func (store fakeStore) Path(name string) string                   { return name }
func (store fakeStore) Marshal(schema.AppConfig) ([]byte, error)  { return nil, nil }
func (store fakeStore) LoadFile(string) (schema.AppConfig, error) { return schema.AppConfig{}, nil }

// depApp is a minimal valid (passes validate.Validate), no-network app with the given
// depends_on list. digestPin is defined in plan_test.go (same package).
func depApp(name string, deps ...string) schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion:   schema.SchemaVersion,
		Type:            schema.ZincContainer,
		AppNameID:       name,
		ImageMeta:       schema.ImageMeta{Image: "img" + digestPin},
		StartConditions: schema.StartConditions{DependsOn: deps},
	}
}

func depSvc(store ports.Store, engine ports.Runtime) Service {
	return New(store, engine, nil, nil, netenforce.Enforcer{})
}

// web → vpn → base: each dependency (and its own dependencies) must come up before the
// app that needs it, deepest first.
func TestLaunch_StartsDependenciesDepthFirst(t *testing.T) {
	store := fakeStore{apps: map[string]schema.AppConfig{
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
	store := fakeStore{apps: map[string]schema.AppConfig{
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
	store := fakeStore{apps: map[string]schema.AppConfig{"web": depApp("web", "ghost")}}
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
	store := fakeStore{apps: map[string]schema.AppConfig{
		"a": depApp("a", "b"),
		"b": depApp("b", "a"),
	}}
	engine := newFakeRuntime()
	err := depSvc(store, engine).Launch(store.apps["a"], baseOpts())
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

// Fail-closed: a NetworkList this build can't enforce yet (here host-scoped egress)
// aborts the launch before any dependency or container starts.
func TestLaunch_UnsupportedNetworkFailsClosed(t *testing.T) {
	cfg := depApp("app", "vpn")
	cfg.NetworkMeta = schema.NetworkMeta{NetworkLists: []schema.NetworkList{{Host: true}}}
	store := fakeStore{apps: map[string]schema.AppConfig{"app": cfg, "vpn": depApp("vpn")}}
	engine := newFakeRuntime()
	err := depSvc(store, engine).Launch(store.apps["app"], baseOpts())
	if err == nil || !strings.Contains(err.Error(), "not supported in this build yet") {
		t.Fatalf("expected fail-closed unsupported-network error, got %v", err)
	}
	if len(engine.started) != 0 {
		t.Fatalf("nothing should start when the network is unsupported, got %v", engine.started)
	}
}

// tier-3 LAN publish (Ingress && Host) is enforceable now, so checkNetwork accepts it.
func TestCheckNetwork_Tier3PublishAllowed(t *testing.T) {
	cfg := depApp("pub")
	cfg.NetworkMeta = schema.NetworkMeta{NetworkLists: []schema.NetworkList{{
		Ingress: true, Host: true, Ports: []int{80},
	}}}
	if err := checkNetwork(cfg); err != nil {
		t.Fatalf("tier-3 publish should be allowed, got: %v", err)
	}
}

// A tier-2 producer (self-scoped ingress) is enforceable now, so checkNetwork accepts it.
func TestCheckNetwork_Tier2ProducerAllowed(t *testing.T) {
	cfg := depApp("db")
	cfg.NetworkMeta = schema.NetworkMeta{NetworkLists: []schema.NetworkList{{
		Ingress: true, Ports: []int{5432},
	}}}
	if err := checkNetwork(cfg); err != nil {
		t.Fatalf("tier-2 producer should be allowed, got: %v", err)
	}
}

// A tier-2 consumer (egress naming a sibling AppName) is enforceable now.
func TestCheckNetwork_Tier2ConsumerAllowed(t *testing.T) {
	cfg := depApp("client")
	cfg.NetworkMeta = schema.NetworkMeta{NetworkLists: []schema.NetworkList{{AppName: "db"}}}
	if err := checkNetwork(cfg); err != nil {
		t.Fatalf("tier-2 consumer should be allowed, got: %v", err)
	}
}

// A tier-2 app may not also carry other networking - coexistence is deferred, fail closed.
func TestCheckNetwork_Tier2MixRejected(t *testing.T) {
	cfg := depApp("db")
	cfg.NetworkMeta = schema.NetworkMeta{NetworkLists: []schema.NetworkList{
		{Ingress: true, Ports: []int{5432}},
		{IPv4CIDR: []string{"1.1.1.1/32"}, Ports: []int{443}},
	}}
	err := checkNetwork(cfg)
	if err == nil || !strings.Contains(err.Error(), "not supported in this build yet") {
		t.Fatalf("mixing links with other networking should fail closed, got: %v", err)
	}
}

// An ingress list that names an AppName is contradictory and rejected.
func TestCheckNetwork_IngressWithAppNameRejected(t *testing.T) {
	cfg := depApp("db")
	cfg.NetworkMeta = schema.NetworkMeta{NetworkLists: []schema.NetworkList{{
		Ingress: true, AppName: "client", Ports: []int{5432},
	}}}
	err := checkNetwork(cfg)
	if err == nil || !strings.Contains(err.Error(), "cannot target an AppName") {
		t.Fatalf("ingress with AppName should be rejected, got: %v", err)
	}
}
