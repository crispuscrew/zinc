package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/fs"
)

func TestDependencyCyclesFailBeforeExecution(check *testing.T) {
	svc, cfg := serviceFixture(check)
	svc.Store = &fs.Store{Root: check.TempDir()}
	cfg.AppNameID = "alpha"
	cfg.StartConditions.DependsOn = []string{"beta"}
	other := cfg
	other.AppNameID = "beta"
	other.StartConditions.DependsOn = []string{"alpha"}
	for _, item := range []schema.AppConfig{cfg, other} {
		data, err := json.Marshal(item)
		if err != nil {
			check.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(svc.Store.Root, item.AppNameID+".yaml"), data, 0o600); err != nil {
			check.Fatal(err)
		}
	}
	if err := svc.checkDependencies(cfg, nil, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "cycle") {
		check.Fatalf("got %v", err)
	}
}
