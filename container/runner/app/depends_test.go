package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestLaunchDependencyOrdering(t *testing.T) {
	store := fakeStore{apps: map[string]schema.AppConfig{
		"web": depApp("web", "vpn"), "vpn": depApp("vpn", "base"), "base": depApp("base"),
	}}
	engine := newFakeRuntime()
	if err := depSvc(store, engine).Launch(store.apps["web"], baseOpts()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(engine.started, []string{"base", "vpn", "web"}) {
		t.Fatal(engine.started)
	}
	engine = newFakeRuntime("vpn")
	if err := depSvc(store, engine).Launch(store.apps["web"], baseOpts()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(engine.started, []string{"web"}) {
		t.Fatal(engine.started)
	}
}

func TestLaunchDependencyFailures(t *testing.T) {
	for _, test := range []struct {
		apps map[string]schema.AppConfig
		want string
	}{
		{map[string]schema.AppConfig{"web": depApp("web", "ghost")}, `depends on "ghost"`},
		{map[string]schema.AppConfig{"web": depApp("web", "vpn"), "vpn": depApp("vpn", "web")}, "dependency cycle"},
	} {
		engine := newFakeRuntime()
		err := depSvc(fakeStore{test.apps}, engine).Launch(test.apps["web"], baseOpts())
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%v: want %s", err, test.want)
		}
		if len(engine.started) != 0 {
			t.Fatal(engine.started)
		}
	}
}

func TestLaunchDiamondStartsOnce(t *testing.T) {
	store := fakeStore{apps: map[string]schema.AppConfig{
		"root": depApp("root", "web", "mail"), "web": depApp("web", "vpn"), "mail": depApp("mail", "vpn"), "vpn": depApp("vpn"),
	}}
	engine := newFakeRuntime()
	engine.detachedStart = true
	if err := depSvc(store, engine).Launch(store.apps["root"], baseOpts()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(engine.started, []string{"vpn", "web", "mail", "root"}) {
		t.Fatal(engine.started)
	}
}

func TestRunningOrdinaryAppRefused(t *testing.T) {
	engine := newFakeRuntime("web")
	err := depSvc(nil, engine).Launch(depApp("web"), baseOpts())
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatal(err)
	}
	if len(engine.commands) != 0 {
		t.Fatal("running app was torn down")
	}
}
