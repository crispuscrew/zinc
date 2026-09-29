package runner

import (
	"os"
	"testing"
)

func TestInstanceLikeExplicitPathsKeepTheirOwnTypeAndAddress(t *testing.T) {
	for _, path := range []string{"./shell@work.yaml", "./shell@work"} {
		t.Run(path, func(t *testing.T) {
			calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
			writeConfigs(t, map[string]string{"shell": "Type: ZincContainer\nAppNameID: shell\n"})
			t.Chdir(t.TempDir())
			if err := os.WriteFile(path, []byte("Type: ZincVirtualization\nAppNameID: file-guest\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := Launch(path); err != nil {
				t.Fatal(err)
			}
			if err := Stop(path); err != nil {
				t.Fatal(err)
			}
			assertCalls(t, calls, "zvr\nrun\n"+path+"\nzvr\nstop\n"+path+"\n")
		})
	}
}

func TestYAMLSuffixedInstanceCannotSelectACollidingFile(t *testing.T) {
	calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
	writeConfigs(t, map[string]string{
		"shell":           "Type: ZincContainer\nAppNameID: shell\n",
		"shell@work.yaml": "Type: ZincVirtualization\nAppNameID: shell@work.yaml\n",
	})
	t.Chdir(t.TempDir())
	if err := os.WriteFile("shell@work.yaml", []byte("Type: ZincVirtualization\nAppNameID: wrong-guest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []func(string) error{Launch, Stop} {
		if err := action("shell@work.yaml"); err == nil {
			t.Fatal("ambiguous instance/path was accepted")
		}
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatalf("runtime executed for instance/path collision: %v", err)
	}
}
