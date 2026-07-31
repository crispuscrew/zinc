package main

// Bus attribution: turning something observed on the HOST session bus back into the Zinc app
// and instance it belongs to, plus the reporting half of `zcr where`.
//
// The desktop's first proposal was for each app to own a bus name like
// zinc.app.<app>.<instance>. That is not available, and the reason is worth keeping: the
// proxy is a relay with no bus identity of its own, so `--own=zinc.app.x` grants THE APP
// permission to claim that name. The app then has to volunteer to claim it - a self-asserted
// identity, which is the exact thing attribution exists to stop trusting.
//
// What Zinc can publish instead is the mapping it already holds by construction. Zinc creates
// the proxy container, names it after the app, and gives it the only handle on the real bus
// the app will ever be behind. Nothing here asks the app anything.
//
// The chain the consumer walks, and how solid each link is:
//
//  1. connection -> pid. The host bus answers GetConnectionUnixProcessID with a host pid it
//     took from SO_PEERCRED when the connection was made. The kernel's answer about the peer,
//     not the peer's claim about itself. Solid.
//  2. pid -> proxy container. `zcr bus` reads the runtime's live list. Solid at the instant it
//     is read, and best-effort across time: the bus captured its pid at connect time, and a
//     proxy that has since died and had its pid reused would be attributed to whoever holds
//     the number now. Narrow, because the bus drops the connection when the proxy exits, and
//     unavoidable without a bus that hands out something better than a pid.
//  3. proxy container -> app@instance. The name Zinc gave it, read back. Solid, except that
//     recovering the instance from a runtime name needs the store to disambiguate a dotted app
//     name (see paths.ParseRuntime).
//
// Measured, and worth knowing before reading an empty answer as "this app has no bus":
// xdg-dbus-proxy opens ONE upstream connection PER CLIENT. An app with no live bus client
// therefore contributes no connection to the host bus at all, and an app with several
// contributes several unique names, all carrying the proxy's one pid. Many-to-one is normal.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/crispuscrew/zinc/container/runner/adapters/dbusproxy"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
)

// busReport is what an app's filtered bus is made of on the host: the socket the app connects
// to, and the container Zinc runs to serve it. Nil (JSON null) for an app with no DBusMeta,
// which has neither - saying so is the point, since printing a path that was never created
// would send a consumer looking for a file that cannot exist.
type busReport struct {
	Socket string `json:"socket"`
	Proxy  string `json:"proxy"`
}

// whereReport is `zcr where`'s answer: the layout of one instance, whether or not it is
// running. Deliberately carries nothing read from the runtime (no pid, no up/down) - every
// field is decided before the app starts, so this keeps answering when podman does not.
type whereReport struct {
	Address   string     `json:"address"`
	App       string     `json:"app"`
	Instance  string     `json:"instance"`
	Container string     `json:"container"`
	State     string     `json:"state"`
	Bus       *busReport `json:"bus"`
}

// busRow is one line of the attribution table: a running proxy, the pid that identifies it on
// the host, and the app@instance it was created for. Flat rather than nesting busReport,
// because this is the shape a consumer indexes BY pid, and a pid two levels down in a
// per-record object is awkward to select on.
type busRow struct {
	Address   string `json:"address"`
	App       string `json:"app"`
	Instance  string `json:"instance"`
	Container string `json:"container"`
	Proxy     string `json:"proxy"`
	PID       int    `json:"pid"`
	Socket    string `json:"socket"`
}

// whereOf builds the report for one address. hasBus is whether the app config asked for a
// filtered bus; the caller has the config, and passing the one bit keeps this pure enough to
// test the output shape without a store.
func whereOf(addr paths.Address, hasBus bool, runtimeDir string) (whereReport, error) {
	stateDir, err := paths.StateDir(addr)
	if err != nil {
		return whereReport{}, err
	}
	report := whereReport{
		Address:   addr.String(),
		App:       addr.App,
		Instance:  addr.Instance,
		Container: addr.Runtime(),
		State:     stateDir,
	}
	if hasBus {
		// The socket is empty when XDG_RUNTIME_DIR is unset, which is the same condition that
		// makes launching this app fail outright ("DBusMeta needs XDG_RUNTIME_DIR set"). The
		// proxy name is still reported, because it is decided by the app name alone.
		report.Bus = &busReport{
			Socket: dbusproxy.HostSocketPath(runtimeDir, addr.Runtime()),
			Proxy:  dbusproxy.ContainerName(addr.Runtime()),
		}
	}
	return report, nil
}

// renderWhere is the human form: one "label: value" per line, the labels being the contract
// for anything cutting on the colon. Every label is always present, including for an app with
// no bus, so a reader never has to tell "no bus" apart from "this zcr is older than the
// field".
func renderWhere(report whereReport) string {
	socket, proxy := "none", "none"
	if report.Bus != nil {
		socket, proxy = report.Bus.Socket, report.Bus.Proxy
		if socket == "" {
			socket = "unknown (XDG_RUNTIME_DIR is unset, so this app could not launch here either)"
		}
	}
	return fmt.Sprintf("state: %s\ncontainer: %s\nbus-socket: %s\nbus-proxy: %s\n",
		report.State, report.Container, socket, proxy)
}

// busRows turns the runtime's live container pids into the attribution table: one row per
// running D-Bus proxy. pids is every running container by name; defined answers whether a
// name is a defined app, which is what recovers "app@instance" from a runtime name.
//
// Sorted by address so two calls in a row produce the same bytes - a consumer diffing the
// table, or a test asserting it, would otherwise be at the mercy of map iteration order.
func busRows(pids map[string]int, runtimeDir string, defined func(string) bool) []busRow {
	rows := []busRow{} // never nil: the JSON form of "nothing running" must be [], not null
	for container, pid := range pids {
		runtimeName, isProxy := dbusproxy.AppOfProxy(container)
		if !isProxy {
			continue
		}
		addr := paths.ParseRuntime(runtimeName, defined)
		rows = append(rows, busRow{
			Address:   addr.String(),
			App:       addr.App,
			Instance:  addr.Instance,
			Container: runtimeName,
			Proxy:     container,
			PID:       pid,
			Socket:    dbusproxy.HostSocketPath(runtimeDir, runtimeName),
		})
	}
	sort.Slice(rows, func(one, two int) bool { return rows[one].Address < rows[two].Address })
	return rows
}

// renderBus is the human/awk form: one tab-separated line per proxy, pid second so the column
// a consumer looks a connection up by is easy to cut out. No header, matching `zcr ps`, since
// a header is one more thing a naive reader has to skip.
func renderBus(rows []busRow) string {
	var out strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&out, "%s\t%d\t%s\t%s\n", row.Address, row.PID, row.Proxy, row.Socket)
	}
	return out.String()
}

// printJSON writes the machine form. Indented because a person runs this by hand too, and jq
// does not care either way; the field names and their nesting are the contract, not the
// whitespace.
func printJSON(value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

// splitJSONFlag pulls --json out of a command's arguments and returns the rest. Hand-rolled
// rather than a flag.FlagSet because the flag reads better AFTER the app name (`zcr where
// notes@work --json`) and a FlagSet stops parsing at the first positional argument.
func splitJSONFlag(argv []string) (rest []string, asJSON bool, err error) {
	for _, arg := range argv {
		switch {
		case arg == "--json":
			asJSON = true
		case strings.HasPrefix(arg, "-"):
			return nil, false, fmt.Errorf("unknown flag %q", arg)
		default:
			rest = append(rest, arg)
		}
	}
	return rest, asJSON, nil
}

// cmdWhere answers "where does this instance keep things, what is it called at runtime, and
// where is its bus".
//
// It exists so nothing outside Zinc has to hardcode the layout. A desktop that wants to show
// a user where an app's state lives, or that names a container to look it up, would otherwise
// mirror the rules in paths - and two copies of a layout drift the first time either side
// changes. Asking costs a process; assuming costs a bug nobody sees until the paths differ.
//
// Deliberately not folded into `inspect`, which is a passthrough to `podman inspect`:
// intercepting it would put Zinc in the business of parsing and re-emitting podman's output
// forever, and the answer here is about an instance whether or not it is running.
//
// The app has to be defined, because the bus answer comes from its config: a guess at whether
// an unknown name has a bus would be a path that may not exist, reported with the same
// confidence as one that does.
func cmdWhere(svc app.Service, opt options.HostOptions, argv []string) error {
	rest, asJSON, err := splitJSONFlag(argv)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, whereUsage)
	}
	if len(rest) != 1 {
		return fmt.Errorf("%s", whereUsage)
	}
	addr, err := paths.ParseAddress(rest[0])
	if err != nil {
		return err
	}
	if strings.TrimSpace(addr.App) == "" {
		return fmt.Errorf("%s", whereUsage)
	}
	if strings.Contains(addr.App, "/") {
		// A path names a file, and the container name and state directory are derived from an
		// app ADDRESS. Answering for a path would report a container named after a filename,
		// which is not what anything runs.
		return fmt.Errorf("%q looks like a path; `where` takes an app address (%s)", rest[0], whereUsage)
	}
	// Through the shared load path, so this refuses a VM app for the same reason every other
	// command does: a guest has no container, no proxy and no socket, and reporting the layout
	// one would have had is an answer that never becomes true.
	cfg, err := loadApp(svc, addr.String())
	if err != nil {
		return err
	}
	report, err := whereOf(addr, !cfg.DBusMeta.IsZero(), opt.RuntimeDir)
	if err != nil {
		return err
	}
	if asJSON {
		return printJSON(report)
	}
	fmt.Print(renderWhere(report))
	return nil
}

const whereUsage = "usage: zcr where <app[@instance]> [--json]"

// cmdBus prints the attribution table: every running D-Bus proxy, the host pid that identifies
// it, and the app@instance it belongs to.
//
// This is the reverse direction of `where`, and it is the one the desktop actually needs: it
// observes a connection on the host bus and has no name to look up, only a pid it got from
// GetConnectionUnixProcessID. Resolving that pid needs the whole table, not one app's entry.
//
// It stops at the pid on purpose. Asking the bus which unique names map to that pid is one
// call the consumer is already positioned to make (it is holding a bus connection; that is how
// it saw the thing in the first place), and doing it here would put a D-Bus client - a
// protocol implementation, an auth handshake, a dependency - inside the sandbox runtime for no
// isolation gain.
func cmdBus(svc app.Service, opt options.HostOptions, argv []string) error {
	rest, asJSON, err := splitJSONFlag(argv)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, busUsage)
	}
	if len(rest) != 0 {
		return fmt.Errorf("%s", busUsage)
	}
	pids, err := svc.PIDs()
	if err != nil {
		return err
	}
	rows := busRows(pids, opt.RuntimeDir, svc.Exists)
	if asJSON {
		return printJSON(rows)
	}
	fmt.Print(renderBus(rows))
	return nil
}

const busUsage = "usage: zcr bus [--json]"
