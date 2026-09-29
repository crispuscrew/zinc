package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, text string) Project {
	t.Helper()
	var project Project
	if err := yaml.Unmarshal([]byte(text), &project); err != nil {
		t.Fatal(err)
	}
	return project
}

func importOne(t *testing.T, text string) App {
	t.Helper()
	apps := ToApps(decode(t, text))
	if len(apps) != 1 {
		t.Fatalf("want one app: %+v", apps)
	}
	return apps[0]
}

func hasNote(app App, text string) bool { return strings.Contains(strings.Join(app.Notes, "\n"), text) }

func containerApp(name string) schema.AppConfig {
	return schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer, AppNameID: name, ImageMeta: schema.ImageMeta{Image: "localhost/app:local"}}
}

func exportApp(t *testing.T, cfg schema.AppConfig) (Service, []string) {
	t.Helper()
	project, notes, err := FromApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return project.Services[cfg.AppNameID], notes
}

func TestDecodeScalarSequenceAndLabels(t *testing.T) {
	project := decode(t, "services:\n  app:\n    command: sh -c hello\n    expose: 5432\n    ports: [8080:80, 9090]\n    labels: [zinc.description=my app]\n")
	service := project.Services["app"]
	if !slices.Equal(service.Command, StringList{"sh -c hello"}) || !slices.Equal(service.Expose, StringList{"5432"}) || !slices.Equal(service.Ports, StringList{"8080:80", "9090"}) {
		t.Fatal(service)
	}
	if service.Labels["zinc.description"] != "my app" {
		t.Fatal(service.Labels)
	}
}

func TestDecodeDependencies(t *testing.T) {
	short := decode(t, "services:\n  app:\n    depends_on: [db, cache]\n").Services["app"].DependsOn
	if !slices.Equal(short.Names(), []string{"cache", "db"}) || short["db"].Condition != ConditionStarted {
		t.Fatal(short)
	}
	long := decode(t, "services:\n  app:\n    depends_on:\n      db:\n        condition: service_healthy\n").Services["app"].DependsOn
	if long["db"].Condition != ConditionHealthy {
		t.Fatal(long)
	}
}

func TestEnvironmentExplicitValuesOnly(t *testing.T) {
	for _, input := range []string{"{TOKEN: 'a=b c', EMPTY: ''}", "['TOKEN=a=b c', 'EMPTY=']"} {
		app := importOne(t, "services:\n  app:\n    environment: "+input+"\n")
		if app.Config.StartConditions.EntrypointEnv["TOKEN"] != "a=b c" {
			t.Fatal(app.Config.StartConditions)
		}
	}
	for _, input := range []string{"[HOST_SECRET]", "{HOST_SECRET: null}", "[TOKEN=one, TOKEN=two]"} {
		var project Project
		if err := yaml.Unmarshal([]byte("services:\n  app:\n    environment: "+input+"\n"), &project); err == nil {
			t.Errorf("implicit/ambiguous environment accepted: %s", input)
		}
	}
}
