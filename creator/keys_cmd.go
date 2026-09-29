package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/crispuscrew/zinc/creator/internal/keys"
)

func loadKeys() keys.Active {
	store, err := keys.DefaultStore()
	if err == nil {
		var active keys.Active
		active, err = store.Load()
		if err == nil {
			return active
		}
	}
	fmt.Fprintln(os.Stderr, "zc: keybinds: "+err.Error()+" - using default")
	return keys.Active{Name: "default", Scheme: keys.Default}
}

func cmdKeys(argv []string) error {
	store, err := keys.DefaultStore()
	if err != nil {
		return err
	}
	sub := "list"
	if len(argv) > 0 {
		sub = argv[0]
	}
	switch sub {
	case "list":
		active, _ := store.Load()
		names, err := store.List()
		if err != nil {
			return err
		}
		for _, name := range names {
			mark, kind := "  ", "custom"
			if name == active.Name {
				mark = "* "
			}
			if keys.IsBuiltin(name) {
				kind = "built-in"
			}
			fmt.Printf("%s%-20s %s\n", mark, name, kind)
		}
	case "show":
		if len(argv) > 1 {
			scheme, err := store.Resolve(argv[1])
			if err != nil {
				return err
			}
			return printScheme(argv[1], scheme)
		}
		active, err := store.Load()
		if err != nil {
			return err
		}
		return printScheme(active.Name, active.Scheme)
	case "set":
		if len(argv) != 2 {
			return fmt.Errorf("usage: zc keys set <scheme>")
		}
		if err := store.SetActive(argv[1]); err != nil {
			return err
		}
		fmt.Printf("active keybind scheme: %s\n", argv[1])
	case "edit":
		name := "default"
		if len(argv) > 1 {
			name = argv[1]
		}
		scheme, path, err := store.EnsureEditable(name)
		if err != nil {
			return err
		}
		if err := openInEditor(path); err != nil {
			return err
		}
		if err := store.Validate(scheme); err != nil {
			return err
		}
		fmt.Printf("saved scheme %q (%s)\n  activate with: zc keys set %s\n", scheme, path, scheme)
	case "validate":
		if len(argv) > 1 {
			if err := store.Validate(argv[1]); err != nil {
				return err
			}
			fmt.Printf("ok: scheme %q is valid\n", argv[1])
			return nil
		}
		active, err := store.Load()
		if err != nil {
			return err
		}
		fmt.Printf("ok: active scheme %q is valid\n", active.Name)
	case "path":
		fmt.Println(store.Dir)
	default:
		return fmt.Errorf("unknown keys subcommand %q (want list|show|set|edit|validate|path)", sub)
	}
	return nil
}

func printScheme(name string, scheme keys.Scheme) error {
	fmt.Printf("scheme %q\n", name)
	for _, context := range keys.Contexts {
		fmt.Printf("  [%s]\n", keys.ContextName[context])
		for _, action := range keys.ActionsByContext[context] {
			if hint := scheme.Hint(context, action); hint != "" {
				fmt.Printf("    %-14s %s\n", action, hint)
			}
		}
	}
	return nil
}

func openInEditor(path string) error {
	argv := strings.Fields(os.Getenv("EDITOR"))
	if len(argv) == 0 {
		argv = []string{"vim"}
	}
	command := exec.Command(argv[0], append(argv[1:], path)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}
