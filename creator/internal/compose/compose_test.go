package compose

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func decode(t *testing.T, text string) Project {
	t.Helper()
	var project Project
	if err := yaml.Unmarshal([]byte(text), &project); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return project
}

// importOne decodes a one-service file and returns the app it became.
func importOne(t *testing.T, text string) App {
	t.Helper()
	apps := ToApps(decode(t, text))
	if len(apps) != 1 {
		t.Fatalf("want 1 app, got %d", len(apps))
	}
	return apps[0]
}

func hasNote(app App, substring string) bool {
	for _, note := range app.Notes {
		if strings.Contains(note, substring) {
			return true
		}
	}
	return false
}

// Compose fields are written both as scalars and as sequences, and real files use both
// spellings freely. A decoder that only accepted one would reject most of them.
func TestDecode_ScalarOrSequence(t *testing.T) {
	project := decode(t, `
services:
  one:
    image: alpine
    command: sh -c 'echo hi'
    expose: 5432
    ports:
      - 8080:80
      - 9090
`)
	service := project.Services["one"]
	if want := (StringList{"sh -c 'echo hi'"}); !slices.Equal(service.Command, want) {
		t.Errorf("scalar command = %v, want %v", service.Command, want)
	}
	// A YAML integer is still a port; decoding it as a string is what makes `expose: 5432`
	// work, and people write it that way constantly.
	if want := (StringList{"5432"}); !slices.Equal(service.Expose, want) {
		t.Errorf("scalar int expose = %v, want %v", service.Expose, want)
	}
	if want := (StringList{"8080:80", "9090"}); !slices.Equal(service.Ports, want) {
		t.Errorf("ports = %v, want %v", service.Ports, want)
	}
}

// depends_on has a short list form and a long map form, and both are everywhere.
func TestDecode_DependsOnBothForms(t *testing.T) {
	short := decode(t, "services:\n  app:\n    depends_on: [db, cache]\n")
	if got := short.Services["app"].DependsOn.Names(); !slices.Equal(got, []string{"cache", "db"}) {
		t.Errorf("short form = %v", got)
	}
	if got := short.Services["app"].DependsOn["db"].Condition; got != ConditionStarted {
		t.Errorf("short form condition = %q, want %q", got, ConditionStarted)
	}

	long := decode(t, `
services:
  app:
    depends_on:
      db:
        condition: service_healthy
`)
	if got := long.Services["app"].DependsOn["db"].Condition; got != ConditionHealthy {
		t.Errorf("long form condition = %q, want %q", got, ConditionHealthy)
	}
}

// The governing rule of importing: compose cannot say what a service may REACH, so nothing
// is inferred. An imported app arrives with no network at all rather than with the open
// posture the compose service actually had.
func TestImport_FailsClosedOnNetwork(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n")
	if len(app.Config.NetworkMeta.NetworkLists) != 0 {
		t.Fatalf("an imported app must start with no network, got %v", app.Config.NetworkMeta.NetworkLists)
	}
	if !hasNote(app, "no network") {
		t.Errorf("the closed posture must be stated, notes were %v", app.Notes)
	}
}

// Published ports are the one thing compose states outright. `ports` reaches the LAN and
// `expose` reaches only siblings, which is exactly the Host distinction.
func TestImport_PortsAndExpose(t *testing.T) {
	app := importOne(t, `
services:
  app:
    image: alpine
    ports: ["8080:80"]
    expose: [5432]
`)
	lists := app.Config.NetworkMeta.NetworkLists
	if len(lists) != 2 {
		t.Fatalf("want a published list and a sibling list, got %v", lists)
	}
	if !lists[0].Ingress || !lists[0].Host || !slices.Equal(lists[0].Ports, []int{80}) {
		t.Errorf("ports should publish to the LAN: %+v", lists[0])
	}
	if !lists[1].Ingress || lists[1].Host || !slices.Equal(lists[1].Ports, []int{5432}) {
		t.Errorf("expose should stay between siblings: %+v", lists[1])
	}
	// The host-side number is dropped, which changes what the app is reachable on.
	if !hasNote(app, "exposed as 80, not 8080") {
		t.Errorf("the remapped port must be stated, notes were %v", app.Notes)
	}
}

// A compose command written as one string is shell-split by compose itself, so taking the
// whole string as the executable would author an app whose entrypoint is a filename with
// spaces in it: it passes validation and fails at exec.
func TestImport_ScalarCommandIsSplitToItsExecutable(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    command: nginx -g daemon off;\n")
	if got := app.Config.StartConditions.Entrypoint; got != "nginx" {
		t.Fatalf("Entrypoint = %q, want %q", got, "nginx")
	}
	if !hasNote(app, "was reduced to") {
		t.Errorf("dropping the arguments must be stated, notes were %v", app.Notes)
	}
}

// CMD-SHELL is the most common healthcheck form in real files; refusing it would refuse
// most healthchecks that exist. It is representable as argv through a shell.
func TestImport_Healthchecks(t *testing.T) {
	shell := importOne(t, `
services:
  app:
    image: alpine
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
`)
	if want := []string{"sh", "-c", "pg_isready -U postgres"}; !slices.Equal(shell.Config.StartConditions.ReadyCheck, want) {
		t.Errorf("CMD-SHELL = %v, want %v", shell.Config.StartConditions.ReadyCheck, want)
	}

	argv := importOne(t, `
services:
  app:
    image: alpine
    healthcheck:
      test: ["CMD", "test", "-f", "/run/ready"]
`)
	if want := []string{"test", "-f", "/run/ready"}; !slices.Equal(argv.Config.StartConditions.ReadyCheck, want) {
		t.Errorf("CMD = %v, want %v", argv.Config.StartConditions.ReadyCheck, want)
	}

	// NONE switches an inherited check off; there is nothing to import and nothing to warn
	// about beyond the absence.
	none := importOne(t, "services:\n  app:\n    image: alpine\n    healthcheck:\n      test: [\"NONE\"]\n")
	if len(none.Config.StartConditions.ReadyCheck) != 0 {
		t.Errorf("NONE should import no probe, got %v", none.Config.StartConditions.ReadyCheck)
	}
}

// A dependent asking to wait for a dependency that has no runnable probe would wait for
// nothing; the import says so rather than leaving the reader to find out at launch.
func TestImport_HealthyWaitOnAProbelessDependency(t *testing.T) {
	apps := ToApps(decode(t, `
services:
  app:
    image: alpine
    depends_on:
      db:
        condition: service_healthy
  db:
    image: alpine
`))
	var dependent App
	for _, app := range apps {
		if app.Config.AppNameID == "app" {
			dependent = app
		}
	}
	if !hasNote(dependent, "defines no healthcheck") {
		t.Errorf("a healthy-wait on a probeless dependency must be stated, notes were %v", dependent.Notes)
	}
}

// Zinc's baseline is cap-drop ALL. A compose file re-adding every capability is asking for
// the one thing the sandbox exists to refuse, so it is dropped rather than honoured.
func TestImport_CapAddAllRefused(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    cap_add: [ALL, NET_RAW]\n")
	if slices.Contains(app.Config.Capabilities, "ALL") {
		t.Fatalf("cap_add ALL must not be honoured, got %v", app.Config.Capabilities)
	}
	if !slices.Contains(app.Config.Capabilities, "NET_RAW") {
		t.Errorf("a named capability should still come across, got %v", app.Config.Capabilities)
	}
}

// An unqualified compose mount is read-write. Importing that silently would hand an app
// write access nobody chose, so the fail-closed reading wins and says so.
func TestImport_VolumeDefaultsToReadOnly(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    volumes: [\"/srv/data:/data\"]\n")
	if len(app.Config.Volumes) != 1 {
		t.Fatalf("want one mount, got %v", app.Config.Volumes)
	}
	if app.Config.Volumes[0].Writable || app.Config.Volumes[0].Executable {
		t.Errorf("an unqualified mount must import read-only and noexec, got %+v", app.Config.Volumes[0])
	}
	if !hasNote(app, "read-only and noexec") {
		t.Errorf("the tightening must be stated, notes were %v", app.Notes)
	}

	explicit := importOne(t, "services:\n  app:\n    image: alpine\n    volumes: [\"/srv/data:/data:rw\"]\n")
	if !explicit.Config.Volumes[0].Writable {
		t.Errorf("an explicit rw must be honoured, got %+v", explicit.Config.Volumes[0])
	}
}

// A named volume has no host path, so there is nothing for Zinc to mount. Inventing one
// would be inventing a location.
func TestImport_NamedVolumeDropped(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    volumes: [\"dbcache:/var/cache\"]\n")
	if len(app.Config.Volumes) != 0 {
		t.Fatalf("a named volume has no host path to mount, got %v", app.Config.Volumes)
	}
	if !hasNote(app, "named compose volume") {
		t.Errorf("the drop must be stated, notes were %v", app.Notes)
	}
}

// Zinc passes the user to podman BY NAME so it must exist in the image; a bare uid would
// always 'work' and could land on a user the image does not have.
func TestImport_User(t *testing.T) {
	named := importOne(t, "services:\n  app:\n    image: alpine\n    user: postgres\n")
	if user := named.Config.InternalUserMeta; !user.UseNonRootUser || user.NonRootUserName != "postgres" {
		t.Errorf("named user = %+v", user)
	}

	numeric := importOne(t, "services:\n  app:\n    image: alpine\n    user: \"1000\"\n")
	if numeric.Config.InternalUserMeta != (schema.InternalUserMeta{}) {
		t.Errorf("a bare uid must not be imported, got %+v", numeric.Config.InternalUserMeta)
	}
	if !hasNote(numeric, "BY NAME") {
		t.Errorf("the drop must be stated, notes were %v", numeric.Notes)
	}
}

// compose's memory limits are binary despite the single-letter spelling. A sub-MiB limit
// would round to zero, and zero means unlimited - the opposite of what was asked - so it is
// refused instead.
func TestImport_MemoryQuantities(t *testing.T) {
	for _, testCase := range []struct {
		text string
		want int64
	}{
		{"512M", 512}, {"2g", 2048}, {"1073741824", 1024}, {"256mb", 256},
		{"512k", 0}, {"nonsense", 0}, {"", 0},
	} {
		got, ok := memoryMiB(testCase.text)
		if testCase.want == 0 {
			if ok {
				t.Errorf("memoryMiB(%q) = %d, want refused", testCase.text, got)
			}
			continue
		}
		if !ok || got != testCase.want {
			t.Errorf("memoryMiB(%q) = %d (ok=%v), want %d", testCase.text, got, ok, testCase.want)
		}
	}
}

// An app name becomes a filename and a container name, so a compose key is narrowed to the
// legal charset rather than trusted.
func TestImport_NameCoercion(t *testing.T) {
	for _, testCase := range []struct{ service, want string }{
		{"Web App", "web-app"},
		{"api/v2", "api-v2"},
		{"_leading", "leading"},
		{"...", "imported"},
	} {
		if got := appName(testCase.service); got != testCase.want {
			t.Errorf("appName(%q) = %q, want %q", testCase.service, got, testCase.want)
		}
	}
}

// compose's "always" restarts a container the user stopped on purpose, which is the one
// thing Autorestart is documented not to do. It still imports, but the difference is said.
func TestImport_RestartAlwaysIsNarrowed(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    restart: always\n")
	if !app.Config.StartConditions.Autorestart {
		t.Error("restart: always should still set Autorestart")
	}
	if !hasNote(app, "restarts only on failure") {
		t.Errorf("the narrowing must be stated, notes were %v", app.Notes)
	}
}

// --- export ---

func exportApp(t *testing.T, cfg schema.AppConfig) (Service, []string) {
	t.Helper()
	project, notes, err := FromApp(cfg)
	if err != nil {
		t.Fatalf("FromApp: %v", err)
	}
	return project.Services[cfg.AppNameID], notes
}

func containerApp(name string) schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     name,
		ImageMeta:     schema.ImageMeta{Image: "localhost/app:local"},
	}
}

// The baseline the runner applies to every app is stated outright, because in compose it is
// not the default: a service with no cap_drop runs with podman's default capability set.
func TestExport_StatesTheBaseline(t *testing.T) {
	service, _ := exportApp(t, containerApp("app"))
	if !slices.Contains(service.CapDrop, "ALL") {
		t.Errorf("cap_drop = %v, want ALL", service.CapDrop)
	}
	if !slices.Contains(service.SecurityOpt, "no-new-privileges:true") {
		t.Errorf("security_opt = %v", service.SecurityOpt)
	}
}

// The one thing a reader of the generated file must not conclude is that running it
// reproduces the sandbox. An app with egress rules says so loudly.
func TestExport_EgressLockdownIsReportedAsLost(t *testing.T) {
	cfg := containerApp("app")
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{IPv4CIDR: []string{"1.1.1.1/32"}, Ports: []int{443}}}
	service, notes := exportApp(t, cfg)

	var loud bool
	for _, note := range notes {
		if strings.Contains(note, "EGRESS LOCK-DOWN IS NOT REPRESENTED") {
			loud = true
		}
	}
	if !loud {
		t.Fatalf("an app with egress rules must say the lock-down is not in the file, notes were %v", notes)
	}
	// And it must not be rendered as though it were something compose enforces.
	if len(service.Ports) != 0 || len(service.Expose) != 0 {
		t.Errorf("an egress list is not a published port: ports=%v expose=%v", service.Ports, service.Expose)
	}
}

// A readiness probe is the one Zinc field that has an exact compose counterpart, since the
// runner already installs it as the container's healthcheck.
func TestExport_ReadyCheckBecomesHealthcheck(t *testing.T) {
	cfg := containerApp("app")
	cfg.StartConditions.ReadyCheck = []string{"test", "-f", "/run/ready"}
	cfg.StartConditions.ReadyTimeoutSec = 30
	service, notes := exportApp(t, cfg)

	if service.Healthcheck == nil {
		t.Fatal("want a healthcheck")
	}
	if want := (StringList{"CMD", "test", "-f", "/run/ready"}); !slices.Equal(service.Healthcheck.Test, want) {
		t.Errorf("test = %v, want %v", service.Healthcheck.Test, want)
	}
	// ReadyTimeoutSec must NOT be written to healthcheck.timeout. That field bounds how long
	// one probe may run; ReadyTimeoutSec bounds how long a dependent waits for the app. Writing
	// one as the other exports a different promise under the same number.
	if service.Healthcheck.Timeout != "" {
		t.Errorf("timeout = %q: ReadyTimeoutSec is a dependent's wait, not a per-probe limit", service.Healthcheck.Timeout)
	}
	var explained bool
	for _, note := range notes {
		if strings.Contains(note, "ReadyTimeoutSec") {
			explained = true
		}
	}
	if !explained {
		t.Errorf("the dropped timeout must be stated, notes were %v", notes)
	}
}

// A dependency's readiness is a property of the DEPENDENCY's config, which this app's
// definition does not contain. Claiming service_healthy for one that defines no healthcheck
// would make the compose file hang rather than run.
func TestExport_DependsOnClaimsOnlyStarted(t *testing.T) {
	cfg := containerApp("app")
	cfg.StartConditions.DependsOn = []string{"db"}
	service, _ := exportApp(t, cfg)

	if got := service.DependsOn["db"].Condition; got != ConditionStarted {
		t.Errorf("condition = %q, want %q", got, ConditionStarted)
	}
}

// Published ports are the fields compose does carry: LAN-published becomes `ports`, a
// sibling link's published ports become `expose`.
func TestExport_IngressLists(t *testing.T) {
	cfg := containerApp("app")
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{
		{Ingress: true, Host: true, Ports: []int{80}},
		{Ingress: true, Ports: []int{5432}},
	}
	service, _ := exportApp(t, cfg)

	if want := (StringList{"80:80"}); !slices.Equal(service.Ports, want) {
		t.Errorf("ports = %v, want %v", service.Ports, want)
	}
	if want := (StringList{"5432"}); !slices.Equal(service.Expose, want) {
		t.Errorf("expose = %v, want %v", service.Expose, want)
	}
}

// Routing through a sibling has no compose spelling at all, and the failure mode of
// pretending otherwise is an app that reaches the internet directly.
func TestExport_RoutingIsReportedAsLost(t *testing.T) {
	cfg := containerApp("app")
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{AppName: "vpn", Via: true, IPv4CIDR: []string{"0.0.0.0/0"}}}
	cfg.NetworkMeta.DNSServers = []string{"10.0.0.1"}
	_, notes := exportApp(t, cfg)

	var found bool
	for _, note := range notes {
		if strings.Contains(note, "Compose has no routing") {
			found = true
		}
	}
	if !found {
		t.Errorf("routing must be reported as not represented, notes were %v", notes)
	}
}

// A guest is not a container, so there is nothing to describe. Refused rather than
// half-rendered into a file that would look like it might work.
func TestExport_VMAppRefused(t *testing.T) {
	cfg := containerApp("guest")
	cfg.Type = schema.ZincVirtualization
	if _, _, err := FromApp(cfg); err == nil {
		t.Fatal("a VM app has no compose equivalent and must be refused")
	}
}

// A compose file that binds a port to loopback is saying "not for the network". Publishing it
// to the LAN would be the importer widening the sandbox from what the file asked for, which is
// the one thing it must not do.
func TestImport_LoopbackBoundPortIsNotPublishedToTheLAN(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    ports: [\"127.0.0.1:9000:9000\"]\n")
	for _, list := range app.Config.NetworkMeta.NetworkLists {
		if list.Host {
			t.Fatalf("a loopback-bound port must not become a LAN publish: %+v", list)
		}
	}
	if !hasNote(app, "bound to loopback") {
		t.Errorf("the narrowing must be stated, notes were %v", app.Notes)
	}
}

// NET_ADMIN lets an app flush the egress ruleset in its own netns and SYS_ADMIN contains it,
// so neither is ever granted by an import of someone else's file. Everything else is carried
// but said out loud - a capability on top of cap-drop ALL is a deliberate widening.
func TestImport_DangerousCapabilitiesRefused(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    cap_add: [NET_ADMIN, CAP_SYS_ADMIN, NET_RAW]\n")
	for _, refused := range []string{"NET_ADMIN", "SYS_ADMIN"} {
		if slices.Contains(app.Config.Capabilities, refused) {
			t.Errorf("%s must not be granted by an import, got %v", refused, app.Config.Capabilities)
		}
	}
	if !slices.Contains(app.Config.Capabilities, "NET_RAW") {
		t.Errorf("an ordinary capability should still come across, got %v", app.Config.Capabilities)
	}
	if !hasNote(app, "NET_RAW was carried over") {
		t.Errorf("a carried capability must be stated, notes were %v", app.Notes)
	}
}

// `1000:1000` is at least as common as a bare `1000` and means the same thing. Testing the
// whole string let the spelling the guard refuses walk through as a "name" of "1000".
func TestImport_NumericUserWithGroupAlsoDropped(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    user: \"1000:1000\"\n")
	if app.Config.InternalUserMeta != (schema.InternalUserMeta{}) {
		t.Fatalf("a numeric uid:gid must not be imported, got %+v", app.Config.InternalUserMeta)
	}
}

// A cap the compose file set, quietly becoming no cap at all, is the worst shape for a limit
// on the tool whose job is to bound what an app takes.
func TestImport_UnreadableLimitIsStated(t *testing.T) {
	app := importOne(t, `
services:
  app:
    image: alpine
    deploy:
      resources:
        limits:
          memory: 1.5G
          cpus: lots
`)
	if app.Config.ResourcesMeta.MaxRamMiB != 0 {
		t.Fatalf("expected the unreadable limit to be dropped, got %d", app.Config.ResourcesMeta.MaxRamMiB)
	}
	if !hasNote(app, "NO memory limit") || !hasNote(app, "NO cpu limit") {
		t.Errorf("both unreadable limits must be stated, notes were %v", app.Notes)
	}
}

// `:z` is the SELinux relabel suffix, means read-WRITE, and is the spelling most likely to
// appear on this project's own platform. It used to land read-only with no note at all.
func TestImport_RelabelMountStillWarns(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    volumes: [\"/srv/conf:/etc/nginx:z\"]\n")
	if app.Config.Volumes[0].Writable {
		t.Error("a mount that does not say rw imports read-only")
	}
	if !hasNote(app, "read-only and noexec") {
		t.Errorf("the tightening must be stated for :z too, notes were %v", app.Notes)
	}
}

// `./data` is relative to the compose file and `~` is a shell's; the runner hands the path to
// podman from wherever it happens to be, so keeping either mounts a different directory than
// the file meant - and podman creates the missing one rather than failing.
func TestImport_RelativeHostPathDropped(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    image: alpine\n    volumes: [\"./data:/data\", \"~/certs:/certs\"]\n")
	if len(app.Config.Volumes) != 0 {
		t.Fatalf("relative and ~ host paths must not be imported verbatim, got %+v", app.Config.Volumes)
	}
	if !hasNote(app, "relative to the compose file") {
		t.Errorf("the drop must be stated, notes were %v", app.Notes)
	}
}

// --service names the compose key the user is reading, which may not be the coerced app name.
func TestImport_ServiceKeyIsRetained(t *testing.T) {
	apps := ToApps(decode(t, "services:\n  Web App:\n    image: alpine\n"))
	if apps[0].Service != "Web App" {
		t.Errorf("Service = %q, want the compose key", apps[0].Service)
	}
	if apps[0].Config.AppNameID != "web-app" {
		t.Errorf("AppNameID = %q, want the coerced name", apps[0].Config.AppNameID)
	}
}

// compose's `command` replaces the image's CMD and leaves its ENTRYPOINT running; Zinc's one
// field becomes --entrypoint, which replaces it. On postgres/redis/mysql that entrypoint is
// what creates the data dir and drops privileges.
func TestImport_BareCommandBecomingEntrypointIsStated(t *testing.T) {
	app := importOne(t, "services:\n  cache:\n    image: redis\n    command: redis-server\n")
	if app.Config.StartConditions.Entrypoint != "redis-server" {
		t.Fatalf("Entrypoint = %q", app.Config.StartConditions.Entrypoint)
	}
	if !hasNote(app, "it will no longer run") {
		t.Errorf("the semantic difference must be stated, notes were %v", app.Notes)
	}
}

// The long syntax is how modern compose files are written. Refusing it aborted the entire
// import over a spelling, which is not the same thing as a file Zinc cannot represent.
func TestDecode_LongSyntax(t *testing.T) {
	app := importOne(t, `
services:
  web:
    image: alpine
    ports:
      - target: 80
        published: "8080"
        protocol: tcp
      - target: 5000
        host_ip: 127.0.0.1
        published: "5000"
    volumes:
      - type: bind
        source: /srv/site
        target: /usr/share/nginx/html
        read_only: true
    labels:
      - com.example.role=frontend
`)
	lists := app.Config.NetworkMeta.NetworkLists
	if len(lists) == 0 {
		t.Fatalf("the long-form ports should have imported, got %+v", lists)
	}
	var published, sibling []int
	for _, list := range lists {
		if list.Host {
			published = list.Ports
		} else {
			sibling = list.Ports
		}
	}
	if !slices.Contains(published, 80) {
		t.Errorf("long-form port 80 should publish to the LAN, got %v", published)
	}
	// host_ip 127.0.0.1 keeps its meaning through the long form too: not for the network.
	if !slices.Contains(sibling, 5000) {
		t.Errorf("a loopback-bound long-form port should stay sibling-only, got %v", sibling)
	}
	if len(app.Config.Volumes) != 1 || app.Config.Volumes[0].HostMount != "/srv/site" {
		t.Fatalf("the long-form volume should have imported, got %+v", app.Config.Volumes)
	}
	if app.Config.Volumes[0].Writable {
		t.Error("read_only: true must import read-only")
	}
}

// Labels are written both ways in the wild, and the description round-trips through them.
func TestDecode_ListFormLabels(t *testing.T) {
	project := decode(t, "services:\n  app:\n    image: alpine\n    labels:\n      - zinc.description=my app\n")
	if got := project.Services["app"].Labels["zinc.description"]; got != "my app" {
		t.Errorf("list-form label = %q, want %q", got, "my app")
	}
}
