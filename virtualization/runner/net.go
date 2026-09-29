package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/crispuscrew/zinc/common/domain/nftrules"
	"github.com/crispuscrew/zinc/virtualization/runner/app"
)

// countersNote is said in every readout because the number invites exactly one wrong reading and
// nothing else in the output prevents it.
const countersNote = "counters describe the installed TAP packet policy since this launch; endpoint returns require both policies before acceptance"

// A guest without Interfaces has no NIC. A networked guest uses provisioned TAPs.
const (
	postureFiltered   = "filtered"
	postureUnfiltered = "isolated"
)

type netReport struct {
	App      string                 `json:"app"`
	Posture  string                 `json:"posture"`
	Note     string                 `json:"note,omitempty"`
	Counters []nftrules.RuleCounter `json:"counters"`
}

func cmdNet(svc app.Service, argv []string) error {
	name, flags := splitFlags(argv)
	if name == "" {
		return fmt.Errorf("usage: zvr net <app> [--json]")
	}
	cfg, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	// An empty list, never a null: a consumer iterating an unfiltered guest's counters should
	// find none rather than handle the absent field as a third case.
	report := netReport{App: cfg.AppNameID, Posture: postureUnfiltered, Counters: []nftrules.RuleCounter{}}
	counters, filtered, err := svc.NetCounters(cfg.AppNameID)
	if err != nil {
		return err
	}
	if filtered {
		report.Posture, report.Note, report.Counters = postureFiltered, countersNote, counters
	}
	if flags["--json"] {
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	}
	return printNetReport(report)
}

func printNetReport(report netReport) error {
	fmt.Printf("app:     %s\n", report.App)
	fmt.Printf("posture: %s\n", report.Posture)
	if report.Posture != postureFiltered {
		fmt.Println("this guest has no provisioned network namespace; new launches with no Interfaces have no NIC")
		fmt.Println("and no ruleset to count; this does not attest the NICs of a legacy running guest.")
		return nil
	}
	fmt.Printf("note:    %s\n", report.Note)
	fmt.Println()
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "CHAIN\tVERDICT\tRULE\tPACKETS\tBYTES")
	for _, counter := range report.Counters {
		fmt.Fprintf(table, "%s\t%s\t%s\t%d\t%d\n",
			counter.Chain, counter.Verdict, counter.Label, counter.Packets, counter.Bytes)
	}
	return table.Flush()
}
