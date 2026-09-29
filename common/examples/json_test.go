package examples_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func TestDNSExamples(test *testing.T) {
	for _, path := range examplePaths(test, "dns", ".json") {
		test.Run(path, func(test *testing.T) {
			var meta schema.DNSMeta
			decodeJSON(test, path, &meta)
			config := schema.AppConfig{
				SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer,
				AppNameID: "dns-example", ImageMeta: schema.ImageMeta{Image: "localhost/example:local"},
				NetworkMeta: schema.NetworkMeta{DNS: meta},
			}
			if err := validate.Validate(config); err != nil {
				test.Fatal(err)
			}
			upstreams, err := network.Upstreams(meta)
			if err != nil || len(upstreams) == 0 {
				test.Fatalf("unusable DNS transport example: %v", err)
			}
		})
	}
}

func TestVMOptionsExamples(test *testing.T) {
	for _, path := range examplePaths(test, "runtime/vm", ".json") {
		test.Run(path, func(test *testing.T) {
			var options vmoptions.Config
			decodeJSON(test, path, &options)
			if err := vmoptions.Validate(options); err != nil {
				test.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join("apps", options.AppNameID+".yaml"))
			if err != nil {
				test.Fatal(err)
			}
			config := decodeApp(test, data)
			if config.Type != schema.ZincVirtualization || config.ImageMeta.Image != options.Image || config.AppNameID != options.AppNameID {
				test.Fatal("VM options do not bind to their example app's type, image and identity")
			}
		})
	}
}

func decodeJSON(test *testing.T, path string, target any) {
	test.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		test.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		test.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		test.Fatalf("expected exactly one JSON document, got %v", err)
	}
}
