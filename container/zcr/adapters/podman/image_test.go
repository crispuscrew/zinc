package podman

import "testing"

func TestParseSearch(t *testing.T) {
	out := "docker.io/library/alpine\tA minimal image\ndocker.io/library/nginx\tWeb server\n"
	got := parseSearch(out)
	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d", len(got))
	}
	if got[0].Name != "docker.io/library/alpine" || got[0].Description != "A minimal image" {
		t.Fatalf("bad parse: %+v", got[0])
	}
}

func TestParseSearch_Empty(t *testing.T) {
	if got := parseSearch("\n\n"); len(got) != 0 {
		t.Fatalf("blank output should yield no results, got %+v", got)
	}
}

func TestPickRepoDigest_MatchesRepo(t *testing.T) {
	out := "docker.io/library/alpine@sha256:aaa\nquay.io/x/alpine@sha256:bbb\n"
	got, err := pickRepoDigest("docker.io/library/alpine:3.20", out)
	if err != nil {
		t.Fatal(err)
	}
	if got != "docker.io/library/alpine@sha256:aaa" {
		t.Fatalf("should pick the matching repo, got %q", got)
	}
}

func TestPickRepoDigest_FallbackFirst(t *testing.T) {
	// A bare "alpine" won't match the full docker.io path, so the sole digest wins.
	got, err := pickRepoDigest("alpine", "docker.io/library/alpine@sha256:aaa\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "docker.io/library/alpine@sha256:aaa" {
		t.Fatalf("got %q", got)
	}
}

func TestPickRepoDigest_None(t *testing.T) {
	if _, err := pickRepoDigest("alpine", "\n"); err == nil {
		t.Fatal("expected an error when RepoDigests has no @sha256 entry")
	}
}

func TestResolve_AlreadyPinned(t *testing.T) {
	ref := "docker.io/library/alpine@sha256:abc"
	got, err := Resolver{}.Resolve(ref)
	if err != nil || got != ref {
		t.Fatalf("an already-pinned ref should pass through unchanged, got %q err %v", got, err)
	}
}
