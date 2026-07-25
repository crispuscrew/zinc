// Command dmenu is a small dmenu-like example: it reads newline-separated lines from stdin,
// shows them in the reusable overlay menu, and prints the chosen line to stdout. It exists to
// prove the menu module is importable and usable by a non-launcher program - a plain stdin
// filter, not zlg - depending on nothing but the menu package and the standard library.
//
// Usage: printf 'one\ntwo\nthree\n' | dmenu
package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/crispuscrew/zinc/menu"
)

func main() {
	// One menu.Item per non-empty input line; the line text is the Label the fuzzy filter
	// matches against.
	var items []menu.Item
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		items = append(items, menu.Item{Label: line})
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// The activate callback prints the picked line and returns nil, which closes the menu with
	// that item selected.
	activate := func(item menu.Item) error {
		fmt.Println(item.Label)
		return nil
	}

	// Run returns the chosen index (unused here - the callback already printed the line) or -1
	// when the user cancelled, in which case nothing was printed.
	_, err := menu.Run(items, activate, menu.Options{Prompt: "> ", AppID: "dmenu"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
