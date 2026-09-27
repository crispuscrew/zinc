package app

import (
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/adapters/dbusproxy"
	"github.com/crispuscrew/zinc/container/runner/adapters/netenforce"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

type fakeRuntime struct {
	running       map[string]bool
	started       []string
	startOpts     []options.HostOptions
	detachedStart bool
	commands      []ports.Command
	sessionCmd    []string
	sessionEnv    map[string]string
	exited        bool
}

func newFakeRuntime(names ...string) *fakeRuntime {
	engine := &fakeRuntime{running: map[string]bool{}}
	for _, name := range names {
		engine.running[name] = true
	}
	return engine
}
func (engine *fakeRuntime) AppRunArgs(cfg schema.AppConfig, opt options.HostOptions, flags []string) ([]string, error) {
	return append([]string{"run", "--name", cfg.AppNameID}, flags...), nil
}
func (engine *fakeRuntime) Exec(command ports.Command) error {
	engine.commands = append(engine.commands, command)
	return nil
}
func (engine *fakeRuntime) Capture(ports.Command) (string, error) { return "", nil }
func (engine *fakeRuntime) StartApp(cfg schema.AppConfig, opt options.HostOptions, args []string, onFail func()) error {
	engine.started = append(engine.started, cfg.AppNameID)
	engine.startOpts = append(engine.startOpts, opt)
	if !engine.detachedStart {
		engine.running[cfg.AppNameID] = true
	}
	return nil
}
func (engine *fakeRuntime) OpenSession(name string, command []string, environment map[string]string, opt options.HostOptions, hold bool) error {
	engine.sessionCmd, engine.sessionEnv = command, environment
	return nil
}
func (engine *fakeRuntime) Exists(name string) bool           { return engine.exited || engine.running[name] }
func (engine *fakeRuntime) IsRunning(name string) bool        { return engine.running[name] }
func (engine *fakeRuntime) Do([]string) error                 { return nil }
func (engine *fakeRuntime) Running() (map[string]bool, error) { return engine.running, nil }
func (engine *fakeRuntime) Logs(string, int) (string, error)  { return "", nil }
func (engine *fakeRuntime) PodOf(name string) (string, error) {
	if engine.running[name] {
		return name + "-pod", nil
	}
	return "", nil
}
func (engine *fakeRuntime) PIDs() (map[string]int, error) {
	pids := map[string]int{}
	for name := range engine.running {
		pids[name] = len(pids) + 1
	}
	return pids, nil
}

type fakeStore struct{ apps map[string]schema.AppConfig }

func (store fakeStore) Load(name string) (schema.AppConfig, error) {
	cfg, exists := store.apps[name]
	if !exists {
		return schema.AppConfig{}, fmt.Errorf("app %q not found", name)
	}
	return cfg, nil
}
func (store fakeStore) LoadResolved(name string) (schema.AppConfig, error) { return store.Load(name) }
func (store fakeStore) LoadFileResolved(path string) (schema.AppConfig, error) {
	return store.Load(path)
}
func (store fakeStore) List() ([]string, error)                   { return nil, nil }
func (store fakeStore) Save(schema.AppConfig) error               { return nil }
func (store fakeStore) Delete(string) error                       { return nil }
func (store fakeStore) Exists(name string) bool                   { _, exists := store.apps[name]; return exists }
func (store fakeStore) Path(name string) string                   { return name }
func (store fakeStore) Marshal(schema.AppConfig) ([]byte, error)  { return nil, nil }
func (store fakeStore) LoadFile(string) (schema.AppConfig, error) { return schema.AppConfig{}, nil }

func depApp(name string, dependencies ...string) schema.AppConfig {
	return schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer, AppNameID: name,
		ImageMeta: schema.ImageMeta{Image: "img" + digestPin}, StartConditions: schema.StartConditions{DependsOn: dependencies}}
}
func depSvc(store ports.Store, engine ports.Runtime) Service {
	return New(store, engine, nil, nil, netenforce.Enforcer{}, dbusproxy.Broker{}, nil, nil, nil)
}
