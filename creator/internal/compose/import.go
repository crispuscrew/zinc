package compose

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

type App struct {
	Service string
	Config  schema.AppConfig
	Notes   []string
}

func ToApps(project Project) []App {
	names := slices.Sorted(maps.Keys(project.Services))
	apps := make([]App, 0, len(names))
	for _, name := range names {
		apps = append(apps, serviceToApp(name, project))
	}
	return apps
}

// Compose does not state egress policy. Only explicit, representable ingress is
// imported; no raw backend flags are inferred from another format's privileges.
func serviceToApp(name string, project Project) App {
	service := project.Services[name]
	var notes []string
	note := func(format string, args ...any) { notes = append(notes, fmt.Sprintf(format, args...)) }
	cfg := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer, AppNameID: appName(name),
		LauncherMeta: schema.LauncherMeta{Description: service.Labels["zinc.description"], Icon: service.Labels["zinc.icon"], Group: service.Labels["zinc.group"]},
		ImageMeta:    schema.ImageMeta{Image: service.Image},
	}
	if cfg.AppNameID != name {
		note("service %q was renamed to %q: app names use lowercase [a-z0-9._-]", name, cfg.AppNameID)
	}
	head, argv := entrypoint(service)
	cfg.StartConditions.Entrypoint = head
	cfg.StartConditions.EntrypointEnv = maps.Clone(map[string]string(service.Environment))
	cfg.StartConditions.ReadOnlyRootfs = service.ReadOnly
	if len(service.Command) > 0 && len(service.Entrypoint) > 0 {
		note("both entrypoint and command were set; command %q was dropped", []string(service.Command))
	}
	if len(argv) > 1 {
		note("entrypoint %q was reduced to %q: arguments were dropped; use an explicit wrapper", strings.Join(argv, " "), head)
	}
	if len(service.Entrypoint) == 0 && len(service.Command) > 0 {
		note("command became Entrypoint, replacing the image's setup ENTRYPOINT: it will no longer run")
	}
	switch service.Restart {
	case "on-failure":
		cfg.StopConditions.Autorestart = true
	case "always", "unless-stopped":
		cfg.StopConditions.Autorestart = true
		note("restart: %s became StopConditions.Autorestart, which restarts only on failure; a clean exit or manual stop stays stopped", service.Restart)
	case "", "no":
	default:
		note("restart policy %q was not imported", service.Restart)
	}
	for _, dependency := range service.DependsOn.Names() {
		cfg.StartConditions.DependsOn = append(cfg.StartConditions.DependsOn, appName(dependency))
		if service.DependsOn[dependency].Condition == ConditionHealthy {
			note("depends_on %s: service_healthy is not representable; schema v4 defines no healthcheck/readiness wait, only startup ordering", dependency)
		}
	}
	if service.Healthcheck != nil {
		note("healthcheck/readiness is not representable in schema v4 and was not imported")
	}
	for _, capability := range service.CapAdd {
		note("cap_add: %s was not imported: schema v4 has no capability field; raw RunnerFlags require explicit review", capability)
	}
	for _, option := range service.SecurityOpt {
		if option != "no-new-privileges:true" && option != "no-new-privileges" {
			note("security_opt %q was not imported", option)
		}
	}
	for _, field := range slices.Sorted(maps.Keys(service.Unrepresented)) {
		note("source field %q is not represented and was not imported", field)
	}
	cfg.InternalUserMeta = importUser(service.User, note)
	cfg.ResourcesMeta = resources(service, note)
	cfg.Volumes = importVolumes(service, note)
	cfg.NetworkMeta = importNetwork(service, note)
	if len(service.Networks) > 0 || len(project.Networks) > 0 {
		note("Compose network membership/topology was not imported; app connectivity requires explicit peer rules")
	}
	if len(cfg.NetworkMeta.Interfaces) == 0 {
		note("no network: no NIC or egress grant was inferred from Compose")
	} else {
		note("only representable inbound ports came across; outbound connections remain denied by default")
	}
	for _, dependency := range service.DependsOn.Names() {
		note("depends_on %q starts the app but grants no connection to it; author NetworkMeta rules explicitly", dependency)
	}
	return App{Service: name, Config: cfg, Notes: notes}
}
