// Command hzv is the HyprZinc Virtualization manager — the rootless libvirt/qemu
// VM TUI. This is a build skeleton: it exists so all three tools share the same
// module layout and reproducible container build. The Bubbletea UI arrives in
// milestone M9 (see ../ROADMAP.md).
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "hzv (HyprZinc Virtualization): not implemented yet — see ROADMAP M9")
}
