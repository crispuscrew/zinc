package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/crispuscrew/zinc/common/domain/nftrules"
	"github.com/crispuscrew/zinc/container/runner/adapters/netenforce"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
)

const countersNote = "counters describe installed policy since this launch, not lifetime totals; endpoint returns require both policies before acceptance"
const postureFiltered, postureIsolated = "filtered", "isolated"
const netUsage = "usage: zcr net [app[@instance]] [--json]"

type netEntry struct {
	Address  string `json:"address"`
	App      string `json:"app"`
	Instance string `json:"instance,omitempty"`
	Posture  string `json:"posture"`
	Netns    string `json:"netns,omitempty"`
}

type netReport struct {
	netEntry
	Note     string                 `json:"note,omitempty"`
	Counters []nftrules.RuleCounter `json:"counters"`
}

func cmdNet(svc app.Service, opt options.HostOptions, argv []string) error {
	var name string
	asJSON := false
	for _, argument := range argv {
		switch {
		case argument == "--json":
			asJSON = true
		case strings.HasPrefix(argument, "-"):
			return fmt.Errorf("unknown flag %q\n%s", argument, netUsage)
		case name == "":
			name = argument
		default:
			return fmt.Errorf("unexpected argument %q\n%s", argument, netUsage)
		}
	}
	if name == "" {
		return netList(svc, asJSON)
	}
	return netCounters(svc, opt, name, asJSON)
}
func netList(svc app.Service, asJSON bool) error {
	entries, err := netEntries(svc)
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(struct {
			Apps []netEntry `json:"apps"`
		}{entries})
	}
	return printEntries(entries)
}
func printEntries(entries []netEntry) error {
	if len(entries) == 0 {
		return nil
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ADDRESS\tPOSTURE\tNETNS")
	for _, entry := range entries {
		fmt.Fprintf(table, "%s\t%s\t%s\n", entry.Address, entry.Posture, entry.Netns)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	fmt.Println("filtered: provisioned namespace with ordered default-deny policy (zcr net <app> reads counters)")
	fmt.Println("isolated: no Interfaces, --network none; only its own localhost")
	return nil
}
func netCounters(svc app.Service, opt options.HostOptions, name string, asJSON bool) error {
	cfg, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	addr, err := paths.ParseAddress(name)
	if err != nil {
		return err
	}
	if strings.Contains(name, "/") || strings.HasSuffix(name, ".yaml") {
		addr = paths.Address{App: cfg.AppNameID}
	}
	attached, err := observedFiltered(svc, cfg.AppNameID)
	if err != nil {
		return err
	}
	if attached != (len(cfg.NetworkMeta.Interfaces) > 0) {
		return fmt.Errorf("running attachment differs from configuration; inspect the installed policy before reporting posture")
	}
	raw, filtered, err := svc.NetCounters(cfg, opt)
	if err != nil {
		return err
	}
	report := netReport{netEntry: entryFor(addr, cfg.AppNameID, filtered), Counters: []nftrules.RuleCounter{}}
	if filtered {
		report.Note = countersNote
		report.Counters, err = nftrules.ParseCounters([]byte(raw))
		if err != nil {
			return fmt.Errorf("%s: %w", cfg.AppNameID, err)
		}
	}
	if asJSON {
		return writeJSON(report)
	}
	return printReport(report)
}
func printReport(report netReport) error {
	fmt.Printf("address: %s\nposture: %s\n", report.Address, report.Posture)
	if report.Posture != postureFiltered {
		fmt.Println("no Interfaces: --network none, no external NIC and no firewall counters")
		return nil
	}
	fmt.Printf("netns: %s\nnote: %s\n", report.Netns, countersNote)
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "CHAIN\tVERDICT\tRULE\tPACKETS\tBYTES")
	for _, counter := range report.Counters {
		fmt.Fprintf(table, "%s\t%s\t%s\t%d\t%d\n", counter.Chain, counter.Verdict, counter.Label, counter.Packets, counter.Bytes)
	}
	return table.Flush()
}
func observedFiltered(svc app.Service, runtime string) (bool, error) {
	pod, err := svc.PodOf(runtime)
	if err != nil {
		return false, fmt.Errorf("%s: could not read attachment: %w", runtime, err)
	}
	return pod != "", nil
}
func netEntries(svc app.Service) ([]netEntry, error) {
	defined, err := svc.List()
	if err != nil {
		return nil, err
	}
	running, err := svc.Running()
	if err != nil {
		return nil, err
	}
	var names []string
	for name, alive := range running {
		if alive {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	entries := []netEntry{}
	for _, name := range names {
		addr, known := addressOf(name, defined)
		if !known {
			continue
		}
		filtered, err := observedFiltered(svc, name)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entryFor(addr, name, filtered))
	}
	return entries, nil
}
func entryFor(addr paths.Address, runtime string, filtered bool) netEntry {
	entry := netEntry{Address: addr.String(), App: addr.App, Instance: addr.Instance, Posture: postureIsolated}
	if filtered {
		entry.Posture, entry.Netns = postureFiltered, netenforce.PodName(runtime)
	}
	return entry
}
func addressOf(runtime string, defined []string) (paths.Address, bool) {
	known := map[string]bool{}
	for _, name := range defined {
		known[name] = true
	}
	if known[runtime] {
		return paths.Address{App: runtime}, true
	}
	cut := strings.LastIndex(runtime, paths.Separator)
	if cut <= 0 {
		return paths.Address{}, false
	}
	appName, instance := runtime[:cut], runtime[cut+len(paths.Separator):]
	if instance == "" || !known[appName] {
		return paths.Address{}, false
	}
	return paths.Address{App: appName, Instance: instance}, true
}
func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
