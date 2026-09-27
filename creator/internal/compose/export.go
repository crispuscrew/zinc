package compose

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func FromApp(cfg schema.AppConfig) (Project, []string, error) {
	if cfg.Type == schema.ZincVirtualization {
		return Project{}, nil, fmt.Errorf("%s is a VM app: compose describes containers", cfg.AppNameID)
	}
	var notes []string
	note := func(format string, args ...any) { notes = append(notes, fmt.Sprintf(format, args...)) }
	service := Service{
		Image: cfg.ImageMeta.Image, ContainerName: cfg.AppNameID,
		CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"},
		PidsLimit: cfg.ResourcesMeta.PIDsLimit, ReadOnly: cfg.StartConditions.ReadOnlyRootfs,
		Environment: Environment(maps.Clone(cfg.StartConditions.EntrypointEnv)),
	}
	if entry := strings.TrimSpace(cfg.StartConditions.Entrypoint); entry != "" {
		service.Entrypoint = StringList{entry}
	}
	if cfg.StopConditions.Autorestart {
		service.Restart = "on-failure"
	}
	if user := cfg.InternalUserMeta; user.UseNonRootUser {
		service.User = user.NonRootUserName
	}
	if limits := resourceLimits(cfg.ResourcesMeta); limits != nil {
		service.Deploy = &Deploy{Resources: Resources{Limits: limits}}
	}
	for _, dependency := range cfg.StartConditions.DependsOn {
		if service.DependsOn == nil {
			service.DependsOn = Dependencies{}
		}
		service.DependsOn[dependency] = Depend{Condition: ConditionStarted}
	}
	service.Labels = Labels{}
	for key, value := range map[string]string{"zinc.description": cfg.LauncherMeta.Description, "zinc.icon": cfg.LauncherMeta.Icon, "zinc.group": cfg.LauncherMeta.Group} {
		if value != "" {
			service.Labels[key] = value
		}
	}
	service.Volumes = volumeMounts(cfg, note)
	service.Ports, service.Expose = exportNetwork(cfg.NetworkMeta, note)
	service.DNS = exportDNS(cfg.NetworkMeta.DNS, note)
	if len(cfg.NetworkMeta.Interfaces) == 0 {
		service.NetworkMode = "none"
	} else {
		note("THE EGRESS LOCK-DOWN IS NOT REPRESENTED. Compose cannot express Zinc's ordered, default-deny peer rules, interface identity, or stateful restrictions; this file does not reproduce the sandbox.")
	}
	exportLosses(cfg, note)
	return Project{Name: cfg.AppNameID, Services: map[string]Service{cfg.AppNameID: service}}, notes, nil
}

func exportLosses(cfg schema.AppConfig, note func(string, ...any)) {
	if len(cfg.CreatorFlags)+len(cfg.RunnerFlags) > 0 {
		note("CreatorFlags/RunnerFlags are not represented: raw backend argv was not interpreted or executed; the generated baseline may differ from the source's effective configuration")
	}
	if len(cfg.ImageMeta.Install) > 0 {
		note("ImageMeta.Install is not represented: this file names the BASE image, not Zinc's derived build")
	}
	if cfg.ImageMeta.SourceTag != "" {
		note("ImageMeta.SourceTag provenance is not represented")
	}
	if cfg.StartConditions.Attached || cfg.StartConditions.AttachedEntrypoint != "" || len(cfg.StartConditions.AttachedEnv) > 0 {
		note("Attached lifecycle, AttachedEntrypoint and AttachedEnv are not represented; they are per-session runtime behavior")
	}
	if cfg.StartConditions.Terminal || cfg.StopConditions.KeepAlive || cfg.StopConditions.Background {
		note("Terminal, KeepAlive and Background are not represented")
	}
	if cfg.MinimizeFingerprint {
		note("MinimizeFingerprint is not represented")
	}
	if cfg.HostTheme {
		note("HostTheme is not represented")
	}
	if cfg.InternalUserMeta.KeepUserID {
		note("InternalUserMeta.KeepUserID is not represented")
	}
	if len(cfg.Configs) > 0 {
		note("Configs are not represented: bundle paths are resolved by Zinc at launch")
	}
	if len(cfg.Keys) > 0 {
		note("Keys below name host paths and are not portable")
	}
	if !cfg.DBusMeta.IsZero() {
		note("DBusMeta (%d Talk, %d Own) is not represented: imported apps get NO session bus, not these grants", len(cfg.DBusMeta.Talk), len(cfg.DBusMeta.Own))
	}
	if cfg.NotificationMeta != (schema.NotificationMeta{}) {
		note("NotificationMeta is not represented")
	}
	note("DisplayMeta and AudioMeta are not represented: display sockets, GPU access and directional audio/device permissions require Zinc's runtime wiring")
}

func resourceLimits(resources schema.ResourcesMeta) *Limits {
	var limits Limits
	if resources.MaxCPUCores > 0 {
		limits.CPUs = strconv.FormatFloat(resources.MaxCPUCores, 'f', -1, 64)
	}
	if resources.MaxRamMiB > 0 {
		limits.Memory = strconv.FormatInt(resources.MaxRamMiB, 10) + "M"
	}
	if limits == (Limits{}) {
		return nil
	}
	return &limits
}
