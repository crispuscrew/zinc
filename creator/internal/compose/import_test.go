package compose

import (
	"slices"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestImportDefaultsAndLifecycle(t *testing.T) {
	app := importOne(t, "services:\n  Web App:\n    image: alpine\n    restart: always\n    read_only: true\n    command: nginx -g daemon off;\n")
	if app.Service != "Web App" || app.Config.AppNameID != "web-app" {
		t.Fatal(app)
	}
	if len(app.Config.NetworkMeta.Interfaces) != 0 || !hasNote(app, "no network") {
		t.Fatal("network inferred")
	}
	if !app.Config.StopConditions.Autorestart || !hasNote(app, "restarts only on failure") || !app.Config.StartConditions.ReadOnlyRootfs {
		t.Fatal(app)
	}
	if app.Config.StartConditions.Entrypoint != "nginx" || !hasNote(app, "was reduced to") || !hasNote(app, "it will no longer run") {
		t.Fatal(app)
	}
}

func TestImportRemovedFeaturesAreExplicitLossWithoutFlags(t *testing.T) {
	app := importOne(t, `services:
  app:
    image: alpine
    cap_add: [ALL, NET_ADMIN, CAP_SYS_ADMIN, NET_RAW]
    healthcheck:
      test: [CMD-SHELL, "test -f /run/ready"]
    depends_on:
      db:
        condition: service_healthy
    tunnel: /keys/wg.conf
    privileged: true
`)
	for _, value := range []string{"ALL", "NET_ADMIN", "CAP_SYS_ADMIN", "NET_RAW", "healthcheck", "service_healthy", "tunnel", "privileged"} {
		if !hasNote(app, value) {
			t.Errorf("lost source %s was not reported: %v", value, app.Notes)
		}
	}
	if len(app.Config.RunnerFlags)+len(app.Config.CreatorFlags) != 0 {
		t.Fatal("import invented raw privilege grants")
	}
	if !slices.Equal(app.Config.StartConditions.DependsOn, []string{"db"}) {
		t.Fatal("startup ordering lost")
	}
}

func TestImportUserAndNameCoercion(t *testing.T) {
	for _, value := range []string{"1000", "1000:1000", ":group"} {
		app := importOne(t, "services:\n  app:\n    user: '"+value+"'\n")
		if app.Config.InternalUserMeta != (schema.InternalUserMeta{}) || !hasNote(app, "BY NAME") {
			t.Fatal(app)
		}
	}
	named := importOne(t, "services:\n  app:\n    user: postgres:users\n")
	if !named.Config.InternalUserMeta.UseNonRootUser || named.Config.InternalUserMeta.NonRootUserName != "postgres" || !hasNote(named, "group was dropped") {
		t.Fatal(named)
	}
	for input, expected := range map[string]string{"Web App": "web-app", "api/v2": "api-v2", "_leading": "leading", "...": "imported"} {
		if appName(input) != expected {
			t.Errorf("%s", input)
		}
	}
}

func TestImportVolumesPreserveExplicitAccess(t *testing.T) {
	for _, options := range []string{"", ":z", ":ro", ":rw"} {
		app := importOne(t, "services:\n  app:\n    volumes: ['/srv/data:/data"+options+"']\n")
		if len(app.Config.Volumes) != 1 || app.Config.Volumes[0].Writable != (options == ":rw") || app.Config.Volumes[0].Executable {
			t.Fatal(app.Config.Volumes)
		}
		if (options == "" || options == ":z") && !hasNote(app, "read-only and noexec") {
			t.Fatal(app.Notes)
		}
	}
	app := importOne(t, "services:\n  app:\n    volumes: ['cache:/cache', './data:/data', '~/keys:/keys']\n")
	if len(app.Config.Volumes) != 0 || !hasNote(app, "named compose volume") || !hasNote(app, "relative to the compose file") {
		t.Fatal(app)
	}
}

func TestImportResourceLimits(t *testing.T) {
	for input, expected := range map[string]int64{"512M": 512, "2g": 2048, "1073741824": 1024, "256mb": 256} {
		actual, ok := memoryMiB(input)
		if !ok || actual != expected {
			t.Errorf("%s: %d %v", input, actual, ok)
		}
	}
	for _, input := range []string{"", "512k", "nonsense", "1.5G", "9223372036854775807g"} {
		if _, ok := memoryMiB(input); ok {
			t.Errorf("accepted %q", input)
		}
	}
	app := importOne(t, "services:\n  app:\n    deploy:\n      resources:\n        limits:\n          cpus: lots\n          memory: 1.5G\n")
	if !hasNote(app, "NO cpu limit") || !hasNote(app, "NO memory limit") {
		t.Fatal(app.Notes)
	}
}
