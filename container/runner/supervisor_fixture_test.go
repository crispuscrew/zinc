package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/adapters/dbusproxy"
	"github.com/crispuscrew/zinc/container/runner/adapters/fs"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

const supervisorTestRoot = "ZINC_TEST_SUPERVISOR_ROOT"
const supervisorTestMode = "ZINC_TEST_SUPERVISOR_MODE"

// Re-exec the real CLI handler and inherited-FD decoder, with recorded runtime effects.
func TestMain(tests *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__supervise" && os.Getenv(supervisorTestRoot) != "" {
		if os.Getenv(supervisorTestMode) == "silent" {
			os.Exit(23)
		}
		if os.Getenv(supervisorTestMode) == "delayed" {
			root := os.Getenv(supervisorTestRoot)
			if err := os.WriteFile(filepath.Join(root, "decoding"), nil, 0o600); err != nil {
				os.Exit(24)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(root, "decode-now")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					os.Exit(25)
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		arguments := os.Args[2:]
		if os.Getenv(supervisorTestMode) == "identity" {
			arguments = []string{"different"}
		}
		err := cmdSupervise(supervisorService(os.Getenv(supervisorTestRoot), true), arguments)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(tests.Run())
}

func supervisorService(root string, helper bool) app.Service {
	store := &fs.Store{Root: filepath.Join(root, "apps")}
	engine := &supervisorRuntime{root: root, helper: helper}
	return app.New(store, engine, nil, nil, supervisorNetwork{}, dbusproxy.Broker{}, nil, nil, nil)
}

type supervisorRuntime struct {
	ports.Runtime
	root   string
	helper bool
}

func (engine *supervisorRuntime) Running() (map[string]bool, error) {
	if engine.helper {
		if os.Getenv(supervisorTestMode) == "observer" {
			return nil, errors.New("runtime unavailable")
		}
		if err := os.WriteFile(filepath.Join(engine.root, "ready"), nil, 0o600); err != nil {
			return nil, err
		}
	}
	return map[string]bool{}, nil
}

func (engine *supervisorRuntime) IsRunning(string) bool { return false }
func (engine *supervisorRuntime) AppRunArgs(schema.AppConfig, options.HostOptions, []string) ([]string, error) {
	return []string{"run"}, nil
}
func (engine *supervisorRuntime) StartApp(cfg schema.AppConfig, opt options.HostOptions, args []string, onFail func()) error {
	if _, err := os.Stat(filepath.Join(engine.root, "ready")); err != nil {
		return errors.New("app started before supervisor readiness")
	}
	if os.Getenv(supervisorTestMode) == "app-failure" {
		return errors.New("app startup failed")
	}
	return os.WriteFile(filepath.Join(engine.root, "started"), []byte(cfg.AppNameID), 0o600)
}

func (engine *supervisorRuntime) Exec(command ports.Command) error {
	resource := filepath.Join(engine.root, "resource")
	switch command.Args[0] {
	case "prepare":
		file, err := os.OpenFile(resource, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		return file.Close()
	case "cleanup":
		if err := os.Remove(resource); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		temporary := filepath.Join(engine.root, "cleaned.tmp")
		if err := os.WriteFile(temporary, []byte(command.Args[1]), 0o600); err != nil {
			return err
		}
		return os.Rename(temporary, filepath.Join(engine.root, "cleaned.json"))
	default:
		return fmt.Errorf("unexpected runtime command %v", command.Args)
	}
}

type supervisorNetwork struct{}

func (supervisorNetwork) RunFlags(schema.AppConfig) []string { return nil }
func (supervisorNetwork) Prepare(schema.AppConfig, options.HostOptions) ([]ports.Command, error) {
	return []ports.Command{{Args: []string{"prepare"}}}, nil
}
func (supervisorNetwork) Teardown(cfg schema.AppConfig) []ports.Command {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	return []ports.Command{{Args: []string{"cleanup", string(encoded)}}}
}
func (supervisorNetwork) Counters(schema.AppConfig, options.HostOptions) (ports.Command, bool) {
	return ports.Command{}, false
}
