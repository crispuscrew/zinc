package examples_test

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"gopkg.in/yaml.v3"
)

func TestCanonicalApps(test *testing.T) {
	checkApps(test, "apps")
}

func TestLauncherDemoApps(test *testing.T) {
	directory := os.Getenv("ZINC_DEMO_APPS")
	if directory == "" {
		directory = "../../launcher/demo/apps"
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			test.Skip("module-only checkout: set ZINC_DEMO_APPS to validate launcher fixtures")
		}
	}
	checkApps(test, directory)
}

func checkApps(test *testing.T, directory string) {
	test.Helper()
	paths := examplePaths(test, directory, ".yaml", ".yml")
	for _, path := range paths {
		test.Run(filepath.Base(path), func(test *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				test.Fatal(err)
			}
			config := decodeApp(test, data)
			if filepath.Base(path) == "firefox-broken.yaml" {
				checkBroken(test, validate.Validate(config))
				return
			}
			if err := validate.Validate(config); err != nil {
				test.Fatal(err)
			}
			if config.AppNameID != strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) {
				test.Fatalf("AppNameID %q differs from fixture filename", config.AppNameID)
			}
			migrated, err := schema.Migrate(data)
			if err != nil || !bytes.Equal(migrated, data) {
				test.Fatalf("canonical fixture requires migration: %v", err)
			}
		})
	}
}

func decodeApp(test *testing.T, data []byte) schema.AppConfig {
	test.Helper()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var config schema.AppConfig
	if err := decoder.Decode(&config); err != nil {
		test.Fatalf("strict canonical YAML decode: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		test.Fatalf("expected exactly one YAML document, got %v", err)
	}
	return config
}

func checkBroken(test *testing.T, err error) {
	test.Helper()
	if err == nil {
		test.Fatal("deliberately broken example passed validation")
	}
	wanted := []string{
		"SchemaVersion: got 1, want 4", "AppNameID", "ImageMeta.Image",
		"StartConditions.Attached: requires Terminal", "ResourcesMeta.MaxCPUCores",
		"ResourcesMeta.MaxRamMiB", "Volumes[0].HostMount", "Keys[0].Type",
		"NetworkMeta.RulesByPriority[0].Protocols",
	}
	problems := strings.Split(err.Error(), "\n")
	if len(problems) != len(wanted) {
		test.Fatalf("expected %d intentional errors, got %d:\n%v", len(wanted), len(problems), err)
	}
	for _, expected := range wanted {
		if !strings.Contains(err.Error(), expected) {
			test.Errorf("missing intentional error %q:\n%v", expected, err)
		}
	}
}

func examplePaths(test *testing.T, directory string, extensions ...string) []string {
	test.Helper()
	var paths []string
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		for _, extension := range extensions {
			if !entry.IsDir() && filepath.Ext(path) == extension {
				paths = append(paths, path)
			}
		}
		return nil
	})
	if err != nil {
		test.Fatal(err)
	}
	if len(paths) == 0 {
		test.Fatalf("no examples found in %s", directory)
	}
	return paths
}
