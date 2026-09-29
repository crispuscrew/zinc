package netenforce

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

func TestUserNamespaceSelectionBindsEveryOperation(test *testing.T) {
	for _, current := range []uint64{456, 789} {
		cfg, manifest := configured()
		enforcer := adapter(manifest)
		calls := 0
		enforcer.UserNamespace = sync.OnceValues(func() (uint64, error) {
			calls++
			return current, nil
		})
		steps, err := enforcer.Prepare(cfg, options.HostOptions{})
		if err != nil {
			test.Fatal(err)
		}
		counter, active := enforcer.Counters(cfg, options.HostOptions{})
		if !active {
			test.Fatal("missing counter operation")
		}
		teardown := enforcer.Teardown(cfg)
		commands := append(steps, counter, teardown[1])
		want := "ns:" + manifest.UserNamespace
		if current == manifest.UserInode {
			want = "host"
		}
		for _, command := range commands {
			if flagValue(command.Args, "--userns") != want {
				test.Fatalf("identity %d: inconsistent namespace: %v", current, command.Args)
			}
			if command.Args[0] == "run" {
				script := command.Args[len(command.Args)-1]
				for _, identity := range []string{"net:[123]", "user:[456]"} {
					if position := strings.Index(script, identity); position < 0 || position > strings.Index(script, "nft ") {
						test.Fatalf("namespace %s not checked before nft: %s", identity, script)
					}
				}
			}
		}
		if calls != 1 {
			test.Fatalf("identity was probed %d times", calls)
		}
	}
}

func TestPrepareResolvesIdentityOnlyOnceBeforeExecution(test *testing.T) {
	cfg, manifest := configured()
	enforcer := adapter(manifest)
	calls := 0
	enforcer.UserNamespace = func() (uint64, error) {
		calls++
		return manifest.UserInode + uint64(calls-1), nil
	}
	steps, err := enforcer.Prepare(cfg, options.HostOptions{})
	if err != nil || calls != 1 {
		test.Fatalf("prepare: calls=%d err=%v", calls, err)
	}
	for _, step := range steps {
		if flagValue(step.Args, "--userns") != "host" || strings.Contains(strings.Join(step.Args, " "), "podman unshare") {
			test.Fatalf("identity selection escaped into execution: %v", step.Args)
		}
	}
}

func TestUserNamespaceProbeFailureRefusesOperations(test *testing.T) {
	for _, failure := range []error{errors.New("probe denied"), nil} {
		cfg, manifest := configured()
		enforcer := adapter(manifest)
		enforcer.UserNamespace = func() (uint64, error) { return 0, failure }
		if steps, err := enforcer.Prepare(cfg, options.HostOptions{}); err == nil || len(steps) != 0 {
			test.Fatal("failed or zero identity produced a launch plan")
		}
		counter, active := enforcer.Counters(cfg, options.HostOptions{})
		if !active {
			test.Fatal("probe failure claimed isolation")
		}
		teardown := enforcer.Teardown(cfg)
		for _, command := range []ports.Command{counter, teardown[len(teardown)-1]} {
			if flagValue(command.Args, "--network") != "none" || slices.Contains(command.Args, "--userns") ||
				!strings.Contains(command.Args[len(command.Args)-1], "exit 1") {
				test.Fatalf("probe failure did not produce an explicit error: %+v", command)
			}
		}
	}
}

func TestUserNamespaceIdentityParsing(test *testing.T) {
	for _, input := range []string{"user:[4026533112]", "user:[4026533112]\n"} {
		if inode, err := parseUserNamespace(input); err != nil || inode != 4026533112 {
			test.Fatalf("identity %q: %d %v", input, inode, err)
		}
	}
	for _, input := range []string{"", "user:[0]", "net:[456]", "user:[-1]", "user:[abc]",
		"user:[18446744073709551616]", "user:[456]\nuser:[789]", "user:[456] trailing"} {
		if _, err := parseUserNamespace(input); err == nil {
			test.Fatalf("accepted invalid identity %q", input)
		}
	}
}

func flagValue(arguments []string, name string) string {
	index := slices.Index(arguments, name)
	if index < 0 || index+1 >= len(arguments) {
		return ""
	}
	return arguments[index+1]
}
