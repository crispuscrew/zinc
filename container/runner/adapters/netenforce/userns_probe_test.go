package netenforce

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestEngineUserNamespaceProbesLocalRootlessAndRootful(test *testing.T) {
	for _, mode := range []string{"false true", "false false"} {
		test.Run(mode, func(test *testing.T) {
			probes, reads := 0, 0
			command := func(arguments ...string) (string, error) {
				if slices.Equal(arguments, []string{"info", "--format", "{{.Host.ServiceIsRemote}} {{.Host.Security.Rootless}}"}) {
					return mode, nil
				}
				if mode != "false true" || !slices.Equal(arguments, []string{"unshare", "readlink", "/proc/self/ns/user"}) {
					test.Fatalf("unexpected identity command: %v", arguments)
				}
				probes++
				return "user:[456]\n", nil
			}
			current := func() (string, error) {
				reads++
				return "user:[789]", nil
			}
			inode, err := engineUserNamespace(command, current)
			if err != nil {
				test.Fatal(err)
			}
			if mode == "false true" && (inode != 456 || probes != 1 || reads != 0) {
				test.Fatal("rootless identity was not read from Podman")
			}
			if mode == "false false" && (inode != 789 || probes != 0 || reads != 1) {
				test.Fatal("rootful identity did not preserve the local caller namespace")
			}
		})
	}
}

func TestEngineUserNamespaceRejectsUnknownOrFailedProbes(test *testing.T) {
	for _, mode := range []string{"true true", "true false", "", "false", "warning\nfalse true"} {
		_, err := engineUserNamespace(func(...string) (string, error) { return mode, nil }, func() (string, error) {
			test.Fatal("unverified engine used caller namespace")
			return "", nil
		})
		if err == nil {
			test.Fatalf("accepted mode %q", mode)
		}
	}
	for _, mode := range []string{"error", "false true", "false false"} {
		failure := errors.New("permission denied")
		_, err := engineUserNamespace(func(arguments ...string) (string, error) {
			if mode != "error" && arguments[0] == "info" {
				return mode, nil
			}
			return "", failure
		}, func() (string, error) { return "", failure })
		if !errors.Is(err, failure) || !strings.Contains(err.Error(), "probe Podman") {
			test.Fatalf("probe failure was hidden: %v", err)
		}
	}
}
