package main

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/virtualization/runner/app"
)

func cmdStop(svc app.Service, argv []string) error {
	if len(argv) < 1 || len(argv) > 2 || len(argv) == 2 && argv[1] != "--force" {
		return fmt.Errorf("usage: zvr stop <app> [--force]")
	}
	if err := svc.Stop(argv[0], len(argv) == 2, app.DefaultStopTimeout); err != nil {
		return err
	}
	fmt.Println("stopped " + argv[0])
	return nil
}

func cmdPS(svc app.Service, argv []string) error {
	if len(argv) != 0 {
		return fmt.Errorf("usage: zvr ps")
	}
	states, err := svc.Running()
	if err != nil {
		return err
	}
	if len(states) == 0 {
		fmt.Println("no guests running")
		return nil
	}
	fmt.Printf("%-24s %-8s %s\n", "APP", "PID", "STATE")
	for _, state := range states {
		fmt.Printf("%-24s %-8d %s\n", state.Name, state.PID, describe(state.Guest, state.Detail))
	}
	return nil
}

func cmdStatus(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zvr status <app>")
	}
	state, err := svc.State(argv[0])
	if err != nil {
		return err
	}
	if !state.Alive {
		fmt.Printf("%s: not running\n", argv[0])
	} else {
		fmt.Printf("%s: running (pid %d, guest %s)\n", state.Name, state.PID, describe(state.Guest, state.Detail))
	}
	if state.Detail != "" {
		fmt.Println("detail: " + state.Detail)
	}
	return nil
}

func describe(guest, detail string) string {
	if guest != "" {
		return guest
	}
	if detail != "" {
		return "unknown (" + detail + ")"
	}
	return "unknown"
}

func cmdReset(svc app.Service, argv []string) error {
	if len(argv) != 2 || argv[1] != "--confirm" {
		return fmt.Errorf("reset deletes the guest disk, UEFI variables and TPM state; usage: zvr reset <app> --confirm")
	}
	if err := svc.Reset(argv[0]); err != nil {
		return err
	}
	fmt.Printf("%s: guest state deleted; base image and runtime options retained\n", argv[0])
	return nil
}

func cmdConsole(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zvr console <app>")
	}
	state, err := svc.State(argv[0])
	if err != nil {
		return err
	}
	if !state.Alive {
		return fmt.Errorf("%s is not running", argv[0])
	}
	socket := svc.Runtime.ConsolePath(argv[0])
	fmt.Printf("serial console socket: %s\nattach with: socat -,raw,echo=0 'unix-connect:%s'\n", socket, strings.ReplaceAll(socket, "'", `'\''`))
	return nil
}

// Retained for the separately owned net command. Run/install use strict parsers.
func splitFlags(argv []string) (string, map[string]bool) {
	flags := map[string]bool{}
	name := ""
	for _, arg := range argv {
		if strings.HasPrefix(arg, "--") {
			flags[arg] = true
		} else if name == "" {
			name = arg
		}
	}
	return name, flags
}
