package podman

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/session"
)

func TerminalLaunch(term, runArgs []string, hold bool) []string {
	out := append([]string{}, term...)
	if !hold {
		return append(append(out, "podman"), runArgs...)
	}
	script := "podman " + shellJoin(runArgs) +
		`; status=$?; printf '\n[zinc] exited (status %s) - press Enter to close\n' "$status"; read _`
	return append(out, "sh", "-c", script)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'" }

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func HolderCmd() []string { return []string{"sleep", "infinity"} }

func ExecArgs(app string, command []string, environment ...map[string]string) []string {
	args := []string{"exec", "-it"}
	for _, values := range environment {
		args = append(args, session.EnvArgs(values)...)
	}
	return append(append(args, app), command...)
}

func (Runtime) OpenSession(app string, command []string, environment map[string]string, opt options.HostOptions, hold bool) error {
	if len(opt.Terminal) == 0 {
		return fmt.Errorf("%s: no terminal emulator configured (set ZINC_TERMINAL)", app)
	}
	argv := TerminalLaunch(opt.Terminal, ExecArgs(app, command, environment), hold)
	return exec.Command(argv[0], argv[1:]...).Run()
}

func appCmd(cfg schema.AppConfig, opt options.HostOptions, runArgs []string) (*exec.Cmd, error) {
	var process *exec.Cmd
	if cfg.StartConditions.Terminal {
		if len(opt.Terminal) == 0 {
			return nil, fmt.Errorf("%s: terminal app but no terminal emulator configured (set ZINC_TERMINAL)", cfg.AppNameID)
		}
		argv := TerminalLaunch(opt.Terminal, runArgs, false)
		process = exec.Command(argv[0], argv[1:]...)
	} else {
		process = exec.Command("podman", runArgs...)
	}
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return process, nil
}

func (Runtime) StartApp(cfg schema.AppConfig, opt options.HostOptions, runArgs []string, onFail func()) error {
	process, err := appCmd(cfg, opt, runArgs)
	if err != nil {
		return err
	}
	if err := process.Start(); err != nil {
		return fmt.Errorf("launch %s: %w", cfg.AppNameID, err)
	}
	go func() {
		if err := process.Wait(); err != nil && onFail != nil {
			onFail()
		}
	}()
	return nil
}
