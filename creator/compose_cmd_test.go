package main

import (
	"slices"
	"testing"
)

// Go's flag package stops at the first non-flag argument, so a command written
// `<thing> --flag` has to be split by hand. Parsing the whole argv instead read `-o` as
// another positional and silently wrote nothing - the bug this guards.
func TestSplitLeadingArg(t *testing.T) {
	thing, flags, err := splitLeadingArg([]string{"gateway", "-o", "compose.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if thing != "gateway" {
		t.Errorf("positional = %q, want gateway", thing)
	}
	if want := []string{"-o", "compose.yaml"}; !slices.Equal(flags, want) {
		t.Errorf("flags = %v, want %v", flags, want)
	}

	if _, _, err := splitLeadingArg([]string{"-o", "compose.yaml"}); err == nil {
		t.Error("a leading flag means the required argument is missing")
	}
	if _, _, err := splitLeadingArg(nil); err == nil {
		t.Error("no arguments at all must be a usage error")
	}
}

// An already-pinned image is left alone and needs no registry; a localhost/ reference is
// exempt from pinning by the same rule the validator applies.
func TestPinImage_LeavesPinnedReferencesAlone(t *testing.T) {
	const pinned = "docker.io/library/alpine@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d"
	for _, ref := range []string{pinned, "localhost/zinc/app:local"} {
		got, note, err := pinImage(ref)
		if err != nil {
			t.Fatalf("pinImage(%q): %v", ref, err)
		}
		if got != ref {
			t.Errorf("pinImage(%q) = %q, want it unchanged", ref, got)
		}
		if note != "" {
			t.Errorf("pinImage(%q) note = %q, want none", ref, note)
		}
	}
	if _, _, err := pinImage("  "); err == nil {
		t.Error("a service with no image must be an error, not an empty app")
	}
}
