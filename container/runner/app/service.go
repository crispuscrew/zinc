// Package app orchestrates container launches through the runner's ports.
package app

import (
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

type Service struct {
	store    ports.Store
	runtime  ports.Runtime
	builder  ports.ImageBuilder
	resolver ports.ImageResolver
	net      ports.NetEnforcer
	bus      ports.DBusBroker
	display  ports.DisplayBroker
	audio    ports.AudioBroker
	notify   ports.NotifyBroker
}

func New(store ports.Store, runtime ports.Runtime, builder ports.ImageBuilder, resolver ports.ImageResolver, network ports.NetEnforcer, bus ports.DBusBroker, display ports.DisplayBroker, audio ports.AudioBroker, notify ports.NotifyBroker) Service {
	return Service{store, runtime, builder, resolver, network, bus, display, audio, notify}
}

func (svc Service) address(name string) paths.Address {
	defined := func(string) bool { return false }
	if svc.store != nil {
		defined = svc.store.Exists
	}
	return paths.ParseRuntime(name, defined)
}

func (svc Service) Build(cfg schema.AppConfig) error { return svc.builder.Build(cfg) }

func (svc Service) ensureImage(cfg schema.AppConfig) error {
	if !derived.HasInstall(cfg) {
		return nil
	}
	if got, err := svc.builder.Fingerprint(derived.DerivedImageRef(cfg.AppNameID)); err == nil && got == derived.BuildFingerprint(cfg) {
		return nil
	}
	return svc.builder.Build(cfg)
}

func (svc Service) List() ([]string, error)                    { return svc.store.List() }
func (svc Service) Load(name string) (schema.AppConfig, error) { return svc.store.Load(name) }
func (svc Service) LoadResolved(name string) (schema.AppConfig, error) {
	return svc.store.LoadResolved(name)
}
func (svc Service) LoadFileResolved(path string) (schema.AppConfig, error) {
	return svc.store.LoadFileResolved(path)
}
func (svc Service) Save(cfg schema.AppConfig) error                { return svc.store.Save(cfg) }
func (svc Service) Delete(name string) error                       { return svc.store.Delete(name) }
func (svc Service) Exists(name string) bool                        { return svc.store.Exists(name) }
func (svc Service) Path(name string) string                        { return svc.store.Path(name) }
func (svc Service) Marshal(cfg schema.AppConfig) ([]byte, error)   { return svc.store.Marshal(cfg) }
func (svc Service) LoadFile(path string) (schema.AppConfig, error) { return svc.store.LoadFile(path) }
func (svc Service) Search(term string) ([]ports.Result, error)     { return svc.resolver.Search(term) }
func (svc Service) Resolve(reference string) (string, error)       { return svc.resolver.Resolve(reference) }
func (svc Service) Running() (map[string]bool, error)              { return svc.runtime.Running() }
func (svc Service) PIDs() (map[string]int, error)                  { return svc.runtime.PIDs() }
func (svc Service) Logs(name string, tail int) (string, error)     { return svc.runtime.Logs(name, tail) }
func (svc Service) Do(args []string) error                         { return svc.runtime.Do(args) }
func (svc Service) PodOf(name string) (string, error)              { return svc.runtime.PodOf(name) }
